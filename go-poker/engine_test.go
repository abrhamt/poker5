package poker

import (
	"testing"
)

// TestEngineStartHand verifies that a freshly-initialised engine can run a
// hand to completion without errors.
func TestEngineStartHand(t *testing.T) {
	rand := 1
	_ = rand
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	eng, _, err := state.GetOrCreate("t1")
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	eng.InitGame([]string{"Alice", "Bot Bob", "Bot Carol"})
	if !eng.StartHand() {
		t.Fatalf("StartHand returned false")
	}
	if len(eng.Players) != 3 {
		t.Fatalf("expected 3 players, got %d", len(eng.Players))
	}
	if eng.Game.Pot != 30 {
		t.Errorf("expected pot 30 (SB+BB), got %d", eng.Game.Pot)
	}
	// Cards should be assigned to all players.
	for _, p := range eng.Players {
		if p.Cards[0] == "1B" || p.Cards[1] == "1B" {
			t.Errorf("player %s did not receive hole cards: %v", p.Name, p.Cards)
		}
	}
}

// TestEngineAdvanceToShowdown plays a 4-player bot-only hand until either
// showdown or a single survivor remains.
func TestEngineAdvanceToShowdown(t *testing.T) {
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	eng, _, err := state.GetOrCreate("t2")
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	eng.InitGame([]string{"Bot Alice", "Bot Bob", "Bot Carol", "Bot Dave"})
	if !eng.StartHand() {
		t.Fatalf("StartHand returned false")
	}

	maxSteps := 500
	for i := 0; i < maxSteps; i++ {
		if !eng.AdvanceOneStep() {
			break
		}
		if !eng.Game.GameStarted {
			break
		}
	}
	if eng.Game.GameStarted {
		t.Errorf("hand did not finish within %d steps", maxSteps)
	}
}

// TestEngineHumanAction exercises the human action path end-to-end.
func TestEngineHumanAction(t *testing.T) {
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	eng, _, err := state.GetOrCreate("t3")
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	eng.InitGame([]string{"Player", "Bot A", "Bot B"})
	if !eng.StartHand() {
		t.Fatalf("StartHand returned false")
	}

	// The human player starts as the dealer (seat 0). Make sure they are not
	// the bot acting first: rotate so the human is the dealer.
	for eng.Game.PhaseIndex == 0 && !isCurrentHuman(eng) {
		eng.AdvanceOneStep()
	}
	if isCurrentHuman(eng) {
		// Trigger fold and ensure state advances.
		playerName := eng.Players[eng.CurrentPlayerIx%len(eng.Players)].Name
		if !eng.HumanAction(playerName, "fold", 0) {
			t.Fatalf("HumanAction returned false for player %s", playerName)
		}
	}
}

func isCurrentHuman(e *GameEngine) bool {
	idx := e.CurrentPlayerIx % len(e.Players)
	return !e.Players[idx].IsBot
}

// TestSidePots builds a hand where players go all-in at different amounts
// and verifies the side-pot builder produces the expected distribution.
func TestSidePots(t *testing.T) {
	a := &Player{Name: "A", TotalBet: 100, Folded: false}
	b := &Player{Name: "B", TotalBet: 300, Folded: false}
	c := &Player{Name: "C", TotalBet: 600, Folded: false}
	pots := buildSidePots([]*Player{a, b, c})
	if len(pots) != 3 {
		t.Fatalf("expected 3 side pots, got %d", len(pots))
	}
	if pots[0].amount != 300 { // 100 * 3 players
		t.Errorf("pot0: expected 300, got %d", pots[0].amount)
	}
	if pots[1].amount != 400 { // 200 * 2 players
		t.Errorf("pot1: expected 400, got %d", pots[1].amount)
	}
	if pots[2].amount != 300 {
		t.Errorf("pot2: expected 300, got %d", pots[2].amount)
	}
}

func TestEngineTwoHumanPhaseAdvancement(t *testing.T) {
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	eng, _, err := state.GetOrCreate("th1")
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	eng.InitGame([]string{"Alice", "Bob"})
	if !eng.StartHand() {
		t.Fatalf("StartHand returned false")
	}

	if eng.Game.Phase != "preflop" {
		t.Fatalf("expected preflop, got %s", eng.Game.Phase)
	}

	p1 := eng.Players[eng.CurrentPlayerIx%2].Name
	if !eng.HumanAction(p1, "call", 10) {
		t.Fatalf("p1 call failed")
	}

	p2 := eng.Players[eng.CurrentPlayerIx%2].Name
	if !eng.HumanAction(p2, "check", 0) {
		t.Fatalf("p2 check failed")
	}

	if eng.Game.Phase != "flop" {
		t.Fatalf("expected flop after preflop checks, got %s", eng.Game.Phase)
	}
	if len(eng.Game.CommunityCards) != 3 {
		t.Fatalf("expected 3 flop community cards, got %d", len(eng.Game.CommunityCards))
	}

	p1 = eng.Players[eng.CurrentPlayerIx%2].Name
	if !eng.HumanAction(p1, "check", 0) {
		t.Fatalf("p1 flop check failed")
	}
	p2 = eng.Players[eng.CurrentPlayerIx%2].Name
	if !eng.HumanAction(p2, "check", 0) {
		t.Fatalf("p2 flop check failed")
	}

	if eng.Game.Phase != "turn" {
		t.Fatalf("expected turn after flop checks, got %s", eng.Game.Phase)
	}
	if len(eng.Game.CommunityCards) != 4 {
		t.Fatalf("expected 4 turn community cards, got %d", len(eng.Game.CommunityCards))
	}

	p1 = eng.Players[eng.CurrentPlayerIx%2].Name
	if !eng.HumanAction(p1, "check", 0) {
		t.Fatalf("p1 turn check failed")
	}
	p2 = eng.Players[eng.CurrentPlayerIx%2].Name
	if !eng.HumanAction(p2, "check", 0) {
		t.Fatalf("p2 turn check failed")
	}

	if eng.Game.Phase != "river" {
		t.Fatalf("expected river after turn checks, got %s", eng.Game.Phase)
	}
	if len(eng.Game.CommunityCards) != 5 {
		t.Fatalf("expected 5 river community cards, got %d", len(eng.Game.CommunityCards))
	}

	p1 = eng.Players[eng.CurrentPlayerIx%2].Name
	if !eng.HumanAction(p1, "check", 0) {
		t.Fatalf("p1 river check failed")
	}
	p2 = eng.Players[eng.CurrentPlayerIx%2].Name
	if !eng.HumanAction(p2, "check", 0) {
		t.Fatalf("p2 river check failed")
	}

	if !eng.Game.Intermission || eng.Game.LastWinner == nil {
		t.Fatalf("expected showdown intermission after river, got intermission=%v, winner=%v", eng.Game.Intermission, eng.Game.LastWinner)
	}
}