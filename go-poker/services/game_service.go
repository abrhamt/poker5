package services

import (
	"context"
	"errors"
	"sync"
	"time"

	poker "github.com/zuse/poker5/go-poker"
	"github.com/zuse/poker5/go-poker/repository"
)

var (
	ErrTableFull           = errors.New("table is full")
	ErrAlreadySeated       = errors.New("already seated at table")
	ErrNotYourTurn         = errors.New("it is not your turn")
	ErrPlayerNotFound      = errors.New("player not seated at table")
	ErrInvalidAction       = errors.New("invalid poker action")
	ErrNotHost             = errors.New("only the room host can start early")
	ErrPublicNoManualStart = errors.New("public tables start automatically; manual start is not available")
	ErrHandInProgress      = errors.New("a hand is currently in progress; wait for it to finish before joining")
	ErrInsufficientFunds   = errors.New("your wallet balance must be greater than the big blind to join this table")
)

const (
	DefaultTurnSeconds  = 25
	FastStartSeconds    = 2
	IntermissionSeconds = 10
)

type ActiveTable struct {
	mu                 sync.Mutex
	RoomCode           string
	RoomType           string
	HostUserID         int64
	BuyIn              int64
	MaxPlayers         int32
	Engine             *poker.GameEngine
	UserMap            map[int64]int
	UserIDs            map[string]int64
	TurnTimer          *time.Timer
	TimerExpiry        time.Time
	TurnSeconds        int
	CountdownActive    bool
	CountdownTimer     *time.Timer
	CountdownEndsAt    time.Time
	IntermissionTimer  *time.Timer
	IntermissionEndsAt time.Time
}

type GameService struct {
	mu       sync.RWMutex
	tables   map[string]*ActiveTable
	wallet   *WalletService
	sse      *SSEHub
	settings *SettingsService
	storage  poker.Storage
	rooms    *RoomService
}

// NewGameService wires GameService to storage for live game state. Pass a
// poker.SQLiteStorage (see poker.NewSQLiteStorageFromDB) in production so a
// server restart doesn't strand seated players' chips - a fresh table would
// otherwise come back empty while their wallets stay debited from the
// original buy-in. poker.NewMemoryStorage() is fine for tests.
func NewGameService(wallet *WalletService, sse *SSEHub, settings *SettingsService, storage poker.Storage, rooms *RoomService) *GameService {
	return &GameService{
		tables:   make(map[string]*ActiveTable),
		wallet:   wallet,
		sse:      sse,
		settings: settings,
		storage:  storage,
		rooms:    rooms,
	}
}

// lookupTable finds a room's live table, rehydrating it from storage via
// GetOrCreateTable if the process has restarted since anyone last touched
// it (s.tables is in-memory only and starts empty on every boot). Without
// this, every entry point except JoinTable would report "table not found"
// for a table that actually still has seated players and chips sitting in
// storage, leaving them unable to act or cash out until someone re-joins.
func (s *GameService) lookupTable(ctx context.Context, roomCode string) (*ActiveTable, error) {
	s.mu.RLock()
	table, exists := s.tables[roomCode]
	s.mu.RUnlock()
	if exists {
		return table, nil
	}

	if s.rooms == nil {
		return nil, ErrTableNotFound
	}
	room, err := s.rooms.GetRoomByCode(ctx, roomCode)
	if err != nil {
		return nil, ErrTableNotFound
	}
	return s.GetOrCreateTable(room), nil
}

