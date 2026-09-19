package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

const (
	MinGatewayDeposit int64 = 10
	MaxGatewayDeposit int64 = 100_000

	GatewayDepositStatusPending   = "pending"
	GatewayDepositStatusCredited  = "credited"
	GatewayDepositStatusFailed    = "failed"
	GatewayDepositStatusCancelled = "cancelled"
	GatewayDepositStatusExpired   = "expired"

	RouterEventSucceeded = "payment.succeeded"
	RouterEventFailed    = "payment.failed"
	RouterEventCancelled = "payment.cancelled"
	RouterEventExpired   = "payment.expired"
	RouterEventTest      = "payment.test"

	gatewayCheckoutExpiry  = 3600
	GatewayReconcileAfter  = 70 * time.Minute
	GatewayReconcileEvery  = 10 * time.Minute
	gatewayReconcileBatch  = 20
	gatewayReferencePrefix = "GP"
)

var (
	ErrGatewayDepositsDisabled = errors.New("automatic deposits are not available right now")
	ErrGatewayAmountInvalid    = fmt.Errorf("enter a whole amount between %d and %d ETB", MinGatewayDeposit, MaxGatewayDeposit)
	ErrGatewayDepositNotFound  = errors.New("that deposit was not found")
)

type GatewayCheckout struct {
	Reference   string `json:"reference"`
	CheckoutURL string `json:"checkout_url"`
	Amount      int64  `json:"amount"`
}

type GatewayDepositStatus struct {
	Reference     string `json:"reference"`
	Status        string `json:"status"`
	Amount        int64  `json:"amount"`
	TransactionID string `json:"transaction_id,omitempty"`
}

type RouterEventData struct {
	ID        string `json:"id"`
	Reference string `json:"reference"`
	Status    string `json:"status"`
	Amount    string `json:"amount"`
}

type GatewayDepositService struct {
	q            *repository.Queries
	db           *sql.DB
	wallet       *WalletService
	settings     *SettingsService
	router       PaymentRouter
	publicAppURL string
}

func NewGatewayDepositService(q *repository.Queries, db *sql.DB, wallet *WalletService, settings *SettingsService, router PaymentRouter, publicAppURL string) *GatewayDepositService {
	return &GatewayDepositService{
		q:            q,
		db:           db,
		wallet:       wallet,
		settings:     settings,
		router:       router,
		publicAppURL: strings.TrimRight(strings.TrimSpace(publicAppURL), "/"),
	}
}

func (s *GatewayDepositService) Configured() bool {
	return s.router != nil && s.router.Configured() && s.publicAppURL != ""
}

func (s *GatewayDepositService) WebhookSecret() string {
	if s.router == nil {
		return ""
	}
	return s.router.WebhookSecret()
}

func (s *GatewayDepositService) Enabled(ctx context.Context) (bool, error) {
	if !s.Configured() {
		return false, nil
	}
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return false, err
	}
	return settings.GatewayDepositsEnabled, nil
}

func (s *GatewayDepositService) Start(ctx context.Context, user repository.User, amount int64) (*GatewayCheckout, error) {
	if amount < MinGatewayDeposit || amount > MaxGatewayDeposit {
		return nil, ErrGatewayAmountInvalid
	}
	enabled, err := s.Enabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, ErrGatewayDepositsDisabled
	}

	suffix, err := utilities.GenerateTxID()
	if err != nil {
		return nil, err
	}
	reference := gatewayReferencePrefix + suffix

	depositID, err := s.q.CreateGatewayDeposit(ctx, repository.CreateGatewayDepositParams{
		UserID:    user.ID,
		Reference: reference,
		Amount:    amount,
		Status:    GatewayDepositStatusPending,
	})
	if err != nil {
		return nil, err
	}

	payment, err := s.router.CreatePayment(ctx, RouterCreatePayment{
		Reference:   reference,
		AmountETB:   amount,
		ReturnURL:   s.publicAppURL + "/wallet?deposit=" + reference,
		Description: "Golden Poker wallet deposit",
		Phone:       user.PhoneNumber,
		Name:        user.Username,
		ExpiresIn:   gatewayCheckoutExpiry,
		Metadata:    map[string]any{"user_id": user.ID, "username": user.Username},
	})
	if err != nil {
		note := clip(err.Error(), 255)
		_ = s.q.ResolveGatewayDeposit(ctx, repository.ResolveGatewayDepositParams{
			Status:     GatewayDepositStatusFailed,
			LastEvent:  "create_failed",
			Note:       note,
			ResolvedAt: time.Now().Unix(),
			ID:         depositID,
		})
		return nil, err
	}

	if err := s.q.AttachGatewayCheckout(ctx, repository.AttachGatewayCheckoutParams{
		RouterPaymentID: payment.ID,
		CheckoutUrl:     clip(payment.CheckoutURL, 1024),
		ID:              depositID,
	}); err != nil {
		return nil, err
	}

	log.Printf("gateway deposit started reference=%s user=%d amount=%d payment=%s", reference, user.ID, amount, payment.ID)
	return &GatewayCheckout{Reference: reference, CheckoutURL: payment.CheckoutURL, Amount: amount}, nil
}

