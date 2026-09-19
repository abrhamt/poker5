package services

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

var (
	ErrInsufficientBalance = errors.New("insufficient wallet balance")
)

const (
	TxTypeDeposit      = "deposit"
	TxTypeBuyIn        = "buy_in"
	TxTypeCashOut      = "cash_out"
	TxTypeWinnerPayout = "winner_payout"
	TxTypeSiteRake     = "site_rake"
	TxTypeReferral     = "referral_commission"

	RakeModePercentage = "percentage"
	RakeModeSmallBlind = "small_blind"

	houseUsername = "house_treasury"
)

type WalletService struct {
	q           *repository.Queries
	houseUserID int64
}

func NewWalletService(q *repository.Queries) *WalletService {
	return &WalletService{q: q}
}

// EnsureHouseAccount gets or creates the system "house" account that holds
// the site's rake earnings. Safe to call repeatedly (idempotent).
func (s *WalletService) EnsureHouseAccount(ctx context.Context) (int64, error) {
	if s.houseUserID != 0 {
		return s.houseUserID, nil
	}

	user, err := s.q.GetUserByUsername(ctx, houseUsername)
	if err == nil {
		s.houseUserID = user.ID
		return s.houseUserID, nil
	}

	refCode, err := utilities.GenerateReferralCode()
	if err != nil {
		return 0, err
	}
	passHash, err := utilities.HashPassword(utilities.GenerateSessionToken())
	if err != nil {
		return 0, err
	}

	id, err := s.q.CreateUser(ctx, repository.CreateUserParams{
		Username:     houseUsername,
		PhoneNumber:  "house-system-account",
		PasswordHash: passHash,
		Wallet:       0,
		ReferralCode: refCode,
		ReferredBy:   sql.NullString{},
		Role:         "house",
	})
	if err != nil {
		return 0, err
	}
	s.houseUserID = id
	return s.houseUserID, nil
}

func (s *WalletService) HouseUserID() int64 {
	return s.houseUserID
}

func (s *WalletService) Deposit(ctx context.Context, userID int64, amount int64) (*repository.Transaction, error) {
	if amount <= 0 {
		return nil, errors.New("invalid deposit amount")
	}
	return s.moveWallet(ctx, userID, amount, TxTypeDeposit, "deposit")
}

func (s *WalletService) BuyIn(ctx context.Context, userID int64, amount int64) (*repository.Transaction, error) {
	if amount <= 0 {
		return nil, errors.New("invalid buy-in amount")
	}

	user, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Wallet < amount {
		return nil, ErrInsufficientBalance
	}

	return s.moveWallet(ctx, userID, -amount, TxTypeBuyIn, "buy_in")
}

func (s *WalletService) CashOut(ctx context.Context, userID int64, amount int64) (*repository.Transaction, error) {
	if amount <= 0 {
		return nil, nil
	}
	return s.moveWallet(ctx, userID, amount, TxTypeCashOut, "cash_out")
}

// DepositWith credits a deposit through the caller's own queries handle, so a
// bank deposit can move the wallet inside the same database transaction that
// claims the receipt. Crediting money and recording which receipt paid for it
// have to commit together or not at all — a wallet credit whose receipt row
// was rolled back is money with no reference, and the reference is the only
// thing stopping the same receipt from being spent twice.
func (s *WalletService) DepositWith(ctx context.Context, q *repository.Queries, userID int64, amount int64, reason string) (*repository.Transaction, error) {
	if amount <= 0 {
		return nil, errors.New("invalid deposit amount")
	}
	return s.moveWalletWith(ctx, q, userID, amount, TxTypeDeposit, reason)
}

func (s *WalletService) moveWallet(ctx context.Context, userID int64, delta int64, txType, reason string) (*repository.Transaction, error) {
	return s.moveWalletWith(ctx, s.q, userID, delta, txType, reason)
}

func (s *WalletService) moveWalletWith(ctx context.Context, q *repository.Queries, userID int64, delta int64, txType, reason string) (*repository.Transaction, error) {
	txID, err := utilities.GenerateTxID()
	if err != nil {
		return nil, err
	}

	if err := q.UpdateUserWallet(ctx, repository.UpdateUserWalletParams{Wallet: delta, ID: userID}); err != nil {
		return nil, err
	}

	id, err := q.CreateTransaction(ctx, repository.CreateTransactionParams{
		UserID:        userID,
		Amount:        delta,
		Type:          txType,
		Reason:        reason,
		TransactionID: txID,
	})
	if err != nil {
		return nil, err
	}
	return &repository.Transaction{
		ID:            id,
		UserID:        userID,
		Amount:        delta,
		Type:          txType,
		Reason:        reason,
		TransactionID: txID,
	}, nil
}

// PayoutResult is the outcome of raking a single pot/side-pot award.
type PayoutResult struct {
	WinnerNet      int64
	SiteRake       int64
	ReferralCut    int64
	ReferrerUserID int64
}

