package poker

import (
	"math"
)

const (
	MaxRaisesPerRound    = 3
	ReraiseRatioStep     = 0.12
	ReraiseValueRatio    = 0.34
	ReraiseTopPairRatio  = 0.32
	StrengthTieDelta     = 0.25
	OddsTieDelta         = 0.02
	OpponentThreshold    = 3
	AggFactor            = 0.1
	ThresholdFactor      = 0.3
	MinHandsForWeight    = 10
	WeightGrowth         = 10
	AllInHandPreflop     = 0.85
	AllInHandPostflop    = 0.38

	MRatioDeadMax        = 1
	MRatioRedMax         = 5
	MRatioOrangeMax      = 10
	MRatioYellowMax      = 20
	DeadPushRatio        = 0.35
	RedPushRatio         = 0.7
	RedCallRatio         = 0.85
	OrangePushRatio      = 0.6
	OrangeCallRatio      = 0.8
	YellowRaiseRatio     = 0.6
	YellowCallRatio      = 0.7
	YellowShoveRatio     = 0.85
	PremiumPreflop       = 0.8
	PremiumPostflop      = 0.55
	GreenMaxStackBet     = 0.25
	ChipLeaderRaiseDelta = 0.05
	ShortStackCallDelta  = 0.05
	ShortStackRelative   = 0.6
	MinPreflopBluffRatio = 0.45

	CommitSPRMin         = 1.5
	CommitSPRMax         = 5.5
	CommitInvestStart    = 0.1
	CommitInvestEnd      = 0.6
	CommitCallRatioRef   = 0.25
	CommitmentPenaltyMax = 0.25

	PostflopCallBarrier  = 0.16

	EliminationRiskStart = 0.25
	EliminationRiskFull  = 0.8
	EliminationPenaltyMax = 0.25

	BotActionDelay       = 3.0
)

type BotDecision struct {
	Action string `json:"action"`
	Amount int    `json:"amount"`
}

type BotContext struct {
	CurrentBet      int
	Pot             int
	SmallBlind      int
	BigBlind        int
	RaisesThisRound int
	CurrentPhaseIdx int
	Players         []*Player
	LastRaise       int
	CommunityCards  []string
}

func suitSymbol(suit byte) string {
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

func FormatCard(code string) string {
	if len(code) < 2 {
		return code
	}
	r := string(code[0])
	if code[0] == 'T' {
		r = "10"
	}
	return r + suitSymbol(code[1])
}

func roundTo10(x float64) float64 { return math.Round(x/10) * 10 }

func handTiebreaker(h *Hand) float64 {
	base := 15.0
	value := 0.0
	factor := 1.0 / base
	for _, c := range h.Cards {
		value += float64(c.Rank) * factor
		factor /= base
	}
	return value
}

func SolvedHandScore(h *Hand) float64 {
	if h == nil {
		return 0
	}
	return float64(h.Rank) + handTiebreaker(h)
}

func solvedHandUsesHoleCards(hole []string, h *Hand) bool {
	if h == nil {
		return false
	}
	holeCards := []Card{NewCard(hole[0]), NewCard(hole[1])}
	for _, c := range h.Cards {
		for _, hc := range holeCards {
			if hc.Rank == c.Rank && hc.Suit == c.Suit {
				return true
			}
		}
	}
	return false
}

func calcFoldRate(p *Player) float64 {
	if p.Stats.Hands == 0 {
		return 0
	}
	return float64(p.Stats.Folds) / float64(p.Stats.Hands)
}

func avgFoldRate(opponents []*Player) float64 {
	if len(opponents) == 0 {
		return 0
	}
	sum := 0.0
	for _, p := range opponents {
		sum += calcFoldRate(p)
	}
	return sum / float64(len(opponents))
}

func isPocketPair(hole []string) bool {
	return len(hole) >= 2 && NewCard(hole[0]).Rank == NewCard(hole[1]).Rank
}