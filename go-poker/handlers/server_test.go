package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	poker "github.com/zuse/poker5/go-poker"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite in-memory: %v", err)
	}

	if err := repository.InitDBSchema(db); err != nil {
		t.Fatalf("execute InitDBSchema: %v", err)
	}
	return db
}

func TestFiberServerRoutes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := NewFiberServer(db)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("health check failed: %v", err)
	}

	regPayload, _ := json.Marshal(map[string]string{
		"username":     "alice",
		"phone_number": "0911223344",
		"password":     "password123",
	})

	req = httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regPayload))
	req.Header.Set("Content-Type", "application/json")
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("register failed: %v", err)
	}

	loginPayload, _ := json.Marshal(map[string]string{
		"login":    "alice",
		"password": "password123",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginPayload))
	req.Header.Set("Content-Type", "application/json")
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %v", err)
	}

	rawCookie := resp.Header.Get("Set-Cookie")
	if rawCookie == "" {
		t.Fatalf("expected poker_session cookie on login")
	}
	cookie := strings.Split(rawCookie, ";")[0]

	depositPayload, _ := json.Marshal(map[string]int64{
		"amount": 5000,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/wallet/deposit", bytes.NewReader(depositPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("deposit failed: %v", err)
	}

	createRoomPayload, _ := json.Marshal(map[string]interface{}{
		"room_name": "Alice Private Table",
		"buy_in":    1000,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/rooms/create-private", bytes.NewReader(createRoomPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create private room failed: %v", err)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var roomData map[string]interface{}
	_ = json.Unmarshal(bodyBytes, &roomData)
	roomCode, ok := roomData["room_code"].(string)
	if !ok || len(roomCode) != 5 {
		t.Fatalf("expected 5-digit room code, got %v", roomData)
	}

	joinCodePayload, _ := json.Marshal(map[string]string{
		"code": roomCode,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/rooms/join-code", bytes.NewReader(joinCodePayload))
	req.Header.Set("Content-Type", "application/json")
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("join by code failed: %v", err)
	}

	joinTablePayload, _ := json.Marshal(map[string]interface{}{
		"buy_in": 20.00,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/table/"+roomCode+"/join", bytes.NewReader(joinTablePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("join table failed: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/table/"+roomCode+"/state", nil)
	req.Header.Set("Cookie", cookie)
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("table state failed: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/rooms/public", nil)
	req.Header.Set("Cookie", cookie)
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("public rooms failed: %v", err)
	}

	bodyBytes, _ = io.ReadAll(resp.Body)
	var lobbyData struct {
		Rooms []map[string]interface{} `json:"rooms"`
		Tiers []services.BlindTier     `json:"tiers"`
		// The lobby offers a way back to whichever table the player is still
		// sitting at, which is the private one they just joined.
		ActiveRoomCode string `json:"active_room_code"`
	}
	if err := json.Unmarshal(bodyBytes, &lobbyData); err != nil {
		t.Fatalf("decode public rooms: %v", err)
	}
	if len(lobbyData.Tiers) == 0 {
		t.Fatalf("expected blind tiers in the lobby payload, got %s", bodyBytes)
	}
	if lobbyData.ActiveRoomCode != roomCode {
		t.Fatalf("expected active_room_code %q, got %q", roomCode, lobbyData.ActiveRoomCode)
	}
}

func TestGameServiceTwoPlayerHand(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	q := repository.New(db)
	authSvc := services.NewAuthService(q)
	walletSvc := services.NewWalletService(q)
	sseHub := services.NewSSEHub()
	roomSvc := services.NewRoomService(q)
	settingsSvc := services.NewSettingsService(q)
	gameSvc := services.NewGameService(walletSvc, sseHub, settingsSvc, poker.NewMemoryStorage(), roomSvc)

	u1, err := authSvc.Register(context.Background(), services.RegisterRequest{
		Username:    "player1",
		PhoneNumber: "0911000001",
		Password:    "pass",
	})
	if err != nil {
		t.Fatalf("register u1: %v", err)
	}
	u2, err := authSvc.Register(context.Background(), services.RegisterRequest{
		Username:    "player2",
		PhoneNumber: "0711000002",
		Password:    "pass",
	})
	if err != nil {
		t.Fatalf("register u2: %v", err)
	}

	_, _ = walletSvc.Deposit(context.Background(), u1.ID, 5000)
	_, _ = walletSvc.Deposit(context.Background(), u2.ID, 5000)

	// Private-room joins now bring the caller's live wallet balance to the
	// felt, so re-fetch u1/u2 to pick up the deposits above.
	u1Fresh, err := q.GetUserByID(context.Background(), u1.ID)
	if err != nil {
		t.Fatalf("refetch u1: %v", err)
	}
	u1 = &u1Fresh
	u2Fresh, err := q.GetUserByID(context.Background(), u2.ID)
	if err != nil {
		t.Fatalf("refetch u2: %v", err)
	}
	u2 = &u2Fresh

	room, err := roomSvc.CreatePrivateRoom(context.Background(), u1.ID, "Test Room", 10, 6)
	if err != nil {
		t.Fatalf("create private room: %v", err)
	}
	roomCode := room.RoomCode

	err = gameSvc.JoinTable(context.Background(), room, u1)
	if err != nil {
		t.Fatalf("join u1: %v", err)
	}
	err = gameSvc.JoinTable(context.Background(), room, u2)
	if err != nil {
		t.Fatalf("join u2: %v", err)
	}

	table, err := gameSvc.GetTable(roomCode)
	if err != nil {
		t.Fatalf("get table: %v", err)
	}

	if table.Engine.Game.GameStarted {
		t.Fatalf("expected game to wait for the countdown, not auto-start immediately")
	}
	if !table.CountdownActive {
		t.Fatalf("expected countdown to be active once 2 players joined")
	}

	// Host starts early, skipping the rest of the countdown.
	if err := gameSvc.StartHandManually(context.Background(), roomCode, u1.ID); err != nil {
		t.Fatalf("host start now: %v", err)
	}
	if !table.Engine.Game.GameStarted {
		t.Fatalf("expected game to be started after host start-now")
	}

	state1 := gameSvc.GetTableState(context.Background(), roomCode, "player1")
	state2 := gameSvc.GetTableState(context.Background(), roomCode, "player2")

	if state1["room_code"] != roomCode || state2["room_code"] != roomCode {
		t.Fatalf("mismatched room codes in state")
	}

	err = gameSvc.LeaveTable(context.Background(), roomCode, u1.ID)
	if err != nil {
		t.Fatalf("leave u1: %v", err)
	}
	err = gameSvc.LeaveTable(context.Background(), roomCode, u2.ID)
	if err != nil {
		t.Fatalf("leave u2: %v", err)
	}
}

// Session expiry slides so an active player is never signed out mid-hand, but
// only when the session is close enough to expiring to be worth a write, and
// never past the absolute ceiling.
func TestSessionExpirySlides(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := NewFiberServer(db)

	regPayload, _ := json.Marshal(map[string]string{
		"username":     "slider",
		"phone_number": "0911224477",
		"password":     "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regPayload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("register failed: %v", err)
	}
	cookie := strings.Split(resp.Header.Get("Set-Cookie"), ";")[0]
	token := strings.TrimPrefix(cookie, "poker_session=")

	expiry := func() time.Time {
		t.Helper()
		var out time.Time
		row := srv.DB.QueryRow("SELECT expires_at FROM user_sessions WHERE session_token = ?", token)
		if err := row.Scan(&out); err != nil {
			t.Fatalf("read session expiry: %v", err)
		}
		return out
	}
	setSession := func(expiresAt, createdAt time.Time) {
		t.Helper()
		if _, err := srv.DB.Exec(
			"UPDATE user_sessions SET expires_at = ?, created_at = ? WHERE session_token = ?",
			expiresAt, createdAt, token,
		); err != nil {
			t.Fatalf("age session: %v", err)
		}
	}
	touch := func() {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.Header.Set("Cookie", cookie)
		resp, err := srv.App.Test(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("authenticated request failed: %v", err)
		}
	}

	// A fresh session has most of its life left, so using it must not cost a write.
	fresh := expiry()
	touch()
	if got := expiry(); !got.Equal(fresh) {
		t.Fatalf("expected no renewal on a fresh session: %v -> %v", fresh, got)
	}

	// Inside the renewal window it slides back out to a full idle period.
	now := time.Now()
	setSession(now.Add(2*time.Hour), now.Add(-services.SessionIdleTTL+2*time.Hour))
	touch()
	renewed := expiry()
	if want := now.Add(services.SessionIdleTTL); renewed.Before(want.Add(-time.Hour)) {
		t.Fatalf("expected the session to slide to ~%v, got %v", want, renewed)
	}

	// Past the absolute ceiling it is left to die on schedule, however active
	// the player is.
	nearDeath := now.Add(1 * time.Hour)
	setSession(nearDeath, now.Add(-services.SessionAbsoluteTTL-24*time.Hour))
	touch()
	if got := expiry(); got.After(nearDeath.Add(time.Minute)) {
		t.Fatalf("expected no renewal past the absolute ceiling, got %v", got)
	}
}

// The Secure attribute on the session cookie is env-driven: off by default so
// HTTP and LAN-IP development still signs in, on when explicitly enabled.
func TestSessionCookieSecureFlag(t *testing.T) {
	login := func(t *testing.T, secure string) string {
		t.Helper()
		t.Setenv("SESSION_COOKIE_SECURE", secure)

		db := setupTestDB(t)
		defer db.Close()
		srv := NewFiberServer(db)

		regPayload, _ := json.Marshal(map[string]string{
			"username":     "cookie",
			"phone_number": "0911224488",
			"password":     "password123",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regPayload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := srv.App.Test(req)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("register failed: %v", err)
		}
		return resp.Header.Get("Set-Cookie")
	}

	off := login(t, "")
	if strings.Contains(off, "secure") || strings.Contains(off, "Secure") {
		t.Fatalf("expected no Secure attribute by default, got %q", off)
	}
	// The attributes that are always on, whatever the transport.
	for _, want := range []string{"poker_session=", "HttpOnly", "SameSite=Lax", "path=/"} {
		if !strings.Contains(off, want) {
			t.Fatalf("expected %q in the session cookie, got %q", want, off)
		}
	}

	on := login(t, "true")
	if !strings.Contains(on, "secure") && !strings.Contains(on, "Secure") {
		t.Fatalf("expected a Secure attribute with SESSION_COOKIE_SECURE=true, got %q", on)
	}
}

// The wallet ledger is paged server-side; out-of-range requests clamp to a real
// page rather than returning an empty list or an error.
func TestWalletTransactionsPagination(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := NewFiberServer(db)

	regPayload, _ := json.Marshal(map[string]string{
		"username":     "ledger",
		"phone_number": "0911223399",
		"password":     "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regPayload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("register failed: %v", err)
	}
	cookie := strings.Split(resp.Header.Get("Set-Cookie"), ";")[0]

	// One more deposit than a page holds, so there is a second page to reach.
	deposits := services.DefaultTransactionsPageSize + 3
	for i := 0; i < deposits; i++ {
		body, _ := json.Marshal(map[string]int64{"amount": int64(10 + i)})
		req = httptest.NewRequest(http.MethodPost, "/api/wallet/deposit", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)
		resp, err = srv.App.Test(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("deposit %d failed: %v", i, err)
		}
	}

	type ledgerPage struct {
		Transactions []map[string]interface{} `json:"transactions"`
		Page         int                      `json:"page"`
		PageSize     int                      `json:"page_size"`
		Total        int64                    `json:"total"`
		TotalPages   int                      `json:"total_pages"`
	}

	get := func(query string) ledgerPage {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/wallet/transactions"+query, nil)
		req.Header.Set("Cookie", cookie)
		resp, err := srv.App.Test(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("transactions%s failed: %v", query, err)
		}
		body, _ := io.ReadAll(resp.Body)
		var page ledgerPage
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatalf("decode transactions: %v (%s)", err, body)
		}
		return page
	}

	first := get("")
	if first.Page != 1 || first.PageSize != services.DefaultTransactionsPageSize {
		t.Fatalf("expected page 1 at the default size, got page=%d size=%d", first.Page, first.PageSize)
	}
	if int64(deposits) != first.Total {
		t.Fatalf("expected %d transactions, got %d", deposits, first.Total)
	}
	if len(first.Transactions) != services.DefaultTransactionsPageSize {
		t.Fatalf("expected a full first page, got %d rows", len(first.Transactions))
	}
	if first.TotalPages != 2 {
		t.Fatalf("expected 2 pages, got %d", first.TotalPages)
	}

	second := get("?page=2")
	if second.Page != 2 || len(second.Transactions) != deposits-services.DefaultTransactionsPageSize {
		t.Fatalf("unexpected second page: page=%d rows=%d", second.Page, len(second.Transactions))
	}
	if second.Transactions[0]["id"] == first.Transactions[0]["id"] {
		t.Fatalf("second page repeated the first page's newest row")
	}

	// Past the end clamps back to the last page; below the start clamps to 1.
	if beyond := get("?page=99"); beyond.Page != 2 || len(beyond.Transactions) == 0 {
		t.Fatalf("expected page 99 to clamp to the last page, got page=%d rows=%d", beyond.Page, len(beyond.Transactions))
	}
	if under := get("?page=0"); under.Page != 1 {
		t.Fatalf("expected page 0 to clamp to page 1, got %d", under.Page)
	}

	// An oversized page_size is capped rather than letting a client pull the
	// whole ledger in one request.
	if huge := get("?page_size=5000"); huge.PageSize != services.MaxTransactionsPageSize {
		t.Fatalf("expected page_size to cap at %d, got %d", services.MaxTransactionsPageSize, huge.PageSize)
	}
}

// This server is a JSON API and serves no files: an unrouted path answers with
// JSON rather than HTML, a page, or Fiber's plain-text default.
func TestUnroutedPathsReturnJSON(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := NewFiberServer(db)

	for _, path := range []string{
		"/api/does-not-exist",
		// Former file routes — the bundle is served by the web server now.
		"/static/poker/cards/AS.svg",
		"/assets/index.js",
		// Former SPA routes, which belong to the static bundle.
		"/lobby",
		"/",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp, err := srv.App.Test(req)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("%s: expected a JSON content type, got %q", path, ct)
		}
	}
}

// The admin dashboard is the one server-rendered page left, and it must keep
// working while its UI is still HTML.
func TestAdminDashboardStillRenders(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "admin-password-123")

	db := setupTestDB(t)
	defer db.Close()

	srv := NewFiberServer(db)

	loginPayload, _ := json.Marshal(map[string]string{
		"login":    "admin",
		"password": "admin-password-123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginPayload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("admin login failed: %v", err)
	}
	cookie := strings.Split(resp.Header.Get("Set-Cookie"), ";")[0]

	req = httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.Header.Set("Cookie", cookie)
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("admin dashboard failed: %v (status %d)", err, resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	// Its assets ship in the frontend bundle, so they must be referenced at the
	// paths the web server publishes them on — not the old /static tree.
	for _, want := range []string{"/legacy/admin.css", "/legacy/admin.js"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("expected the admin page to load %s", want)
		}
	}
	if strings.Contains(string(body), "/static/") {
		t.Fatalf("admin page still references the removed /static tree")
	}
}

func TestSSEHubNoPanicOnDisconnect(t *testing.T) {
	hub := services.NewSSEHub()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, unsub := hub.Subscribe("table-1")
				hub.Broadcast("table-1", "ping", map[string]string{"foo": "bar"})
				unsub()
			}
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				hub.Broadcast("table-1", "state", map[string]int{"val": j})
			}
		}()
	}
	wg.Wait()
}

func TestStorageLeaveTablePersistence(t *testing.T) {
	storage := poker.NewMemoryStorage()
	g, _, _, err := storage.GetOrCreateGame("room-test")
	if err != nil {
		t.Fatalf("get or create game: %v", err)
	}
	p1 := &poker.Player{ID: 1, Name: "james", SeatIndex: 0, Chips: 2000}
	p2 := &poker.Player{ID: 2, Name: "alex", SeatIndex: 1, Chips: 2000}
	if err := storage.SaveGame(g, []*poker.Player{p1, p2}); err != nil {
		t.Fatalf("save initial players: %v", err)
	}

	if err := storage.SaveGame(g, []*poker.Player{p1}); err != nil {
		t.Fatalf("save after leave: %v", err)
	}

	_, loaded, err := storage.LoadGame("room-test")
	if err != nil {
		t.Fatalf("load game: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 player after leave, got %d", len(loaded))
	}
	if loaded[0].ID != 1 {
		t.Fatalf("expected remaining player ID=1, got %d", loaded[0].ID)
	}
}
