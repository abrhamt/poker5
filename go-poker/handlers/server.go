package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	poker "github.com/zuse/poker5/go-poker"
	dbschema "github.com/zuse/poker5/go-poker/db"
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

	// stopBackground ends the deposit queue worker. Tests call it through
	// Stop; a running server keeps it for the life of the process.
	stopBackground func()
}

// Stop ends the server's background work. Listening is unaffected — this is
// about the goroutines started at construction.
func (s *FiberServer) Stop() {
	if s.stopBackground != nil {
		s.stopBackground()
	}
}

// ServerOption customizes construction. It exists mainly so tests can inject a
// recording OTP sender: the throttle and attempt rules are assertions about how
// many messages went out, which nothing can observe from the database alone.
type ServerOption func(*serverConfig)

type serverConfig struct {
	otpSender       services.OTPSender
	receiptVerifier services.ReceiptVerifier
}

// WithOTPSender replaces the console sender.
func WithOTPSender(sender services.OTPSender) ServerOption {
	return func(cfg *serverConfig) { cfg.otpSender = sender }
}

// WithReceiptVerifier replaces the HTTP receipt verifier, so deposit rules can
// be tested without calling a third party.
func WithReceiptVerifier(verifier services.ReceiptVerifier) ServerOption {
	return func(cfg *serverConfig) { cfg.receiptVerifier = verifier }
}

func NewFiberServer(db *sql.DB, opts ...ServerOption) *FiberServer {
	_ = utilities.LoadDotEnv(".env")

	// A server that cannot create its schema cannot serve a single request, so
	// this is fatal rather than logged and stepped over: failing at start-up is
	// the only way the operator finds out before the players do.
	if err := dbschema.Apply(context.Background(), db); err != nil {
		log.Fatalf("database schema: %v", err)
	}

	cfg := &serverConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	// GEEZSMS_TOKEN is what decides whether codes actually leave the machine.
	// Its absence falls back to the console sender, which prints in
	// development and refuses to send anywhere else.
	otpSender := cfg.otpSender
	if otpSender == nil {
		if geez := services.NewGeezSMSSender(); geez != nil {
			log.Printf("OTP sender: GeezSMS")
			otpSender = geez
		} else {
			otpSender = services.NewConsoleSender()
		}
	}

	app := fiber.New(fiber.Config{
		AppName: "Golden Poker Platform",
	})

	q := repository.New(db)

	authSvc := services.NewAuthService(q, db, otpSender)
	walletSvc := services.NewWalletService(q)
	sseHub := services.NewSSEHub()
	roomSvc := services.NewRoomService(q)
	settingsSvc := services.NewSettingsService(q)
	adminSvc := services.NewAdminService(q, walletSvc)

	verifier := cfg.receiptVerifier
	if verifier == nil {
		verifier = services.NewHTTPReceiptVerifier()
	}
	depositSvc := services.NewDepositService(q, db, walletSvc, settingsSvc, verifier)
	// Receipts queued during a verifier outage are re-checked on their own, so
	// the admin queue only holds what genuinely needs a person.
	stopSweeper := depositSvc.StartQueueWorker(context.Background(), depositSweepInterval())

	// Persist live table/player state in the same database as everything else,
	// so a server restart doesn't strand seated players' chips (their wallet
	// was already debited on buy-in with nowhere to recover it from if the
	// in-memory table simply vanished).
	var gameStorage poker.Storage
	if pgStorage, err := poker.NewPostgresStorageFromDB(db); err != nil {
		log.Printf("game storage: falling back to in-memory (state won't survive a restart): %v", err)
		gameStorage = poker.NewMemoryStorage()
	} else {
		gameStorage = pgStorage
	}
	gameSvc := services.NewGameService(walletSvc, sseHub, settingsSvc, gameStorage, roomSvc)

	bootstrap(context.Background(), authSvc, walletSvc)

	secureCookies := secureCookiesEnabled()
	if !secureCookies {
		log.Printf("warning: session cookies are being sent WITHOUT the Secure flag — " +
			"set SESSION_COOKIE_SECURE=true once the site is served over HTTPS")
	}

	authHandler := NewAuthHandler(authSvc, secureCookies)
	walletHandler := NewWalletHandler(walletSvc, authSvc, depositSvc)
	sseHandler := NewSSEHandler(sseHub)
	roomHandler := NewRoomHandler(roomSvc, authSvc, gameSvc)
	gameHandler := NewGameHandler(gameSvc, authSvc, roomSvc)
	adminHandler := NewAdminHandler(adminSvc, settingsSvc, authSvc, depositSvc)

	s := &FiberServer{
		stopBackground: stopSweeper,

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
	auth.Post("/verify-otp", authHandler.VerifyOTP)
	auth.Post("/resend-otp", authHandler.ResendOTP)
	auth.Post("/forgot-password", authHandler.ForgotPassword)
	auth.Post("/verify-reset-otp", authHandler.VerifyResetOTP)
	auth.Post("/reset-password", authHandler.ResetPassword)
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
	wallet.Get("/deposit-info", walletHandler.DepositInfo)
	wallet.Post("/deposit/receipt", walletHandler.SubmitReceipt)
	wallet.Get("/transactions", walletHandler.GetTransactions)

	apiAdmin := api.Group("/admin", adminHandler.RequireAdmin)
	apiAdmin.Post("/settings", adminHandler.UpdateSettings)
	apiAdmin.Get("/users", adminHandler.SearchUsers)
	apiAdmin.Get("/transactions", adminHandler.SearchTransactions)
	apiAdmin.Get("/deposits", adminHandler.ListPendingDeposits)
	apiAdmin.Post("/deposits/:id/approve", adminHandler.ApproveDeposit)
	apiAdmin.Post("/deposits/:id/reject", adminHandler.RejectDeposit)
	apiAdmin.Post("/deposits/:id/credit", adminHandler.CreditDeposit)

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

// depositSweepInterval reads DEPOSIT_SWEEP_INTERVAL, a Go duration such as
// "90s" or "5m". An unparseable value falls back rather than failing start-up:
// a typo here should not take the site down.
func depositSweepInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("DEPOSIT_SWEEP_INTERVAL"))
	if raw == "" {
		return services.QueueSweepInterval
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		log.Printf("warning: DEPOSIT_SWEEP_INTERVAL=%q is not a valid duration; using %s", raw, services.QueueSweepInterval)
		return services.QueueSweepInterval
	}
	return interval
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
