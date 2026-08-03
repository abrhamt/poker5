package services

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

var (
	ErrUserExists        = errors.New("username or phone number already registered")
	ErrInvalidCreds      = errors.New("invalid username/phone or password")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrInvalidReferral   = errors.New("invalid referral code")
)

type AuthService struct {
	q *repository.Queries
}

func NewAuthService(q *repository.Queries) *AuthService {
	return &AuthService{q: q}
}

type RegisterRequest struct {
	Username     string `json:"username"`
	PhoneNumber  string `json:"phone_number"`
	Password     string `json:"password"`
	ReferralCode string `json:"referral_code,omitempty"`
}

type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*repository.User, error) {
	if req.Username == "" || req.PhoneNumber == "" || req.Password == "" {
		return nil, errors.New("missing required fields")
	}

	_, err := s.q.GetUserByUsername(ctx, req.Username)
	if err == nil {
		return nil, ErrUserExists
	}

	_, err = s.q.GetUserByPhone(ctx, req.PhoneNumber)
	if err == nil {
		return nil, ErrUserExists
	}

	var referrerCode sql.NullString
	if req.ReferralCode != "" {
		refUser, err := s.q.GetUserByReferralCode(ctx, req.ReferralCode)
		if err != nil {
			return nil, ErrInvalidReferral
		}
		referrerCode = sql.NullString{String: refUser.ReferralCode, Valid: true}
	}

	passHash, err := utilities.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	refCode, err := utilities.GenerateReferralCode()
	if err != nil {
		return nil, err
	}

	res, err := s.q.CreateUser(ctx, repository.CreateUserParams{
		Username:     req.Username,
		PhoneNumber:  req.PhoneNumber,
		PasswordHash: passHash,
		Wallet:       "0.00",
		ReferralCode: refCode,
		ReferredBy:   referrerCode,
	})
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	user, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*repository.User, string, error) {
	var user repository.User
	var err error

	user, err = s.q.GetUserByUsername(ctx, req.Login)
	if err != nil {
		user, err = s.q.GetUserByPhone(ctx, req.Login)
		if err != nil {
			return nil, "", ErrInvalidCreds
		}
	}

	if !utilities.CheckPassword(req.Password, user.PasswordHash) {
		return nil, "", ErrInvalidCreds
	}

	token := utilities.GenerateSessionToken()
	expiresAt := time.Now().Add(24 * 7 * time.Hour)

	err = s.q.CreateSessionToken(ctx, repository.CreateSessionTokenParams{
		SessionToken: token,
		UserID:       user.ID,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		return nil, "", err
	}

	return &user, token, nil
}

func (s *AuthService) GetUserByToken(ctx context.Context, token string) (*repository.User, error) {
	if token == "" {
		return nil, ErrUnauthorized
	}
	user, err := s.q.GetUserBySessionToken(ctx, token)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return &user, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.q.DeleteSessionToken(ctx, token)
}
