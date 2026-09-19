package services

import (
	"context"
	"errors"
	"testing"

	"github.com/zuse/poker5/go-poker/internal/testdb"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

type fakeRouter struct {
	configured bool
	created    []RouterCreatePayment
	createErr  error
	fetched    map[string]*RouterPayment
	fetchErr   error
}

func (f *fakeRouter) Configured() bool      { return f.configured }
func (f *fakeRouter) WebhookSecret() string { return "whsec_fake" }

func (f *fakeRouter) CreatePayment(_ context.Context, req RouterCreatePayment) (*RouterPayment, error) {
	f.created = append(f.created, req)
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &RouterPayment{
		ID:          "pay_" + req.Reference,
		Status:      "pending",
		CheckoutURL: "https://checkout.test/" + req.Reference,
		Reference:   req.Reference,
	}, nil
}

func (f *fakeRouter) FetchPayment(_ context.Context, id string) (*RouterPayment, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	payment, ok := f.fetched[id]
	if !ok {
		return nil, &PaymentRouterError{Status: 404, Code: "not_found"}
	}
	return payment, nil
}

func newGatewayFixture(t *testing.T, router *fakeRouter, enabled bool) (*GatewayDepositService, *repository.Queries, repository.User) {
	t.Helper()

	db := testdb.New(t)
	q := repository.New(db)
	ctx := context.Background()

	passHash, _ := utilities.HashPassword("password123")
	userID, err := q.CreateUser(ctx, repository.CreateUserParams{
		Username:     "alice",
		PhoneNumber:  "+251911223344",
		PasswordHash: passHash,
		ReferralCode: "ABC123",
		Role:         "player",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}

	settings := NewSettingsService(q)
	if err := settings.Update(ctx, UpdateSettingsRequest{
		RakeMode:               RakeModePercentage,
		RakePercentage:         5,
		CountdownSeconds:       15,
		GatewayDepositsEnabled: enabled,
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	svc := NewGatewayDepositService(q, db, NewWalletService(q), settings, router, "https://poker.test/")
	return svc, q, user
}

func TestGatewayStartCreatesPendingRowAndCheckout(t *testing.T) {
	router := &fakeRouter{configured: true}
	svc, q, user := newGatewayFixture(t, router, true)
	ctx := context.Background()

	checkout, err := svc.Start(ctx, user, 150)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if checkout.CheckoutURL != "https://checkout.test/"+checkout.Reference || checkout.Amount != 150 {
		t.Fatalf("unexpected checkout %+v", checkout)
	}

	row, err := q.GetGatewayDepositByReference(ctx, checkout.Reference)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.Status != GatewayDepositStatusPending || row.RouterPaymentID != "pay_"+checkout.Reference || row.Amount != 150 {
		t.Fatalf("unexpected row %+v", row)
	}

	req := router.created[0]
	if req.ReturnURL != "https://poker.test/wallet?deposit="+checkout.Reference {
		t.Fatalf("unexpected return url %s", req.ReturnURL)
	}
	if req.Phone != "+251911223344" || req.Name != "alice" || req.AmountETB != 150 {
		t.Fatalf("unexpected create request %+v", req)
	}
	if walletBalance(t, q, user.ID) != 0 {
		t.Fatal("wallet was credited before the payment completed")
	}
}

func TestGatewayStartRejectsBadAmountsAndDisabledToggle(t *testing.T) {
	router := &fakeRouter{configured: true}
	svc, _, user := newGatewayFixture(t, router, false)
	ctx := context.Background()

	for _, amount := range []int64{0, 9, MaxGatewayDeposit + 1} {
		if _, err := svc.Start(ctx, user, amount); !errors.Is(err, ErrGatewayAmountInvalid) {
			t.Fatalf("amount %d: expected ErrGatewayAmountInvalid, got %v", amount, err)
		}
	}
	if _, err := svc.Start(ctx, user, 50); !errors.Is(err, ErrGatewayDepositsDisabled) {
		t.Fatalf("expected ErrGatewayDepositsDisabled, got %v", err)
	}
	if len(router.created) != 0 {
		t.Fatal("router was called for a refused deposit")
	}
}

func TestGatewayStartWithUnconfiguredRouterIsDisabled(t *testing.T) {
	svc, _, user := newGatewayFixture(t, &fakeRouter{configured: false}, true)
	if _, err := svc.Start(context.Background(), user, 50); !errors.Is(err, ErrGatewayDepositsDisabled) {
		t.Fatalf("expected ErrGatewayDepositsDisabled, got %v", err)
	}
}

func TestGatewayStartMarksRowFailedWhenRouterRefuses(t *testing.T) {
	router := &fakeRouter{configured: true, createErr: &PaymentRouterError{Status: 502, Code: "gateway_error", Message: "down"}}
	svc, q, user := newGatewayFixture(t, router, true)
	ctx := context.Background()

	_, err := svc.Start(ctx, user, 50)
	var routerErr *PaymentRouterError
	if !errors.As(err, &routerErr) {
		t.Fatalf("expected router error, got %v", err)
	}

	rows, err := q.ListGatewayDepositsByUser(ctx, repository.ListGatewayDepositsByUserParams{UserID: user.ID, Limit: 5})
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != GatewayDepositStatusFailed || rows[0].LastEvent != "create_failed" {
		t.Fatalf("unexpected rows %+v", rows)
	}
}

func TestGatewaySucceededEventCreditsOnce(t *testing.T) {
	router := &fakeRouter{configured: true}
	svc, q, user := newGatewayFixture(t, router, true)
	ctx := context.Background()

	checkout, err := svc.Start(ctx, user, 150)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	data := RouterEventData{ID: "pay_x", Reference: checkout.Reference, Status: "succeeded", Amount: "150.00"}

	for range 2 {
		if err := svc.ApplyEvent(ctx, RouterEventSucceeded, data); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}

	if got := walletBalance(t, q, user.ID); got != 150 {
		t.Fatalf("wallet = %d, want 150 after one credit", got)
	}
	status, err := svc.Status(ctx, user.ID, checkout.Reference)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Status != GatewayDepositStatusCredited || status.TransactionID == "" {
		t.Fatalf("unexpected status %+v", status)
	}
	txns, err := q.ListTransactionsByUserID(ctx, repository.ListTransactionsByUserIDParams{UserID: user.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(txns) != 1 || txns[0].Type != TxTypeDeposit || txns[0].Amount != 150 {
		t.Fatalf("unexpected ledger %+v", txns)
	}
}

func TestGatewayFailureEventsCloseWithoutCredit(t *testing.T) {
	for _, event := range []string{RouterEventFailed, RouterEventCancelled, RouterEventExpired} {
		t.Run(event, func(t *testing.T) {
			router := &fakeRouter{configured: true}
			svc, q, user := newGatewayFixture(t, router, true)
			ctx := context.Background()

			checkout, err := svc.Start(ctx, user, 20)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			if err := svc.ApplyEvent(ctx, event, RouterEventData{Reference: checkout.Reference}); err != nil {
				t.Fatalf("apply: %v", err)
			}

			status, _ := svc.Status(ctx, user.ID, checkout.Reference)
			want, _ := gatewayStatusForEvent(event)
			if status.Status != want {
				t.Fatalf("status = %s, want %s", status.Status, want)
			}
			if walletBalance(t, q, user.ID) != 0 {
				t.Fatal("wallet was credited on a failure event")
			}

			if err := svc.ApplyEvent(ctx, RouterEventFailed, RouterEventData{Reference: checkout.Reference}); err != nil {
				t.Fatalf("second apply: %v", err)
			}
			status, _ = svc.Status(ctx, user.ID, checkout.Reference)
			if status.Status != want {
				t.Fatalf("terminal row was overwritten to %s", status.Status)
			}
		})
	}
}

func TestGatewayLateSuccessAfterExpiryStillCredits(t *testing.T) {
	router := &fakeRouter{configured: true}
	svc, q, user := newGatewayFixture(t, router, true)
	ctx := context.Background()

	checkout, err := svc.Start(ctx, user, 40)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := svc.ApplyEvent(ctx, RouterEventExpired, RouterEventData{Reference: checkout.Reference}); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if err := svc.ApplyEvent(ctx, RouterEventSucceeded, RouterEventData{Reference: checkout.Reference, Amount: "40.00"}); err != nil {
		t.Fatalf("late success: %v", err)
	}

	if got := walletBalance(t, q, user.ID); got != 40 {
		t.Fatalf("wallet = %d, want 40", got)
	}
	row, _ := q.GetGatewayDepositByReference(ctx, checkout.Reference)
	if row.Status != GatewayDepositStatusCredited || row.Note != "credited after expired" {
		t.Fatalf("unexpected row %+v", row)
	}
}

func TestGatewayAmountMismatchIsNotCredited(t *testing.T) {
	router := &fakeRouter{configured: true}
	svc, q, user := newGatewayFixture(t, router, true)
	ctx := context.Background()

	checkout, err := svc.Start(ctx, user, 40)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := svc.ApplyEvent(ctx, RouterEventSucceeded, RouterEventData{Reference: checkout.Reference, Amount: "400.00"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if walletBalance(t, q, user.ID) != 0 {
		t.Fatal("mismatched amount was credited")
	}
	status, _ := svc.Status(ctx, user.ID, checkout.Reference)
	if status.Status != GatewayDepositStatusPending {
		t.Fatalf("status = %s, want pending", status.Status)
	}
}

func TestGatewayUnknownReferenceAndEventAreIgnored(t *testing.T) {
	svc, _, _ := newGatewayFixture(t, &fakeRouter{configured: true}, true)
	ctx := context.Background()

	if err := svc.ApplyEvent(ctx, RouterEventSucceeded, RouterEventData{Reference: "GPUNKNOWN"}); err != nil {
		t.Fatalf("unknown reference: %v", err)
	}
	if err := svc.ApplyEvent(ctx, "payment.something", RouterEventData{Reference: "GPUNKNOWN"}); err != nil {
		t.Fatalf("unknown event: %v", err)
	}
	if err := svc.ApplyEvent(ctx, RouterEventSucceeded, RouterEventData{}); err != nil {
		t.Fatalf("empty reference: %v", err)
	}
}

func TestGatewayStatusIsScopedToOwner(t *testing.T) {
	svc, _, user := newGatewayFixture(t, &fakeRouter{configured: true}, true)
	ctx := context.Background()

	checkout, err := svc.Start(ctx, user, 20)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := svc.Status(ctx, user.ID+1, checkout.Reference); !errors.Is(err, ErrGatewayDepositNotFound) {
		t.Fatalf("expected ErrGatewayDepositNotFound, got %v", err)
	}
}

func TestGatewayReconcileAppliesRouterStatus(t *testing.T) {
	router := &fakeRouter{configured: true, fetched: map[string]*RouterPayment{}}
	svc, q, user := newGatewayFixture(t, router, true)
	ctx := context.Background()

	paid, err := svc.Start(ctx, user, 30)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	dead, err := svc.Start(ctx, user, 10)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	router.fetched["pay_"+paid.Reference] = &RouterPayment{ID: "pay_" + paid.Reference, Status: "succeeded", Amount: "30.00"}
	router.fetched["pay_"+dead.Reference] = &RouterPayment{ID: "pay_" + dead.Reference, Status: "expired"}

	if _, err := svc.db.ExecContext(ctx, "UPDATE gateway_deposits SET created_at = NOW() - INTERVAL '2 hours'"); err != nil {
		t.Fatalf("age rows: %v", err)
	}

	result := svc.RunReconcile(ctx)
	if result.Credited != 1 || result.Closed != 1 || result.Errors != 0 {
		t.Fatalf("unexpected result %+v", result)
	}
	if walletBalance(t, q, user.ID) != 30 {
		t.Fatalf("wallet = %d, want 30", walletBalance(t, q, user.ID))
	}
	status, _ := svc.Status(ctx, user.ID, dead.Reference)
	if status.Status != GatewayDepositStatusExpired {
		t.Fatalf("expired row status = %s", status.Status)
	}
}

func TestGatewayReconcileSkipsFreshRows(t *testing.T) {
	router := &fakeRouter{configured: true, fetched: map[string]*RouterPayment{}}
	svc, _, user := newGatewayFixture(t, router, true)
	ctx := context.Background()

	checkout, err := svc.Start(ctx, user, 30)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	router.fetched["pay_"+checkout.Reference] = &RouterPayment{ID: "pay_" + checkout.Reference, Status: "succeeded", Amount: "30.00"}

	result := svc.RunReconcile(ctx)
	if result.Credited != 0 || result.Closed != 0 {
		t.Fatalf("fresh row was reconciled: %+v", result)
	}
}
