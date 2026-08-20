package services

import (
	"context"
	"errors"
	"time"

	poker "github.com/zuse/poker5/go-poker"
	"github.com/zuse/poker5/go-poker/repository"
)

// unixMilliOrZero renders a deadline for the client. A zero time.Time is "no
// deadline set", which UnixMilli would otherwise report as a huge negative
// number from the year 1; clients read 0 as "nothing to count down to".
func unixMilliOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func (s *GameService) startIntermissionTimer(table *ActiveTable) {
	if table.IntermissionTimer != nil {
		table.IntermissionTimer.Stop()
	}
	table.IntermissionEndsAt = time.Now().Add(IntermissionSeconds * time.Second)
	roomCode := table.RoomCode
	table.IntermissionTimer = time.AfterFunc(IntermissionSeconds*time.Second, func() {
		s.fireNextHand(roomCode)
	})
}

func (s *GameService) fireNextHand(roomCode string) {
	s.mu.RLock()
	table, exists := s.tables[roomCode]
	s.mu.RUnlock()
	if !exists {
		return
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	table.IntermissionTimer = nil

	if !table.Engine.Game.Intermission || len(table.Engine.Players) < 2 {
		return
	}

	table.Engine.Game.Intermission = false
	table.Engine.StartHand()
	s.resetTurnTimer(table)
	s.broadcastTableUpdate(table)
}

func (s *GameService) SubmitAction(ctx context.Context, roomCode string, user *repository.User, action string, amount int) error {
	table, err := s.lookupTable(ctx, roomCode)
	if err != nil {
		return err
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	if !table.Engine.Game.GameStarted || table.Engine.Game.GameFinished {
		return errors.New("game is not active")
	}

	activePlayer := s.getCurrentTurnPlayer(table)
	if activePlayer == nil || activePlayer.Name != user.Username {
		return ErrNotYourTurn
	}

	ok := table.Engine.HumanAction(user.Username, action, amount)
	if !ok {
		return ErrInvalidAction
	}

	if table.Engine.Game.Intermission && table.Engine.Game.LastWinner != nil {
		s.handlePotCommission(ctx, table)
		s.startIntermissionTimer(table)
	}

	s.resetTurnTimer(table)
	s.broadcastTableUpdate(table)
	return nil
}

func (s *GameService) getCurrentTurnPlayer(table *ActiveTable) *poker.Player {
	if len(table.Engine.Players) == 0 {
		return nil
	}
	idx := table.Engine.CurrentPlayerIx % len(table.Engine.Players)
	return table.Engine.Players[idx]
}

func (s *GameService) resetTurnTimer(table *ActiveTable) {
	if table.TurnTimer != nil {
		table.TurnTimer.Stop()
	}

	if !table.Engine.Game.GameStarted || table.Engine.Game.Intermission || table.Engine.Game.GameFinished {
		return
	}

	current := s.getCurrentTurnPlayer(table)
	if current == nil || current.Folded || current.AllIn {
		return
	}

	table.TimerExpiry = time.Now().Add(time.Duration(table.TurnSeconds) * time.Second)
	roomCode := table.RoomCode

	table.TurnTimer = time.AfterFunc(time.Duration(table.TurnSeconds)*time.Second, func() {
		s.handleTurnTimeout(roomCode)
	})
}

func (s *GameService) handleTurnTimeout(roomCode string) {
	s.mu.RLock()
	table, exists := s.tables[roomCode]
	s.mu.RUnlock()

	if !exists {
		return
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	if !table.Engine.Game.GameStarted || table.Engine.Game.Intermission {
		return
	}

	player := s.getCurrentTurnPlayer(table)
	if player == nil || player.Folded || player.AllIn {
		return
	}

	if player.RoundBet >= table.Engine.Game.CurrentBet {
		table.Engine.HumanAction(player.Name, "check", 0)
	} else {
		table.Engine.HumanAction(player.Name, "fold", 0)
	}

	// One timeout is all the guessing this server does on a player's behalf.
	// They are sat out — keeping their seat and their chips, but dealt out of
	// every hand until they say they are back — because the alternative is a
	// dead phone paying blinds every orbit until the stack is gone.
	if !player.IsBot && !player.SittingOut {
		player.SittingOut = true
		s.startSitOutEviction(table, player.Name)
		s.sse.Broadcast(table.RoomCode, "player-sat-out", map[string]interface{}{
			"player": player.Name,
		})
	}

	if table.Engine.Game.Intermission && table.Engine.Game.LastWinner != nil {
		s.handlePotCommission(context.Background(), table)
		s.startIntermissionTimer(table)
	}

	s.resetTurnTimer(table)
	s.broadcastTableUpdate(table)
}

func (s *GameService) handlePotCommission(ctx context.Context, table *ActiveTable) {
	payouts := table.Engine.Game.Payouts
	table.Engine.Game.Payouts = nil
	if len(payouts) == 0 {
		return
	}

	settings, err := s.settings.Get(ctx)
	if err != nil {
		return
	}
	smallBlind := int64(table.Engine.Game.SmallBlind)

	for _, payout := range payouts {
		if payout.Amount <= 0 {
			continue
		}
		winnerUID, ok := table.UserIDs[payout.Winner]
		if !ok {
			continue
		}

		result, err := s.wallet.ProcessHandPayout(ctx, winnerUID, int64(payout.Amount), smallBlind, settings)
		if err != nil {
			continue
		}

		raked := result.SiteRake + result.ReferralCut
		if raked <= 0 {
			continue
		}
		for _, p := range table.Engine.Players {
			if p.Name == payout.Winner {
				p.Chips -= int(raked)
				if p.Chips < 0 {
					p.Chips = 0
				}
				break
			}
		}
	}
}

func (s *GameService) broadcastTableUpdate(table *ActiveTable) {
	state := table.Engine.ToDict()
	state["room_code"] = table.RoomCode
	state["buy_in"] = table.BuyIn
	state["room_type"] = table.RoomType
	state["max_players"] = table.MaxPlayers
	state["host_user_id"] = table.HostUserID

	remainingSecs := int(time.Until(table.TimerExpiry).Seconds())
	if remainingSecs < 0 {
		remainingSecs = 0
	}
	state["turn_remaining_seconds"] = remainingSecs
	state["turn_total_seconds"] = table.TurnSeconds
	// Absolute deadline as well as the remaining count: clients tick against
	// the deadline so a slow response or a throttled tab can't drift.
	state["turn_ends_at_ms"] = unixMilliOrZero(table.TimerExpiry)

	state["countdown_active"] = table.CountdownActive
	countdownRemaining := int(time.Until(table.CountdownEndsAt).Seconds())
	if countdownRemaining < 0 {
		countdownRemaining = 0
	}
	state["countdown_remaining_seconds"] = countdownRemaining
	state["countdown_ends_at_ms"] = unixMilliOrZero(table.CountdownEndsAt)
	state["intermission_ends_at_ms"] = unixMilliOrZero(table.IntermissionEndsAt)

	activeP := s.getCurrentTurnPlayer(table)
	if activeP != nil {
		state["current_turn_player"] = activeP.Name
	}

	s.sse.Broadcast(table.RoomCode, "game-state", state)
	s.sse.Broadcast("lobby", "lobby-update", map[string]interface{}{"room_code": table.RoomCode})
}

func (s *GameService) GetTableState(ctx context.Context, roomCode string, currentUsername string) map[string]interface{} {
	table, err := s.lookupTable(ctx, roomCode)
	if err != nil {
		return map[string]interface{}{
			"room_code": roomCode,
			"status":    "empty",
			"players":   []interface{}{},
		}
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	state := table.Engine.ToDict()
	state["room_code"] = table.RoomCode
	state["buy_in"] = table.BuyIn
	state["room_type"] = table.RoomType
	state["max_players"] = table.MaxPlayers
	state["host_user_id"] = table.HostUserID
	state["is_host"] = table.HostUserID != 0 && table.UserIDs[currentUsername] == table.HostUserID

	remainingSecs := int(time.Until(table.TimerExpiry).Seconds())
	if remainingSecs < 0 {
		remainingSecs = 0
	}
	state["turn_remaining_seconds"] = remainingSecs
	state["turn_total_seconds"] = table.TurnSeconds
	// Absolute deadline as well as the remaining count: clients tick against
	// the deadline so a slow response or a throttled tab can't drift.
	state["turn_ends_at_ms"] = unixMilliOrZero(table.TimerExpiry)

	state["countdown_active"] = table.CountdownActive
	countdownRemaining := int(time.Until(table.CountdownEndsAt).Seconds())
	if countdownRemaining < 0 {
		countdownRemaining = 0
	}
	state["countdown_remaining_seconds"] = countdownRemaining
	state["countdown_ends_at_ms"] = unixMilliOrZero(table.CountdownEndsAt)
	state["intermission_ends_at_ms"] = unixMilliOrZero(table.IntermissionEndsAt)

	activeP := s.getCurrentTurnPlayer(table)
	if activeP != nil {
		state["current_turn_player"] = activeP.Name
		state["is_my_turn"] = (activeP.Name == currentUsername)
	}

	playersList, _ := state["players"].([]map[string]interface{})
	for _, pMap := range playersList {
		pName, _ := pMap["name"].(string)
		var actualPlayer *poker.Player
		for _, p := range table.Engine.Players {
			if p.Name == pName {
				actualPlayer = p
				break
			}
		}

		if actualPlayer != nil {
			pMap["chips"] = actualPlayer.Chips
			pMap["round_bet"] = actualPlayer.RoundBet
			pMap["total_bet"] = actualPlayer.TotalBet
			pMap["folded"] = actualPlayer.Folded
			pMap["all_in"] = actualPlayer.AllIn
			pMap["dealer"] = actualPlayer.IsDealer
			pMap["small_blind"] = actualPlayer.IsSmallBlind
			pMap["big_blind"] = actualPlayer.IsBigBlind
			pMap["sitting_out"] = actualPlayer.SittingOut

			if pName == currentUsername {
				state["is_sitting_out"] = actualPlayer.SittingOut
			}

			if pName == currentUsername {
				pMap["is_me"] = true
				pMap["cards"] = actualPlayer.Cards
				pMap["hand_name"] = table.Engine.SolveHandFor(actualPlayer)
			} else {
				pMap["is_me"] = false
				if table.Engine.Game.Intermission || table.Engine.Game.GameFinished {
					pMap["cards"] = actualPlayer.Cards
					pMap["hand_name"] = table.Engine.SolveHandFor(actualPlayer)
				} else {
					pMap["cards"] = [2]string{"1B", "1B"}
					pMap["hand_name"] = ""
				}
			}
		}
	}
	state["players"] = playersList
	return state
}
