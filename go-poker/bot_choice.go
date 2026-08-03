package poker

import (
	"math"
	"math/rand"
)

func ChooseBotAction(player *Player, ctx BotContext) *BotDecision {
	currentBet := ctx.CurrentBet
	pot := ctx.Pot
	smallBlind := ctx.SmallBlind
	bigBlind := ctx.BigBlind
	raisesThisRound := ctx.RaisesThisRound
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
	mRatio := float64(player.Chips) / float64(maxInt(1, smallBlind+bigBlind))

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

	preflop := len(communityCards) == 0
	result := evaluateHandStrength(player, communityCards, preflop)
	strength := result.Strength
	solvedHand := result.SolvedHand

	strengthBase := strength / 10
	strengthRatio := strengthBase
	mZone := MZone(mRatio)

	raiseSize := func(ratio float64) int {
		target := float64(pot+needToCall) * ratio
		val := math.Max(float64(minRaiseAmount), target)
		val = math.Min(float64(player.Chips), val)
		return int(roundTo10(val))
	}

	if preflop {
		if mZone == MZoneDead {
			if strengthRatio >= DeadPushRatio {
				return &BotDecision{Action: "raise", Amount: player.Chips}
			}
			if needsToCall {
				return &BotDecision{Action: "fold"}
			}
			return &BotDecision{Action: "check"}
		}
		if mZone == MZoneRed {
			if canShove && strengthRatio >= RedPushRatio {
				return &BotDecision{Action: "raise", Amount: player.Chips}
			}
			if needsToCall && strengthRatio >= RedCallRatio {
				return &BotDecision{Action: "call", Amount: minInt(player.Chips, needToCall)}
			}
			if needsToCall {
				return &BotDecision{Action: "fold"}
			}
			return &BotDecision{Action: "check"}
		}
		if mZone == MZoneOrange {
			if canShove && strengthRatio >= OrangePushRatio {
				return &BotDecision{Action: "raise", Amount: player.Chips}
			}
			if needsToCall && strengthRatio >= OrangeCallRatio {
				return &BotDecision{Action: "call", Amount: minInt(player.Chips, needToCall)}
			}
			if needsToCall {
				return &BotDecision{Action: "fold"}
			}
			return &BotDecision{Action: "check"}
		}
	}

	if strengthRatio >= 0.75 {
		if canRaise {
			amt := raiseSize(0.75)
			return &BotDecision{Action: "raise", Amount: amt}
		}
		if needsToCall {
			return &BotDecision{Action: "call", Amount: minInt(player.Chips, needToCall)}
		}
		return &BotDecision{Action: "check"}
	}

	if strengthRatio >= 0.45 || (solvedHand != nil && solvedHand.Rank >= 2) {
		if needsToCall {
			if potOdds <= 0.4 || stackRatio <= 0.25 {
				return &BotDecision{Action: "call", Amount: minInt(player.Chips, needToCall)}
			}
			return &BotDecision{Action: "fold"}
		}
		if canRaise && rand.Float64() < 0.3 {
			amt := raiseSize(0.5)
			return &BotDecision{Action: "raise", Amount: amt}
		}
		return &BotDecision{Action: "check"}
	}

	if needsToCall {
		if potOdds < 0.15 && needToCall <= player.Chips/10 {
			return &BotDecision{Action: "call", Amount: minInt(player.Chips, needToCall)}
		}
		return &BotDecision{Action: "fold"}
	}
	return &BotDecision{Action: "check"}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
