package poker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Server bundles the HTTP dependencies: storage-backed game state, a
// directory of static assets and a directory of html/template files.
//
// It is the Go port of the Django urlpatterns + views module pair in
// poker/urls.py and poker/views.py.
type Server struct {
	State          *GameState
	StaticDir      string
	TemplateDir    string
	StaticPrefix   string
	tmpl           *template.Template
	renderTable    *template.Template
	renderHole     *template.Template
	qrCodeRenderer QRCodeRenderer
	WinProb        *WinProbClient
	config         ConfigStore
	wpMu           sync.RWMutex
	wpEnabled      bool
}

// QRCodeRenderer is a pluggable interface for generating QR code SVG
// bodies. The default implementation writes a minimal text placeholder;
// replace with a real implementation (e.g. wrapping a library) when
// deploying.
type QRCodeRenderer interface {
	Render(text string) ([]byte, error)
}

// StaticPlaceholderQR is the default QR renderer: it returns a tiny SVG
// containing the supplied text. Swap in a real implementation via
// Server.SetQRRenderer when you want scanable codes.
type StaticPlaceholderQR struct{}

// Render satisfies QRCodeRenderer.
func (StaticPlaceholderQR) Render(text string) ([]byte, error) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200">
  <rect width="200" height="200" fill="white"/>
  <text x="100" y="100" text-anchor="middle" font-family="monospace" font-size="14" fill="black">%s</text>
