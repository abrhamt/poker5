package poker

import (
	"encoding/json"
	"net/http"
)

type Server struct {
	State              *GameState
	wpEnabled          bool
	wpServiceConfigured bool
}


func NewServer(state *GameState, staticDir, templateDir string) (*Server, error) {
	return &Server{State: state}, nil
}

func EnsureDirs(dirs ...string) error { return nil }

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	})
	mux.HandleFunc("/api/start/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TableID string   `json:"table_id"`
			Players []string `json:"players"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		eng, _, _ := s.State.GetOrCreate(req.TableID)
		eng.InitGame(req.Players)
		eng.StartHand()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    true,
			"state": eng.ToDict(),
		})
	})
	mux.HandleFunc("/api/act/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TableID    string `json:"table_id"`
			PlayerName string `json:"player_name"`
			Action     string `json:"action"`
			Amount     int    `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		eng, _, _ := s.State.GetOrCreate(req.TableID)
		ok := eng.HumanAction(req.PlayerName, req.Action, req.Amount)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    ok,
			"state": eng.ToDict(),
		})
	})
	mux.HandleFunc("/api/state/", func(w http.ResponseWriter, r *http.Request) {
		tableID := r.URL.Query().Get("table_id")
		if tableID == "" {
			tableID = "demo"
		}
		eng, _, _ := s.State.GetOrCreate(tableID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"version": eng.Game.Version,
			"state":   eng.ToDict(),
		})
	})
	mux.HandleFunc("/api/qrcode/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg></svg>`))
	})
	mux.HandleFunc("/api/admin/wpmode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				Enabled *bool `json:"enabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Enabled != nil {
				s.wpEnabled = *body.Enabled
			} else {
				s.wpEnabled = !s.wpEnabled
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":            s.wpEnabled,
			"service_configured": s.wpServiceConfigured,
		})
	})
	return mux
}

func (s *Server) SetWinProbClient(addr string, iterations int) {
	s.wpServiceConfigured = true
}

func (s *Server) WinProbEnabled() bool { return s.wpEnabled }