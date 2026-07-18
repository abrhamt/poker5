package poker

import (
	"testing"
)

// TestCard covers the basic Card struct + String() / Equal() behavior
// mirrored from poker/test_game.py::section("CARD TESTS").
func TestCard(t *testing.T) {
	c1 := NewCard("As")
	c2 := NewCard("Kh")
	c3 := NewCard("Td")
	c4 := NewCard("2c")
	c5 := NewCard("5s")

	check := func(cond bool, label string, t *testing.T) {
		if !cond {
			t.Errorf("FAIL: %s", label)
		}
	}

	check(c1.Rank == 13, "Ace rank == 13", t)
	check(c1.Suit == 's', "Ace suit == 's'", t)
	check(c2.Rank == 12, "King rank == 12", t)
	check(c3.Rank == 9, "Ten rank == 9", t)
	check(c4.Rank == 1, "Two rank == 1", t)
	check(c5.Rank == 4, "Five rank == 4", t)
	check(c1.String() == "As", "Card string == 'As'", t)
	check(c3.String() == "10d", "Ten displayed as '10d'", t)
	check(!c1.Equal(c2), "Ace != King", t)
	check(NewCard("As").Equal(NewCard("As")), "Card equality", t)
}

func TestHandTypes(t *testing.T) {
	check := func(cond bool, label string, t *testing.T) {
		if !cond {
			t.Errorf("FAIL: %s", label)
		}
	}

	// Royal Flush
	h := Solve([]string{"As", "Ks", "Qs", "Js", "Ts"}, StandardRules())
	check(h.Rank == 9, "Royal Flush rank == 9", t)
	check(h.Name == HandStraightFlush || h.Name == HandRoyalFlush, "Royal Flush name (got "+h.Name+")", t)
	containsRoyal := false
	for _, r := range h.Descr {
		if r == 'R' || r == 'r' {
			containsRoyal = true
		}
	}
	if h.Name == HandRoyalFlush {
		containsRoyal = true
	}
	check(containsRoyal, "Royal Flush description mentions Royal", t)

	// Straight Flush
	h = Solve([]string{"9s", "8s", "7s", "6s", "5s"}, StandardRules())
	check(h.Rank == 9, "Straight Flush rank == 9", t)
	check(h.Name == HandStraightFlush, "Straight Flush name (got "+h.Name+")", t)

	// Four of a Kind
	h = Solve([]string{"Ah", "Ac", "As", "Ad", "Kd"}, StandardRules())
	check(h.Rank == 8, "Four of a Kind rank == 8", t)
	check(h.Name == HandFourOfAKind, "Four of a Kind name", t)

	// Full House
	h = Solve([]string{"3c", "3s", "3d", "7h", "7c"}, StandardRules())
	check(h.Rank == 7, "Full House rank == 7", t)
	check(h.Name == HandFullHouse, "Full House name", t)

	// Flush
	h = Solve([]string{"Ah", "Kh", "Qh", "9h", "3h"}, StandardRules())
	check(h.Rank == 6, "Flush rank == 6", t)
	check(h.Name == HandFlush, "Flush name", t)

	// Straight
	h = Solve([]string{"9c", "8d", "7h", "6s", "5c"}, StandardRules())
	check(h.Rank == 5, "Straight rank == 5", t)
	check(h.Name == HandStraight, "Straight name", t)

	// Wheel
	h = Solve([]string{"Ac", "2d", "3h", "4s", "5c"}, StandardRules())
	check(h.Rank == 5, "Wheel rank == 5", t)
	check(h.Name == HandStraight, "Wheel is Straight", t)

	// Three of a Kind
	h = Solve([]string{"2h", "2d", "2c", "9s", "Kd"}, StandardRules())
	check(h.Rank == 4, "Three of a Kind rank == 4", t)
	check(h.Name == HandThreeOfAKind, "Three of a Kind name", t)

	// Two Pair
	h = Solve([]string{"Ah", "Ad", "Kc", "Ks", "3h"}, StandardRules())
	check(h.Rank == 3, "Two Pair rank == 3", t)
	check(h.Name == HandTwoPair, "Two Pair name", t)

	// One Pair
	h = Solve([]string{"4h", "4d", "Kc", "Qs", "Jh"}, StandardRules())
	check(h.Rank == 2, "One Pair rank == 2", t)
	check(h.Name == HandOnePair, "One Pair name", t)

	// High Card
	h = Solve([]string{"Ah", "Kd", "Qc", "Js", "8h"}, StandardRules())
	check(h.Rank == 1, "High Card rank == 1", t)
	check(h.Name == HandHighCard, "High Card name", t)
}

