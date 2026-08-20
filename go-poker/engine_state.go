package poker

import (
	"sync"
)

func (e *GameEngine) saveState() {
	if e.Game == nil || e.Storage == nil {
		return
	}
	if e.Game.PhaseIndex < len(Phases) {
		e.Game.Phase = Phases[e.Game.PhaseIndex]
	} else {
		e.Game.Phase = Phases[0]
	}
	e.Game.Version++
	if len(e.Game.Notifications) > MaxNotifications {
		e.Game.Notifications = e.Game.Notifications[len(e.Game.Notifications)-MaxNotifications:]
	}
	for i, p := range e.Players {
		p.SeatIndex = i
	}
	_ = e.Storage.SaveGame(e.Game, e.Players)
	if e.OnStateChanged != nil {
		e.OnStateChanged()
	}
}

func (e *GameEngine) SolveHandFor(p *Player) string {
	if p == nil || p.Folded {
		return ""
	}
	if p.Cards[0] == "" || p.Cards[0] == "1B" || p.Cards[1] == "" || p.Cards[1] == "1B" {
		return ""
	}
	if len(e.Game.CommunityCards) < 3 {
		return ""
	}
	full := []string{p.Cards[0], p.Cards[1]}
	full = append(full, e.Game.CommunityCards...)
	h := Solve(full, StandardRules())
	if h == nil {
		return ""
	}
	return h.Descr
}

func (e *GameEngine) ToDict() map[string]interface{} {
	reveal := e.Game.OpenCardsMode || e.Game.SpectatorMode
	phase := ""
	if e.Game.PhaseIndex < len(Phases) {
		phase = Phases[e.Game.PhaseIndex]
	}
	players := make([]map[string]interface{}, 0, len(e.Players))
	for _, p := range e.Players {
		cards := [2]string{"1B", "1B"}
		if reveal {
			cards = p.Cards
		}
		handName := ""
		if reveal {
			handName = e.SolveHandFor(p)
		}
		players = append(players, map[string]interface{}{
			"name":            p.Name,
			"chips":           p.Chips,
			"round_bet":       p.RoundBet,
			"total_bet":       p.TotalBet,
			"folded":          p.Folded,
			"all_in":          p.AllIn,
			"is_bot":          p.IsBot,
			"dealer":          p.IsDealer,
			"small_blind":     p.IsSmallBlind,
			"big_blind":       p.IsBigBlind,
			"cards":           cards,
			"sitting_out":     p.SittingOut,
			"seat_index":      p.SeatIndex,
			"stats":           p.Stats,
			"win_probability": p.WinProbability,
			"hand_name":       handName,
		})
	}
	notifs := e.Game.Notifications
	if len(notifs) > MaxNotifications {
		notifs = notifs[len(notifs)-MaxNotifications:]
	}
	return map[string]interface{}{
		"phase":                  phase,
		"pot":                    e.Game.Pot,
		"current_bet":            e.Game.CurrentBet,
		"last_raise":             e.Game.LastRaise,
		"small_blind":            e.Game.SmallBlind,
		"big_blind":              e.Game.BigBlind,
		"raises_this_round":      e.Game.RaisesThisRound,
		"dealer_orbit_count":     e.Game.DealerOrbitCount,
		"community_cards":        e.Game.CommunityCards,
		"players":                players,
		"game_started":           e.Game.GameStarted,
		"game_finished":          e.Game.GameFinished,
		"open_cards_mode":        e.Game.OpenCardsMode,
		"spectator_mode":         e.Game.SpectatorMode,
		"total_hands":            e.Game.TotalHands,
		"notifications":          notifs,
		"version":                e.Game.Version,
		"intermission":           e.Game.Intermission,
		"intermission_started_at": e.Game.IntermissionStartedAt,
		"winner":                 e.Game.LastWinner,
		"pot_awards":             e.Game.PotAwards,
	}
}

type GameState struct {
	Storage Storage
	engines map[string]*GameEngine
	mu      sync.Mutex
}

func NewGameState(storage Storage) *GameState {
	return &GameState{Storage: storage, engines: map[string]*GameEngine{}}
}

func (gs *GameState) GetOrCreate(tableID string) (*GameEngine, bool, error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if eng, ok := gs.engines[tableID]; ok {
		return eng, false, nil
	}
	g, players, created, err := gs.Storage.GetOrCreateGame(tableID)
	if err != nil {
		return nil, false, err
	}
	eng := &GameEngine{
		Game:    g,
		Players: players,
		Storage: gs.Storage,
	}
	gs.engines[tableID] = eng
	return eng, created, nil
}

func (gs *GameState) Engines() map[string]*GameEngine {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	out := make(map[string]*GameEngine, len(gs.engines))
	for k, v := range gs.engines {
		out[k] = v
	}
	return out
}

func (gs *GameState) SetEngine(tableID string, eng *GameEngine) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.engines[tableID] = eng
}

func (gs *GameState) Remove(tableID string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	delete(gs.engines, tableID)
}