</svg>`, template.HTMLEscapeString(text))
	return []byte(body), nil
}

// NewServer wires together a Server with the supplied dependencies. The
// static and template directories must already exist on disk.
func NewServer(state *GameState, staticDir, templateDir string) (*Server, error) {
	s := &Server{
		State:          state,
		StaticDir:      staticDir,
		TemplateDir:    templateDir,
		StaticPrefix:   "/static/",
		qrCodeRenderer: StaticPlaceholderQR{},
	}
	if cs, ok := state.Storage.(ConfigStore); ok {
		s.config = cs
		if v, found, err := cs.GetConfig("win_prob_enabled"); err == nil && found {
			s.wpEnabled = v == "1" || v == "true"
		}
	}
	if err := s.loadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

// SetQRRenderer replaces the default QR renderer.
func (s *Server) SetQRRenderer(r QRCodeRenderer) { s.qrCodeRenderer = r }

// SetWinProbClient wires the win-probability microservice into the server.
// The microservice URL is e.g. http://localhost:18081.
func (s *Server) SetWinProbClient(addr string, iterations int) {
	if addr == "" {
		s.WinProb = nil
		return
	}
	s.WinProb = NewWinProbClient(addr, iterations)
}

// WinProbEnabled returns the current admin-controlled toggle.
func (s *Server) WinProbEnabled() bool {
	s.wpMu.RLock()
	defer s.wpMu.RUnlock()
	return s.wpEnabled
}

// SetWinProbEnabled updates the toggle and (when possible) persists it.
func (s *Server) SetWinProbEnabled(enabled bool) {
	s.wpMu.Lock()
	s.wpEnabled = enabled
	s.wpMu.Unlock()
	if s.config != nil {
		val := "0"
		if enabled {
			val = "1"
		}
		_ = s.config.SetConfig("win_prob_enabled", val)
	}
}

// loadTemplates parses the html/template files and registers the `static`
// helper used by the templates.
func (s *Server) loadTemplates() error {
	if s.TemplateDir == "" {
		return errors.New("template directory is empty")
	}
	funcs := template.FuncMap{
		"static": func(path string) string {
			return s.StaticPrefix + strings.TrimPrefix(path, "/")
		},
		"split": strings.Split,
	}
	tablePath := filepath.Join(s.TemplateDir, "table.html")
	holePath := filepath.Join(s.TemplateDir, "hole_cards.html")
	var err error
	s.renderTable, err = template.New(filepath.Base(tablePath)).Funcs(funcs).ParseFiles(tablePath)
	if err != nil {
		return fmt.Errorf("parse table template: %w", err)
	}
	s.renderHole, err = template.New(filepath.Base(holePath)).Funcs(funcs).ParseFiles(holePath)
	if err != nil {
		return fmt.Errorf("parse hole cards template: %w", err)
	}
	s.tmpl = s.renderTable
	return nil
}

// tableContext is the data passed to the table.html template.
type tableContext struct {
	TableID string
	Seats   []int
}

// holeCardsContext is the data passed to hole_cards.html.
type holeCardsContext struct {
	TableID   string
	Params    []string
	ParamsRaw string
}

// Routes returns an http.Handler with every endpoint registered. It mirrors
// poker/urls.py.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/hole-cards/", s.handleHoleCards)
	mux.HandleFunc("/api/start/", s.handleStart)
	mux.HandleFunc("/api/act/", s.handleAct)
	mux.HandleFunc("/api/state/", s.handleState)
	mux.HandleFunc("/api/sync/", s.handleSync)
	mux.HandleFunc("/api/qrcode/", s.handleQRCode)
	mux.HandleFunc("/api/advance/", s.handleAdvance)
	mux.HandleFunc("/api/debug/", s.handleDebug)
	mux.HandleFunc("/api/admin/wpmode", s.handleAdminWPMode)
	mux.HandleFunc("/api/game/next/", s.handleGameNext)
	mux.HandleFunc("/api/seats/join/", s.handleSeatsJoin)
	mux.HandleFunc("/api/seats/leave/", s.handleSeatsLeave)
	mux.Handle("/static/", http.StripPrefix(s.StaticPrefix, http.FileServer(http.Dir(s.StaticDir))))
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	tableID := r.URL.Query().Get("table_id")
	if tableID == "" {
		tableID = GenerateTableID()
	}
	seats := make([]int, 6)
	for i := range seats {
		seats[i] = i
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTable.Execute(w, tableContext{TableID: tableID, Seats: seats}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleHoleCards(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query().Get("params")
	tableID := r.URL.Query().Get("table_id")
	parts := strings.Split(params, "-")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderHole.Execute(w, holeCardsContext{
		TableID:   tableID,
		Params:    parts,
		ParamsRaw: params,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, out interface{}) error {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return errors.New("empty body")
	}
	return json.Unmarshal(body, out)
}

type startRequest struct {
	TableID string   `json:"table_id"`
	Players []string `json:"players"`
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req startRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.TableID == "" {
		req.TableID = GenerateTableID()
	}
	eng, _, err := s.State.GetOrCreate(req.TableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	eng.InitGame(req.Players)
	for _, p := range eng.Players {
		p.Stats = NewStats()
	}
	eng.StartHand()
	eng.saveState()
	state := eng.ToDict()
	s.enrichWinProb(state)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"table_id": req.TableID,
		"state":    state,
	})
}

type actRequest struct {
	TableID    string `json:"table_id"`
	PlayerName string `json:"player_name"`
	Action     string `json:"action"`
	Amount     int    `json:"amount"`
}

func (s *Server) handleAct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req actRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.TableID == "" || req.PlayerName == "" || req.Action == "" {
		writeError(w, http.StatusBadRequest, "Missing fields")
		return
	}
	eng, _, err := s.State.GetOrCreate(req.TableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := eng.LoadFromStorage(req.TableID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !eng.HumanAction(req.PlayerName, req.Action, req.Amount) {
		writeError(w, http.StatusBadRequest, "Invalid action")
		return
	}
	state := eng.ToDict()
	s.enrichWinProb(state)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"state": state,
	})
}

type advanceRequest struct {
	TableID string `json:"table_id"`
}

func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req advanceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.TableID == "" {
		writeError(w, http.StatusBadRequest, "Missing table_id")
		return
	}
	eng, _, err := s.State.GetOrCreate(req.TableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := eng.LoadFromStorage(req.TableID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	eng.AdvanceOneStep()
	state := eng.ToDict()
	s.enrichWinProb(state)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"state":   state,
		"version": eng.Game.Version,
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	tableID := r.URL.Query().Get("table_id")
	if tableID == "" {
		writeError(w, http.StatusBadRequest, "Missing table_id")
		return
	}
	sinceVersion, _ := strconv.Atoi(r.URL.Query().Get("since_version"))
	storage, ok := s.State.Storage.(*SQLiteStorage)
	if ok {
		g, _, err := storage.LoadGame(tableID)
		if err != nil {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		if g.Version <= sinceVersion {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		eng, _, err := s.State.GetOrCreate(tableID)
		if err == nil {
			_ = eng.LoadFromStorage(tableID)
			notifs := eng.Game.Notifications
			if len(notifs) > MaxNotifications {
				notifs = notifs[len(notifs)-MaxNotifications:]
			}
			state := eng.ToDict()
			s.enrichWinProb(state)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"version":       g.Version,
				"state":         state,
				"notifications": notifs,
			})
			return
		}
	} else {
		eng, ok := s.State.Engines()[tableID]
		if !ok || eng == nil || eng.Game == nil {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		if eng.Game.Version <= sinceVersion {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		notifs := eng.Game.Notifications
		if len(notifs) > MaxNotifications {
			notifs = notifs[len(notifs)-MaxNotifications:]
		}
		state := eng.ToDict()
		s.enrichWinProb(state)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"version":       eng.Game.Version,
			"state":         state,
			"notifications": notifs,
		})
	}
}

type syncRequest struct {
	TableID       string                 `json:"table_id"`
	State         map[string]interface{} `json:"state"`
	Notifications []string               `json:"notifications"`
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req syncRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.TableID == "" {
		req.TableID = "default"
	}
	if req.State == nil {
		writeError(w, http.StatusBadRequest, "Missing state")
		return
	}
	eng, _, err := s.State.GetOrCreate(req.TableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if community, ok := req.State["community_cards"]; ok {
		if cards, ok := community.([]interface{}); ok {
			out := make([]string, 0, len(cards))
			for _, c := range cards {
				out = append(out, fmt.Sprintf("%v", c))
			}
			eng.Game.CommunityCards = out
		}
	}
	if pot, ok := req.State["pot"].(float64); ok {
		eng.Game.Pot = int(pot)
	}
	if bet, ok := req.State["current_bet"].(float64); ok {
		eng.Game.CurrentBet = int(bet)
	}
	if phase, ok := req.State["phase"].(string); ok {
		eng.Game.Phase = phase
		for i, p := range Phases {
			if p == phase {
				eng.Game.PhaseIndex = i
			}
		}
	}
	eng.Game.Version++
	if len(req.Notifications) > 0 {
		eng.Game.Notifications = req.Notifications
	}
	eng.saveState()
	state := eng.ToDict()
	s.enrichWinProb(state)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"state": state,
	})
}

func (s *Server) handleQRCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	text := r.URL.Query().Get("text")
	if text == "" {
		writeError(w, http.StatusBadRequest, "Missing text")
		return
	}
	body, err := s.qrCodeRenderer.Render(text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write(body)
}

func (s *Server) handleDebug(w http.ResponseWriter, r *http.Request) {
	tableID := r.URL.Query().Get("table_id")
	eng, ok := s.State.Engines()[tableID]
	if !ok {
		writeError(w, http.StatusNotFound, "No engine")
		return
	}
	state := eng.ToDict()
	s.enrichWinProb(state)
	writeJSON(w, http.StatusOK, state)
}

// handleAdminWPMode toggles the win-probability display on or off.
//
//	GET  /api/admin/wpmode                -> {"enabled": true, "service_configured": true}
//	POST /api/admin/wpmode {"enabled": t/f} -> {"enabled": <new value>}
func (s *Server) handleAdminWPMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":            s.WinProbEnabled(),
			"service_configured": s.WinProb != nil,
			"service_url":        winProbURL(s),
		})
	case http.MethodPost:
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}
		if body.Enabled == nil {
			// Toggle if no explicit value.
			s.SetWinProbEnabled(!s.WinProbEnabled())
		} else {
			s.SetWinProbEnabled(*body.Enabled)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":            s.WinProbEnabled(),
			"service_configured": s.WinProb != nil,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST required")
	}
}

func winProbURL(s *Server) string {
	if s.WinProb == nil {
		return ""
	}
	return s.WinProb.BaseURL
}

// enrichWinProb annotates each player's win_probability in the supplied
// state dict if (a) the admin toggle is on and (b) a WinProbClient is
// configured. It does nothing otherwise.
func (s *Server) enrichWinProb(state map[string]interface{}) {
	if !s.WinProbEnabled() || s.WinProb == nil {
		return
	}
	players, ok := state["players"].([]map[string]interface{})
	if !ok {
		return
	}
	board := []string{}
	if c, ok := state["community_cards"].([]string); ok {
		board = c
	}
	for i, p := range players {
		folded, _ := p["folded"].(bool)
		if folded {
			continue
		}
		c1, c2, ok := extractCards(p["cards"])
		if !ok || c1 == "" || c2 == "" || c1 == "1B" || c2 == "1B" {
			continue
		}
		oppCount := 0
		for j, other := range players {
			if j == i {
				continue
			}
			of, _ := other["folded"].(bool)
			if !of {
				oppCount++
			}
		}
		if oppCount == 0 {
			continue
		}
		wp, err := s.WinProb.Evaluate([]string{c1, c2}, board, oppCount)
		if err != nil {
			log.Printf("enrichWinProb: evaluate err %v", err)
			continue
		}
		p["win_probability"] = wp
	}
}

// extractCards pulls two card codes out of the various shapes the cards
// field can take: a Go [2]string (produced by GameEngine.ToDict), a
// []interface{} of strings (after JSON round-tripping), or []string.
func extractCards(raw interface{}) (string, string, bool) {
	switch v := raw.(type) {
	case [2]string:
		return v[0], v[1], true
	case []string:
		if len(v) < 2 {
			return "", "", false
		}
		return v[0], v[1], true
	case []interface{}:
		if len(v) < 2 {
			return "", "", false
		}
		s1, ok1 := v[0].(string)
		s2, ok2 := v[1].(string)
		if !ok1 || !ok2 {
			return "", "", false
		}
		return s1, s2, true
	}
	return "", "", false
}

// EnsureDirs creates the static / template dirs if they don't exist.
func EnsureDirs(staticDir, templateDir string) error {
	for _, d := range []string{staticDir, templateDir} {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

type gameNextRequest struct {
	TableID string `json:"table_id"`
}

// handleGameNext starts the next hand for the supplied table. The engine
// must be in intermission (between hands); calling it while a hand is in
// progress returns 409.
func (s *Server) handleGameNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req gameNextRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.TableID == "" {
		writeError(w, http.StatusBadRequest, "Missing table_id")
		return
	}
	eng, _, err := s.State.GetOrCreate(req.TableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := eng.LoadFromStorage(req.TableID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !eng.Game.Intermission {
		writeError(w, http.StatusConflict, "Game is not in intermission")
		return
	}
	eng.Game.Intermission = false
	eng.Game.LastWinner = nil
	eng.Game.PotAwards = nil
	eng.NextHand()
	state := eng.ToDict()
	s.enrichWinProb(state)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"state": state,
	})
}

type seatsJoinRequest struct {
	TableID   string `json:"table_id"`
	Name      string `json:"name"`
	SeatIndex *int   `json:"seat_index,omitempty"`
}

// handleSeatsJoin adds (or re-activates) a player at the supplied seat
// during intermission. If seat_index is omitted the first free seat is
// used. Only allowed while intermission is true.
func (s *Server) handleSeatsJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req seatsJoinRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.TableID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "Missing fields")
		return
	}
	eng, _, err := s.State.GetOrCreate(req.TableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := eng.LoadFromStorage(req.TableID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := eng.AddOrRenameSeat(req.Name, req.SeatIndex); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	state := eng.ToDict()
	s.enrichWinProb(state)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"state": state,
	})
}

type seatsLeaveRequest struct {
	TableID   string `json:"table_id"`
	SeatIndex int    `json:"seat_index"`
}

// handleSeatsLeave removes the player at the supplied seat during
// intermission. Only allowed while intermission is true.
func (s *Server) handleSeatsLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req seatsLeaveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.TableID == "" {
		writeError(w, http.StatusBadRequest, "Missing fields")
		return
	}
	eng, _, err := s.State.GetOrCreate(req.TableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := eng.LoadFromStorage(req.TableID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := eng.RemoveSeat(req.SeatIndex); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	state := eng.ToDict()
	s.enrichWinProb(state)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"state": state,
	})
}