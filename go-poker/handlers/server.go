package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v3"
	poker "github.com/zuse/poker5/go-poker"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
	"github.com/zuse/poker5/go-poker/utilities"
)

type FiberServer struct {
	App         *fiber.App
	DB          *sql.DB
	Queries     *repository.Queries
	AuthService *services.AuthService
	Wallet      *services.WalletService
	RoomService *services.RoomService
	GameService *services.GameService
	SSE         *services.SSEHub
}

func NewFiberServer(db *sql.DB) *FiberServer {
	_ = repository.InitDBSchema(db)
	_ = utilities.LoadDotEnv(".env")

	app := fiber.New(fiber.Config{
		AppName: "Golden Poker Platform",
	})

	q := repository.New(db)

	authSvc := services.NewAuthService(q)
	walletSvc := services.NewWalletService(q)
	sseHub := services.NewSSEHub()
	roomSvc := services.NewRoomService(q)
	settingsSvc := services.NewSettingsService(q)
	adminSvc := services.NewAdminService(q, walletSvc)

	// Persist live table/player state in the same SQLite database as
	// everything else, so a server restart doesn't strand seated players'
	// chips (their wallet was already debited on buy-in with nowhere to
	// recover it from if the in-memory table simply vanished).
	var gameStorage poker.Storage
	if sqliteStorage, err := poker.NewSQLiteStorageFromDB(db); err != nil {
		log.Printf("game storage: falling back to in-memory (state won't survive a restart): %v", err)
		gameStorage = poker.NewMemoryStorage()
	} else {
		gameStorage = sqliteStorage
	}
	gameSvc := services.NewGameService(walletSvc, sseHub, settingsSvc, gameStorage, roomSvc)

	bootstrap(context.Background(), authSvc, walletSvc)

	secureCookies := secureCookiesEnabled()
	if !secureCookies {
		log.Printf("warning: session cookies are being sent WITHOUT the Secure flag — " +
			"set SESSION_COOKIE_SECURE=true once the site is served over HTTPS")
	}

	authHandler := NewAuthHandler(authSvc, secureCookies)
	walletHandler := NewWalletHandler(walletSvc, authSvc)
	sseHandler := NewSSEHandler(sseHub)
	roomHandler := NewRoomHandler(roomSvc, authSvc, gameSvc)
	gameHandler := NewGameHandler(gameSvc, authSvc, roomSvc)
	adminHandler := NewAdminHandler(adminSvc, settingsSvc, authSvc)

	s := &FiberServer{
		App:         app,
		DB:          db,
		Queries:     q,
		AuthService: authSvc,
		Wallet:      walletSvc,
		RoomService: roomSvc,
		GameService: gameSvc,
		SSE:         sseHub,
	}

	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "go-poker"})
	})

	// Every player-facing page is rendered by the React app in frontend/;
	// only the admin dashboard is still server-rendered HTML.
	admin := app.Group("/admin", adminHandler.RequireAdmin)
	admin.Get("/", adminHandler.RenderDashboard)

	api := app.Group("/api")

	auth := api.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/logout", authHandler.Logout)
	auth.Get("/me", authHandler.Me)

	rooms := api.Group("/rooms")
	rooms.Post("/create-private", roomHandler.CreatePrivate)
	rooms.Post("/quick-join", roomHandler.QuickJoin)
	rooms.Post("/join-code", roomHandler.JoinByCode)
	rooms.Get("/public", roomHandler.ListPublic)

	table := api.Group("/table")
	table.Post("/:id/join", gameHandler.Join)
	table.Post("/:id/leave", gameHandler.Leave)
	table.Post("/:id/act", gameHandler.Act)
	table.Post("/:id/start", gameHandler.Start)
	table.Post("/:id/sit-in", gameHandler.SitIn)
	table.Get("/:id/state", gameHandler.GetState)

	wallet := api.Group("/wallet")
	wallet.Post("/deposit", walletHandler.Deposit)
	wallet.Get("/transactions", walletHandler.GetTransactions)

	apiAdmin := api.Group("/admin", adminHandler.RequireAdmin)
	apiAdmin.Post("/settings", adminHandler.UpdateSettings)
	apiAdmin.Get("/users", adminHandler.SearchUsers)
	apiAdmin.Get("/transactions", adminHandler.SearchTransactions)

	api.Get("/events", sseHandler.HandleEvents)

	// Anything unrouted is a client mistake, not a page: this server is a JSON
	// API and serves no files. The React app (and, later, the admin app) are
	// static bundles served by the web server in front of it.
	app.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	})

	return s
}

// bootstrap ensures the house treasury and admin accounts exist. Both are
// idempotent and safe to run on every startup.
func bootstrap(ctx context.Context, authSvc *services.AuthService, walletSvc *services.WalletService) {
	if _, err := walletSvc.EnsureHouseAccount(ctx); err != nil {
		log.Printf("warning: failed to ensure house account: %v", err)
	}

	adminUsername := envOrDefault("ADMIN_USERNAME", "admin")
	adminPhone := envOrDefault("ADMIN_PHONE", "0900000000")
	adminPassword := envOrDefault("ADMIN_PASSWORD", "")
	if adminPassword == "" {
		log.Printf("warning: ADMIN_PASSWORD not set; skipping admin account seeding")
		return
	}
	if _, err := authSvc.EnsureAdminUser(ctx, adminUsername, adminPhone, adminPassword); err != nil {
		log.Printf("warning: failed to ensure admin account: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (s *FiberServer) Listen(addr string) error {
	fmt.Printf("Fiber v3 poker server listening on %s\n", addr)
	return s.App.Listen(addr)
}
