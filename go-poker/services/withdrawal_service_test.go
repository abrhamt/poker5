package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zuse/poker5/go-poker/internal/testdb"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

type withdrawalFixture struct {
	svc      *WithdrawalService
	q        *repository.Queries
	settings *SettingsService
	userID   int64
}

func newWithdrawalFixture(t *testing.T, balance int64) withdrawalFixture {
	t.Helper()

	db := testdb.New(t)
	q := repository.New(db)
	ctx := context.Background()

	passHash, _ := utilities.HashPassword("password123")
	userID, err := q.CreateUser(ctx, repository.CreateUserParams{
		Username:     "alice",
		PhoneNumber:  "+251911223344",
		PasswordHash: passHash,
		Wallet:       balance,
		ReferralCode: "ABC123",
		Role:         "player",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	settings := NewSettingsService(q)
	f := withdrawalFixture{
		svc:      NewWithdrawalService(q, db, NewWalletService(q), settings),
		q:        q,
		settings: settings,
		userID:   userID,
	}
	f.configure(t, func(r *UpdateSettingsRequest) {})
	return f
}

// configure saves withdrawal-enabled defaults, then lets the test change any
// of them.
func (f withdrawalFixture) configure(t *testing.T, edit func(*UpdateSettingsRequest)) {
	t.Helper()
	req := UpdateSettingsRequest{
		RakeMode:                     RakeModePercentage,
		RakePercentage:               5,
		CountdownSeconds:             15,
		WithdrawalsEnabled:           true,
		WithdrawalMinAmount:          50,
		WithdrawalCancelEnabled:      true,
		WithdrawalResetCooldownHours: 24,
	}
	edit(&req)
	if err := f.settings.Update(context.Background(), req); err != nil {
		t.Fatalf("update settings: %v", err)
	}
}

func cbeRequest(amount int64) WithdrawalRequest {
	return WithdrawalRequest{Method: "cbe", AccountNumber: "1000 1234 56789", AccountName: "Alice  Bekele", Amount: amount}
}

func countTransactions(t *testing.T, q *repository.Queries, userID int64, txType string) int {
	t.Helper()
	rows, err := q.ListTransactionsByUserID(context.Background(), repository.ListTransactionsByUserIDParams{UserID: userID, Limit: 100})
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	n := 0
	for _, r := range rows {
		if r.Type == txType {
			n++
		}
	}
	return n
}

func TestWithdrawalRequestDebitsWalletAndRecordsRequest(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()

	row, err := f.svc.Request(ctx, f.userID, cbeRequest(200))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if row.Status != WithdrawalStatusPending || row.Amount != 200 {
		t.Fatalf("row = %+v", row)
	}
	if row.AccountNumber != "1000123456789" || row.AccountName != "Alice Bekele" {
		t.Fatalf("destination not normalized: %q / %q", row.AccountNumber, row.AccountName)
	}
	if got := walletBalance(t, f.q, f.userID); got != 300 {
		t.Fatalf("wallet = %d, want 300", got)
	}
	if n := countTransactions(t, f.q, f.userID, TxTypeWithdrawal); n != 1 {
		t.Fatalf("withdrawal ledger rows = %d, want 1", n)
	}
}

func TestWithdrawalRequestValidation(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()

	cases := []struct {
		name string
		req  WithdrawalRequest
		want func(error) bool
	}{
		{"below minimum", cbeRequest(49), func(err error) bool {
			var e WithdrawalTooSmallError
			return errors.As(err, &e) && e.Min == 50
		}},
		{"more than the balance", cbeRequest(501), func(err error) bool { return errors.Is(err, ErrInsufficientBalance) }},
		{"zero", cbeRequest(0), func(err error) bool { return err != nil }},
		{"bad CBE account", WithdrawalRequest{Method: "cbe", AccountNumber: "12345", AccountName: "A", Amount: 100},
			func(err error) bool { return errors.Is(err, utilities.ErrInvalidCBEAccount) }},
		{"Safaricom number for Telebirr", WithdrawalRequest{Method: "telebirr", AccountNumber: "0711223344", AccountName: "A", Amount: 100},
			func(err error) bool { return errors.Is(err, utilities.ErrInvalidTelebirrAccount) }},
		{"unknown method", WithdrawalRequest{Method: "paypal", AccountNumber: "1", AccountName: "A", Amount: 100},
			func(err error) bool { return errors.Is(err, ErrWithdrawalMethodInvalid) }},
		{"no account name", WithdrawalRequest{Method: "telebirr", AccountNumber: "0911223344", AccountName: "  ", Amount: 100},
			func(err error) bool { return errors.Is(err, ErrWithdrawalNameRequired) }},
	}
	for _, tc := range cases {
		if _, err := f.svc.Request(ctx, f.userID, tc.req); !tc.want(err) {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
	}
	if got := walletBalance(t, f.q, f.userID); got != 500 {
		t.Fatalf("a refused request moved money: wallet = %d", got)
	}
}

func TestWithdrawalMinimumFollowsSetting(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	f.configure(t, func(r *UpdateSettingsRequest) { r.WithdrawalMinAmount = 100 })

	var tooSmall WithdrawalTooSmallError
	if _, err := f.svc.Request(context.Background(), f.userID, cbeRequest(99)); !errors.As(err, &tooSmall) || tooSmall.Min != 100 {
		t.Fatalf("err = %v, want minimum 100", err)
	}
}

func TestWithdrawalRefusedWhileDisabled(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	f.configure(t, func(r *UpdateSettingsRequest) { r.WithdrawalsEnabled = false })

	if _, err := f.svc.Request(context.Background(), f.userID, cbeRequest(100)); !errors.Is(err, ErrWithdrawalsDisabled) {
		t.Fatalf("err = %v, want ErrWithdrawalsDisabled", err)
	}
}

func TestOnlyOnePendingWithdrawalAtATime(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()

	if _, err := f.svc.Request(ctx, f.userID, cbeRequest(100)); err != nil {
		t.Fatalf("first request: %v", err)
	}
	if _, err := f.svc.Request(ctx, f.userID, cbeRequest(100)); !errors.Is(err, ErrWithdrawalPending) {
		t.Fatalf("second request err = %v, want ErrWithdrawalPending", err)
	}
	if got := walletBalance(t, f.q, f.userID); got != 400 {
		t.Fatalf("wallet = %d, want 400", got)
	}
}

// Two requests fired at once must not both get through: the unique index, not
// the read before it, is what holds the line.
func TestConcurrentWithdrawalRequestsDebitOnce(t *testing.T) {
	f := newWithdrawalFixture(t, 1000)
	ctx := context.Background()

	const attempts = 8
	var wg sync.WaitGroup
	errs := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.svc.Request(ctx, f.userID, cbeRequest(100))
		}()
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrWithdrawalPending):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d requests succeeded, want exactly 1", succeeded)
	}
	if got := walletBalance(t, f.q, f.userID); got != 900 {
		t.Fatalf("wallet = %d, want 900", got)
	}
}

