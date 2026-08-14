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

	res, err := s.q.CreateUser(ctx, repository.CreateUserParams{
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
	id, err := res.LastInsertId()
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

func (s *WalletService) moveWallet(ctx context.Context, userID int64, delta int64, txType, reason string) (*repository.Transaction, error) {
	txID, err := utilities.GenerateTxID()
	if err != nil {
		return nil, err
	}

	if err := s.q.UpdateUserWallet(ctx, repository.UpdateUserWalletParams{Wallet: delta, ID: userID}); err != nil {
		return nil, err
	}

	res, err := s.q.CreateTransaction(ctx, repository.CreateTransactionParams{
		UserID:        userID,
		Amount:        delta,
		Type:          txType,
		Reason:        reason,
		TransactionID: txID,
	})
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
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

func (s *WalletService) GetUserTransactions(ctx context.Context, userID int64) ([]repository.Transaction, error) {
	return s.q.GetTransactionsByUserID(ctx, userID)
}
