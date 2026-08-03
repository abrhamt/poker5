package poker

type MZoneType string

const (
	MZoneDead   MZoneType = "dead"
	MZoneRed    MZoneType = "red"
	MZoneOrange MZoneType = "orange"
	MZoneYellow MZoneType = "yellow"
	MZoneGreen  MZoneType = "green"
)

func MZone(mRatio float64) MZoneType {
	switch {
	case mRatio <= MRatioDeadMax:
		return MZoneDead
	case mRatio <= MRatioRedMax:
		return MZoneRed
	case mRatio <= MRatioOrangeMax:
		return MZoneOrange
	case mRatio <= MRatioYellowMax:
		return MZoneYellow
	default:
		return MZoneGreen
	}
}

type CommitmentMetrics struct {
	InvestedRatio     float64
	CommitmentPenalty float64
}

func computeCommitmentMetrics(needToCall int, player *Player, spr float64, remainingStreets int) CommitmentMetrics {
	totalPotContribution := player.TotalBet + needToCall
	initialChips := player.Chips + player.TotalBet
	investedRatio := 0.0
	if initialChips > 0 {
		investedRatio = float64(totalPotContribution) / float64(initialChips)
	}

	sprFactor := 0.0
	if spr <= CommitSPRMin {
		sprFactor = 1.0
	} else if spr < CommitSPRMax {
		sprFactor = 1.0 - (spr-CommitSPRMin)/(CommitSPRMax-CommitSPRMin)
	}

	investedFactor := 0.0
	if investedRatio >= CommitInvestEnd {
		investedFactor = 1.0
	} else if investedRatio > CommitInvestStart {
		investedFactor = (investedRatio - CommitInvestStart) / (CommitInvestEnd - CommitInvestStart)
	}

	streetFactor := float64(remainingStreets) / 3.0
	commitmentPenalty := sprFactor * investedFactor * streetFactor * CommitmentPenaltyMax

	return CommitmentMetrics{
		InvestedRatio:     investedRatio,
		CommitmentPenalty: commitmentPenalty,
	}
}

type EliminationRisk struct {
	RiskRatio          float64
	EliminationPenalty float64
}

func computeEliminationRisk(stackRatio float64) EliminationRisk {
	riskRatio := 0.0
	if stackRatio >= EliminationRiskFull {
		riskRatio = 1.0
	} else if stackRatio > EliminationRiskStart {
		riskRatio = (stackRatio - EliminationRiskStart) / (EliminationRiskFull - EliminationRiskStart)
	}
	eliminationPenalty := riskRatio * EliminationPenaltyMax

	return EliminationRisk{
		RiskRatio:          riskRatio,
		EliminationPenalty: eliminationPenalty,
	}
}

type PostflopContext struct {
	TopPair     bool
	OverPair    bool
	DrawChance  float64
	DrawOuts    int
	DrawEquity  float64
	TextureRisk float64
}

func computePostflopContext(player *Player, communityCards []string, preflop bool) PostflopContext {
	if preflop {
		return PostflopContext{}
	}
	hole := []string{player.Cards[0], player.Cards[1]}
	handCtx := analyzeHandContext(hole, communityCards)
	drawCtx := analyzeDrawPotential(hole, communityCards)
	texture := evaluateBoardTexture(communityCards)
	outs := drawCtx.Outs
	equity := float64(outs) * 0.04
	if len(communityCards) == 4 {
		equity = float64(outs) * 0.02
	}
	return PostflopContext{
		TopPair:     handCtx.IsTopPair,
		OverPair:    handCtx.IsOverPair,
		DrawChance:  float64(outs) * 0.04,
		DrawOuts:    outs,
		DrawEquity:  equity,
		TextureRisk: texture,
	}
}

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
			break
		}
	}
	relIdx := (seatIdx - refIdx + len(active)) % len(active)
	if len(active) <= 1 {
		return 0
	}
	return float64(relIdx) / float64(len(active)-1)
}

func findNextActivePlayer(players []*Player, startIdx int) *Player {
	for i := 1; i <= len(players); i++ {
		idx := (startIdx + i) % len(players)
		if !players[idx].Folded {
			return players[idx]
		}
	}
	return players[startIdx%len(players)]
}

type HandEvalResult struct {
	Strength   float64
	SolvedHand *Hand
}

func evaluateHandStrength(player *Player, communityCards []string, preflop bool) HandEvalResult {
	if preflop {
		s := preflopHandScore(player.Cards[0], player.Cards[1])
		return HandEvalResult{Strength: s, SolvedHand: nil}
	}
	all := append([]string{player.Cards[0], player.Cards[1]}, communityCards...)
	solved := Solve(all, StandardRules())
	score := SolvedHandScore(solved)
	return HandEvalResult{Strength: score, SolvedHand: solved}
}
