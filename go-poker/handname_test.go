package poker

import "testing"

// TestSolveHandFor exercises the hand-name computation that powers the
// per-seat hand description shown under the hole cards.
func TestSolveHandFor(t *testing.T) {
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	eng, _, err := state.GetOrCreate("handnames")
	if err != nil {
		t.Fatalf("get_or_create: %v", err)
	}
	eng.InitGame([]string{"Alice", "Bot Bob", "Bot Carol"})
	if !eng.StartHand() {
		t.Fatalf("start_hand returned false")
	}

	// Force known hole cards for Alice.
	eng.Players[1].Cards = [2]string{"As", "Ks"}

	// Force known community cards (flop completes a Royal Flush draw).
	eng.Game.CommunityCards = []string{"Qs", "Js", "Ts"}

	got := eng.SolveHandFor(eng.Players[1])
	if got != "Royal Flush" {
		t.Errorf("expected 'Royal Flush', got %q", got)
	}

	// Folded player -> empty string.
	eng.Players[1].Folded = true
	if got := eng.SolveHandFor(eng.Players[1]); got != "" {
		t.Errorf("folded player should have empty hand name, got %q", got)
	}
	eng.Players[1].Folded = false

	// Preflop (no community cards) -> empty.
	eng.Game.CommunityCards = nil
	if got := eng.SolveHandFor(eng.Players[1]); got != "" {
		t.Errorf("preflop should produce empty hand name, got %q", got)
	}

	// Flop with two pair (player has KQ, board has KK + something else).
	eng.Players[1].Cards = [2]string{"Kd", "Qh"}
	eng.Game.CommunityCards = []string{"Ks", "5c", "2d"}
	got = eng.SolveHandFor(eng.Players[1])
	if got != "K's" {
		t.Errorf("expected pair of kings, got %q", got)
	}

	// Flop with two pair (board has AA, player holds KK).
	eng.Players[1].Cards = [2]string{"Kc", "Kh"}
	eng.Game.CommunityCards = []string{"Ad", "As", "5c"}
	got = eng.SolveHandFor(eng.Players[1])
	if got != "A's & K's" {
		t.Errorf("expected two-pair descr, got %q", got)
	}
}