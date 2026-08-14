package poker

import "time"

var Phases = []string{"preflop", "flop", "turn", "river", "showdown"}

type Winner struct {
	Name         string   `json:"name"`
	SeatIndex    int      `json:"seat_index"`
	Amount       int      `json:"amount"`
	HandName     string   `json:"hand_name"`
	HandRank     int      `json:"hand_rank"`
	WinningCards []string `json:"winning_cards"`
	HoleCards    []string `json:"hole_cards"`
	BoardCards   []string `json:"board_cards"`
	IsTie        bool     `json:"is_tie"`
	TiedWith     []string `json:"tied_with,omitempty"`
}

type PotAward struct {
	PotIndex int      `json:"pot_index"`
	Amount   int      `json:"amount"`
	Winner   string   `json:"winner"`
	HandName string   `json:"hand_name"`
	Cards    []string `json:"cards"`
}

// Payout records a single chip credit to a single player from a single pot,
// including split-pot ties where PotAward only records one of the winners.
// It's the hook point callers use to apply rake/referral commission to each
// individual share of a hand's winnings.
type Payout struct {
	PotIndex int    `json:"pot_index"`
	Winner   string `json:"winner"`
	Amount   int    `json:"amount"`
}

const MaxNotifications = 8

const (
	DefaultSmallBlind = 10
	DefaultBigBlind   = 20
	DefaultLastRaise  = 20
	StartingChips     = 2000
)

type Stats struct {
	Hands          int `json:"hands"`
	HandsWon       int `json:"hands_won"`
	VPIP           int `json:"vpip"`
	PFR            int `json:"pfr"`
	Calls          int `json:"calls"`
	AggressiveActs int `json:"aggressive_acts"`
	Showdowns      int `json:"showdowns"`
	ShowdownsWon   int `json:"showdowns_won"`
	Folds          int `json:"folds"`
	FoldsPreflop   int `json:"folds_preflop"`
	FoldsPostflop  int `json:"folds_postflop"`
	AllIns         int `json:"allins"`
}

func NewStats() Stats {
	return Stats{}
}

type BotLine struct {
	PreflopAggressor       bool  `json:"preflop_aggressor"`
	CBetIntent             *bool `json:"cbet_intent"`
	BarrelIntent           *bool `json:"barrel_intent"`
	CBetMade               bool  `json:"cbet_made"`
	BarrelMade             bool  `json:"barrel_made"`
	NonValueAggressionMade bool  `json:"non_value_aggression_made"`
}

func NewBotLine() BotLine {
	return BotLine{}
}

type Player struct {
	ID             int       `json:"id,omitempty"`
	GameID         int64     `json:"game_id,omitempty"`
	Name           string    `json:"name"`
	SeatIndex      int       `json:"seat_index"`
	IsBot          bool      `json:"is_bot"`
	Chips          int       `json:"chips"`
	RoundBet       int       `json:"round_bet"`
	TotalBet       int       `json:"total_bet"`
	Folded         bool      `json:"folded"`
	AllIn          bool      `json:"all_in"`
	HasActed       bool      `json:"has_acted"`
	IsDealer       bool      `json:"dealer"`
	IsSmallBlind   bool      `json:"small_blind"`
	IsBigBlind     bool      `json:"big_blind"`
	Cards          [2]string `json:"cards"`
	WinProbability *float64  `json:"win_probability,omitempty"`
	HandName       string    `json:"hand_name,omitempty"`
	Stats          Stats     `json:"stats"`
	BotLine        BotLine   `json:"bot_line"`
}

type Game struct {
	ID                    int64      `json:"id,omitempty"`
	TableID               string     `json:"table_id"`
	Phase                 string     `json:"phase"`
	PhaseIndex            int        `json:"-"`
	Pot                   int        `json:"pot"`
	CurrentBet            int        `json:"current_bet"`
	LastRaise             int        `json:"last_raise"`
	SmallBlind            int        `json:"small_blind"`
	BigBlind              int        `json:"big_blind"`
	RaisesThisRound       int        `json:"raises_this_round"`
	DealerOrbitCount      int        `json:"dealer_orbit_count"`
	GameStarted           bool       `json:"game_started"`
	GameFinished          bool       `json:"game_finished"`
	OpenCardsMode         bool       `json:"open_cards_mode"`
	SpectatorMode         bool       `json:"spectator_mode"`
	InitialDealerName     *string    `json:"initial_dealer_name,omitempty"`
	Deck                  []string   `json:"-"`
	CardGraveyard         []string   `json:"-"`
	CommunityCards        []string   `json:"community_cards"`
	CurrentPlayerIdx      int        `json:"-"`
	TotalHands            int        `json:"total_hands"`
	CreatedAt             time.Time  `json:"created_at,omitempty"`
	UpdatedAt             time.Time  `json:"updated_at,omitempty"`
	Version               int        `json:"version"`
	Notifications         []string   `json:"notifications"`
	Intermission          bool       `json:"intermission"`
	LastWinner            *Winner    `json:"last_winner,omitempty"`
	PotAwards             []PotAward `json:"pot_awards,omitempty"`
	Payouts               []Payout   `json:"-"`
	IntermissionStartedAt int64      `json:"intermission_started_at,omitempty"`
}

func NewGame(tableID string) *Game {
	return &Game{
		TableID:          tableID,
		Phase:            Phases[0],
		PhaseIndex:       0,
		Pot:              0,
		CurrentBet:       0,
		LastRaise:        DefaultLastRaise,
		SmallBlind:       DefaultSmallBlind,
		BigBlind:         DefaultBigBlind,
		RaisesThisRound:  0,
		DealerOrbitCount: -1,
		GameStarted:      false,
		GameFinished:     false,
		OpenCardsMode:    false,
		SpectatorMode:    false,
		Deck:             []string{},
		CardGraveyard:    []string{},
		CommunityCards:   []string{},
		CurrentPlayerIdx: 0,
		TotalHands:       0,
		Version:          0,
		Notifications:    []string{},
	}
}

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
