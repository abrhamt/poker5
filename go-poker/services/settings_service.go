package services

import (
	"context"
	"errors"
	"strings"

	"github.com/zuse/poker5/go-poker/repository"
)

const (
	MinRakePercentage float64 = 0
	MaxRakePercentage float64 = 20

	MinReferralPercentage float64 = 0
	MaxReferralPercentage float64 = 50

	MinCountdownSeconds = 5
	MaxCountdownSeconds = 120

	MinIntermissionSeconds     = 1
	MaxIntermissionSeconds     = 30
	DefaultIntermissionSeconds = 3

	// The withdrawal minimum is a policy, not a technical limit, so the range
	// only rules out nonsense: zero, and a floor nobody could ever reach.
	MinWithdrawalMinAmount     = 1
	MaxWithdrawalMinAmount     = 100_000
	DefaultWithdrawalMinAmount = 50

	MaxWithdrawalCooldownHours = 720
)

var (
	ErrInvalidSettings = errors.New("settings value out of allowed range")
	// Enabling real deposits without an account to send money to would make
	// every deposit fail the receiver check, so the two are validated together
	// rather than left to be noticed by the first player who tries.
	ErrDepositAccountRequired = errors.New("set the deposit account name and number before enabling bank deposits")
)

// MaxDepositAccountName / MaxDepositAccountNumber match their columns.
const (
	MaxDepositAccountName   = 128
	MaxDepositAccountNumber = 32
)

type SettingsService struct {
	q *repository.Queries
}

func NewSettingsService(q *repository.Queries) *SettingsService {
	return &SettingsService{q: q}
}

func (s *SettingsService) Get(ctx context.Context) (repository.SiteSetting, error) {
	return s.q.GetSiteSettings(ctx)
}

type UpdateSettingsRequest struct {
	RakeMode                  string
	RakePercentage            float64
	ReferralPercentagePctMode float64
	ReferralPercentageSbMode  float64
	CountdownSeconds          int32
	RealDepositsEnabled       bool
	DepositAccountName        string
	DepositAccountNumber      string
	GatewayDepositsEnabled    bool
	IntermissionSeconds       int32

	WithdrawalsEnabled           bool
	WithdrawalMinAmount          int64
	WithdrawalCancelEnabled      bool
	WithdrawalResetCooldownHours int32
}

func (s *SettingsService) Update(ctx context.Context, req UpdateSettingsRequest) error {
	if req.RakeMode != RakeModePercentage && req.RakeMode != RakeModeSmallBlind {
		return ErrInvalidSettings
	}
	if req.RakePercentage < MinRakePercentage || req.RakePercentage > MaxRakePercentage {
		return ErrInvalidSettings
	}
	if req.ReferralPercentagePctMode < MinReferralPercentage || req.ReferralPercentagePctMode > MaxReferralPercentage {
		return ErrInvalidSettings
	}
	if req.ReferralPercentageSbMode < MinReferralPercentage || req.ReferralPercentageSbMode > MaxReferralPercentage {
		return ErrInvalidSettings
	}
	if req.CountdownSeconds < MinCountdownSeconds || req.CountdownSeconds > MaxCountdownSeconds {
		return ErrInvalidSettings
	}

	if req.IntermissionSeconds == 0 {
		req.IntermissionSeconds = DefaultIntermissionSeconds
	}
	if req.IntermissionSeconds < MinIntermissionSeconds || req.IntermissionSeconds > MaxIntermissionSeconds {
		return ErrInvalidSettings
	}

	if req.WithdrawalMinAmount == 0 {
		req.WithdrawalMinAmount = DefaultWithdrawalMinAmount
	}
	if req.WithdrawalMinAmount < MinWithdrawalMinAmount || req.WithdrawalMinAmount > MaxWithdrawalMinAmount {
		return ErrInvalidSettings
	}
	if req.WithdrawalResetCooldownHours < 0 || req.WithdrawalResetCooldownHours > MaxWithdrawalCooldownHours {
		return ErrInvalidSettings
	}

	accountName := strings.TrimSpace(req.DepositAccountName)
	accountNumber := strings.TrimSpace(req.DepositAccountNumber)
	if req.RealDepositsEnabled && (accountName == "" || accountNumber == "") {
		return ErrDepositAccountRequired
	}
	if len([]rune(accountName)) > MaxDepositAccountName || len(accountNumber) > MaxDepositAccountNumber {
		return ErrInvalidSettings
	}

	return s.q.UpdateSiteSettings(ctx, repository.UpdateSiteSettingsParams{
		RakeMode:                  req.RakeMode,
		RakePercentage:            req.RakePercentage,
		ReferralPercentagePctMode: req.ReferralPercentagePctMode,
		ReferralPercentageSbMode:  req.ReferralPercentageSbMode,
		CountdownSeconds:          req.CountdownSeconds,
		RealDepositsEnabled:       req.RealDepositsEnabled,
		DepositAccountName:        accountName,
		DepositAccountNumber:      accountNumber,
		GatewayDepositsEnabled:    req.GatewayDepositsEnabled,
		IntermissionSeconds:       req.IntermissionSeconds,

		WithdrawalsEnabled:           req.WithdrawalsEnabled,
		WithdrawalMinAmount:          req.WithdrawalMinAmount,
		WithdrawalCancelEnabled:      req.WithdrawalCancelEnabled,
		WithdrawalResetCooldownHours: req.WithdrawalResetCooldownHours,
	})
}
