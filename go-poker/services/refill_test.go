package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	poker "github.com/zuse/poker5/go-poker"
	"github.com/zuse/poker5/go-poker/repository"
)

// seatAt creates a private 10/20 table with a 500 buy-in and seats the given
// players, each starting with wallet ETB.
func seatAt(t *testing.T, wallet int64, names ...string) (*GameService, *repository.Queries, *ActiveTable, []repository.User) {
	t.Helper()
	roomSvc, gameSvc, q := newRoomFixture(t)
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
		if err := gameSvc.JoinTable(ctx, room, &users[i]); err != nil {
			t.Fatalf("join %s: %v", users[i].Username, err)
		}
	}
	table, err := gameSvc.GetTable(room.RoomCode)
	if err != nil {
		t.Fatalf("get table: %v", err)
	}
	t.Cleanup(func() { stopTimers(table) })
	return gameSvc, q, table, users
}

func stopTimers(table *ActiveTable) {
	table.mu.Lock()
	defer table.mu.Unlock()
	for _, timer := range []*time.Timer{table.TurnTimer, table.CountdownTimer, table.IntermissionTimer} {
		if timer != nil {
			timer.Stop()
		}
	}
	for _, timer := range table.SitOutTimers {
		timer.Stop()
	}
}

func seat(table *ActiveTable, name string) *poker.Player {
	for _, p := range table.Engine.Players {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func TestRefillAddsChipsFromTheWallet(t *testing.T) {
	game, q, table, users := seatAt(t, 1000, "ann1", "ben2")
	ctx := context.Background()

	chips, err := game.Refill(ctx, table.RoomCode, users[0].ID, 300)
	if err != nil {
		t.Fatalf("refill: %v", err)
	}
	if chips != 800 || seat(table, "ann1").Chips != 800 {
		t.Fatalf("chips = %d, want 800", chips)
	}
	if got := walletBalance(t, q, users[0].ID); got != 200 {
		t.Fatalf("wallet = %d, want 200", got)
	}
	rows, _ := q.ListTransactionsByUserID(ctx, repository.ListTransactionsByUserIDParams{UserID: users[0].ID, Limit: 1})
	if len(rows) != 1 || rows[0].Type != TxTypeTopUp || rows[0].Amount != -300 {
		t.Fatalf("ledger row = %+v", rows)
	}
}

func TestRefillRules(t *testing.T) {
	game, q, table, users := seatAt(t, 1000, "cat3", "dan4")
	ctx := context.Background()

	var tooSmall RefillTooSmallError
	if _, err := game.Refill(ctx, table.RoomCode, users[0].ID, 19); !errors.As(err, &tooSmall) || tooSmall.Min != 20 {
		t.Fatalf("below the big blind: %v", err)
	}
	if _, err := game.Refill(ctx, table.RoomCode, users[0].ID, 501); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("more than the wallet: %v", err)
	}
	if got := walletBalance(t, q, users[0].ID); got != 500 {
		t.Fatalf("a refused refill moved money: wallet = %d", got)
	}
	// No ceiling beyond the wallet.
	if _, err := game.Refill(ctx, table.RoomCode, users[0].ID, 500); err != nil {
		t.Fatalf("refill the whole wallet: %v", err)
	}

	stranger := playerWithWallet(t, q, "eve5", 1000)
	if _, err := game.Refill(ctx, table.RoomCode, stranger.ID, 100); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("refill without a seat: %v", err)
	}
}

// A stack must not change under a hand its owner is still playing; folding
// takes them out of the hand and makes the refill fine.
func TestRefillWaitsUntilThePlayerIsOutOfTheHand(t *testing.T) {
	game, _, table, users := seatAt(t, 1000, "fay6", "gus7", "hal8")
	ctx := context.Background()

	table.mu.Lock()
	table.Engine.Game.Intermission = false
	if !table.Engine.StartHand() {
		table.mu.Unlock()
		t.Fatal("hand did not start")
	}
	current := game.getCurrentTurnPlayer(table)
	table.mu.Unlock()

	var currentID int64
	for _, u := range users {
		if u.Username == current.Name {
			currentID = u.ID
		}
	}
	if _, err := game.Refill(ctx, table.RoomCode, currentID, 100); !errors.Is(err, ErrRefillDuringHand) {
		t.Fatalf("refill mid-hand: %v", err)
	}

	table.mu.Lock()
	table.Engine.HumanAction(current.Name, "fold", 0)
	stillPlaying := table.Engine.Game.GameStarted && !table.Engine.Game.Intermission
	table.mu.Unlock()
	if !stillPlaying {
		t.Fatal("with three players the hand should continue after one fold")
	}

	if _, err := game.Refill(ctx, table.RoomCode, currentID, 100); err != nil {
		t.Fatalf("refill after folding: %v", err)
	}
}