// GetOrCreateTable returns the live table for a room, rehydrating it from
// storage on its first access after a server restart (or process start) so
// seated players and their chip stacks survive. Timers (turn/countdown/
// intermission) aren't persisted - they're runtime-only - so a rehydrated
// table that was mid-hand or mid-intermission gets its timers re-armed
// fresh below instead of picking up wherever the old process left off.
func (s *GameService) GetOrCreateTable(room *repository.PokerRoom) *ActiveTable {
	s.mu.Lock()
	defer s.mu.Unlock()

	if table, exists := s.tables[room.RoomCode]; exists {
		return table
	}

	eng := poker.NewGameEngine(s.storage)
	g, players, isNew, err := s.storage.GetOrCreateGame(room.RoomCode)
	if err != nil {
		g = poker.NewGame(room.RoomCode)
		players = []*poker.Player{}
		isNew = true
	}
	eng.Game = g
	eng.Players = players
	if isNew {
		eng.Game.SmallBlind = int(room.SmallBlind)
		eng.Game.BigBlind = int(room.BigBlind)
		eng.Game.LastRaise = int(room.BigBlind)
	}

	var hostID int64
	if room.HostUserID.Valid {
		hostID = room.HostUserID.Int64
	}

	table := &ActiveTable{
		RoomCode:    room.RoomCode,
		RoomType:    room.RoomType,
		HostUserID:  hostID,
		BuyIn:       room.BuyIn,
		MaxPlayers:  room.MaxPlayers,
		Engine:      eng,
		UserMap:     make(map[int64]int),
		UserIDs:     make(map[string]int64),
		TurnSeconds: DefaultTurnSeconds,
	}
	for _, p := range players {
		if p.ID != 0 {
			table.UserMap[int64(p.ID)] = p.SeatIndex
			table.UserIDs[p.Name] = int64(p.ID)
		}
	}

	s.tables[room.RoomCode] = table

	if !isNew && len(players) >= 2 {
		switch {
		case eng.Game.Intermission:
			s.startIntermissionTimer(table)
		case eng.Game.GameStarted:
			s.resetTurnTimer(table)
		default:
			s.evaluateCountdown(context.Background(), table)
		}
	}

	return table
}

// TableLiveSummary is the minimal live snapshot of a table shown on the
// public room list, where we don't want to expose full game state.
type TableLiveSummary struct {
	PlayerCount       int
	GameStarted       bool
	CountdownActive   bool
	CountdownEndsAtMs int64
}

// GetLiveSummary returns the live in-memory snapshot for a room code, if a
// table has been created for it yet (ok=false otherwise, e.g. a public room
// nobody has joined since server start).
func (s *GameService) GetLiveSummary(roomCode string) (TableLiveSummary, bool) {
	s.mu.RLock()
	table, exists := s.tables[roomCode]
	s.mu.RUnlock()
	if !exists {
		return TableLiveSummary{}, false
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	return TableLiveSummary{
		PlayerCount:       len(table.Engine.Players),
		GameStarted:       table.Engine.Game.GameStarted,
		CountdownActive:   table.CountdownActive,
		CountdownEndsAtMs: table.CountdownEndsAt.UnixMilli(),
	}, true
}

// GetSeatedRoomCode returns the room code of the table userID is currently
// seated at, if any. Used to power the lobby's "resume your active table"
// banner.
func (s *GameService) GetSeatedRoomCode(userID int64) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for code, table := range s.tables {
		table.mu.Lock()
		_, seated := table.UserMap[userID]
		table.mu.Unlock()
		if seated {
			return code, true
		}
	}
	return "", false
}

func (s *GameService) GetTable(roomCode string) (*ActiveTable, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	table, exists := s.tables[roomCode]
	if !exists {
		return nil, ErrTableNotFound
	}
	return table, nil
}

var ErrTableNotFound = errors.New("table not found")

func (s *GameService) autoLeaveOtherTables(ctx context.Context, userID int64, currentRoomCode string) {
	s.mu.RLock()
	var otherCodes []string
	for code, tbl := range s.tables {
		if code != currentRoomCode {
			tbl.mu.Lock()
			if _, seated := tbl.UserMap[userID]; seated {
				otherCodes = append(otherCodes, code)
			}
			tbl.mu.Unlock()
		}
	}
	s.mu.RUnlock()

	for _, code := range otherCodes {
		_ = s.LeaveTable(ctx, code, userID)
	}
}