// ProcessHandPayout takes a single pot award (the full chip amount a player
// won at showdown) and splits it into the winner's net share, the site's
// rake, and (if the winner was referred) the referrer's commission. The
// site's and referrer's cuts are credited to their real wallets immediately;
// the winner's net share is NOT credited here — it stays in play as chips
// and only becomes real wallet money at cash-out. Every leg is still
// recorded as a transaction for audit/history purposes.
func (s *WalletService) ProcessHandPayout(ctx context.Context, winnerUserID int64, potAmount int64, smallBlind int64, settings repository.SiteSetting) (PayoutResult, error) {
	result := PayoutResult{WinnerNet: potAmount}
	if potAmount <= 0 {
		return result, nil
	}

	winner, err := s.q.GetUserByID(ctx, winnerUserID)
	if err != nil {
		return result, err
	}

	var siteRake int64
	switch settings.RakeMode {
	case RakeModeSmallBlind:
		siteRake = smallBlind
		if siteRake > potAmount {
			siteRake = potAmount
		}
	default:
		siteRake = int64(math.Floor(float64(potAmount) * settings.RakePercentage / 100))
	}

	var referralCut int64
	var referrerID int64
	if winner.ReferredBy.Valid && winner.ReferredBy.String != "" {
		refUser, err := s.q.GetUserByReferralCode(ctx, winner.ReferredBy.String)
		if err == nil {
			referrerID = refUser.ID
			referralPct := settings.ReferralPercentagePctMode
			if settings.RakeMode == RakeModeSmallBlind {
				referralPct = settings.ReferralPercentageSbMode
			}
			referralCut = int64(math.Floor(float64(potAmount) * referralPct / 100))
		}
	}

	remaining := potAmount - siteRake
	if remaining < 0 {
		remaining = 0
	}
	if referralCut > remaining {
		referralCut = remaining
	}

	winnerNet := potAmount - siteRake - referralCut

	houseID, err := s.EnsureHouseAccount(ctx)
	if err != nil {
		return result, err
	}

	if siteRake > 0 {
		if _, err := s.moveWallet(ctx, houseID, siteRake, TxTypeSiteRake, "hand_rake"); err != nil {
			return result, err
		}
	}
	if referralCut > 0 && referrerID != 0 {
		if _, err := s.moveWallet(ctx, referrerID, referralCut, TxTypeReferral, "referral_commission"); err != nil {
			return result, err
		}
	}

	// Audit-only record of the winner's net share; wallet stays untouched
	// until the player cashes out of the table.
	if winnerNet > 0 {
		txID, err := utilities.GenerateTxID()
		if err == nil {
			_, _ = s.q.CreateTransaction(ctx, repository.CreateTransactionParams{
				UserID:        winnerUserID,
				Amount:        winnerNet,
				Type:          TxTypeWinnerPayout,
				Reason:        "hand_win",
				TransactionID: txID,
			})
		}
	}

	result.WinnerNet = winnerNet
	result.SiteRake = siteRake
	result.ReferralCut = referralCut
	result.ReferrerUserID = referrerID
	return result, nil
}

// TransactionPage is one page of a player's ledger together with everything the
// client needs to draw pagination controls, so listing never costs two calls.
type TransactionPage struct {
	Transactions []repository.Transaction
	Page         int
	PageSize     int
	Total        int64
	TotalPages   int
}

const (
	DefaultTransactionsPageSize = 12
	MaxTransactionsPageSize     = 50
)

// GetUserTransactions returns the requested page of a player's ledger, newest
// first. Out-of-range requests are clamped rather than rejected: a page past
// the end lands on the last real page, which keeps the UI honest if the ledger
// changed between the client asking and the query running.
func (s *WalletService) GetUserTransactions(ctx context.Context, userID int64, page, pageSize int) (TransactionPage, error) {
	if pageSize <= 0 {
		pageSize = DefaultTransactionsPageSize
	}
	if pageSize > MaxTransactionsPageSize {
		pageSize = MaxTransactionsPageSize
	}
	if page < 1 {
		page = 1
	}

	total, err := s.q.CountTransactionsByUserID(ctx, userID)
	if err != nil {
		return TransactionPage{}, err
	}

	totalPages := 1
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	if page > totalPages {
		page = totalPages
	}

	rows, err := s.q.ListTransactionsByUserID(ctx, repository.ListTransactionsByUserIDParams{
		UserID: userID,
		Limit:  int32(pageSize),
		Offset: int32((page - 1) * pageSize),
	})
	if err != nil {
		return TransactionPage{}, err
	}

	return TransactionPage{
		Transactions: rows,
		Page:         page,
		PageSize:     pageSize,
		Total:        total,
		TotalPages:   totalPages,
	}, nil
}
