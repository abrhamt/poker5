package services

import (
	"context"
	"errors"

	"github.com/zuse/poker5/go-poker/repository"
)

const (
	MinRakePercentage float64 = 0
	MaxRakePercentage float64 = 20

	MinReferralPercentage float64 = 0
	MaxReferralPercentage float64 = 50

	MinCountdownSeconds = 5
	MaxCountdownSeconds = 120
)

var ErrInvalidSettings = errors.New("settings value out of allowed range")

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

	return s.q.UpdateSiteSettings(ctx, repository.UpdateSiteSettingsParams{
		RakeMode:                  req.RakeMode,
		RakePercentage:            req.RakePercentage,
		ReferralPercentagePctMode: req.ReferralPercentagePctMode,
		ReferralPercentageSbMode:  req.ReferralPercentageSbMode,
		CountdownSeconds:          req.CountdownSeconds,
	})
}
