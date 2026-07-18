// Package poker is a direct port of the poker5 Django app to Go.
//
// models.go defines the plain-Go types that mirror the Django models in
// poker/models.py. JSON-encoded columns (deck, community cards, stats, …) are
// exposed here as native Go slices / structs; serialization happens at the
// SQLite storage boundary in storage.go.
package poker

import "time"

// Phases is the ordered list of game phases. It mirrors PHASES in engine.py.
var Phases = []string{"preflop", "flop", "turn", "river", "showdown"}

// Winner captures the outcome of the most recent showdown. It is the
// payload that drives the post-hand modal: the winning player's name,
// the amount they were paid, the descriptive name of their best hand,
// and the 5 cards (2 hole + 3..5 board) that composed it.
//
// Tied pots set IsTie = true and populate TiedWith with the names of the
// other winners.
type Winner struct {
	Name          string   `json:"name"`
	SeatIndex     int      `json:"seat_index"`
	Amount        int      `json:"amount"`
	HandName      string   `json:"hand_name"`
	HandRank      int      `json:"hand_rank"`
	WinningCards  []string `json:"winning_cards"`
	HoleCards     []string `json:"hole_cards"`
	BoardCards    []string `json:"board_cards"`
	IsTie         bool     `json:"is_tie"`
	TiedWith      []string `json:"tied_with,omitempty"`
}

// PotAward is one row in the per-side-pot breakdown returned alongside
// the winner. It is used by the UI to animate chips from the pot to the
// winner.
type PotAward struct {
	PotIndex  int      `json:"pot_index"`
	Amount    int      `json:"amount"`
	Winner    string   `json:"winner"`
	HandName  string   `json:"hand_name"`
	Cards     []string `json:"cards"`
}

// MaxNotifications is the number of recent notifications retained on a game.
const MaxNotifications = 8

// Default chip / blind values used when a new game row is created.
const (
	DefaultSmallBlind = 10
	DefaultBigBlind   = 20
	DefaultLastRaise  = 20
	StartingChips     = 2000
)

// Stats tracks the per-player statistics tracked across hands.
//
// Mirrors the dict literal that poker/engine.py::GameEngine.init_game writes
// into Player.stats_data.
type Stats struct {
	Hands           int     `json:"hands"`
	HandsWon        int     `json:"hands_won"`
	VPIP            int     `json:"vpip"`
	PFR             int     `json:"pfr"`
	Calls           int     `json:"calls"`
	AggressiveActs  int     `json:"aggressive_acts"`
	Showdowns       int     `json:"showdowns"`
	ShowdownsWon    int     `json:"showdowns_won"`
	Folds           int     `json:"folds"`
	FoldsPreflop    int     `json:"folds_preflop"`
	FoldsPostflop   int     `json:"folds_postflop"`
	AllIns          int     `json:"allins"`
}

// NewStats returns a zeroed Stats struct, matching the literal produced by
// GameEngine.init_game.
func NewStats() Stats {
	return Stats{}
}

// BotLine mirrors the per-player bot bookkeeping dictionary the engine keeps
// on each player record (cbet / barrel intent, aggression flags, …).
type BotLine struct {
	PreflopAggressor       bool `json:"preflop_aggressor"`
	CBetIntent             *bool `json:"cbet_intent"`
	BarrelIntent           *bool `json:"barrel_intent"`
	CBetMade               bool `json:"cbet_made"`
	BarrelMade             bool `json:"barrel_made"`
	NonValueAggressionMade bool `json:"non_value_aggression_made"`
}

// NewBotLine returns a freshly zeroed BotLine.
func NewBotLine() BotLine {
	return BotLine{}
}

// Player is the in-memory representation of a single player at the table.
//
// Fields map 1-to-1 to the Player model in poker/models.py, with two notable
// differences:
//   - cards is a [2]string instead of two separate card1 / card2 columns.
//   - bot bookkeeping lives in BotLine instead of being merged into Stats.
type Player struct {
	ID             int     `json:"id,omitempty"`
	GameID         int64   `json:"game_id,omitempty"`
	Name           string  `json:"name"`
	SeatIndex      int     `json:"seat_index"`
	IsBot          bool    `json:"is_bot"`
	Chips          int     `json:"chips"`
	RoundBet       int     `json:"round_bet"`
	TotalBet       int     `json:"total_bet"`
	Folded         bool    `json:"folded"`
	AllIn          bool    `json:"all_in"`
	IsDealer       bool    `json:"dealer"`
	IsSmallBlind   bool    `json:"small_blind"`
	IsBigBlind     bool    `json:"big_blind"`
	Cards          [2]string `json:"cards"`
	WinProbability *float64 `json:"win_probability,omitempty"`
	HandName       string  `json:"hand_name,omitempty"`
	Stats          Stats    `json:"stats"`
	BotLine        BotLine  `json:"bot_line"`
}

