package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zuse/poker5/go-poker/internal/testdb"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

func createPlayer(t *testing.T, q *repository.Queries, name, phone, ref string) *repository.User {
	t.Helper()
	hash, _ := utilities.HashPassword("password123")
	id, err := q.CreateUser(context.Background(), repository.CreateUserParams{
		Username: name, PhoneNumber: phone, PasswordHash: hash, ReferralCode: ref, Role: "player",
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	u, err := q.GetUserByID(context.Background(), id)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return &u
}

func TestHistoryRecordsOnlyWhatEachPlayerMaySee(t *testing.T) {
	db := testdb.New(t)
	q := repository.New(db)
	ctx := context.Background()
	history := NewHistoryService(q, db)

	alice := createPlayer(t, q, "alice", "+251911000001", "AAA001")
	bob := createPlayer(t, q, "bob", "+251911000002", "BBB002")

	// Hand 1: bob folds; alice beats the bot at showdown and pays 2 in rake.
	if err := history.RecordHand(ctx, HandRecord{
		RoomCode: "11111", HandNumber: 1, SmallBlind: 10, BigBlind: 20,
		Board:      []string{"AH", "KH", "2C", "7D", "9S"},
		WinnerName: "alice", WinnerUserID: alice.ID, WinningHand: "Pair of Aces",
		Pot: 60, Rake: 2,
		Players: []HandRecordPlayer{
			{UserID: alice.ID, Name: "alice", Seat: 0, Cards: [2]string{"AS", "3D"}, Invested: 20, Won: 58, ShowedDown: true, HandName: "Pair of Aces"},
			{UserID: bob.ID, Name: "bob", Seat: 1, Cards: [2]string{"QC", "QD"}, Invested: 20, Folded: true},
			{Name: "Bot 1", Seat: 2, Cards: [2]string{"5S", "6S"}, Invested: 20, ShowedDown: true, HandName: "High Card"},
		},
	}); err != nil {
		t.Fatalf("record hand 1: %v", err)
	}
	// Hand 2: alice loses 40 in another room.
	if err := history.RecordHand(ctx, HandRecord{
		RoomCode: "22222", HandNumber: 1, SmallBlind: 20, BigBlind: 40,
		WinnerName: "bob", WinnerUserID: bob.ID, Pot: 80,
		Players: []HandRecordPlayer{
			{UserID: alice.ID, Name: "alice", Seat: 0, Cards: [2]string{"2S", "7C"}, Invested: 40, Folded: true},
			{UserID: bob.ID, Name: "bob", Seat: 1, Cards: [2]string{"JC", "JD"}, Invested: 40, Won: 80},
		},
	}); err != nil {
		t.Fatalf("record hand 2: %v", err)
	}

	rooms, err := history.Rooms(ctx, alice.ID)
	if err != nil {
		t.Fatalf("rooms: %v", err)
	}
	if len(rooms) != 2 || rooms[0].RoomCode != "22222" {
		t.Fatalf("want both rooms, newest first; got %+v", rooms)
	}

	page, err := history.RoomHands(ctx, alice.ID, "11111", 1, 20)
	if err != nil {
		t.Fatalf("room hands: %v", err)
	}
	if page.Total != 1 || len(page.Hands) != 1 {
		t.Fatalf("want 1 hand in room 11111, got %+v", page)
	}
	hand := page.Hands[0]
	if hand.You.Net != 38 || hand.You.Cards[0] != "AS" {
		t.Fatalf("alice's own row is wrong: %+v", hand.You)
	}
	for _, p := range hand.Players {
		if p.Name == "bob" && len(p.Cards) != 0 {
			t.Fatalf("bob folded, so alice must not see his cards; got %v", p.Cards)
		}
		if p.Name == "Bot 1" && len(p.Cards) != 2 {
			t.Fatalf("the bot showed down, so its cards belong in the record; got %v", p.Cards)
		}
	}

	// Each player's history holds their own rows: bob sees his own folded hand.
	bobPage, _ := history.RoomHands(ctx, bob.ID, "11111", 1, 20)
	if bobPage.Total != 1 || bobPage.Hands[0].You.Cards[0] != "QC" {
		t.Fatalf("bob should see exactly his own folded hand: %+v", bobPage)
	}

	stats, err := history.Stats(ctx, alice.ID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Hands != 2 || stats.HandsWon != 1 || stats.Net != -2 || stats.BestHand != "Pair" {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if n := len(stats.Trend); n != 2 || stats.Trend[n-1].Cumulative != stats.Net {
		t.Fatalf("the trend must end at the all-time net: %+v", stats.Trend)
	}
}

func TestProfileChanges(t *testing.T) {
	db := testdb.New(t)
	q := repository.New(db)
	ctx := context.Background()
	profile := NewProfileService(q, nil)

	alice := createPlayer(t, q, "alice", "+251911000001", "AAA001")
	createPlayer(t, q, "bob", "+251911000002", "BBB002")

	if err := profile.SetUsername(ctx, alice, "bob"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("renaming onto another account must fail, got %v", err)
	}
	if err := profile.SetUsername(ctx, alice, "no spaces"); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("want ErrInvalidUsername, got %v", err)
	}
	if err := profile.SetUsername(ctx, alice, "alice_2"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if err := profile.SetAvatar(ctx, alice, AvatarCount); !errors.Is(err, ErrInvalidAvatar) {
		t.Fatalf("want ErrInvalidAvatar, got %v", err)
	}
	if err := profile.SetAvatar(ctx, alice, 3); err != nil {
		t.Fatalf("set avatar: %v", err)
	}

	expires := time.Now().Add(time.Hour)
	for _, token := range []string{"this-device", "other-device"} {
		if err := q.CreateSessionToken(ctx, repository.CreateSessionTokenParams{SessionToken: token, UserID: alice.ID, ExpiresAt: expires}); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	if err := profile.SetPassword(ctx, alice, "this-device", "wrong", "newpass123"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("want ErrWrongPassword, got %v", err)
	}
	if err := profile.SetPassword(ctx, alice, "this-device", "password123", "newpass123"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if _, err := q.GetSessionWithUser(ctx, "this-device"); err != nil {
		t.Fatalf("the session that changed the password must survive: %v", err)
	}
	if _, err := q.GetSessionWithUser(ctx, "other-device"); err == nil {
		t.Fatalf("every other session must be signed out")
	}

	reloaded, _ := q.GetUserByID(ctx, alice.ID)
	if reloaded.Username != "alice_2" || reloaded.Avatar != 3 || !utilities.CheckPassword("newpass123", reloaded.PasswordHash) {
		t.Fatalf("profile not persisted: %+v", reloaded)
	}
}
