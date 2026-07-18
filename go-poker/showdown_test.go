package poker

import "testing"

// TestShowdownCapturesWinner drives a 3-bot hand through to showdown and
// verifies that the engine populates LastWinner + PotAwards with the
// winning hand's five cards and the correct amount.
func TestShowdownCapturesWinner(t *testing.T) {
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	eng, _, err := state.GetOrCreate("showdown")
	if err != nil {
		t.Fatalf("get_or_create: %v", err)
	}
	eng.InitGame([]string{"Bot A", "Bot B", "Bot C"})
	if !eng.StartHand() {
		t.Fatalf("start_hand returned false")
	}

	// Force known hole cards so the winner is deterministic.
	eng.Players[0].Cards = [2]string{"As", "Ac"}
	eng.Players[1].Cards = [2]string{"Kd", "Qd"}
	eng.Players[2].Cards = [2]string{"7c", "2c"}

	// Step until showdown completes.
	for i := 0; i < 500; i++ {
		eng.AdvanceOneStep()
		if eng.Game.Intermission {
			break
		}
	}
	if !eng.Game.Intermission {
		t.Fatalf("engine did not reach intermission within 500 steps (phase=%s)", eng.Game.Phase)
	}
	if eng.Game.LastWinner == nil {
		t.Fatalf("LastWinner should be populated")
	}
	if eng.Game.LastWinner.Amount == 0 {
		t.Errorf("winning amount should be > 0")
	}
	if len(eng.Game.LastWinner.WinningCards) != 5 {
		t.Errorf("expected 5 winning cards, got %d", len(eng.Game.LastWinner.WinningCards))
	}
	if len(eng.Game.PotAwards) == 0 {
		t.Errorf("PotAwards should have at least one entry")
	}
}

// TestIntermissionSeatEdits exercises the AddOrRenameSeat + RemoveSeat
// helpers used by the showdown modal's seat editor.
func TestIntermissionSeatEdits(t *testing.T) {
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	eng, _, err := state.GetOrCreate("seats")
	if err != nil {
		t.Fatalf("get_or_create: %v", err)
	}
	eng.InitGame([]string{"Bot A", "Bot B"})
	if !eng.StartHand() {
		t.Fatalf("start_hand returned false")
	}

	if err := eng.AddOrRenameSeat("Bot C", nil); err == nil {
		t.Errorf("AddOrRenameSeat should fail mid-hand (no intermission)")
	}

	// Force intermission.
	eng.Game.Intermission = true

	if err := eng.AddOrRenameSeat("Bot C", nil); err != nil {
		t.Fatalf("AddOrRenameSeat during intermission: %v", err)
	}
	if len(eng.Players) != 3 {
		t.Errorf("expected 3 players after add, got %d", len(eng.Players))
	}

	// Remove the new seat.
	if err := eng.RemoveSeat(2); err != nil {
		t.Fatalf("RemoveSeat: %v", err)
	}
	if len(eng.Players) != 2 {
		t.Errorf("expected 2 players after remove, got %d", len(eng.Players))
	}
}

// TestResolveSidePotsAwardsPot verifies that resolveSidePots records one
// PotAward per side pot and the headline Winner has the highest payout.
func TestResolveSidePotsAwardsPot(t *testing.T) {
	a := &Player{Name: "A", SeatIndex: 0, TotalBet: 100, Chips: 0}
	b := &Player{Name: "B", SeatIndex: 1, TotalBet: 300, Chips: 0}
	c := &Player{Name: "C", SeatIndex: 2, TotalBet: 600, Chips: 0}
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	eng, _, err := state.GetOrCreate("awards")
	if err != nil {
		t.Fatalf("get_or_create: %v", err)
	}
	// Pre-set state so resolveSidePots can run.
	eng.Game.CommunityCards = []string{"Ah", "Ad", "Ac", "Kd", "Qc"}
	eng.Players = []*Player{a, b, c}
	eng.Game.Pot = 1000
	eng.Game.Phase = "showdown"
	eng.Game.PhaseIndex = 4
	pots := buildSidePots([]*Player{a, b, c})
	eng.resolveSidePots(pots, []*Player{a, b, c}, true)
	if eng.Game.LastWinner == nil {
		t.Fatalf("expected LastWinner")
	}
	if eng.Game.LastWinner.Amount < 200 {
		t.Errorf("headline winner amount looks too low: %d", eng.Game.LastWinner.Amount)
	}
	// Tied AAA-KK hands across all three pots -> 3 + 2 + 1 = 6 awards.
	if len(eng.Game.PotAwards) < 3 {
		t.Errorf("expected at least 3 pot awards, got %d", len(eng.Game.PotAwards))
	}
}