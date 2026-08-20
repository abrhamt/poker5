package services

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

// Session lifetime. Expiry slides: a session that keeps being used keeps being
// renewed, so a player who logs in every few days is never signed out — which
// matters here because being signed out mid-hand leaves their chips on the
// felt, auto-folding on the turn timer until they get back in.
const (
	// SessionIdleTTL is how long a session survives without being used.
	SessionIdleTTL = 7 * 24 * time.Hour

	// SessionAbsoluteTTL caps how long one session can be kept alive by
	// sliding, no matter how active the player is. Past this it expires and
	// they sign in again.
	SessionAbsoluteTTL = 90 * 24 * time.Hour

	// sessionRenewWindow is how little life must remain before a session is
	// slid forward. Set just under SessionIdleTTL so an active player costs
	// roughly one UPDATE per day rather than one per request — the table view
	// re-reads state on every broadcast, so per-request writes would be a lot.
	sessionRenewWindow = 6 * 24 * time.Hour
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
	expiresAt := time.Now().Add(SessionIdleTTL)

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
	row, err := s.q.GetSessionWithUser(ctx, token)
	if err != nil {
		return nil, ErrUnauthorized
	}

	s.slideSession(ctx, token, row.SessionCreatedAt, row.SessionExpiresAt)

	return &repository.User{
		ID:           row.ID,
		Username:     row.Username,
		PhoneNumber:  row.PhoneNumber,
		PasswordHash: row.PasswordHash,
		Wallet:       row.Wallet,
		ReferralCode: row.ReferralCode,
		ReferredBy:   row.ReferredBy,
		Role:         row.Role,
		CreatedAt:    row.CreatedAt,
	}, nil
}

// slideSession pushes a session's expiry back out to a full idle window, but
// only once it is close enough to expiring to be worth a write, and never past
// the absolute ceiling measured from when the session was first issued.
//
// Failures are deliberately swallowed: the caller has already authenticated
// successfully, and refusing the request because a bookkeeping write failed
// would be a worse outcome than a session that expires on its original
// schedule.
func (s *AuthService) slideSession(ctx context.Context, token string, createdAt, expiresAt time.Time) {
	now := time.Now()
	if expiresAt.Sub(now) > sessionRenewWindow {
		return
	}

	ceiling := createdAt.Add(SessionAbsoluteTTL)
	if !now.Before(ceiling) {
		return
	}

	renewed := now.Add(SessionIdleTTL)
	if renewed.After(ceiling) {
		renewed = ceiling
	}

	_ = s.q.ExtendSessionToken(ctx, repository.ExtendSessionTokenParams{
		ExpiresAt:    renewed,
		SessionToken: token,
	})
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.q.DeleteSessionToken(ctx, token)
}
