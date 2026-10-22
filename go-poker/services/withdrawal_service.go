package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

// Withdrawal payout methods.
const (
	WithdrawalMethodCBE      = "cbe"
	WithdrawalMethodTelebirr = "telebirr"
)

// Withdrawal row statuses. Only pending is open; the other three are final.
const (
	WithdrawalStatusPending   = "pending"
	WithdrawalStatusCompleted = "completed"
	WithdrawalStatusRejected  = "rejected"
	WithdrawalStatusCancelled = "cancelled"
)

const (
	TxTypeWithdrawal       = "withdrawal"
	TxTypeWithdrawalRefund = "withdrawal_refund"

	// MaxWithdrawalAccountName matches its column.
	MaxWithdrawalAccountName = 128
	maxWithdrawalNote        = 255
	maxPayoutReference       = 64

	WithdrawalPageSize = 25

	// playerWithdrawalHistory is how many past requests the wallet screen
	// shows. It is a status list, not a statement — the ledger is the statement.
	playerWithdrawalHistory = 10
)

var (
	ErrWithdrawalsDisabled      = errors.New("withdrawals are not available right now")
	ErrWithdrawalPending        = errors.New("you already have a withdrawal waiting to be paid — wait for it to finish or cancel it first")
	ErrWithdrawalMethodInvalid  = errors.New("choose CBE or Telebirr")
	ErrWithdrawalNameRequired   = errors.New("enter the name on the account")
	ErrWithdrawalNameTooLong    = fmt.Errorf("the account name can be at most %d characters", MaxWithdrawalAccountName)
	ErrWithdrawalNotFound       = errors.New("withdrawal not found")
	ErrWithdrawalNotPending     = errors.New("that withdrawal has already been dealt with")
	ErrWithdrawalCancelDisabled = errors.New("withdrawals cannot be cancelled — contact support")
	ErrWithdrawalReasonRequired = errors.New("give the player a reason for the rejection")
	ErrWithdrawalStatusInvalid  = errors.New("unknown withdrawal status")
	ErrWithdrawalAmountInvalid  = errors.New("enter a whole amount in ETB")
)

// WithdrawalTooSmallError carries the minimum in force when the request was
// made, which is a setting and so cannot be a fixed message.
type WithdrawalTooSmallError struct{ Min int64 }

func (e WithdrawalTooSmallError) Error() string {
	return fmt.Sprintf("the minimum withdrawal is %d ETB", e.Min)
}

// WithdrawalCooldownError is the refusal while an account is inside the
// post-reset window. It says when the window ends so the player is not left
// guessing whether to try again in a minute or a day.
type WithdrawalCooldownError struct{ Until time.Time }

func (e WithdrawalCooldownError) Error() string {
	return fmt.Sprintf("withdrawals are paused for a while after a password reset — try again after %s", e.Until.UTC().Format("Jan 2, 15:04 UTC"))
}