func TestHandComparison(t *testing.T) {
	check := func(cond bool, label string, t *testing.T) {
		if !cond {
			t.Errorf("FAIL: %s", label)
		}
	}

	sf := Solve([]string{"9s", "8s", "7s", "6s", "5s"}, StandardRules())
	fk := Solve([]string{"Ah", "Ac", "As", "Ad", "Kd"}, StandardRules())
	fh := Solve([]string{"3c", "3s", "3d", "7h", "7c"}, StandardRules())
	fl := Solve([]string{"Ah", "Kh", "Qh", "9h", "3h"}, StandardRules())
	st := Solve([]string{"9c", "8d", "7h", "6s", "5c"}, StandardRules())
	tk := Solve([]string{"2h", "2d", "2c", "9s", "Kd"}, StandardRules())
	tp := Solve([]string{"Ah", "Ad", "Kc", "Ks", "3h"}, StandardRules())
	op := Solve([]string{"4h", "4d", "Kc", "Qs", "Jh"}, StandardRules())
	hc := Solve([]string{"Ah", "Kd", "Qc", "Js", "8h"}, StandardRules())

	check(sf.Compare(fk) == -1, "Straight Flush stronger than Four of a Kind", t)
	check(fk.Compare(fh) == -1, "Four of a Kind stronger than Full House", t)
	check(fh.Compare(fl) == -1, "Full House stronger than Flush", t)
	check(fl.Compare(st) == -1, "Flush stronger than Straight", t)
	check(st.Compare(tk) == -1, "Straight stronger than Three of a Kind", t)
	check(tk.Compare(tp) == -1, "Three of a Kind stronger than Two Pair", t)
	check(tp.Compare(op) == -1, "Two Pair stronger than One Pair", t)
	check(op.Compare(hc) == -1, "One Pair stronger than High Card", t)
	check(hc.Compare(sf) == 1, "High Card weaker than Straight Flush", t)

	check(fk.LoseTo(sf), "Four of a Kind loses to Straight Flush", t)
	check(!sf.LoseTo(fk), "Straight Flush does not lose to Four of a Kind", t)
}

func TestHandWinners(t *testing.T) {
	check := func(cond bool, label string, t *testing.T) {
		if !cond {
			t.Errorf("FAIL: %s", label)
		}
	}

	sf := Solve([]string{"9s", "8s", "7s", "6s", "5s"}, StandardRules())
	fk := Solve([]string{"Ah", "Ac", "As", "Ad", "Kd"}, StandardRules())
	fh := Solve([]string{"3c", "3s", "3d", "7h", "7c"}, StandardRules())

	winners := Winners([]*Hand{sf, fk, fh})
	check(len(winners) == 1, "Single winner from distinct hands", t)
	check(winners[0].Name == HandStraightFlush, "Straight Flush wins", t)

	// Tie
	h1 := Solve([]string{"Ah", "Ad", "Kc", "Ks", "3h"}, StandardRules())
	h2 := Solve([]string{"Ac", "As", "Kd", "Kh", "3d"}, StandardRules())
	winners = Winners([]*Hand{h1, h2})
	check(len(winners) == 2, "Tie produces 2 winners", t)
	check(winners[0].Name == HandTwoPair, "Tied hand is Two Pair", t)

	// Kicker resolution
	h1 = Solve([]string{"Ah", "Ad", "Kc", "Qs", "3h"}, StandardRules())
	h2 = Solve([]string{"Ac", "As", "Kd", "Jh", "3d"}, StandardRules())
	winners = Winners([]*Hand{h1, h2})
	check(len(winners) == 1, "Kicker breaks tie", t)
	check(winners[0].Cards[2].Rank == 12, "Winner has King kicker", t)

	// 7-card hand selection (best 5)
	h := Solve([]string{"Ah", "Ad", "Ac", "Ks", "Kd", "Qh", "Jh"}, StandardRules())
	check(h.Rank == 7, "Full House from 7 cards", t)
	check(h.Name == HandFullHouse, "Full House from 7 cards", t)
}