// Game is the in-memory representation of a poker table.
//
// It mirrors the Game model in poker/models.py; the original Text columns that
// stored JSON-encoded data (deck, community_cards, card_graveyard,
// notifications) are typed as native Go slices.
type Game struct {
	ID                int64     `json:"id,omitempty"`
	TableID           string    `json:"table_id"`
	Phase             string    `json:"phase"`
	PhaseIndex        int       `json:"-"`
	Pot               int       `json:"pot"`
	CurrentBet        int       `json:"current_bet"`
	LastRaise         int       `json:"last_raise"`
	SmallBlind        int       `json:"small_blind"`
	BigBlind          int       `json:"big_blind"`
	RaisesThisRound   int       `json:"raises_this_round"`
	DealerOrbitCount  int       `json:"dealer_orbit_count"`
	GameStarted       bool      `json:"game_started"`
	GameFinished      bool      `json:"game_finished"`
	OpenCardsMode     bool      `json:"open_cards_mode"`
	SpectatorMode     bool      `json:"spectator_mode"`
	InitialDealerName *string   `json:"initial_dealer_name,omitempty"`
	Deck              []string  `json:"-"`
	CardGraveyard     []string  `json:"-"`
	CommunityCards    []string  `json:"community_cards"`
	CurrentPlayerIdx  int       `json:"-"`
	TotalHands        int       `json:"total_hands"`
	CreatedAt         time.Time `json:"created_at,omitempty"`
	UpdatedAt         time.Time `json:"updated_at,omitempty"`
	Version           int       `json:"version"`
	Notifications     []string  `json:"notifications"`
	// Intermission is true after a showdown while the engine is waiting
	// for the next hand to start (either via the 5-second countdown or
	// an explicit /api/game/next call). The winner modal is shown to the
	// client while this flag is true.
	Intermission bool `json:"intermission"`
	// LastWinner is the outcome of the most recent showdown, populated
	// whenever Intermission becomes true.
	LastWinner *Winner `json:"last_winner,omitempty"`
	// PotAwards lists the per-side-pot payouts of the most recent
	// showdown. Used by the UI to animate chips from pot -> winner.
	PotAwards []PotAward `json:"pot_awards,omitempty"`
	// IntermissionStartedAt is the timestamp at which the intermission
	// began (used to compute the countdown client-side).
	IntermissionStartedAt int64 `json:"intermission_started_at,omitempty"`
}

// NewGame returns a Game populated with the same defaults the Django model
// applies (see poker/models.py::Game).
func NewGame(tableID string) *Game {
	return &Game{
		TableID:         tableID,
		Phase:           Phases[0],
		PhaseIndex:      0,
		Pot:             0,
		CurrentBet:      0,
		LastRaise:       DefaultLastRaise,
		SmallBlind:      DefaultSmallBlind,
		BigBlind:        DefaultBigBlind,
		RaisesThisRound: 0,
		DealerOrbitCount: -1,
		GameStarted:     false,
		GameFinished:    false,
		OpenCardsMode:   false,
		SpectatorMode:   false,
		Deck:            []string{},
		CardGraveyard:   []string{},
		CommunityCards:  []string{},
		CurrentPlayerIdx: 0,
		TotalHands:      0,
		Version:         0,
		Notifications:   []string{},
	}
}

// SuitSymbol returns the Unicode glyph used by the original Python
// SUIT_SYMBOLS map in bot.py. Kept here so callers don't have to depend on the
// bot package for a one-line lookup.
func SuitSymbol(suit byte) string {
	switch suit {
	case 'C':
		return "\u2663"
	case 'D':
		return "\u2666"
	case 'H':
		return "\u2665"
	case 'S':
		return "\u2660"
	default:
		return string(suit)
	}
}