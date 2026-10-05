package poker

import "testing"

func actor(e *GameEngine) *Player {
	return e.Players[e.CurrentPlayerIx%len(e.Players)]
}

func startedTable(t *testing.T, names ...string) *GameEngine {
	t.Helper()
	e := seatPlayers(names...)
	if !e.StartHand() {
		t.Fatal("hand did not start")
	}
	return e
}

func TestAllInRaisesWithTheWholeStack(t *testing.T) {
	e := startedTable(t, "alice", "bob", "carol")
	p := actor(e)
	stack := p.Chips
	before := p.RoundBet

	if !e.HumanAction(p.Name, "allin", 0) {
		t.Fatal("allin refused")
	}
	if p.Chips != 0 || !p.AllIn {
		t.Fatalf("not all in: chips %d, all_in %v", p.Chips, p.AllIn)
	}
	if e.Game.CurrentBet != before+stack {
		t.Fatalf("current bet = %d, want %d", e.Game.CurrentBet, before+stack)
	}
	for _, other := range e.Players {
		if other != p && !other.Folded && other.HasActed {
			t.Fatalf("%s should get to act again after the shove", other.Name)
		}
	}
}

// A stack smaller than the call goes in as a call for less: it must not lower
// or reset the bet everyone else faces.
func TestShortAllInIsACallForLess(t *testing.T) {
	e := startedTable(t, "alice", "bob", "carol")
	big := actor(e)
	if !e.HumanAction(big.Name, "raise", 400) {
		t.Fatal("raise refused")
	}
	bet := e.Game.CurrentBet

	short := actor(e)
	short.Chips = 50
	if !e.HumanAction(short.Name, "allin", 0) {
		t.Fatal("short allin refused")
	}
	if !short.AllIn || e.Game.CurrentBet != bet {
		t.Fatalf("short all-in changed the bet: all_in %v, bet %d -> %d", short.AllIn, bet, e.Game.CurrentBet)
	}
}

func TestCheckFacingABetIsRefused(t *testing.T) {
	e := startedTable(t, "alice", "bob", "carol")
	p := actor(e)
	if p.RoundBet >= e.Game.CurrentBet {
		t.Fatal("first preflop actor should be facing the big blind")
	}
	chips := p.Chips

	if e.HumanAction(p.Name, "check", 0) {
		t.Fatal("check against a bet was accepted")
	}
	if actor(e) != p || p.HasActed || p.Chips != chips {
		t.Fatal("refused check still used up the turn")
	}
	if !e.HumanAction(p.Name, "call", 0) {
		t.Fatal("call refused")
	}
}

func TestUnknownActionIsRefused(t *testing.T) {
	e := startedTable(t, "alice", "bob")
	p := actor(e)
	for _, action := range []string{"all-in", "shove", ""} {
		if e.HumanAction(p.Name, action, 0) {
			t.Fatalf("%q was accepted", action)
		}
	}
	if actor(e) != p || p.HasActed {
		t.Fatal("refused action still used up the turn")
	}
}