func (s *GatewayDepositService) Status(ctx context.Context, userID int64, reference string) (*GatewayDepositStatus, error) {
	row, err := s.q.GetGatewayDepositByReference(ctx, strings.TrimSpace(reference))
	if err != nil || row.UserID != userID {
		return nil, ErrGatewayDepositNotFound
	}
	return &GatewayDepositStatus{
		Reference:     row.Reference,
		Status:        row.Status,
		Amount:        row.Amount,
		TransactionID: row.TransactionID.String,
	}, nil
}

func (s *GatewayDepositService) ApplyEvent(ctx context.Context, event string, data RouterEventData) error {
	reference := strings.TrimSpace(data.Reference)
	if reference == "" {
		return nil
	}
	target, known := gatewayStatusForEvent(event)
	if !known {
		log.Printf("gateway deposit: ignoring event %q for %s", event, reference)
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	row, err := qtx.GetGatewayDepositForUpdate(ctx, reference)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("gateway deposit: no row for reference %s (event %s)", reference, event)
			return nil
		}
		return err
	}

	if row.Status == GatewayDepositStatusCredited {
		return nil
	}
	if target != GatewayDepositStatusCredited && row.Status != GatewayDepositStatusPending {
		return nil
	}

	resolve := repository.ResolveGatewayDepositParams{
		Status:     target,
		LastEvent:  clip(event, 32),
		ResolvedAt: time.Now().Unix(),
		ID:         row.ID,
	}

	if target == GatewayDepositStatusCredited {
		if reported, ok := parseWholeETB(data.Amount); ok && reported != row.Amount {
			log.Printf("gateway deposit: amount mismatch reference=%s stored=%d reported=%d; not credited", reference, row.Amount, reported)
			return nil
		}
		walletTx, err := s.wallet.DepositWith(ctx, qtx, row.UserID, row.Amount, "gateway "+reference)
		if err != nil {
			return err
		}
		resolve.TransactionID = sql.NullString{String: walletTx.TransactionID, Valid: true}
		if row.Status != GatewayDepositStatusPending {
			resolve.Note = "credited after " + row.Status
		}
	}

	if err := qtx.ResolveGatewayDeposit(ctx, resolve); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("gateway deposit %s -> %s (event %s, payment %s)", reference, target, event, data.ID)
	return nil
}

func gatewayStatusForEvent(event string) (string, bool) {
	switch event {
	case RouterEventSucceeded:
		return GatewayDepositStatusCredited, true
	case RouterEventFailed:
		return GatewayDepositStatusFailed, true
	case RouterEventCancelled:
		return GatewayDepositStatusCancelled, true
	case RouterEventExpired:
		return GatewayDepositStatusExpired, true
	default:
		return "", false
	}
}

func parseWholeETB(amount string) (int64, bool) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return 0, false
	}
	whole, fraction, _ := strings.Cut(amount, ".")
	if strings.Trim(fraction, "0") != "" {
		return 0, false
	}
	value, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