func (s *GameService) JoinTable(ctx context.Context, room *repository.PokerRoom, user *repository.User) error {
	s.autoLeaveOtherTables(ctx, user.ID, room.RoomCode)

	table := s.GetOrCreateTable(room)

	table.mu.Lock()
	defer table.mu.Unlock()

	if _, ok := table.UserMap[user.ID]; ok {
		return nil
	}

	if table.Engine.Game.GameStarted && !table.Engine.Game.Intermission {
		return ErrHandInProgress
	}

	if int32(len(table.Engine.Players)) >= table.MaxPlayers {
		return ErrTableFull
	}

	// Private rooms have no fixed buy-in: a player brings their whole
	// wallet balance to the felt, as long as it clears the big blind.
	// Public rooms keep the fixed tier buy-in set at room creation.
	buyInAmount := table.BuyIn
	if table.RoomType == "private" {
		bigBlind := int64(table.Engine.Game.BigBlind)
		if user.Wallet <= bigBlind {
			return ErrInsufficientFunds
		}
		buyInAmount = user.Wallet
	}

	chips := int(buyInAmount)
	if chips <= 0 {
		chips = poker.StartingChips
	}

	_, err := s.wallet.BuyIn(ctx, user.ID, buyInAmount)
	if err != nil {
		return err
	}

	seatIdx := len(table.Engine.Players)
	player := &poker.Player{
		ID:        int(user.ID),
		Name:      user.Username,
		SeatIndex: seatIdx,
		IsBot:     false,
		Chips:     chips,
		Cards:     [2]string{"1B", "1B"},
		Stats:     poker.NewStats(),
		BotLine:   poker.NewBotLine(),
	}

	table.Engine.Players = append(table.Engine.Players, player)
	table.UserMap[user.ID] = seatIdx
	table.UserIDs[user.Username] = user.ID
	s.persistTable(table)

	s.evaluateCountdown(ctx, table)

	s.broadcastTableUpdate(table)
	return nil
}

// persistTable writes the table's current players/game state to storage
// immediately. Seat changes (join/leave) don't otherwise flow through the
// engine's own saveState() calls (those fire on betting/showdown actions),
// so without this a restart right after a join or leave would rehydrate
// stale seating. Caller must hold table.mu.
func (s *GameService) persistTable(table *ActiveTable) {
	if table.Engine.Storage == nil {
		return
	}
	_ = table.Engine.Storage.SaveGame(table.Engine.Game, table.Engine.Players)
}

// evaluateCountdown drives the "wait for players" state machine. Caller must
// hold table.mu.
func (s *GameService) evaluateCountdown(ctx context.Context, table *ActiveTable) {
	if table.Engine.Game.GameStarted || table.Engine.Game.Intermission {
		// A hand is either in progress or about to start again via the
		// intermission timer - the join-countdown machinery doesn't apply.
		return
	}
	count := len(table.Engine.Players)

	if count < 2 {
		s.cancelCountdownLocked(table)
		return
	}

	if int32(count) >= table.MaxPlayers {
		s.cancelCountdownLocked(table)
		s.startCountdownLocked(table, FastStartSeconds)
		return
	}

	if !table.CountdownActive {
		seconds := DefaultTurnSeconds
		if settings, err := s.settings.Get(ctx); err == nil {
			seconds = int(settings.CountdownSeconds)
		}
		s.startCountdownLocked(table, seconds)
	}
	// else: countdown already running, more joiners don't reset it.
}

func (s *GameService) startCountdownLocked(table *ActiveTable, seconds int) {
	table.CountdownActive = true
	table.CountdownEndsAt = time.Now().Add(time.Duration(seconds) * time.Second)
	roomCode := table.RoomCode
	table.CountdownTimer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		s.fireCountdown(roomCode)
	})
}

func (s *GameService) cancelCountdownLocked(table *ActiveTable) {
	if table.CountdownTimer != nil {
		table.CountdownTimer.Stop()
		table.CountdownTimer = nil
	}
	table.CountdownActive = false
}

