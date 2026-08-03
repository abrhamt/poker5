package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

var (
	ErrInsufficientBalance = errors.New("insufficient wallet balance")
)

type WalletService struct {
	q *repository.Queries
}

func NewWalletService(q *repository.Queries) *WalletService {
	return &WalletService{q: q}
}

func (s *WalletService) Deposit(ctx context.Context, userID int64, amount float64) (*repository.Transaction, error) {
	if amount <= 0 {
		return nil, errors.New("invalid deposit amount")
	}

	txID, err := utilities.GenerateTxID()
	if err != nil {
		return nil, err
	}

	amountStr := fmt.Sprintf("%.2f", amount)
	err = s.q.UpdateUserWallet(ctx, repository.UpdateUserWalletParams{
		Wallet: amountStr,
		ID:     userID,
	})
	if err != nil {
		return nil, err
	}

	res, err := s.q.CreateTransaction(ctx, repository.CreateTransactionParams{
		UserID:        userID,
		Amount:        amountStr,
		Reason:        "deposit",
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
		Amount:        amountStr,
		Reason:        "deposit",
		TransactionID: txID,
	}, nil
}

func (s *WalletService) BuyIn(ctx context.Context, userID int64, amount float64) (*repository.Transaction, error) {
	if amount <= 0 {
		return nil, errors.New("invalid buy-in amount")
	}

	user, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	currentWallet, _ := strconv.ParseFloat(user.Wallet, 64)
	if currentWallet < amount {
		return nil, ErrInsufficientBalance
	}

	txID, err := utilities.GenerateTxID()
	if err != nil {
		return nil, err
	}

	deductStr := fmt.Sprintf("%.2f", -amount)
	err = s.q.UpdateUserWallet(ctx, repository.UpdateUserWalletParams{
		Wallet: deductStr,
		ID:     userID,
	})
	if err != nil {
		return nil, err
	}

	res, err := s.q.CreateTransaction(ctx, repository.CreateTransactionParams{
		UserID:        userID,
		Amount:        fmt.Sprintf("%.2f", amount),
		Reason:        "buy_in",
		TransactionID: txID,
	})
	if err != nil {
		return nil, err
	}

	id, _ := res.LastInsertId()
	return &repository.Transaction{
		ID:            id,
		UserID:        userID,
		Amount:        fmt.Sprintf("%.2f", amount),
		Reason:        "buy_in",
		TransactionID: txID,
	}, nil
}

func (s *WalletService) ProcessPotCommission(ctx context.Context, winnerID int64, totalPot float64, commissionPct float64, refSharePct float64) (float64, float64, error) {
	commission := totalPot * (commissionPct / 100.0)
	netPayout := totalPot - commission

	winner, err := s.q.GetUserByID(ctx, winnerID)
	if err == nil {
		payoutTxID, _ := utilities.GenerateTxID()
		_ = s.q.UpdateUserWallet(ctx, repository.UpdateUserWalletParams{
			Wallet: fmt.Sprintf("%.2f", netPayout),
			ID:     winnerID,
		})
		_, _ = s.q.CreateTransaction(ctx, repository.CreateTransactionParams{
			UserID:        winnerID,
			Amount:        fmt.Sprintf("%.2f", netPayout),
			Reason:        "payout_win",
			TransactionID: payoutTxID,
		})

		if winner.ReferredBy.Valid && winner.ReferredBy.String != "" {
			refUser, err := s.q.GetUserByReferralCode(ctx, winner.ReferredBy.String)
			if err == nil {
				refBonus := commission * (refSharePct / 100.0)
				if refBonus > 0 {
					refTxID, _ := utilities.GenerateTxID()
					_ = s.q.UpdateUserWallet(ctx, repository.UpdateUserWalletParams{
						Wallet: fmt.Sprintf("%.2f", refBonus),
						ID:     refUser.ID,
					})
					_, _ = s.q.CreateTransaction(ctx, repository.CreateTransactionParams{
						UserID:        refUser.ID,
						Amount:        fmt.Sprintf("%.2f", refBonus),
						Reason:        "referral_reward",
						TransactionID: refTxID,
					})
				}
			}
		}
	}

	return netPayout, commission, nil
}

func (s *WalletService) GetUserTransactions(ctx context.Context, userID int64) ([]repository.Transaction, error) {
	return s.q.GetTransactionsByUserID(ctx, userID)
}