func TestPublicRefillFollowsSetting(t *testing.T) {
	roomSvc, game, q := newRoomFixture(t)
	ctx := context.Background()
	user := playerWithWallet(t, q, "ivy9", 5000)

	room, err := roomSvc.CreatePublicRoom(ctx, 10, 0, "")
	if err != nil {
		t.Fatalf("create public room: %v", err)
	}
	if err := game.JoinTable(ctx, room, &user); err != nil {
		t.Fatalf("join: %v", err)
	}
	table, _ := game.GetTable(room.RoomCode)
	t.Cleanup(func() { stopTimers(table) })

	settings := NewSettingsService(q)
	if err := settings.Update(ctx, UpdateSettingsRequest{
		RakeMode: RakeModePercentage, RakePercentage: 5, CountdownSeconds: 15, PublicRefillEnabled: false,
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if _, err := game.Refill(ctx, room.RoomCode, user.ID, 100); !errors.Is(err, ErrRefillDisabled) {
		t.Fatalf("public refill while off: %v", err)
	}

	if err := settings.Update(ctx, UpdateSettingsRequest{
		RakeMode: RakeModePercentage, RakePercentage: 5, CountdownSeconds: 15, PublicRefillEnabled: true,
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if _, err := game.Refill(ctx, room.RoomCode, user.ID, 100); err != nil {
		t.Fatalf("public refill while on: %v", err)
	}
}

// A busted player is sat out with their seat kept. Sitting back in needs
// chips; a refill provides them and sits the player in.
func TestBustedPlayerRefillsAndSitsBackIn(t *testing.T) {
	game, _, table, users := seatAt(t, 1000, "jon1", "kim2")
	ctx := context.Background()

	table.mu.Lock()
	seat(table, "jon1").Chips = 0
	game.sitOutBustedPlayers(table)
	table.mu.Unlock()

	broke := seat(table, "jon1")
	if broke == nil || !broke.SittingOut {
		t.Fatalf("busted player should keep the seat, sat out: %+v", broke)
	}
	if err := game.SitIn(ctx, table.RoomCode, users[0].ID); !errors.Is(err, ErrOutOfChips) {
		t.Fatalf("sit in with no chips: %v", err)
	}

	if _, err := game.Refill(ctx, table.RoomCode, users[0].ID, 400); err != nil {
		t.Fatalf("refill: %v", err)
	}
	if broke.SittingOut || broke.Chips != 400 {
		t.Fatalf("after refill: %+v", broke)
	}
	if _, pending := table.SitOutTimers["jon1"]; pending {
		t.Fatal("refill should cancel the eviction timer")
	}
}

func TestConcurrentRefillsNeverOverdrawTheWallet(t *testing.T) {
	game, q, table, users := seatAt(t, 1000, "lea3", "max4")
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = game.Refill(ctx, table.RoomCode, users[0].ID, 200)
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if !errors.Is(err, ErrInsufficientBalance) {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 2 {
		t.Fatalf("%d refills succeeded, want 2", ok)
	}
	if got := walletBalance(t, q, users[0].ID); got != 100 {
		t.Fatalf("wallet = %d, want 100", got)
	}
	if got := seat(table, "lea3").Chips; got != 900 {
		t.Fatalf("chips = %d, want 900", got)
	}
}

// When the pause between hands ends with fewer than two players able to play,
// the table must go back to waiting, or a refill could never start it again.
func TestTableReturnsToWaitingWhenTooFewCanPlay(t *testing.T) {
	game, _, table, users := seatAt(t, 1000, "ned5", "ola6")
	ctx := context.Background()

	table.mu.Lock()
	table.Engine.Game.GameStarted = true
	table.Engine.Game.Intermission = true
	seat(table, "ned5").Chips = 0
	game.sitOutBustedPlayers(table)
	table.mu.Unlock()

	game.fireNextHand(table.RoomCode)

	table.mu.Lock()
	started := table.Engine.Game.GameStarted
	table.mu.Unlock()
	if started {
		t.Fatal("table stayed started with no hand to play")
	}

	if _, err := game.Refill(ctx, table.RoomCode, users[0].ID, 300); err != nil {
		t.Fatalf("refill: %v", err)
	}
	table.mu.Lock()
	countdown := table.CountdownActive
	table.mu.Unlock()
	if !countdown {
		t.Fatal("refill should restart the countdown to the next hand")
	}
}
