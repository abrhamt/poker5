package services

import (
	"context"
	"testing"
	"time"

	poker "github.com/zuse/poker5/go-poker"
	"github.com/zuse/poker5/go-poker/internal/testdb"
	"github.com/zuse/poker5/go-poker/repository"
)

// historyTable seats the named players at a private 10/20 table, 500 buy-in,
// with hand recording switched on — the wiring handlers/server.go does.
func historyTable(t *testing.T, wallet int64, names ...string) (*GameService, *HistoryService, *ActiveTable, []repository.User) {
	t.Helper()
	db := testdb.New(t)
	q := repository.New(db)
	roomSvc := NewRoomService(q)
	game := NewGameService(NewWalletService(q), NewSSEHub(), NewSettingsService(q), poker.NewMemoryStorage(), roomSvc)
	history := NewHistoryService(q, db)
	game.SetHistory(history)

	ctx := context.Background()
	users := make([]repository.User, 0, len(names))
	for _, name := range names {
		users = append(users, playerWithWallet(t, q, name, wallet))
	}
	room, err := roomSvc.CreatePrivateRoom(ctx, users[0].ID, "", 10, 6, 500)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	for i := range users {
		if err := game.JoinTable(ctx, room, &users[i]); err != nil {
			t.Fatalf("join %s: %v", users[i].Username, err)
		}
	}
	table, err := game.GetTable(room.RoomCode)
	if err != nil {
		t.Fatalf("get table: %v", err)
	}
	t.Cleanup(func() { stopTimers(table) })
	return game, history, table, users
}

// handsRecorded waits for a player's history to reach n hands: recording
// happens off the table lock, so it lands just after the hand ends.
func handsRecorded(t *testing.T, history *HistoryService, userID int64, n int64) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Second)
	for {
		stats, err := history.Stats(ctx, userID)
		if err != nil {
			t.Fatalf("stats: %v", err)
		}
		if stats.Hands == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("history holds %d hands, want %d", stats.Hands, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func userNamed(users []repository.User, name string) *repository.User {
	for i := range users {
		if users[i].Username == name {
			return &users[i]
		}
	}
	return nil
}

// A heads-up all-in busts the loser. Their hand is recorded with the full
// stack lost, a refill puts them back in without disturbing that record, and
// the next hand they play is recorded too.
func TestBustRefillAndNextHandAreAllRecorded(t *testing.T) {
	game, history, table, users := historyTable(t, 1000, "pat1", "quin2")
	ctx := context.Background()

	// Rig the deal: quin2 holds aces and wins, pat1 holds 7-2 and busts.
	table.mu.Lock()
	table.Engine.Game.Intermission = false
	if !table.Engine.StartHand() {
		table.mu.Unlock()
		t.Fatal("hand did not start")
	}
	seat(table, "pat1").Cards = [2]string{"7D", "2C"}
	seat(table, "quin2").Cards = [2]string{"AS", "AH"}
	table.Engine.Game.Deck = []string{"KD", "9H", "4S", "3C", "8D", "TC", "JC", "QC"}
	first := game.getCurrentTurnPlayer(table).Name
	table.mu.Unlock()

	second := "quin2"
	if first == "quin2" {
		second = "pat1"
	}
	if err := game.SubmitAction(ctx, table.RoomCode, userNamed(users, first), "allin", 0); err != nil {
		t.Fatalf("%s all-in: %v", first, err)
	}
	if err := game.SubmitAction(ctx, table.RoomCode, userNamed(users, second), "call", 0); err != nil {
		t.Fatalf("%s call: %v", second, err)
	}

	pat, quin := userNamed(users, "pat1"), userNamed(users, "quin2")
	handsRecorded(t, history, pat.ID, 1)
	handsRecorded(t, history, quin.ID, 1)

	table.mu.Lock()
	broke := seat(table, "pat1")
	busted := broke.Chips == 0 && broke.SittingOut
	table.mu.Unlock()
	if !busted {
		t.Fatalf("pat1 should be busted and sat out: chips=%d sittingOut=%v", broke.Chips, broke.SittingOut)
	}

	// The record says pat1 put in the whole 500 stack and took nothing back.
	hands, err := history.RoomHands(ctx, pat.ID, table.RoomCode, 1, 20)
	if err != nil || len(hands.Hands) != 1 {
		t.Fatalf("pat1's history: %+v, %v", hands, err)
	}
	if you := hands.Hands[0].You; you.Net != -500 || you.Cards[0] != "7D" {
		t.Fatalf("pat1's busting hand is recorded wrong: %+v", you)
	}

	// Refilling sits them in and must not touch their record.
	if _, err := game.Refill(ctx, table.RoomCode, pat.ID, 300); err != nil {
		t.Fatalf("refill: %v", err)
	}
	stats, _ := history.Stats(ctx, pat.ID)
	if stats.Hands != 1 || stats.Net != -500 {
		t.Fatalf("a refill changed pat1's stats: %+v", stats)
	}

	// The table deals again with the refilled player, and that hand is recorded.
	table.mu.Lock()
	table.Engine.Game.GameStarted = true
	table.mu.Unlock()
	game.fireNextHand(table.RoomCode)

	table.mu.Lock()
	dealt := table.Engine.Game.GameStarted && !table.Engine.Game.Intermission
	next := game.getCurrentTurnPlayer(table)
	table.mu.Unlock()
	if !dealt || next == nil {
		t.Fatal("the refilled player was not dealt into the next hand")
	}
	if err := game.SubmitAction(ctx, table.RoomCode, userNamed(users, next.Name), "fold", 0); err != nil {
		t.Fatalf("fold: %v", err)
	}

	handsRecorded(t, history, pat.ID, 2)
	handsRecorded(t, history, quin.ID, 2)
	stats, _ = history.Stats(ctx, pat.ID)
	if stats.Hands != 2 {
		t.Fatalf("pat1 should have two hands on record: %+v", stats)
	}
}
