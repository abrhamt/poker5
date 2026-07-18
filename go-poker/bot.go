package poker

import (
	"math"
	"math/rand"
	"strings"
)

// Constants from poker/bot.py. Kept exported (CamelCase) for callers that
// want to tweak thresholds, but the bot itself reads them from these
// constants.
const (
	MaxRaisesPerRound   = 3
	ReraiseRatioStep    = 0.12
	ReraiseValueRatio   = 0.34
	ReraiseTopPairRatio = 0.32
	StrengthTieDelta    = 0.25
	OddsTieDelta        = 0.02
	OpponentThreshold   = 3
	AggFactor           = 0.1
	ThresholdFactor     = 0.3
	MinHandsForWeight   = 10
	WeightGrowth        = 10
	AllInHandPreflop    = 0.85
	AllInHandPostflop   = 0.38

	MRatioDeadMax      = 1
	MRatioRedMax       = 5
	MRatioOrangeMax    = 10
	MRatioYellowMax    = 20
	DeadPushRatio      = 0.35
	RedPushRatio       = 0.7
	RedCallRatio       = 0.85
	OrangePushRatio    = 0.6
	OrangeCallRatio    = 0.8
	YellowRaiseRatio   = 0.6
	YellowCallRatio    = 0.7
	YellowShoveRatio   = 0.85
	PremiumPreflop     = 0.8
	PremiumPostflop    = 0.55
	GreenMaxStackBet   = 0.25
	ChipLeaderRaiseDelta = 0.05
	ShortStackCallDelta  = 0.05
	ShortStackRelative   = 0.6
	MinPreflopBluffRatio = 0.45

	CommitSPRMin        = 1.5
	CommitSPRMax        = 5.5
	CommitInvestStart   = 0.1
	CommitInvestEnd     = 0.6
	CommitCallRatioRef  = 0.25
	CommitmentPenaltyMax = 0.25

	PostflopCallBarrier = 0.16

	EliminationRiskStart = 0.25
	EliminationRiskFull  = 0.8
	EliminationPenaltyMax = 0.25

	BotActionDelay = 3.0
)

// BotDecision is the structured decision returned by ChooseBotAction.
//
// Mirrors the dict literal {action, amount} returned by choose_bot_action.
type BotDecision struct {
	Action string `json:"action"`
	Amount int    `json:"amount"`
}

// BotContext is the per-tick context the engine hands to the bot. It mirrors
// the dict literal built in engine.py::_process_bot_action.
type BotContext struct {
	CurrentBet       int
	Pot              int
	SmallBlind       int
	BigBlind         int
	RaisesThisRound  int
	CurrentPhaseIdx  int
	Players          []*Player
	LastRaise        int
	CommunityCards   []string
}

// suitSymbol returns the Unicode glyph for a suit character.
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

// FormatCard renders a card code for display: "T" becomes "10" and the suit
// is replaced by its Unicode glyph.
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

// roundTo10 mirrors round(x / 10) * 10 from bot.py.
func roundTo10(x float64) float64 { return math.Round(x/10) * 10 }

// handTiebreaker returns a fractional value used to break ties between two
// hands of the same rank. Higher is better, mirroring bot.py::hand_tiebreaker.
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

// SolvedHandScore returns rank + tiebreaker, used when comparing two
// solved Hand objects numerically.
func SolvedHandScore(h *Hand) float64 {
	if h == nil {
		return 0
	}
	return float64(h.Rank) + handTiebreaker(h)
}

// solvedHandUsesHoleCards mirrors bot.py::solved_hand_uses_hole_cards.
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

// calcFoldRate mirrors bot.py::calc_fold_rate.
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

// HandContext mirrors the dict returned by analyze_hand_context in bot.py.
type HandContext struct {
	IsTopPair  bool
	IsOverPair bool
}

func analyzeHandContext(hole, board []string) HandContext {
	cards := append([]string{}, hole...)
	cards = append(cards, board...)
	hand := Solve(cards, StandardRules())
	boardRanks := []int{}
	for _, c := range board {
		boardRanks = append(boardRanks, NewCard(c).Rank)
	}
	highest := 0
	for _, r := range boardRanks {
		if r > highest {
			highest = r
		}
	}
	pp := isPocketPair(hole)
	isTop := false
	isOver := false
	if hand.Name == HandOnePair && len(hand.Cards) > 0 {
		pairRank := hand.Cards[0].Rank
		isTop = pairRank == highest
		isOver = pp && pairRank > highest
	}
	return HandContext{IsTopPair: isTop, IsOverPair: isOver}
}

// DrawPotential mirrors the dict returned by analyze_draw_potential.
type DrawPotential struct {
	FlushDraw    bool
	StraightDraw bool
	Outs         int
}