func (s *GameService) fireCountdown(roomCode string) {
	s.mu.RLock()
	table, exists := s.tables[roomCode]
	s.mu.RUnlock()
	if !exists {
		return
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	table.CountdownActive = false
	table.CountdownTimer = nil

	if len(table.Engine.Players) < 2 || table.Engine.Game.GameStarted {
		return
	}

	table.Engine.Game.Intermission = false
	table.Engine.StartHand()
	s.resetTurnTimer(table)
	s.broadcastTableUpdate(table)
}

// startIntermissionTimer schedules the next hand to auto-start once the
// post-showdown intermission window elapses. Caller must hold table.mu.
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

func (s *GameService) LeaveTable(ctx context.Context, roomCode string, userID int64) error {
	table, err := s.lookupTable(ctx, roomCode)
	if err != nil {
		return err
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	seatIdx, ok := table.UserMap[userID]
	if !ok {
		return nil
	}

	var remainingChips int
	for i, p := range table.Engine.Players {
		if p.SeatIndex == seatIdx {
			remainingChips = p.Chips
			table.Engine.Players = append(table.Engine.Players[:i], table.Engine.Players[i+1:]...)
			break
		}
	}

	delete(table.UserMap, userID)

	for i, p := range table.Engine.Players {
		p.SeatIndex = i
		if uid, found := table.UserIDs[p.Name]; found {
			table.UserMap[uid] = i
		}
	}

	if remainingChips > 0 {
		_, _ = s.wallet.CashOut(ctx, userID, int64(remainingChips))
	}

	if len(table.Engine.Players) < 2 {
		table.Engine.Game.GameStarted = false
		if table.TurnTimer != nil {
			table.TurnTimer.Stop()
		}
		if table.IntermissionTimer != nil {
			table.IntermissionTimer.Stop()
			table.IntermissionTimer = nil
		}
		s.cancelCountdownLocked(table)
	}

	s.persistTable(table)
	s.broadcastTableUpdate(table)
	return nil
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

// StartHandManually lets a private room's host start the hand immediately,
// skipping the rest of the countdown. Public tables only ever start via the
// countdown itself.
func (s *GameService) StartHandManually(ctx context.Context, roomCode string, userID int64) error {
	table, err := s.lookupTable(ctx, roomCode)
	if err != nil {
		return err
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	if table.RoomType != "private" {
		return ErrPublicNoManualStart
	}
	if table.HostUserID == 0 || table.HostUserID != userID {
		return ErrNotHost
	}
	if len(table.Engine.Players) < 2 {
		return errors.New("minimum 2 players required to start")
	}

	s.cancelCountdownLocked(table)
	if table.IntermissionTimer != nil {
		table.IntermissionTimer.Stop()
		table.IntermissionTimer = nil
	}
	table.Engine.Game.Intermission = false
	table.Engine.StartHand()
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

	if table.Engine.Game.Intermission && table.Engine.Game.LastWinner != nil {
		s.handlePotCommission(context.Background(), table)
		s.startIntermissionTimer(table)
	}

	s.resetTurnTimer(table)
	s.broadcastTableUpdate(table)
}

// handlePotCommission rakes every individual payout from the hand that just
// ended (main pot and any side pots) and pays out the referral commission,
// crediting the site's and referrer's real wallets immediately. The
// winner(s) keep their net share as in-play chips; only the raked-off
// portion actually leaves the table. Called with table.mu already held.
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

	state["countdown_active"] = table.CountdownActive
	countdownRemaining := int(time.Until(table.CountdownEndsAt).Seconds())
	if countdownRemaining < 0 {
		countdownRemaining = 0
	}
	state["countdown_remaining_seconds"] = countdownRemaining
	state["countdown_ends_at_ms"] = table.CountdownEndsAt.UnixMilli()
	state["intermission_ends_at_ms"] = table.IntermissionEndsAt.UnixMilli()

	activeP := s.getCurrentTurnPlayer(table)
	if activeP != nil {
		state["current_turn_player"] = activeP.Name
	}

	s.sse.Broadcast(table.RoomCode, "game-state", state)
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

	state["countdown_active"] = table.CountdownActive
	countdownRemaining := int(time.Until(table.CountdownEndsAt).Seconds())
	if countdownRemaining < 0 {
		countdownRemaining = 0
	}
	state["countdown_remaining_seconds"] = countdownRemaining
	state["countdown_ends_at_ms"] = table.CountdownEndsAt.UnixMilli()
	state["intermission_ends_at_ms"] = table.IntermissionEndsAt.UnixMilli()

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
