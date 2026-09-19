package poker

import (
	"os"
	"testing"

	"github.com/zuse/poker5/go-poker/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.RunMain(m))
}

// Every value in a saved table has to survive the round trip through
// PostgreSQL. This is where the port had the most room to go wrong: the flags
// used to be stored as integers and the timestamps as formatted strings, so a
// column that reads back as the wrong type or a silently dropped field would
// show up as a table restoring with folded players unfolded or cards face-up.
func TestPostgresStorageRoundTrip(t *testing.T) {
	db := testdb.New(t)
	storage, err := NewPostgresStorageFromDB(db)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	game, players, created, err := storage.GetOrCreateGame("table-1")
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	if !created || len(players) != 0 {
		t.Fatalf("first call: created=%v players=%d, want true/0", created, len(players))
	}

	prob := 0.42
	dealer := "alice"
	game.Phase = "flop"
	game.Pot = 300
	game.CurrentBet = 100
	game.GameStarted = true
	game.GameFinished = false
	game.OpenCardsMode = true
	game.SpectatorMode = false
	game.Intermission = true
	game.IntermissionStartedAt = 1700000000
	game.InitialDealerName = &dealer
	game.CommunityCards = []string{"AS", "KD", "7H"}
	game.Deck = []string{"2C", "3C"}
	game.TotalHands = 7
	game.Notifications = []Notification{{Seq: 1, Kind: NoteSystem, Text: "hand started"}}

	saved := []*Player{
		{
			ID: 11, Name: "alice", SeatIndex: 0, IsBot: false, Chips: 1500,
			RoundBet: 100, TotalBet: 250, Folded: false, AllIn: true,
			IsDealer: true, IsSmallBlind: false, IsBigBlind: true,
			Cards: [2]string{"AS", "KS"}, SittingOut: false, WinProbability: &prob,
		},
		{
			// A bot, folded, sitting out, holding no cards: the combination
			// where every boolean is the opposite of the seat above.
			ID: 0, Name: "bot-1", SeatIndex: 3, IsBot: true, Chips: 900,
			RoundBet: 0, TotalBet: 40, Folded: true, AllIn: false,
			IsDealer: false, IsSmallBlind: true, IsBigBlind: false,
			Cards: [2]string{"1B", "1B"}, SittingOut: true,
		},
	}
	if err := storage.SaveGame(game, saved); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, loadedPlayers, err := storage.LoadGame("table-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Phase != "flop" || loaded.Pot != 300 || loaded.CurrentBet != 100 || loaded.TotalHands != 7 {
		t.Errorf("scalars = %+v", loaded)
	}
	if !loaded.GameStarted || loaded.GameFinished || !loaded.OpenCardsMode || loaded.SpectatorMode || !loaded.Intermission {
		t.Errorf("game flags round-tripped wrong: started=%v finished=%v open=%v spectator=%v intermission=%v",
			loaded.GameStarted, loaded.GameFinished, loaded.OpenCardsMode, loaded.SpectatorMode, loaded.Intermission)
	}
	if loaded.IntermissionStartedAt != 1700000000 {
		t.Errorf("intermission_started_at = %d", loaded.IntermissionStartedAt)
	}
	if loaded.InitialDealerName == nil || *loaded.InitialDealerName != "alice" {
		t.Errorf("initial dealer = %v", loaded.InitialDealerName)
	}
	if len(loaded.CommunityCards) != 3 || loaded.CommunityCards[0] != "AS" {
		t.Errorf("community cards = %v", loaded.CommunityCards)
	}
	if len(loaded.Notifications) != 1 || loaded.Notifications[0].Text != "hand started" {
		t.Errorf("notifications = %+v", loaded.Notifications)
	}
	// Timestamps used to be written as formatted strings; a zero here means the
	// column and the Go value disagree about their type.
	if loaded.CreatedAt.IsZero() || loaded.UpdatedAt.IsZero() {
		t.Errorf("timestamps = %v / %v", loaded.CreatedAt, loaded.UpdatedAt)
	}
	// PhaseIndex is derived on load and is what the engine advances from.
	if Phases[loaded.PhaseIndex] != "flop" {
		t.Errorf("phase index = %d", loaded.PhaseIndex)
	}

	if len(loadedPlayers) != 2 {
		t.Fatalf("players = %d, want 2", len(loadedPlayers))
	}
	// Ordered by seat, so alice is first.
	alice, bot := loadedPlayers[0], loadedPlayers[1]
	if alice.Name != "alice" || alice.Chips != 1500 || alice.SeatIndex != 0 {
		t.Errorf("alice = %+v", alice)
	}
	if alice.IsBot || alice.Folded || !alice.AllIn || !alice.IsDealer || alice.IsSmallBlind || !alice.IsBigBlind || alice.SittingOut {
		t.Errorf("alice flags round-tripped wrong: %+v", alice)
	}
	if alice.Cards != [2]string{"AS", "KS"} {
		t.Errorf("alice cards = %v", alice.Cards)
	}
	if alice.WinProbability == nil || *alice.WinProbability != prob {
		t.Errorf("alice win probability = %v", alice.WinProbability)
	}
	if !bot.IsBot || !bot.Folded || bot.AllIn || bot.IsDealer || !bot.IsSmallBlind || bot.IsBigBlind || !bot.SittingOut {
		t.Errorf("bot flags round-tripped wrong: %+v", bot)
	}
	// A player with no cards stores NULL and must read back as the card back,
	// not as an empty string the client would try to render.
	if bot.Cards != [2]string{"1B", "1B"} {
		t.Errorf("bot cards = %v", bot.Cards)
	}
	if bot.WinProbability != nil {
		t.Errorf("bot win probability = %v, want nil", bot.WinProbability)
	}

	// Saving again replaces the seats rather than stacking them up — the seat
	// index is UNIQUE per game, so a bug here surfaces as a constraint error.
	if err := storage.SaveGame(loaded, loadedPlayers); err != nil {
		t.Fatalf("second save: %v", err)
	}
	_, again, err := storage.LoadGame("table-1")
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if len(again) != 2 {
		t.Fatalf("players after re-save = %d, want 2", len(again))
	}
}

func TestPostgresStorageConfigStore(t *testing.T) {
	db := testdb.New(t)
	storage, err := NewPostgresStorageFromDB(db)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	if _, ok, err := storage.GetConfig("missing"); err != nil || ok {
		t.Fatalf("missing key: ok=%v err=%v, want false/nil", ok, err)
	}
	if err := storage.SetConfig("blinds", "10/20"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// The upsert path: writing the same key again replaces rather than
	// colliding with the primary key.
	if err := storage.SetConfig("blinds", "25/50"); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	value, ok, err := storage.GetConfig("blinds")
	if err != nil || !ok || value != "25/50" {
		t.Fatalf("get = %q/%v/%v, want 25/50", value, ok, err)
	}
}

// GetOrCreateGame has to be idempotent per table: the second call must find the
// row the first one inserted, not fail on the UNIQUE table_id.
func TestPostgresStorageGetOrCreateIsIdempotent(t *testing.T) {
	db := testdb.New(t)
	storage, err := NewPostgresStorageFromDB(db)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	first, _, created, err := storage.GetOrCreateGame("table-2")
	if err != nil || !created {
		t.Fatalf("first: created=%v err=%v", created, err)
	}
	second, _, created, err := storage.GetOrCreateGame("table-2")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if created {
		t.Error("second call reported a fresh game")
	}
	if first.ID != second.ID {
		t.Errorf("ids differ: %d vs %d", first.ID, second.ID)
	}
}
