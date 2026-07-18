package poker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// locateTemplatesDir walks up from the current test directory until it
// finds the templates/ folder that ships with the package. It makes the
// test independent of where `go test` is invoked from.
func locateTemplatesDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "templates")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate templates directory (wd=%s)", wd)
	return ""
}

// TestServerStartAct exercises the JSON API end-to-end using the in-memory
// storage layer.
func TestServerStartAct(t *testing.T) {
	templateDir := locateTemplatesDir(t)
	tmp := t.TempDir()
	staticDir := filepath.Join(tmp, "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatalf("mkdir static: %v", err)
	}

	storage := NewMemoryStorage()
	state := NewGameState(storage)
	srv, err := NewServer(state, staticDir, templateDir)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	handler := srv.Routes()

	// GET /
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("index: status %d, body=%s", w.Code, w.Body.String())
	}

	// POST /api/start/
	body, _ := json.Marshal(map[string]interface{}{
		"table_id": "abc123",
		"players":  []string{"Alice", "Bot Bob", "Bot Carol"},
	})
	req = httptest.NewRequest(http.MethodPost, "/api/start/", bytes.NewReader(body))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("start: status %d, body=%s", w.Code, w.Body.String())
	}
	var startResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if startResp["ok"] != true {
		t.Fatalf("start.ok missing")
	}
	state2, ok := startResp["state"].(map[string]interface{})
	if !ok {
		t.Fatalf("start.state missing")
	}
	if state2["pot"] == nil {
		t.Fatalf("start.state.pot missing")
	}

	// POST /api/act/
	body, _ = json.Marshal(map[string]interface{}{
		"table_id":    "abc123",
		"player_name": "Alice",
		"action":      "check",
		"amount":      0,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/act/", bytes.NewReader(body))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("act: status %d, body=%s", w.Code, w.Body.String())
	}

	// GET /api/state/?table_id=abc123
	req = httptest.NewRequest(http.MethodGet, "/api/state/?table_id=abc123", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("state: status %d, body=%s", w.Code, w.Body.String())
	}
}

// TestServerQRCode makes sure the QR endpoint returns an SVG.
func TestServerQRCode(t *testing.T) {
	templateDir := locateTemplatesDir(t)
	storage := NewMemoryStorage()
	state := NewGameState(storage)
	srv, err := NewServer(state, t.TempDir(), templateDir)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/qrcode/?text=hello", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("qrcode: status %d, body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("unexpected content-type: %s", ct)
	}
}