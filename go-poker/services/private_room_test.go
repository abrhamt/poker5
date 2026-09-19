package services

import (
	"context"
	"errors"
	"testing"

	poker "github.com/zuse/poker5/go-poker"
	"github.com/zuse/poker5/go-poker/internal/testdb"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

func newRoomFixture(t *testing.T) (*RoomService, *GameService, *repository.Queries) {
	t.Helper()
	db := testdb.New(t)
	q := repository.New(db)
	walletSvc := NewWalletService(q)
	settings := NewSettingsService(q)
	roomSvc := NewRoomService(q)
	gameSvc := NewGameService(walletSvc, NewSSEHub(), settings, poker.NewMemoryStorage(), roomSvc)
	return roomSvc, gameSvc, q
}

func playerWithWallet(t *testing.T, q *repository.Queries, name string, wallet int64) repository.User {
	t.Helper()
	ctx := context.Background()
	passHash, _ := utilities.HashPassword("password123")
	id, err := q.CreateUser(ctx, repository.CreateUserParams{
		Username:     name,
		PhoneNumber:  "+2519" + name[len(name)-1:] + "0000000",
		PasswordHash: passHash,
		ReferralCode: "R" + name[len(name)-1:] + "0000",
		Role:         "player",
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if err := q.UpdateUserWallet(ctx, repository.UpdateUserWalletParams{Wallet: wallet, ID: id}); err != nil {
		t.Fatalf("fund %s: %v", name, err)
	}
	user, err := q.GetUserByID(ctx, id)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return user
}

func TestPrivateRoomDefaultsMinimumBuyInToFiftyBigBlinds(t *testing.T) {
	roomSvc, _, q := newRoomFixture(t)
	host := playerWithWallet(t, q, "host1", 5000)

	room, err := roomSvc.CreatePrivateRoom(context.Background(), host.ID, "", 10, 6, 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if room.BuyIn != 20*DefaultPrivateBuyInBBMultiple {
		t.Fatalf("buy_in = %d, want %d", room.BuyIn, 20*DefaultPrivateBuyInBBMultiple)
	}
}

func TestPrivateRoomRejectsMinimumBuyInBelowTenBigBlinds(t *testing.T) {
	roomSvc, _, q := newRoomFixture(t)
	host := playerWithWallet(t, q, "host2", 5000)

	if _, err := roomSvc.CreatePrivateRoom(context.Background(), host.ID, "", 10, 6, 199); !errors.Is(err, ErrPrivateBuyInLow) {
		t.Fatalf("expected ErrPrivateBuyInLow, got %v", err)
	}
	room, err := roomSvc.CreatePrivateRoom(context.Background(), host.ID, "", 10, 6, 200)
	if err != nil {
		t.Fatalf("create at the floor: %v", err)
	}
	if room.BuyIn != 200 {
		t.Fatalf("buy_in = %d, want 200", room.BuyIn)
	}
}

func TestPrivateRoomJoinEnforcesMinimumBuyIn(t *testing.T) {
	roomSvc, gameSvc, q := newRoomFixture(t)
	ctx := context.Background()
	host := playerWithWallet(t, q, "host3", 5000)
	poor := playerWithWallet(t, q, "poor4", 499)
	rich := playerWithWallet(t, q, "rich5", 1250)

	room, err := roomSvc.CreatePrivateRoom(ctx, host.ID, "", 10, 6, 500)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := gameSvc.JoinTable(ctx, room, &poor); !errors.Is(err, ErrBelowMinimumBuyIn) {
		t.Fatalf("expected ErrBelowMinimumBuyIn, got %v", err)
	}
	if got := walletBalance(t, q, poor.ID); got != 499 {
		t.Fatalf("refused join touched the wallet: %d", got)
	}

	if err := gameSvc.JoinTable(ctx, room, &rich); err != nil {
		t.Fatalf("join above minimum: %v", err)
	}
	if got := walletBalance(t, q, rich.ID); got != 0 {
		t.Fatalf("whole wallet should become chips, wallet = %d", got)
	}
	table, err := gameSvc.GetTable(room.RoomCode)
	if err != nil {
		t.Fatalf("get table: %v", err)
	}
	if len(table.Engine.Players) != 1 || table.Engine.Players[0].Chips != 1250 {
		t.Fatalf("unexpected seated chips %+v", table.Engine.Players)
	}
}

func TestIntermissionDurationFollowsSetting(t *testing.T) {
	_, gameSvc, q := newRoomFixture(t)
	ctx := context.Background()

	if got := gameSvc.intermissionDuration().Seconds(); got != DefaultIntermissionSeconds {
		t.Fatalf("default pause = %v, want %d", got, DefaultIntermissionSeconds)
	}

	settings := NewSettingsService(q)
	if err := settings.Update(ctx, UpdateSettingsRequest{
		RakeMode: RakeModePercentage, RakePercentage: 5, CountdownSeconds: 15, IntermissionSeconds: 7,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := gameSvc.intermissionDuration().Seconds(); got != 7 {
		t.Fatalf("pause = %v, want 7", got)
	}

	err := settings.Update(ctx, UpdateSettingsRequest{
		RakeMode: RakeModePercentage, RakePercentage: 5, CountdownSeconds: 15, IntermissionSeconds: MaxIntermissionSeconds + 1,
	})
	if !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("expected ErrInvalidSettings for %d, got %v", MaxIntermissionSeconds+1, err)
	}
	if err := settings.Update(ctx, UpdateSettingsRequest{
		RakeMode: RakeModePercentage, RakePercentage: 5, CountdownSeconds: 15,
	}); err != nil {
		t.Fatalf("update without pause: %v", err)
	}
	if got := gameSvc.intermissionDuration().Seconds(); got != DefaultIntermissionSeconds {
		t.Fatalf("omitted pause should fall back to %d, got %v", DefaultIntermissionSeconds, got)
	}
}
