package poker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEvaluateWinProbability verifies that the Monte Carlo simulator
// returns reasonable values for a few canonical inputs.
func TestEvaluateWinProbability(t *testing.T) {
	// AA vs 1 random opponent -> very high win rate preflop.
	wp := EvaluateWinProbability([]string{"As", "Ac"}, nil, 1, 500)
	if wp < 80 || wp > 100 {
		t.Errorf("AA preflop should win >=80%% vs 1 opponent, got %.1f", wp)
	}

	// 72o vs 1 random opponent preflop -> fairly low.
	wp = EvaluateWinProbability([]string{"7c", "2d"}, nil, 1, 500)
	if wp < 20 || wp > 50 {
		t.Errorf("72o preflop should be in 20-50%% vs 1 opponent, got %.1f", wp)
	}

	// Flush draw (4 hearts, 2 in hand) vs 1 opponent on the flop.
	wp = EvaluateWinProbability([]string{"Ah", "Kh"}, []string{"Qh", "Jh", "2c"}, 1, 500)
	if wp < 50 {
		t.Errorf("Made flush vs 1 should be very high, got %.1f", wp)
	}

	// Degenerate inputs return 0 rather than panic.
	if got := EvaluateWinProbability(nil, nil, 1, 500); got != 0 {
		t.Errorf("nil hole should return 0, got %.1f", got)
	}
	if got := EvaluateWinProbability([]string{"As"}, nil, 1, 500); got != 0 {
		t.Errorf("single hole card should return 0, got %.1f", got)
	}
	if got := EvaluateWinProbability([]string{"As", "Ac"}, nil, 0, 500); got != 0 {
		t.Errorf("0 opponents should return 0, got %.1f", got)
	}
	if got := EvaluateWinProbability([]string{"As", "Ac"}, nil, 1, 0); got != 0 {
		t.Errorf("0 iterations should return 0, got %.1f", got)
	}
}

// TestAdminWPModeEndpoints exercises the toggle endpoint contract.
func TestAdminWPModeEndpoints(t *testing.T) {
	tmp := t.TempDir()
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	srv, err := NewServer(state, tmp, locateTemplatesDir(t))
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	handler := srv.Routes()

	// GET -> initial state (off, service not configured).
	req := httptest.NewRequest(http.MethodGet, "/api/admin/wpmode", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: status %d, body=%s", w.Code, w.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["enabled"] != false {
		t.Errorf("initial enabled should be false, got %v", got["enabled"])
	}
	if got["service_configured"] != false {
		t.Errorf("service_configured should be false, got %v", got["service_configured"])
	}

	// POST with no body -> toggle (turns on).
	req = httptest.NewRequest(http.MethodPost, "/api/admin/wpmode",
		strings.NewReader("{}"))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("post toggle: status %d, body=%s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["enabled"] != true {
		t.Errorf("after toggle, enabled should be true, got %v", got["enabled"])
	}

	// POST with explicit enabled=false.
	req = httptest.NewRequest(http.MethodPost, "/api/admin/wpmode",
		strings.NewReader(`{"enabled":false}`))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["enabled"] != false {
		t.Errorf("after explicit false, enabled should be false, got %v", got["enabled"])
	}

	// Verify persistence through a fresh server (ConfigStore round-trip).
	srv2, err := NewServer(state, tmp, locateTemplatesDir(t))
	if err != nil {
		t.Fatalf("new server 2: %v", err)
	}
	if srv2.WinProbEnabled() {
		t.Errorf("persistence failed: second server should see enabled=false")
	}

	// Wire a fake winprob microservice and ensure state reports it.
	srv.SetWinProbClient("http://localhost:18081", 100)
	req = httptest.NewRequest(http.MethodGet, "/api/admin/wpmode", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["service_configured"] != true {
		t.Errorf("after SetWinProbClient, service_configured should be true")
	}
}