func analyzeDrawPotential(hole, board []string) DrawPotential {
	all := append([]string{}, hole...)
	all = append(all, board...)
	suits := map[byte]int{}
	for _, c := range all {
		if len(c) < 2 {
			continue
		}
		suits[c[1]]++
	}
	maxSuit := 0
	hasFlush := false
	for _, n := range suits {
		if n > maxSuit {
			maxSuit = n
		}
		if n >= 5 {
			hasFlush = true
		}
	}
	flushDraw := !hasFlush && maxSuit == 4
	flushOuts := 0
	if flushDraw {
		flushOuts = 9
	}

	ranks := []int{}
	for _, c := range all {
		ranks = append(ranks, NewCard(c).Rank)
	}
	hasAce := false
	for _, r := range ranks {
		if r == 13 {
			hasAce = true
		}
	}
	if hasAce {
		ranks = append(ranks, 0)
	}
	unique := []int{}
	seen := map[int]bool{}
	for _, r := range ranks {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	sortInts(unique)

	hasStraight := false
	missingRanks := map[int]bool{}
	straightOuts := 0
	for start := 1; start <= 10; start++ {
		missing := []int{}
		for r := start; r < start+5; r++ {
			if !seen[r] {
				missing = append(missing, r)
			}
		}
		if len(missing) == 0 {
			hasStraight = true
			break
		}
		if len(missing) == 1 {
			flushDraw = flushDraw || true // straightDraw will be set below
			straightDraw := true
			_ = straightDraw
			missingRanks[missing[0]] = true
			if missing[0] == start || missing[0] == start+4 {
				straightOuts = 8
			}
		}
	}
	straightDraw := len(missingRanks) > 0
	if hasStraight {
		straightDraw = false
		straightOuts = 0
	} else if straightDraw && straightOuts == 0 {
		if len(missingRanks) >= 2 {
			straightOuts = 8
		} else {
			straightOuts = 4
		}
	}
	return DrawPotential{FlushDraw: flushDraw, StraightDraw: straightDraw, Outs: flushOuts + straightOuts}
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

var rankMap = map[byte]int{
	'2': 2, '3': 3, '4': 4, '5': 5, '6': 6, '7': 7, '8': 8,
	'9': 9, 'T': 10, 'J': 11, 'Q': 12, 'K': 13, 'A': 14,
}

// evaluateBoardTexture mirrors bot.py::evaluate_board_texture.
func evaluateBoardTexture(board []string) float64 {
	if len(board) < 3 {
		return 0
	}
	ranks := []int{}
	rankCounts := map[int]int{}
	suitCounts := map[byte]int{}
	for _, c := range board {
		if len(c) < 2 {
			continue
		}
		r := rankMap[c[0]]
		s := c[1]
		ranks = append(ranks, r)
		rankCounts[r]++
		suitCounts[s]++
	}
	maxRankCount := 0
	for _, n := range rankCounts {
		if n > maxRankCount {
			maxRankCount = n
		}
	}
	pairRisk := 0.0
	if maxRankCount > 1 {
		pairRisk = float64(maxRankCount-1) / float64(len(board)-1)
	}
	maxSuitCount := 0
	for _, n := range suitCounts {
		if n > maxSuitCount {
			maxSuitCount = n
		}
	}
	suitRisk := float64(maxSuitCount-1) / float64(len(board)-1)
	rfStraight := append([]int{}, ranks...)
	if containsInt(rfStraight, 14) {
		rfStraight = append(rfStraight, 1)
	}
	unique := []int{}
	seen := map[int]bool{}
	for _, r := range rfStraight {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	sortInts(unique)
	maxConsecutive := 1
	currentRun := 1
	for i := 1; i < len(unique); i++ {
		if unique[i] == unique[i-1]+1 {
			currentRun++
		} else {
			currentRun = 1
		}
		if currentRun > maxConsecutive {
			maxConsecutive = currentRun
		}
	}
	connectedness := 0.0
	if maxConsecutive >= 3 {
		connectedness = float64(maxConsecutive-2) / float64(len(board)-2)
		if connectedness < 0 {
			connectedness = 0
		}
	}
	textureRisk := (connectedness + suitRisk + pairRisk) / 3
	if textureRisk < 0 {
		textureRisk = 0
	}
	if textureRisk > 1 {
		textureRisk = 1
	}
	return textureRisk
}

func containsInt(a []int, v int) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}

// preflopHandScore mirrors bot.py::preflop_hand_score.
func preflopHandScore(cardA, cardB string) float64 {
	order := "23456789TJQKA"
	base := map[byte]float64{
		'A': 10, 'K': 8, 'Q': 7, 'J': 6, 'T': 5,
		'9': 4.5, '8': 4, '7': 3.5, '6': 3, '5': 2.5,
		'4': 2, '3': 1.5, '2': 1,
	}
	if len(cardA) < 2 || len(cardB) < 2 {
		return 0
	}
	r1, r2 := cardA[0], cardB[0]
	s1, s2 := cardA[1], cardB[1]
	i1 := strings.IndexByte(order, string(r1)[0])
	i2 := strings.IndexByte(order, string(r2)[0])
	if i1 < 0 || i2 < 0 {
		return 0
	}
	if i1 < i2 {
		r1, r2 = r2, r1
		s1, s2 = s2, s1
		i1, i2 = i2, i1
	}
	score := base[r1]
	if r1 == r2 {
		score *= 2
		if score < 5 {
			score = 5
		}
	}
	if s1 == s2 {
		score += 2
	}
	gap := i1 - i2 - 1
	switch {
	case gap == 1:
		score -= 1
	case gap == 2:
		score -= 2
	case gap == 3:
		score -= 4
	case gap >= 4:
		score -= 5
	}
	if gap <= 1 && i1 < strings.IndexByte(order, 'Q') {
		score += 1
	}
	if score < 0 {
		score = 0
	}
	if score > 10 {
		score = 10
	}
	return score
}

// findNextActivePlayer mirrors bot.py::find_next_active_player.
func findNextActivePlayer(players []*Player, startIdx int) *Player {
	for i := 1; i <= len(players); i++ {
		idx := (startIdx + i) % len(players)
		if !players[idx].Folded {
			return players[idx]
		}
	}
	return players[startIdx%len(players)]
}

// computePositionFactor mirrors bot.py::compute_position_factor.
func computePositionFactor(players []*Player, active []*Player, player *Player, currentPhase int) float64 {
	seatIdx := -1
	for i, p := range active {
		if p == player {
			seatIdx = i
		}
	}
	if seatIdx < 0 {
		return 0
	}
	var firstToAct *Player
	if currentPhase == 0 {
		bbIdx := 0
		for i, p := range players {
			if p.IsBigBlind {
				bbIdx = i
				break
			}
		}
		firstToAct = findNextActivePlayer(players, bbIdx)
	} else {
		dealerIdx := 0
		for i, p := range players {
			if p.IsDealer {
				dealerIdx = i
				break
			}
		}
		firstToAct = findNextActivePlayer(players, dealerIdx)
	}
	refIdx := 0
	for i, p := range active {
		if p == firstToAct {
			refIdx = i
		}
	}
	pos := (seatIdx - refIdx + len(active)) % len(active)
	if len(active) <= 1 {
		return 0
	}
	return float64(pos) / float64(len(active)-1)
}

// HandStrength mirrors the dict returned by evaluate_hand_strength.
type HandStrength struct {
	Strength   float64
	SolvedHand *Hand
}

func evaluateHandStrength(player *Player, communityCards []string, preflop bool) HandStrength {
	if preflop {
		return HandStrength{Strength: preflopHandScore(player.Cards[0], player.Cards[1])}
	}
	cards := []string{player.Cards[0], player.Cards[1]}
	cards = append(cards, communityCards...)
	sh := Solve(cards, StandardRules())
	return HandStrength{Strength: float64(sh.Rank) + handTiebreaker(sh), SolvedHand: sh}
}

// PostflopContext mirrors the dict returned by compute_postflop_context.
type PostflopContext struct {
	TopPair       bool
	OverPair      bool
	DrawChance    bool
	DrawOuts      int
	DrawEquity    float64
	TextureRisk   float64
}

func computePostflopContext(player *Player, communityCards []string, preflop bool) PostflopContext {
	ctx := PostflopContext{}
	if preflop || len(communityCards) < 3 {
		return ctx
	}
	hole := []string{player.Cards[0], player.Cards[1]}
	hctx := analyzeHandContext(hole, communityCards)
	ctx.TopPair = hctx.IsTopPair
	ctx.OverPair = hctx.IsOverPair
	draws := analyzeDrawPotential(hole, communityCards)
	ctx.DrawChance = draws.FlushDraw || draws.StraightDraw
	ctx.DrawOuts = draws.Outs
	if ctx.DrawOuts > 0 {
		df := 0.04
		if len(communityCards) == 4 {
			df = 0.02
		}
		if len(communityCards) == 5 {
			df = 0
		}
		ctx.DrawEquity = math.Min(1, float64(ctx.DrawOuts)*df)
	}
	ctx.TextureRisk = evaluateBoardTexture(communityCards)
	return ctx
}

// MZone returns the M-ratio zone used by Harrington push/fold tables.
func MZone(mRatio float64) string {
	switch {
	case mRatio < MRatioDeadMax:
		return "dead"
	case mRatio <= MRatioRedMax:
		return "red"
	case mRatio <= MRatioOrangeMax:
		return "orange"
	case mRatio <= MRatioYellowMax:
		return "yellow"
	default:
		return "green"
	}
}

// CommitmentMetrics mirrors the dict returned by compute_commitment_metrics.
type CommitmentMetrics struct {
	CommitmentPressure float64
	CommitmentPenalty  float64
}

func computeCommitmentMetrics(needToCall int, player *Player, spr float64, remainingStreets int) CommitmentMetrics {
	projected := player.TotalBet + maxInt(0, needToCall)
	denom := projected + player.Chips
	if denom < 1 {
		denom = 1
	}
	investedRatio := float64(projected) / float64(denom)
	callCostRatio := float64(needToCall) / float64(maxInt(1, player.Chips))
	sprPressure := math.Max(0, math.Min(1, (spr-CommitSPRMin)/(CommitSPRMax-CommitSPRMin)))
	investPressure := math.Max(0, math.Min(1, (investedRatio-CommitInvestStart)/(CommitInvestEnd-CommitInvestStart)))
	callPressure := math.Max(0, math.Min(1, callCostRatio/CommitCallRatioRef))
	streetPressure := math.Min(1, float64(remainingStreets)/2)
	commitmentPressure := (investPressure*0.6 + callPressure*0.4) * sprPressure * streetPressure
	commitmentPenalty := commitmentPressure * CommitmentPenaltyMax
	return CommitmentMetrics{CommitmentPressure: commitmentPressure, CommitmentPenalty: commitmentPenalty}
}

// EliminationMetrics mirrors the dict returned by compute_elimination_risk.
type EliminationMetrics struct {
	EliminationRisk    float64
	EliminationPenalty float64
}

func computeEliminationRisk(stackRatio float64) EliminationMetrics {
	risk := math.Max(0, math.Min(1, (stackRatio-EliminationRiskStart)/(EliminationRiskFull-EliminationRiskStart)))
	return EliminationMetrics{EliminationRisk: risk, EliminationPenalty: risk * EliminationPenaltyMax}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// HarringtonParams bundles the parameters of decide_harrington_action.
type HarringtonParams struct {
	MZone               string
	StrengthRatio       float64
	FacingRaise         bool
	NeedsToCall         bool
	NeedToCall          int
	PlayerChips         int
	CanShove            bool
	CanRaise            bool
	DeadPushThreshold   float64
	RedPushThreshold    float64
	OrangePushThreshold float64
	YellowRaiseThreshold float64
	YellowShoveThreshold float64
	RedCallThreshold    float64
	OrangeCallThreshold float64
	YellowCallThreshold float64
	YellowRaiseSize     func() int
}

// decideHarringtonAction implements the M-ratio push/fold decision tree from
// bot.py::decide_harrington_action.
func decideHarringtonAction(p HarringtonParams) *BotDecision {
	switch p.MZone {
	case "dead":
		if p.FacingRaise && p.NeedsToCall {
			if p.StrengthRatio >= p.DeadPushThreshold {
				if p.CanShove {
					return &BotDecision{Action: "raise", Amount: p.PlayerChips}
				}
				return &BotDecision{Action: "call", Amount: minInt(p.PlayerChips, p.NeedToCall)}
			}
			return &BotDecision{Action: "fold"}
		}
		if p.CanShove && p.StrengthRatio >= p.DeadPushThreshold {
			return &BotDecision{Action: "raise", Amount: p.PlayerChips}
		}
		if p.NeedsToCall {
			return &BotDecision{Action: "fold"}
		}
		return &BotDecision{Action: "check"}
	case "red":
		if p.FacingRaise && p.NeedsToCall {
			if p.StrengthRatio >= p.RedCallThreshold {
				return &BotDecision{Action: "call", Amount: minInt(p.PlayerChips, p.NeedToCall)}
			}
			return &BotDecision{Action: "fold"}
		}
		if p.CanShove && p.StrengthRatio >= p.RedPushThreshold {
			return &BotDecision{Action: "raise", Amount: p.PlayerChips}
		}
		if p.NeedsToCall {
			return &BotDecision{Action: "fold"}
		}
		return &BotDecision{Action: "check"}
	case "orange":
		if p.FacingRaise && p.NeedsToCall {
			if p.StrengthRatio >= p.OrangeCallThreshold {
				return &BotDecision{Action: "call", Amount: minInt(p.PlayerChips, p.NeedToCall)}
			}
			return &BotDecision{Action: "fold"}
		}
		if p.CanShove && p.StrengthRatio >= p.OrangePushThreshold {
			return &BotDecision{Action: "raise", Amount: p.PlayerChips}
		}
		if p.NeedsToCall {
			return &BotDecision{Action: "fold"}
		}
		return &BotDecision{Action: "check"}
	case "yellow":
		if p.FacingRaise && p.NeedsToCall {
			if p.CanShove && p.StrengthRatio >= p.YellowShoveThreshold {
				return &BotDecision{Action: "raise", Amount: p.PlayerChips}
			}
			if p.StrengthRatio >= p.YellowCallThreshold {
				return &BotDecision{Action: "call", Amount: minInt(p.PlayerChips, p.NeedToCall)}
			}
			return &BotDecision{Action: "fold"}
		}
		if p.CanShove && p.StrengthRatio >= p.YellowShoveThreshold {
			return &BotDecision{Action: "raise", Amount: p.PlayerChips}
		}
		if p.CanRaise && p.StrengthRatio >= p.YellowRaiseThreshold {
			return &BotDecision{Action: "raise", Amount: p.YellowRaiseSize()}
		}
		if p.NeedsToCall {
			return &BotDecision{Action: "fold"}
		}
		return &BotDecision{Action: "check"}
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ChooseBotAction is the Go port of bot.py::choose_bot_action.
//
// The behaviour, thresholds, randomization and decision tree match the
// Python implementation 1-to-1. Local helpers are kept as closures to make
// the value/bluff/protection/yellow raise size calculations identical to the
// Python nested functions.
func ChooseBotAction(player *Player, ctx BotContext) *BotDecision {
	currentBet := ctx.CurrentBet
	pot := ctx.Pot
	smallBlind := ctx.SmallBlind
	bigBlind := ctx.BigBlind
	raisesThisRound := ctx.RaisesThisRound
	currentPhaseIdx := ctx.CurrentPhaseIdx
	players := ctx.Players
	lastRaise := ctx.LastRaise
	communityCards := ctx.CommunityCards

	needToCall := currentBet - player.RoundBet
	needsToCall := needToCall > 0
	minRaiseAmount := lastRaise
	if needToCall+lastRaise > minRaiseAmount {
		minRaiseAmount = needToCall + lastRaise
	}
	potOdds := 0.0
	if pot+needToCall > 0 {
		potOdds = float64(needToCall) / float64(pot+needToCall)
	}
	stackRatio := 1.0
	if player.Chips > 0 {
		stackRatio = float64(needToCall) / float64(player.Chips)
	}
	spr := float64(player.Chips) / float64(maxInt(1, pot+needToCall))
	mRatio := float64(player.Chips) / float64(maxInt(1, smallBlind+bigBlind))
	facingRaise := false
	if currentPhaseIdx == 0 {
		facingRaise = currentBet > bigBlind
	} else {
		facingRaise = currentBet > 0
	}
	canRaise := raisesThisRound < MaxRaisesPerRound && player.Chips > bigBlind
	canShove := raisesThisRound < MaxRaisesPerRound

	active := []*Player{}
	for _, p := range players {
		if !p.Folded {
			active = append(active, p)
		}
	}
	opponents := []*Player{}
	for _, p := range active {
		if p.Name != player.Name {
			opponents = append(opponents, p)
		}
	}
	activeOpponents := len(opponents)
	opponentStacks := []int{}
	for _, p := range opponents {
		opponentStacks = append(opponentStacks, p.Chips)
	}
	maxOpponentStack := 0
	for _, s := range opponentStacks {
		if s > maxOpponentStack {
			maxOpponentStack = s
		}
	}
	effectiveStack := player.Chips
	if len(opponentStacks) > 0 && maxOpponentStack < effectiveStack {
		effectiveStack = maxOpponentStack
	}
	amChipLeader := true
	if len(opponentStacks) > 0 {
		amChipLeader = player.Chips > maxOpponentStack
	}
	shortStackRelative := false
	if len(opponentStacks) > 0 && effectiveStack == player.Chips && player.Chips < int(float64(maxOpponentStack)*ShortStackRelative) {
		shortStackRelative = true
	}
	botLine := player.BotLine
	nonValueAggressionMade := botLine.NonValueAggressionMade

	positionFactor := computePositionFactor(players, active, player, currentPhaseIdx)
	preflop := len(communityCards) == 0
	result := evaluateHandStrength(player, communityCards, preflop)
	strength := result.Strength
	solvedHand := result.SolvedHand
	holeCards := []string{player.Cards[0], player.Cards[1]}

	holeImprovesHand := false
	if !preflop && solvedHand != nil {
		if len(communityCards) < 5 {
			holeImprovesHand = solvedHandUsesHoleCards(holeCards, solvedHand)
		} else if len(communityCards) == 5 {
			boardHand := Solve(communityCards, StandardRules())
			holeImprovesHand = SolvedHandScore(solvedHand) > SolvedHandScore(boardHand)
		}
	}

	postflopCtx := computePostflopContext(player, communityCards, preflop)
	topPair := postflopCtx.TopPair
	overPair := postflopCtx.OverPair
	_ = postflopCtx.DrawChance
	drawOuts := postflopCtx.DrawOuts
	drawEquity := postflopCtx.DrawEquity
	textureRisk := postflopCtx.TextureRisk
	isMadeHand := !preflop && solvedHand != nil && solvedHand.Rank >= 2
	isDraw := drawOuts >= 8
	isWeakDraw := drawOuts > 0 && drawOuts < 8
	isDeadHand := !preflop && !isMadeHand && !isDraw && !isWeakDraw

	strengthBase := strength / 10
	strengthRatio := strengthBase
	mZone := MZone(mRatio)
	isGreenZone := mZone == "green"
	premiumHand := strengthRatio >= PremiumPreflop
	if !preflop {
		premiumHand = strengthRatio >= PremiumPostflop
	}
	raiseAggAdj := 0.0
	if amChipLeader {
		raiseAggAdj = -ChipLeaderRaiseDelta
	}
	callTightAdj := 0.0
	if shortStackRelative && stackRatio < EliminationRiskStart {
		callTightAdj = -ShortStackCallDelta
	}

	deadPushThreshold := math.Max(0, DeadPushRatio+raiseAggAdj)
	redPushThreshold := math.Max(0, RedPushRatio+raiseAggAdj)
	orangePushThreshold := math.Max(0, OrangePushRatio+raiseAggAdj)
	yellowRaiseThreshold := math.Max(0, YellowRaiseRatio+raiseAggAdj)
	yellowShoveThreshold := math.Max(0, YellowShoveRatio+raiseAggAdj)
	redCallThreshold := math.Min(1, RedCallRatio+callTightAdj)
	orangeCallThreshold := math.Min(1, OrangeCallRatio+callTightAdj)
	yellowCallThreshold := math.Min(1, YellowCallRatio+callTightAdj)
	useHarrington := preflop && !isGreenZone

	remainingStreets := 3
	switch len(communityCards) {
	case 3:
		remainingStreets = 2
	case 4:
		remainingStreets = 1
	case 5:
		remainingStreets = 0
	}
	cm := computeCommitmentMetrics(needToCall, player, spr, remainingStreets)
	commitmentPenalty := cm.CommitmentPenalty
	eliminationPenalty := 0.0
	if needsToCall {
		er := computeEliminationRisk(stackRatio)
		eliminationPenalty = er.EliminationPenalty
	}

	riskAdjRedCall := math.Min(1, redCallThreshold+eliminationPenalty)
	riskAdjOrangeCall := math.Min(1, orangeCallThreshold+eliminationPenalty)
	riskAdjYellowCall := math.Min(1, yellowCallThreshold+eliminationPenalty)

	callBarrierBase := 0.0
	if preflop {
		callBarrierBase = math.Min(1, math.Max(0, potOdds+callTightAdj))
	} else {
		callBarrierBase = math.Min(1, math.Max(0, PostflopCallBarrier+callTightAdj))
	}
	callBarrier := callBarrierBase
	if preflop {
		callBarrier = math.Min(1, callBarrierBase+commitmentPenalty)
	}

	if !preflop {
		callBarrierAdj := 0.0
		if holeImprovesHand {
			if overPair {
				callBarrierAdj -= 0.03
			} else if topPair {
				callBarrierAdj -= 0.02
			}
		}
		if drawOuts >= 8 {
			switch len(communityCards) {
			case 3:
				callBarrierAdj -= 0.02
			case 4:
				callBarrierAdj -= 0.01
			}
		}
		if activeOpponents <= 1 {
			callBarrierAdj -= 0.02
		}
		if textureRisk > 0.6 {
			callBarrierAdj += 0.02
		}
		if spr < 3 {
			callBarrierAdj -= 0.01
		} else if spr > 6 {
			callBarrierAdj += 0.01
		}
		if callBarrierAdj > 0.04 {
			callBarrierAdj = 0.04
		}
		if callBarrierAdj < -0.04 {
			callBarrierAdj = -0.04
		}

		streetIndex := 0
		switch len(communityCards) {
		case 3:
			streetIndex = 1
		case 4:
			streetIndex = 2
		case 5:
			streetIndex = 3
		}
		raiseLevelForCalls := 0
		if facingRaise && raisesThisRound > 0 {
			raiseLevelForCalls = raisesThisRound
		}
		streetPressure := 0.0
		if needsToCall {
			streetPressure = float64(streetIndex) * 0.01
		}
		weakDrawPressure := 0.0
		if needsToCall && isWeakDraw {
			weakDrawPressure = float64(streetIndex) * 0.01
		}
		deadHandPressure := 0.0
		if needsToCall && isDeadHand {
			deadHandPressure = float64(streetIndex) * 0.02
		}
		barrelPressure := 0.0
		if needsToCall {
			barrelPressure = float64(raiseLevelForCalls) * 0.02
		}
		potOddsAdj := 0.0
		if needsToCall {
			potOddsAdj = math.Max(-0.12, math.Min(0.08, (0.25-potOdds)*0.6))
		}
		potOddsShift := -potOddsAdj
		if needsToCall && isDeadHand {
			potOddsShift *= 0.35
		} else if needsToCall && isWeakDraw {
			potOddsShift *= 0.5
		}
		commitmentShift := 0.0
		if needsToCall {
			commitmentShift = commitmentPenalty * 0.8
		}
		callBarrier = callBarrierBase + callBarrierAdj + potOddsShift + commitmentShift
		callBarrier += streetPressure + weakDrawPressure + deadHandPressure + barrelPressure
		if needsToCall && isDeadHand {
			deadHandFloor := 0.0
			switch streetIndex {
			case 1:
				deadHandFloor = 0.20
			case 2:
				deadHandFloor = 0.22
			case 3:
				deadHandFloor = 0.24
			}
			if callBarrier < deadHandFloor {
				callBarrier = deadHandFloor
			}
		}
		if needsToCall && isWeakDraw {
			if streetIndex >= 2 {
				callBarrier = 1
			} else if streetIndex == 1 && (potOdds > 0.18 || raiseLevelForCalls > 0) {
				callBarrier = 1
			}
		}
		if callBarrier > 1 {
			callBarrier = 1
		}
		if callBarrier < 0.10 {
			callBarrier = 0.10
		}
	}

	eliminationBarrier := callBarrier
	if needsToCall {
		eliminationBarrier = math.Min(1, callBarrier+eliminationPenalty)
	}

	oppAggAdj := 0.0
	if activeOpponents < OpponentThreshold {
		oppAggAdj = float64(OpponentThreshold-activeOpponents) * AggFactor
	}
	thresholdAdj := 0.0
	if activeOpponents < OpponentThreshold {
		thresholdAdj = float64(OpponentThreshold-activeOpponents) * ThresholdFactor
	}
	baseAggr := 0.8 + 0.4*positionFactor
	if !preflop {
		baseAggr = 1 + 0.6*positionFactor
	}
	aggressiveness := baseAggr + oppAggAdj
	raiseThreshold := 8.0 - 2*positionFactor
	if !preflop {
		raiseThreshold = 2.6 - 0.8*positionFactor
	}
	if preflop {
		raiseThreshold = math.Max(1, raiseThreshold-thresholdAdj)
	} else {
		raiseThreshold = math.Max(1, raiseThreshold)
	}
	if amChipLeader {
		raiseThreshold = math.Max(1, raiseThreshold-ChipLeaderRaiseDelta*10)
	}
	decisionStrength := strength
	if !preflop {
		decisionStrength = strengthRatio * 10
	}

	bluffChance := 0.0
	foldRate := 0.0
	avgVPIP := 0.0
	avgAgg := 0.0

	statOpponents := []*Player{}
	for _, p := range players {
		if p.Name != player.Name {
			statOpponents = append(statOpponents, p)
		}
	}
	if len(statOpponents) > 0 {
		sumVPIP := 0.0
		for _, p := range statOpponents {
			sumVPIP += float64(p.Stats.VPIP+1) / float64(p.Stats.Hands+2)
		}
		avgVPIP = sumVPIP / float64(len(statOpponents))
		sumAgg := 0.0
		for _, p := range statOpponents {
			sumAgg += float64(p.Stats.AggressiveActs+1) / float64(p.Stats.Calls+1)
		}
		avgAgg = sumAgg / float64(len(statOpponents))
		foldRate = avgFoldRate(statOpponents)
		avgHands := 0.0
		for _, p := range statOpponents {
			avgHands += float64(p.Stats.Hands)
		}
		avgHands /= float64(len(statOpponents))
		weight := 0.0
		if avgHands >= MinHandsForWeight {
			weight = 1 - math.Exp(-(avgHands-MinHandsForWeight)/WeightGrowth)
		}
		statsWeight := weight
		_ = statsWeight
		bluffChance = math.Min(0.3, foldRate) * weight
		bluffChance *= 1 - textureRisk*0.5
		bluffAggFactor := aggressiveness
		if bluffAggFactor < 0.8 {
			bluffAggFactor = 0.8
		}
		if bluffAggFactor > 1.2 {
			bluffAggFactor = 1.2
		}
		bluffChance = math.Min(0.3, bluffChance*bluffAggFactor)
		if avgVPIP < 0.25 {
			raiseThreshold -= 0.5 * weight
			aggressiveness += 0.1 * weight
		} else if avgVPIP > 0.5 {
			raiseThreshold += 0.5 * weight
			aggressiveness -= 0.1 * weight
		}
		if avgAgg > 1.5 {
			aggressiveness -= 0.1 * weight
		} else if avgAgg < 0.7 {
			aggressiveness += 0.1 * weight
		}
	}

	raiseThreshold = math.Max(1, raiseThreshold-(aggressiveness-1)*0.8)
	if !preflop {
		raiseAdj := 0.0
		if holeImprovesHand {
			if overPair {
				raiseAdj -= 0.35
			} else if topPair {
				raiseAdj -= 0.2
			}
		}
		if drawOuts >= 8 {
			switch len(communityCards) {
			case 3:
				raiseAdj -= 0.15
			case 4:
				raiseAdj -= 0.08
			}
		}
		if activeOpponents <= 1 {
			raiseAdj -= 0.15
		}
		if textureRisk > 0.6 {
			raiseAdj += 0.15
		}
		if spr < 3 {
			raiseAdj -= 0.1
		} else if spr > 6 {
			raiseAdj += 0.1
		}
		if raiseAdj > 0.5 {
			raiseAdj = 0.5
		}
		if raiseAdj < -0.5 {
			raiseAdj = -0.5
		}
		raiseThreshold += raiseAdj
		if raiseThreshold < 1.4 {
			raiseThreshold = 1.4
		}
	}

	raiseLevel := 0
	if facingRaise && raisesThisRound > 0 {
		raiseLevel = raisesThisRound
	}
	raiseThreshold += float64(raiseLevel) * ReraiseRatioStep * 10
	betAggFactor := aggressiveness
	if betAggFactor < 0.9 {
		betAggFactor = 0.9
	}
	if betAggFactor > 1.1 {
		betAggFactor = 1.1
	}
	shoveAggAdj := aggressiveness - 1
	shoveAggAdj *= 0.12
	if shoveAggAdj > 0.08 {
		shoveAggAdj = 0.08
	}
	if shoveAggAdj < -0.08 {
		shoveAggAdj = -0.08
	}

	lineAbort := false
	if !preflop && botLine.PreflopAggressor {
		lineAbort = textureRisk > 0.7 && strengthRatio < 0.45 && drawEquity == 0
	}

	capGreenNonPremium := func(amount int) int {
		if !isGreenZone || premiumHand {
			return amount
		}
		capRatio := GreenMaxStackBet
		if spr < 3 {
			capRatio = 0.3
		} else if spr > 6 {
			capRatio = 0.2
		}
		cap := int(float64(player.Chips) * capRatio)
		if amount > cap {
			return cap
		}
		return amount
	}

	valueBetSize := func() int {
		baseVal := 0.55
		if preflop {
			if strengthRatio >= 0.9 {
				baseVal += 0.15
			}
			baseVal += float64(activeOpponents) * 0.04
			baseVal += (1 - positionFactor) * 0.05
			if positionFactor < 0.3 && strengthRatio >= 0.8 {
				baseVal += 0.1
			}
		} else {
			if textureRisk > 0.6 {
				baseVal = 0.7
			} else if textureRisk > 0.3 {
				baseVal = 0.6
			} else {
				baseVal = 0.45
			}
			if strengthRatio > 0.95 {
				baseVal += 0.1
			}
			baseVal += float64(activeOpponents) * 0.03
			baseVal += (1 - positionFactor) * 0.05
		}
		if spr < 2 {
			baseVal += 0.1
		} else if spr < 4 {
			baseVal += 0.05
		} else if spr > 6 {
			baseVal -= 0.05
		}
		randV := randUniform(-0.1, 0.1)
		factor := baseVal + randV
		if factor < 0.35 {
			factor = 0.35
		}
		if factor > 1 {
			factor = 1
		}
		sized := roundTo10(math.Min(float64(player.Chips), float64(pot+needToCall)*factor*betAggFactor))
		return capGreenNonPremium(int(sized))
	}

	bluffBetSize := func() int {
		baseVal := 0.25 + textureRisk*0.05
		baseVal += float64(activeOpponents) * 0.02
		baseVal += (1 - positionFactor) * 0.03
		if spr < 3 {
			baseVal += 0.05
		} else if spr > 5 {
			baseVal -= 0.05
		}
		randV := randUniform(-0.04, 0.04)
		factor := baseVal + randV
		if factor < 0.2 {
			factor = 0.2
		}
		if factor > 0.45 {
			factor = 0.45
		}
		sized := roundTo10(math.Min(float64(player.Chips), float64(pot+needToCall)*factor*betAggFactor))
		return capGreenNonPremium(int(sized))
	}

	protectionBetSize := func() int {
		baseVal := 0.45 + textureRisk*0.25
		baseVal += float64(activeOpponents) * 0.03
		baseVal += (1 - positionFactor) * 0.04
		if spr < 3 {
			baseVal += 0.1
		} else if spr > 5 {
			baseVal -= 0.05
		}
		randV := randUniform(-0.05, 0.05)
		factor := baseVal + randV
		if factor < 0.35 {
			factor = 0.35
		}
		if factor > 0.8 {
			factor = 0.8
		}
		sized := roundTo10(math.Min(float64(player.Chips), float64(pot+needToCall)*factor*betAggFactor))
		return capGreenNonPremium(int(sized))
	}

	yellowRaiseSize := func() int {
		baseVal := float64(bigBlind) * (2.5 + randUniform(0, 0.5))
		sized := roundTo10(baseVal * betAggFactor)
		amt := int(sized)
		if amt < minRaiseAmount {
			amt = minRaiseAmount
		}
		if amt > player.Chips {
			amt = player.Chips
		}
		return amt
	}

	decision := (*BotDecision)(nil)
	if useHarrington {
		decision = decideHarringtonAction(HarringtonParams{
			MZone:                mZone,
			FacingRaise:          facingRaise,
			NeedsToCall:          needsToCall,
			StrengthRatio:        strengthRatio,
			DeadPushThreshold:    deadPushThreshold,
			RedPushThreshold:     redPushThreshold,
			OrangePushThreshold:  orangePushThreshold,
			YellowRaiseThreshold: yellowRaiseThreshold,
			YellowShoveThreshold: yellowShoveThreshold,
			RedCallThreshold:     riskAdjRedCall,
			OrangeCallThreshold:  riskAdjOrangeCall,
			YellowCallThreshold:  riskAdjYellowCall,
			CanShove:             canShove,
			CanRaise:             canRaise,
			NeedToCall:           needToCall,
			PlayerChips:          player.Chips,
			YellowRaiseSize:      yellowRaiseSize,
		})
	}

	if decision == nil {
		shallowShove := 0.65 - shoveAggAdj
		if shallowShove < 0 {
			shallowShove = 0
		}
		if shallowShove > 1 {
			shallowShove = 1
		}
		shortstackShove := 0.75 - shoveAggAdj
		if shortstackShove < 0 {
			shortstackShove = 0
		}
		if shortstackShove > 1 {
			shortstackShove = 1
		}
		if spr <= 1.2 && strengthRatio >= shallowShove {
			decision = &BotDecision{Action: "raise", Amount: player.Chips}
		} else if preflop && player.Chips <= bigBlind*10 && strengthRatio >= shortstackShove {
			decision = &BotDecision{Action: "raise", Amount: player.Chips}
		}
	}

	if decision == nil {
		if needToCall <= 0 {
			if canRaise && decisionStrength >= raiseThreshold {
				raiseAmt := valueBetSize()
				if raiseAmt < minRaiseAmount {
					raiseAmt = minRaiseAmount
				}
				if math.Abs(decisionStrength-raiseThreshold) <= StrengthTieDelta {
					if rand.Float64() < 0.5 {
						decision = &BotDecision{Action: "check"}
					} else {
						decision = &BotDecision{Action: "raise", Amount: raiseAmt}
					}
				} else {
					decision = &BotDecision{Action: "raise", Amount: raiseAmt}
				}
			} else {
				decision = &BotDecision{Action: "check"}
			}
		} else if canRaise && decisionStrength >= raiseThreshold && stackRatio <= 1.0/3.0 {
			raiseAmt := protectionBetSize()
			if raiseAmt < minRaiseAmount {
				raiseAmt = minRaiseAmount
			}
			if math.Abs(decisionStrength-raiseThreshold) <= StrengthTieDelta {
				if strengthRatio >= eliminationBarrier && stackRatio <= 0.7 {
					if preflop && stackRatio > 0.5 {
						// keep the fold path below
					}
					if rand.Float64() < 0.5 {
						decision = &BotDecision{Action: "raise", Amount: raiseAmt}
					} else if strengthRatio >= eliminationBarrier && stackRatio <= boolToFloat(preflop, 0.5, 0.7) {
						decision = &BotDecision{Action: "call", Amount: minInt(player.Chips, needToCall)}
					} else {
						decision = &BotDecision{Action: "fold"}
					}
				} else if rand.Float64() < 0.5 {
					decision = &BotDecision{Action: "raise", Amount: raiseAmt}
				} else {
					decision = &BotDecision{Action: "fold"}
				}
			} else {
				decision = &BotDecision{Action: "raise", Amount: raiseAmt}
			}
		} else if strengthRatio >= eliminationBarrier && stackRatio <= boolToFloat(preflop, 0.5, 0.7) {
			callAmt := minInt(player.Chips, needToCall)
			if math.Abs(strengthRatio-eliminationBarrier) <= OddsTieDelta {
				if rand.Float64() < 0.5 {
					decision = &BotDecision{Action: "call", Amount: callAmt}
				} else {
					decision = &BotDecision{Action: "fold"}
				}
			} else {
				decision = &BotDecision{Action: "call", Amount: callAmt}
			}
		} else {
			decision = &BotDecision{Action: "fold"}
		}
	}

	isBluff := false
	isStab := false
	if !useHarrington {
		facingAllIn := false
		for _, p := range opponents {
			if p.AllIn {
				facingAllIn = true
			}
		}
		if decision.Action == "fold" && facingAllIn {
			goodThreshold := AllInHandPreflop
			if !preflop {
				goodThreshold = AllInHandPostflop
			}
			riskAdjThreshold := math.Min(1, goodThreshold+eliminationPenalty)
			if strengthRatio >= riskAdjThreshold {
				decision = &BotDecision{Action: "call", Amount: minInt(player.Chips, needToCall)}
			}
		}

		if bluffChance > 0 && canRaise && !facingRaise && (!preflop || strengthRatio >= MinPreflopBluffRatio) && (decision.Action == "check" || decision.Action == "fold") && !facingAllIn && !nonValueAggressionMade {
			if rand.Float64() < bluffChance {
				bluffAmt := bluffBetSize()
				if bluffAmt < minRaiseAmount {
					bluffAmt = minRaiseAmount
				}
				decision = &BotDecision{Action: "raise", Amount: bluffAmt}
				isBluff = true
			}
		}

		if !preflop && currentBet == 0 && decision.Action == "check" && canRaise && !facingRaise && botLine.PreflopAggressor && !lineAbort && strengthRatio < 0.9 {
			if currentPhaseIdx == 1 && boolPtr(botLine.CBetIntent) {
				wantsBluff := strengthRatio < 0.6 && drawEquity == 0
				if !wantsBluff || !nonValueAggressionMade {
					bet := protectionBetSize()
					if strengthRatio < 0.6 && drawEquity == 0 {
						bet = bluffBetSize()
					}
					amt := maxInt(lastRaise, bet)
					if amt > player.Chips {
						amt = player.Chips
					}
					decision = &BotDecision{Action: "raise", Amount: amt}
					if wantsBluff {
						isBluff = true
					}
				}
			} else if currentPhaseIdx == 2 && boolPtr(botLine.BarrelIntent) {
				wantsBluff := strengthRatio < 0.6 && drawEquity == 0
				if !wantsBluff || !nonValueAggressionMade {
					bet := protectionBetSize()
					if strengthRatio < 0.65 && drawEquity == 0 {
						bet = bluffBetSize()
					}
					amt := maxInt(lastRaise, bet)
					if amt > player.Chips {
						amt = player.Chips
					}
					decision = &BotDecision{Action: "raise", Amount: amt}
					if wantsBluff {
						isBluff = true
					}
				}
			}
		}

		if !preflop && decision.Action == "raise" && strengthRatio >= 0.95 && spr <= 2 && rand.Float64() < 0.3 {
			protection := protectionBetSize() * 2
			if decision.Amount < protection {
				decision.Amount = int(protection)
			}
		}

		if !preflop && !needsToCall && strengthRatio >= 0.9 && decision.Action == "raise" && rand.Float64() < 0.3 {
			decision = &BotDecision{Action: "check"}
		}

		if !preflop && currentBet == 0 && decision.Action == "check" && canRaise && !facingRaise && textureRisk < 0.4 && (foldRate > 0.25 || drawEquity > 0) {
			stabChance := math.Max(0.05, math.Min(0.35, 0.05+positionFactor*0.3))
			if rand.Float64() < stabChance && !nonValueAggressionMade {
				betAmt := protectionBetSize()
				amt := maxInt(lastRaise, betAmt)
				decision = &BotDecision{Action: "raise", Amount: amt}
				isStab = true
			}
		}
	}

	reraiseValueRatio := ReraiseValueRatio
	if topPair || overPair {
		reraiseValueRatio = ReraiseTopPairRatio
	}
	if decision.Action == "raise" && raiseLevel > 0 && strengthRatio < reraiseValueRatio {
		if needToCall > 0 {
			decision = &BotDecision{Action: "call", Amount: minInt(player.Chips, needToCall)}
		} else {
			decision = &BotDecision{Action: "check"}
		}
		isBluff = false
		isStab = false
	}

	if decision.Action == "raise" {
		minRaise := needToCall + lastRaise
		if decision.Amount < minRaise && decision.Amount < player.Chips {
			if needToCall > 0 {
				decision = &BotDecision{Action: "call", Amount: minInt(player.Chips, needToCall)}
			} else {
				decision = &BotDecision{Action: "check"}
			}
		}
	}

	_ = isBluff
	_ = isStab
	return decision
}

func boolPtr(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func boolToFloat(b bool, a, c float64) float64 {
	if b {
		return a
	}
	return c
}

// randUniform is a thin wrapper around the standard math/rand API to make
// call sites read like the Python `random.uniform` invocations.
func randUniform(lo, hi float64) float64 {
	return lo + rand.Float64()*(hi-lo)
}