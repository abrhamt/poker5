package poker

import "testing"

func seatPlayers(names ...string) *GameEngine {
	e := &GameEngine{Game: NewGame("t"), Storage: NewMemoryStorage()}
	for i, name := range names {
		e.Players = append(e.Players, &Player{
			Name: name, SeatIndex: i, Chips: 1000,
			Cards: [2]string{"1B", "1B"}, Stats: NewStats(), BotLine: NewBotLine(),
		})
	}
	return e
}

func find(e *GameEngine, name string) *Player {
	for _, p := range e.Players {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// The whole point of sitting out: an absent player stops paying blinds. Without
// this, a dead phone bleeds a small and a big blind every orbit until broke.
func TestSittingOutPlayerPostsNoBlinds(t *testing.T) {
	e := seatPlayers("alice", "bob", "carol")
	away := find(e, "carol")
	away.SittingOut = true
	before := away.Chips

	for hand := 0; hand < 6; hand++ {
		if !e.StartHand() {
			t.Fatalf("hand %d did not start with two active players", hand)
		}
		e.Game.Intermission = false
	}

	if away.Chips != before {
		t.Fatalf("sitting-out player paid %d in blinds over six hands", before-away.Chips)
	}
	if away.IsSmallBlind || away.IsBigBlind || away.IsDealer {
		t.Fatalf("sitting-out player was given a blind or the button")
	}
	if !away.Folded {
		t.Fatalf("sitting-out player should be dealt out of the hand")
	}
	if away.Cards[0] != "1B" {
		t.Fatalf("sitting-out player was dealt hole cards: %v", away.Cards)
	}
}

// The players who are still there must keep paying them, and the heads-up rule
// applies once sitting out leaves only two in the hand.
func TestBlindsStillRotateAmongTheRemaining(t *testing.T) {
	e := seatPlayers("alice", "bob", "carol")
	find(e, "carol").SittingOut = true

	posted := map[string]int{}
	for hand := 0; hand < 4; hand++ {
		if !e.StartHand() {
			t.Fatalf("hand %d did not start", hand)
		}
		for _, p := range e.Players {
			if p.IsSmallBlind {
				posted[p.Name+":sb"]++
			}
			if p.IsBigBlind {
				posted[p.Name+":bb"]++
			}
		}
		e.Game.Intermission = false
	}

	// Heads up, the button is the small blind, so each of the two takes each
	// role twice across four hands.
	for _, key := range []string{"alice:sb", "alice:bb", "bob:sb", "bob:bb"} {
		if posted[key] != 2 {
			t.Fatalf("expected %s twice over four heads-up hands, got %d (%v)", key, posted[key], posted)
		}
	}
	if posted["carol:sb"]+posted["carol:bb"] != 0 {
		t.Fatalf("the sitting-out player took a blind")
	}
}

// With only one player left in, there is no hand — the table waits rather than
// declaring a winner or dealing to nobody.
func TestHandWillNotStartWithOneDealtIn(t *testing.T) {
	e := seatPlayers("alice", "bob")
	find(e, "bob").SittingOut = true

	if e.StartHand() {
		t.Fatalf("a hand started with only one player dealt in")
	}
	if e.Game.GameStarted {
		t.Fatalf("game marked started with nobody to play against")
	}
	if find(e, "alice").Chips != 1000 {
		t.Fatalf("the remaining player was charged a blind for a hand that never ran")
	}
}

// Coming back has to actually restore them to the next deal.
func TestSitInRestoresPlayerToTheNextHand(t *testing.T) {
	e := seatPlayers("alice", "bob", "carol")
	away := find(e, "carol")
	away.SittingOut = true

	e.StartHand()
	if !away.Folded {
		t.Fatalf("expected to be dealt out while sitting out")
	}

	away.SittingOut = false
	e.Game.Intermission = false
	e.StartHand()

	if away.Folded {
		t.Fatalf("player was still dealt out after sitting back in")
	}
	if away.Cards[0] == "1B" {
		t.Fatalf("player was not dealt cards after sitting back in")
	}
}