func TestCompleteWithdrawalMovesNoMoney(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()
	row, _ := f.svc.Request(ctx, f.userID, cbeRequest(200))

	done, err := f.svc.Complete(ctx, row.ID, " FT123 ", "admin")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if done.Status != WithdrawalStatusCompleted || done.PayoutReference != "FT123" || done.ResolvedBy != "admin" {
		t.Fatalf("row = %+v", done)
	}
	if got := walletBalance(t, f.q, f.userID); got != 300 {
		t.Fatalf("wallet = %d, want 300", got)
	}
	if _, err := f.svc.Reject(ctx, row.ID, "late", "admin"); !errors.Is(err, ErrWithdrawalNotPending) {
		t.Fatalf("reject after complete err = %v, want ErrWithdrawalNotPending", err)
	}

	// A finished request no longer blocks a new one.
	if _, err := f.svc.Request(ctx, f.userID, cbeRequest(100)); err != nil {
		t.Fatalf("request after completion: %v", err)
	}
}

func TestRejectWithdrawalRefundsOnce(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()
	row, _ := f.svc.Request(ctx, f.userID, cbeRequest(200))

	if _, err := f.svc.Reject(ctx, row.ID, "  ", "admin"); !errors.Is(err, ErrWithdrawalReasonRequired) {
		t.Fatalf("reject without reason err = %v", err)
	}

	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.svc.Reject(ctx, row.ID, "Name does not match the account", "admin")
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		if err == nil {
			ok++
		} else if !errors.Is(err, ErrWithdrawalNotPending) {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d rejections succeeded, want 1", ok)
	}
	if got := walletBalance(t, f.q, f.userID); got != 500 {
		t.Fatalf("wallet = %d, want 500 after one refund", got)
	}
	if n := countTransactions(t, f.q, f.userID, TxTypeWithdrawalRefund); n != 1 {
		t.Fatalf("refund rows = %d, want 1", n)
	}

	got, _ := f.q.GetWithdrawalByID(ctx, row.ID)
	if got.Status != WithdrawalStatusRejected || got.Note != "Name does not match the account" || !got.RefundTransactionID.Valid {
		t.Fatalf("row = %+v", got)
	}
}

