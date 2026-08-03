package poker

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/gofiber/fiber/v3"

	"github.com/zuse/poker5/go-poker/handlers"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
)

type FiberServer struct {
	App         *fiber.App
	DB          *sql.DB
	Queries     *repository.Queries
	AuthService *services.AuthService
	Wallet      *services.WalletService
	SSE         *services.SSEHub
}

func NewFiberServer(db *sql.DB) *FiberServer {
	app := fiber.New(fiber.Config{
		AppName: "Go Poker API",
	})

	q := repository.New(db)
	authSvc := services.NewAuthService(q)
	walletSvc := services.NewWalletService(q)
	sseHub := services.NewSSEHub()

	authHandler := handlers.NewAuthHandler(authSvc)
	walletHandler := handlers.NewWalletHandler(walletSvc, authSvc)
	sseHandler := handlers.NewSSEHandler(sseHub)
	gameHandler := handlers.NewGameHandler(sseHub)

	s := &FiberServer{
		App:         app,
		DB:          db,
		Queries:     q,
		AuthService: authSvc,
		Wallet:      walletSvc,
		SSE:         sseHub,
	}

	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "go-poker"})
	})

	api := app.Group("/api")

	auth := api.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/logout", authHandler.Logout)
	auth.Get("/me", authHandler.Me)

	wallet := api.Group("/wallet")
	wallet.Post("/deposit", walletHandler.Deposit)
	wallet.Get("/transactions", walletHandler.GetTransactions)

	api.Get("/events", sseHandler.HandleEvents)
	api.Get("/state", gameHandler.GetState)
	api.Post("/action", gameHandler.Action)

	staticDir, _ := filepath.Abs("static")
	app.Get("/static/*", func(c fiber.Ctx) error {
		return c.SendFile(filepath.Join(staticDir, c.Params("*")))
	})

	return s
}

func (s *FiberServer) Listen(addr string) error {
	fmt.Printf("Fiber v3 poker server listening on %s\n", addr)
	return s.App.Listen(addr)
}

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