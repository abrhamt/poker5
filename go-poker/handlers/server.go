package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

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

	authHandler := NewAuthHandler(authSvc)
	walletHandler := NewWalletHandler(walletSvc, authSvc)
	sseHandler := NewSSEHandler(sseHub)
	roomHandler := NewRoomHandler(roomSvc, authSvc)
	gameHandler := NewGameHandler(gameSvc, authSvc, roomSvc)
	viewHandler := NewViewHandler(authSvc, walletSvc, roomSvc, gameSvc)
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

	app.Get("/", viewHandler.RenderHome)
	app.Get("/login", viewHandler.RenderLogin)
	app.Get("/register", viewHandler.RenderRegister)
	app.Get("/lobby", viewHandler.RenderLobby)
	app.Get("/lobby/rooms", viewHandler.RenderLobbyRooms)
	app.Get("/wallet", viewHandler.RenderWallet)
	app.Get("/table/:id", viewHandler.RenderTable)

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
	table.Get("/:id/state", gameHandler.GetState)

	wallet := api.Group("/wallet")
	wallet.Post("/deposit", walletHandler.Deposit)
	wallet.Get("/transactions", walletHandler.GetTransactions)

	apiAdmin := api.Group("/admin", adminHandler.RequireAdmin)
	apiAdmin.Post("/settings", adminHandler.UpdateSettings)
	apiAdmin.Get("/users", adminHandler.SearchUsers)
	apiAdmin.Get("/transactions", adminHandler.SearchTransactions)

	api.Get("/events", sseHandler.HandleEvents)

	staticDir, _ := filepath.Abs("static")
	app.Get("/static/*", func(c fiber.Ctx) error {
		return c.SendFile(filepath.Join(staticDir, c.Params("*")))
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