func TestPlayerCancelsOwnWithdrawal(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()
	row, _ := f.svc.Request(ctx, f.userID, cbeRequest(200))

	if _, err := f.svc.Cancel(ctx, f.userID+999, row.ID); !errors.Is(err, ErrWithdrawalNotFound) {
		t.Fatalf("cancel by another user err = %v, want ErrWithdrawalNotFound", err)
	}

	got, err := f.svc.Cancel(ctx, f.userID, row.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status != WithdrawalStatusCancelled {
		t.Fatalf("status = %q", got.Status)
	}
	if bal := walletBalance(t, f.q, f.userID); bal != 500 {
		t.Fatalf("wallet = %d, want 500", bal)
	}
	if _, err := f.svc.Cancel(ctx, f.userID, row.ID); !errors.Is(err, ErrWithdrawalNotPending) {
		t.Fatalf("second cancel err = %v", err)
	}
}

func TestCancelRefusedWhenSettingIsOff(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()
	row, _ := f.svc.Request(ctx, f.userID, cbeRequest(200))
	f.configure(t, func(r *UpdateSettingsRequest) { r.WithdrawalCancelEnabled = false })

	if _, err := f.svc.Cancel(ctx, f.userID, row.ID); !errors.Is(err, ErrWithdrawalCancelDisabled) {
		t.Fatalf("err = %v, want ErrWithdrawalCancelDisabled", err)
	}
	if bal := walletBalance(t, f.q, f.userID); bal != 300 {
		t.Fatalf("wallet = %d, want 300", bal)
	}
}

func TestWithdrawalCooldownAfterPasswordReset(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()

	now := time.Unix(1_800_000_000, 0)
	f.svc.now = func() time.Time { return now }
	if err := f.q.RecordPasswordReset(ctx, repository.RecordPasswordResetParams{UserID: f.userID, ResetAt: now.Add(-2 * time.Hour).Unix()}); err != nil {
		t.Fatalf("record reset: %v", err)
	}

	var cooldown WithdrawalCooldownError
	if _, err := f.svc.Request(ctx, f.userID, cbeRequest(100)); !errors.As(err, &cooldown) {
		t.Fatalf("err = %v, want a cooldown", err)
	}
	if want := now.Add(22 * time.Hour); !cooldown.Until.Equal(want) {
		t.Fatalf("until = %v, want %v", cooldown.Until, want)
	}
	cfg, err := f.svc.Config(ctx, f.userID)
	if err != nil || cfg.CooldownUntil == nil {
		t.Fatalf("config should report the cooldown: %+v, %v", cfg, err)
	}

	// A shorter window that has already passed lets the request through.
	f.configure(t, func(r *UpdateSettingsRequest) { r.WithdrawalResetCooldownHours = 1 })
	if _, err := f.svc.Request(ctx, f.userID, cbeRequest(100)); err != nil {
		t.Fatalf("request after the window: %v", err)
	}
}

func TestWithdrawalCooldownCanBeTurnedOff(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()
	f.configure(t, func(r *UpdateSettingsRequest) { r.WithdrawalResetCooldownHours = 0 })
	if err := f.q.RecordPasswordReset(ctx, repository.RecordPasswordResetParams{UserID: f.userID, ResetAt: time.Now().Unix()}); err != nil {
		t.Fatalf("record reset: %v", err)
	}
	if _, err := f.svc.Request(ctx, f.userID, cbeRequest(100)); err != nil {
		t.Fatalf("request with cooldown off: %v", err)
	}
}

func TestAdminListsPendingOldestFirst(t *testing.T) {
	f := newWithdrawalFixture(t, 500)
	ctx := context.Background()

	first, _ := f.svc.Request(ctx, f.userID, cbeRequest(100))
	if _, err := f.svc.Complete(ctx, first.ID, "", "admin"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	second, _ := f.svc.Request(ctx, f.userID, cbeRequest(100))

	page, err := f.svc.ListForAdmin(ctx, WithdrawalStatusPending, 0, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Total != 1 || len(page.Rows) != 1 || page.Rows[0].Withdrawal.ID != second.ID || page.Rows[0].Username != "alice" {
		t.Fatalf("pending page = %+v", page)
	}

	all, _ := f.svc.ListForAdmin(ctx, "", f.userID, 1)
	if all.Total != 2 {
		t.Fatalf("total = %d, want 2", all.Total)
	}
	if _, err := f.svc.ListForAdmin(ctx, "bogus", 0, 1); !errors.Is(err, ErrWithdrawalStatusInvalid) {
		t.Fatalf("bogus status err = %v", err)
	}
}
