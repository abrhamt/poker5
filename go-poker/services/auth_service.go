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
	ErrUserExists      = errors.New("username or phone number already registered")
	ErrInvalidCreds    = errors.New("invalid username/phone or password")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrInvalidReferral = errors.New("invalid referral code")
)

type AuthService struct {
	q *repository.Queries
}

func NewAuthService(q *repository.Queries) *AuthService {
	return &AuthService{q: q}
}

type RegisterRequest struct {
	Username     string `json:"username" form:"username"`
	PhoneNumber  string `json:"phone_number" form:"phone_number"`
	Password     string `json:"password" form:"password"`
	ReferralCode string `json:"referral_code,omitempty" form:"referral_code"`
}

type LoginRequest struct {
	Login    string `json:"login" form:"login"`
	Password string `json:"password" form:"password"`
}

func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*repository.User, error) {
	if req.Username == "" || req.PhoneNumber == "" || req.Password == "" {
		return nil, errors.New("missing required fields")
	}

	normalizedPhone, err := utilities.ValidateAndNormalizeEthiopianPhone(req.PhoneNumber)
	if err != nil {
		return nil, err
	}
	req.PhoneNumber = normalizedPhone

	_, err = s.q.GetUserByUsername(ctx, req.Username)
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
		Wallet:       0,
		ReferralCode: refCode,
		ReferredBy:   referrerCode,
		Role:         "player",
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
			if normPhone, normErr := utilities.ValidateAndNormalizeEthiopianPhone(req.Login); normErr == nil {
				user, err = s.q.GetUserByPhone(ctx, normPhone)
			}
		}
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

// EnsureAdminUser upserts the admin account from configured credentials
// (normally sourced from the environment / .env). Safe to call on every
// startup: if the user already exists it's promoted to admin and its
// password refreshed, otherwise it's created fresh.
func (s *AuthService) EnsureAdminUser(ctx context.Context, username, phoneNumber, password string) (*repository.User, error) {
	if username == "" || phoneNumber == "" || password == "" {
		return nil, errors.New("admin username, phone, and password are required")
	}

	passHash, err := utilities.HashPassword(password)
	if err != nil {
		return nil, err
	}

	existing, err := s.q.GetUserByUsername(ctx, username)
	if err == nil {
		if existing.Role != "admin" {
			_ = s.q.UpdateUserRole(ctx, repository.UpdateUserRoleParams{Role: "admin", ID: existing.ID})
		}
		_ = s.q.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{PasswordHash: passHash, ID: existing.ID})
		if existing.PhoneNumber != phoneNumber {
			_ = s.q.UpdateUserPhone(ctx, repository.UpdateUserPhoneParams{PhoneNumber: phoneNumber, ID: existing.ID})
		}
		user, err := s.q.GetUserByID(ctx, existing.ID)
		return &user, err
	}

	refCode, err := utilities.GenerateReferralCode()
	if err != nil {
		return nil, err
	}

	res, err := s.q.CreateUser(ctx, repository.CreateUserParams{
		Username:     username,
		PhoneNumber:  phoneNumber,
		PasswordHash: passHash,
		Wallet:       0,
		ReferralCode: refCode,
		ReferredBy:   sql.NullString{},
		Role:         "admin",
	})
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	user, err := s.q.GetUserByID(ctx, id)
	return &user, err
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