// WithdrawalConfig is what the withdrawal screen needs before it can render.
type WithdrawalConfig struct {
	Enabled       bool     `json:"enabled"`
	MinAmount     int64    `json:"min_amount"`
	CancelEnabled bool     `json:"cancel_enabled"`
	Methods       []string `json:"methods"`
	// CooldownUntil is set while the account is inside the post-reset window.
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`
}

type WithdrawalRequest struct {
	Method        string
	AccountNumber string
	AccountName   string
	Amount        int64
}

type WithdrawalService struct {
	q        *repository.Queries
	db       *sql.DB
	wallet   *WalletService
	settings *SettingsService
	now      func() time.Time
}

func NewWithdrawalService(q *repository.Queries, db *sql.DB, wallet *WalletService, settings *SettingsService) *WithdrawalService {
	return &WithdrawalService{q: q, db: db, wallet: wallet, settings: settings, now: time.Now}
}

// Config reports the settings as they apply to this player, including any
// cooldown they are inside right now.
func (s *WithdrawalService) Config(ctx context.Context, userID int64) (WithdrawalConfig, error) {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return WithdrawalConfig{}, err
	}
	cfg := WithdrawalConfig{
		Enabled:       settings.WithdrawalsEnabled,
		MinAmount:     settings.WithdrawalMinAmount,
		CancelEnabled: settings.WithdrawalCancelEnabled,
		Methods:       []string{WithdrawalMethodCBE, WithdrawalMethodTelebirr},
	}
	until, err := s.cooldownUntil(ctx, userID, settings)
	if err != nil {
		return WithdrawalConfig{}, err
	}
	cfg.CooldownUntil = until
	return cfg, nil
}

// Request takes the money out of the player's wallet and files the request.
// The debit, its ledger row and the request row commit together: a debit
// without a request would be money gone with nothing to pay it out against.
func (s *WithdrawalService) Request(ctx context.Context, userID int64, req WithdrawalRequest) (*repository.Withdrawal, error) {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.WithdrawalsEnabled {
		return nil, ErrWithdrawalsDisabled
	}

	method, account, name, err := validateWithdrawalDestination(req)
	if err != nil {
		return nil, err
	}
	if req.Amount <= 0 {
		return nil, ErrWithdrawalAmountInvalid
	}
	if req.Amount < settings.WithdrawalMinAmount {
		return nil, WithdrawalTooSmallError{Min: settings.WithdrawalMinAmount}
	}

	until, err := s.cooldownUntil(ctx, userID, settings)
	if err != nil {
		return nil, err
	}
	if until != nil {
		return nil, WithdrawalCooldownError{Until: *until}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	// Checked before the debit only so the common case gets the clear message;
	// the unique index below is what actually enforces it.
	if _, err := qtx.GetPendingWithdrawalByUser(ctx, userID); err == nil {
		return nil, ErrWithdrawalPending
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	debited, err := qtx.DebitUserWalletIfSufficient(ctx, repository.DebitUserWalletIfSufficientParams{Amount: req.Amount, ID: userID})
	if err != nil {
		return nil, err
	}
	if debited == 0 {
		return nil, ErrInsufficientBalance
	}

	txID, err := utilities.GenerateTxID()
	if err != nil {
		return nil, err
	}
	if _, err := qtx.CreateTransaction(ctx, repository.CreateTransactionParams{
		UserID:        userID,
		Amount:        -req.Amount,
		Type:          TxTypeWithdrawal,
		Reason:        "withdrawal_" + method,
		TransactionID: txID,
	}); err != nil {
		return nil, err
	}

	row, err := qtx.CreateWithdrawal(ctx, repository.CreateWithdrawalParams{
		UserID:             userID,
		Method:             method,
		AccountNumber:      account,
		AccountName:        name,
		Amount:             req.Amount,
		DebitTransactionID: txID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrWithdrawalPending
		}
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrWithdrawalPending
		}
		return nil, err
	}
	return &row, nil
}

// ListForPlayer returns a player's most recent requests, newest first.
func (s *WithdrawalService) ListForPlayer(ctx context.Context, userID int64) ([]repository.Withdrawal, error) {
	return s.q.ListWithdrawalsByUser(ctx, repository.ListWithdrawalsByUserParams{UserID: userID, Limit: playerWithdrawalHistory})
}

// Cancel lets the player take back a request nobody has paid yet. It is the
// same refund a rejection makes, just started from the other side.
func (s *WithdrawalService) Cancel(ctx context.Context, userID, withdrawalID int64) (*repository.Withdrawal, error) {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.WithdrawalCancelEnabled {
		return nil, ErrWithdrawalCancelDisabled
	}

	// Ownership is checked against the row, not trusted from the request: an
	// id is easy to guess, and cancelling someone else's request would pay
	// their money back to them at a moment they did not choose.
	row, err := s.q.GetWithdrawalByID(ctx, withdrawalID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && row.UserID != userID) {
		return nil, ErrWithdrawalNotFound
	}
	if err != nil {
		return nil, err
	}

	return s.refund(ctx, withdrawalID, WithdrawalStatusCancelled, "Cancelled by the player.", "")
}

// Complete records that an admin has sent the money. Nothing moves in the
// wallet: it left when the request was made.
func (s *WithdrawalService) Complete(ctx context.Context, withdrawalID int64, payoutReference, adminName string) (*repository.Withdrawal, error) {
	row, err := s.q.ResolveWithdrawal(ctx, repository.ResolveWithdrawalParams{
		Status:          WithdrawalStatusCompleted,
		PayoutReference: clip(strings.TrimSpace(payoutReference), maxPayoutReference),
		ResolvedBy:      clip(adminName, 64),
		ResolvedAt:      s.now().Unix(),
		ID:              withdrawalID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, s.notPendingOrMissing(ctx, withdrawalID)
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Reject turns a request down and pays the money back. The reason is shown to
// the player, so it is required: a refusal with no explanation just becomes a
// support ticket.
func (s *WithdrawalService) Reject(ctx context.Context, withdrawalID int64, reason, adminName string) (*repository.Withdrawal, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ErrWithdrawalReasonRequired
	}
	return s.refund(ctx, withdrawalID, WithdrawalStatusRejected, clip(reason, maxWithdrawalNote), clip(adminName, 64))
}

// refund closes a pending request and credits its amount back in one
// transaction. ResolveWithdrawal only matches a pending row, so a second
// caller racing this one gets no row and refunds nothing.
func (s *WithdrawalService) refund(ctx context.Context, withdrawalID int64, status, note, resolvedBy string) (*repository.Withdrawal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	row, err := qtx.ResolveWithdrawal(ctx, repository.ResolveWithdrawalParams{
		Status:     status,
		Note:       note,
		ResolvedBy: resolvedBy,
		ResolvedAt: s.now().Unix(),
		ID:         withdrawalID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return nil, s.notPendingOrMissing(ctx, withdrawalID)
	}
	if err != nil {
		return nil, err
	}

	credit, err := s.wallet.moveWalletWith(ctx, qtx, row.UserID, row.Amount, TxTypeWithdrawalRefund, "withdrawal_"+status)
	if err != nil {
		return nil, err
	}
	refundID := sql.NullString{String: credit.TransactionID, Valid: true}
	if err := qtx.SetWithdrawalRefundTransaction(ctx, repository.SetWithdrawalRefundTransactionParams{RefundTransactionID: refundID, ID: row.ID}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	row.RefundTransactionID = refundID
	return &row, nil
}

func (s *WithdrawalService) notPendingOrMissing(ctx context.Context, withdrawalID int64) error {
	if _, err := s.q.GetWithdrawalByID(ctx, withdrawalID); errors.Is(err, sql.ErrNoRows) {
		return ErrWithdrawalNotFound
	} else if err != nil {
		return err
	}
	return ErrWithdrawalNotPending
}

// WithdrawalPage is one page of the admin list.
type WithdrawalPage struct {
	Rows  []repository.ListWithdrawalsForAdminRow
	Total int64
	Page  int
}

// ListForAdmin pages through requests in one status ("" for all), optionally
// for a single player.
func (s *WithdrawalService) ListForAdmin(ctx context.Context, status string, userID int64, page int) (WithdrawalPage, error) {
	switch status {
	case "", WithdrawalStatusPending, WithdrawalStatusCompleted, WithdrawalStatusRejected, WithdrawalStatusCancelled:
	default:
		return WithdrawalPage{}, ErrWithdrawalStatusInvalid
	}
	if page < 1 {
		page = 1
	}
	total, err := s.q.CountWithdrawalsForAdmin(ctx, repository.CountWithdrawalsForAdminParams{Status: status, UserID: userID})
	if err != nil {
		return WithdrawalPage{}, err
	}
	rows, err := s.q.ListWithdrawalsForAdmin(ctx, repository.ListWithdrawalsForAdminParams{
		Status:    status,
		UserID:    userID,
		RowLimit:  WithdrawalPageSize,
		RowOffset: int32((page - 1) * WithdrawalPageSize),
	})
	if err != nil {
		return WithdrawalPage{}, err
	}
	return WithdrawalPage{Rows: rows, Total: total, Page: page}, nil
}

// PendingCount is the number behind the admin nav badge.
func (s *WithdrawalService) PendingCount(ctx context.Context) (int64, error) {
	return s.q.CountWithdrawalsForAdmin(ctx, repository.CountWithdrawalsForAdminParams{Status: WithdrawalStatusPending})
}

// PendingTotal is the money the site is holding for requests not yet paid.
func (s *WithdrawalService) PendingTotal(ctx context.Context) (int64, error) {
	return s.q.SumWithdrawalAmountByStatus(ctx, WithdrawalStatusPending)
}

func (s *WithdrawalService) cooldownUntil(ctx context.Context, userID int64, settings repository.SiteSetting) (*time.Time, error) {
	if settings.WithdrawalResetCooldownHours <= 0 {
		return nil, nil
	}
	resetAt, err := s.q.GetPasswordResetAt(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	until := time.Unix(resetAt, 0).Add(time.Duration(settings.WithdrawalResetCooldownHours) * time.Hour)
	if !s.now().Before(until) {
		return nil, nil
	}
	return &until, nil
}

func validateWithdrawalDestination(req WithdrawalRequest) (method, account, name string, err error) {
	method = strings.ToLower(strings.TrimSpace(req.Method))
	switch method {
	case WithdrawalMethodCBE:
		account, err = utilities.NormalizeCBEAccount(req.AccountNumber)
	case WithdrawalMethodTelebirr:
		account, err = utilities.NormalizeTelebirrAccount(req.AccountNumber)
	default:
		return "", "", "", ErrWithdrawalMethodInvalid
	}
	if err != nil {
		return "", "", "", err
	}

	name = strings.Join(strings.Fields(req.AccountName), " ")
	if name == "" {
		return "", "", "", ErrWithdrawalNameRequired
	}
	if len([]rune(name)) > MaxWithdrawalAccountName {
		return "", "", "", ErrWithdrawalNameTooLong
	}
	return method, account, name, nil
}
