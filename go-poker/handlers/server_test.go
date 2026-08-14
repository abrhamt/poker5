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
	"testing"

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

	req = httptest.NewRequest(http.MethodGet, "/table/"+roomCode, nil)
	req.Header.Set("Cookie", cookie)
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("render table page failed: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/lobby", nil)
	req.Header.Set("Cookie", cookie)
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("render lobby page failed: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/wallet", nil)
	req.Header.Set("Cookie", cookie)
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("render wallet page failed: %v", err)
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

func TestHTMXFormSubmissions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	srv := NewFiberServer(db)

	formBody := strings.NewReader("username=james&phone_number=0903256937&password=mypassword123")
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", formBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err := srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("HTMX form register failed: status=%d, err=%v", resp.StatusCode, err)
	}
	if redirect := resp.Header.Get("HX-Redirect"); redirect != "/lobby" {
		t.Fatalf("expected HX-Redirect to /lobby, got %s", redirect)
	}

	loginForm := strings.NewReader("login=james&password=mypassword123")
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", loginForm)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err = srv.App.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("HTMX form login failed: status=%d, err=%v", resp.StatusCode, err)
	}
	if redirect := resp.Header.Get("HX-Redirect"); redirect != "/lobby" {
		t.Fatalf("expected HX-Redirect to /lobby, got %s", redirect)
	}
}

func TestShowdownOverlayRendering(t *testing.T) {
	state := map[string]interface{}{
		"winner": &poker.Winner{
			Name:     "Joel",
			Amount:   200,
			HandName: "Two Pair, Kings and Eights",
		},
	}
	html := renderShowdownOverlay(state)
	if !strings.Contains(html, "Joel Wins!") {
		t.Errorf("expected 'Joel Wins!', got %s", html)
	}
	if !strings.Contains(html, "+200 Chips") {
		t.Errorf("expected '+200 Chips', got %s", html)
	}
	if !strings.Contains(html, "Two Pair, Kings and Eights") {
		t.Errorf("expected 'Two Pair, Kings and Eights', got %s", html)
	}
}
