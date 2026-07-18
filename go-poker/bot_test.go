package poker

import (
	"math/rand"
	"testing"
)

// TestBotDecisions mirrors the BOT DECISION TESTS section of
// poker/test_game.py. The Python tests use a global random module so the
// assertions check membership in a known action set; we do the same here.
func TestBotDecisions(t *testing.T) {
	rand.Seed(1)

	makePlayer := func(cards [2]string, chips int, opts ...func(*Player)) *Player {
		p := &Player{
			Name:      "Bot",
			Chips:     chips,
			Cards:     cards,
			Stats:     Stats{Hands: 10, Folds: 2, VPIP: 4, AggressiveActs: 2, Calls: 2},
			BotLine:   NewBotLine(),
		}
		for _, o := range opts {
			o(p)
		}
		return p
	}

	makeCtx := func(pot, currentBet, bigBlind, smallBlind, raises int, phaseIdx int, lastRaise int, community []string, players []*Player) BotContext {
		return BotContext{
			Pot:              pot,
			CurrentBet:       currentBet,
			BigBlind:         bigBlind,
			SmallBlind:       smallBlind,
			RaisesThisRound:  raises,
			CurrentPhaseIdx:  phaseIdx,
			LastRaise:        lastRaise,
			CommunityCards:   community,
			Players:          players,
		}
	}

	// Test 1: Strong preflop hand (AA) should raise/call.
	{
		p1 := makePlayer([2]string{"As", "Ac"}, 2000)
		p2 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other" })
		p3 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other2" })
		ctx := makeCtx(30, 20, 20, 10, 0, 0, 20, nil, []*Player{p1, p2, p3})
		p1.RoundBet = 10
		decision := ChooseBotAction(p1, ctx)
		if decision.Action != "raise" && decision.Action != "call" {
			t.Errorf("AA preflop: action=%s (expected raise or call)", decision.Action)
		}
		t.Logf("    AA preflop decision: %+v", decision)
	}

	// Test 2: Weak preflop hand (72o) should fold or check.
	{
		p1 := makePlayer([2]string{"7c", "2d"}, 2000)
		p2 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other" })
		p3 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other2" })
		ctx := makeCtx(0, 0, 20, 10, 0, 0, 20, nil, []*Player{p1, p2, p3})
		decision := ChooseBotAction(p1, ctx)
		if decision.Action != "fold" && decision.Action != "check" {
			t.Errorf("72o preflop: action=%s (expected fold or check)", decision.Action)
		}
		t.Logf("    72o preflop decision: %+v", decision)
	}

	// Test 3: Postflop with strong hand (TPTK).
	{
		p1 := makePlayer([2]string{"Ah", "Kd"}, 1900, func(p *Player) {
			p.IsBigBlind = true
		})
		p2 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other" })
		p3 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other2" })
		ctx := makeCtx(60, 0, 20, 10, 0, 1, 20, []string{"Ac", "7h", "2s"}, []*Player{p1, p2, p3})
		decision := ChooseBotAction(p1, ctx)
		if decision.Action != "raise" && decision.Action != "call" && decision.Action != "check" {
			t.Errorf("TPTK postflop: action=%s", decision.Action)
		}
		t.Logf("    TPTK postflop decision: %+v", decision)
	}

	// Test 4: Short stack shove.
	{
		p1 := makePlayer([2]string{"As", "Kd"}, 400)
		p2 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other" })
		p3 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other2" })
		ctx := makeCtx(120, 0, 100, 50, 0, 0, 100, nil, []*Player{p1, p2, p3})
		decision := ChooseBotAction(p1, ctx)
		if decision.Action != "raise" || decision.Amount < 400 {
			t.Errorf("Short stack AK: action=%+v", decision)
		}
		t.Logf("    Short stack AK decision: %+v", decision)
	}

	// Test 5: Pot odds call.
	{
		p1 := makePlayer([2]string{"Kh", "Qh"}, 100)
		p2 := makePlayer([2]string{"", ""}, 500, func(p *Player) { p.Name = "Other" })
		p3 := makePlayer([2]string{"", ""}, 500, func(p *Player) { p.Name = "Other2" })
		ctx := makeCtx(1000, 100, 20, 10, 0, 1, 20, []string{"Js", "Tc", "3h"}, []*Player{p1, p2, p3})
		decision := ChooseBotAction(p1, ctx)
		if decision.Action != "call" && decision.Action != "raise" && decision.Action != "fold" {
			t.Errorf("Pot odds call: action=%s", decision.Action)
		}
		t.Logf("    Pot odds call decision: %+v", decision)
	}

	// Test 6: Bad hand vs big bet.
	{
		p1 := makePlayer([2]string{"7c", "2d"}, 500)
		p2 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other" })
		p3 := makePlayer([2]string{"", ""}, 0, func(p *Player) { p.Name = "Other2" })
		ctx := makeCtx(30, 50, 20, 10, 0, 1, 30, []string{"Ac", "Kh", "Qd"}, []*Player{p1, p2, p3})
		decision := ChooseBotAction(p1, ctx)
		if decision.Action != "raise" && decision.Action != "fold" {
			t.Errorf("Bad hand vs big bet: action=%s", decision.Action)
		}
		t.Logf("    Bad hand vs big bet: %+v", decision)
	}

	// Test 7: Bluff potential.
	{
		p1 := makePlayer([2]string{"3c", "4c"}, 1900)
		p2 := makePlayer([2]string{"", ""}, 2000, func(p *Player) {
			p.Name = "Tight"
			p.TotalBet = 10
			p.Stats = Stats{Hands: 10, Folds: 7, VPIP: 1, AggressiveActs: 0, Calls: 1}
		})
		p3 := makePlayer([2]string{"", ""}, 2000, func(p *Player) { p.Name = "Other2" })
		ctx := makeCtx(30, 20, 20, 10, 0, 0, 20, nil, []*Player{p1, p2, p3})
		decision := ChooseBotAction(p1, ctx)
		if decision.Action != "raise" && decision.Action != "fold" && decision.Action != "call" && decision.Action != "check" {
			t.Errorf("Bluff scenario gave unexpected action %s", decision.Action)
		}
		t.Logf("    Bluff scenario (suited connector vs tight): %+v", decision)
	}

	// Test 8: Continuation bet.
	{
		p1 := makePlayer([2]string{"Ah", "Kd"}, 1900, func(p *Player) {
			p.IsDealer = true
			p.RoundBet = 20
			p.BotLine.PreflopAggressor = true
			t := true
			p.BotLine.CBetIntent = &t
		})
		p2 := makePlayer([2]string{"", ""}, 1900, func(p *Player) { p.Name = "Other" })
		p3 := makePlayer([2]string{"", ""}, 1800, func(p *Player) { p.Name = "Other2" })
		ctx := makeCtx(60, 0, 20, 10, 0, 1, 20, []string{"Jh", "7d", "2c"}, []*Player{p1, p2, p3})
		decision := ChooseBotAction(p1, ctx)
		if decision.Action != "raise" && decision.Action != "check" {
			t.Errorf("C-bet scenario: action=%s", decision.Action)
		}
		t.Logf("    Continuation bet scenario: %+v", decision)
	}
}