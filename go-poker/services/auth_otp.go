package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

// OTP policy. The code's TTL only has to cover SMS delivery plus typing — the
// reset token carries its own, longer clock, so choosing a new password can't
// expire the code out from under someone who already proved they hold the
// phone.
const (
	otpCodeTTL           = 5 * time.Minute
	resetTokenTTL        = 10 * time.Minute
	otpMaxAttempts       = 5
	otpResendCooldown    = 60 * time.Second
	otpSendWindow        = time.Hour
	otpMaxSendsPerWindow = 5

	// otpSendTimeout bounds how long a provider can hold up a registration
	// request. The send is synchronous on purpose — "we sent it" has to be
	// true when we say it — which means a hung provider would otherwise hang
	// the handler.
	otpSendTimeout = 20 * time.Second
)

var (
	ErrInvalidOTP            = errors.New("incorrect code")
	ErrOTPExpired            = errors.New("that code has expired — request a new one")
	ErrOTPBurned             = errors.New("too many incorrect attempts — request a new code")
	ErrNoPendingRegistration = errors.New("no pending registration for that phone number — start again")
	ErrNoPendingReset        = errors.New("no reset in progress for that phone number — start again")
	ErrUserNotFound          = errors.New("no account found for that phone number")
	ErrInvalidResetToken     = errors.New("this password reset has expired — start again")
	ErrResetCodeSpent        = errors.New("that code has already been used — start again if you didn't finish")
	ErrSendFailed            = errors.New("couldn't send the code right now — please try again")
)

// ThrottleError reports a refused send. It carries the wait so the handler can
// state the exact number of seconds rather than a vague "try again later".
type ThrottleError struct {
	RetryAfter time.Duration
	message    string
}

func (e *ThrottleError) Error() string { return e.message }

// StartRegistration validates a signup, stashes it as pending, and sends the
// code. No users row exists until VerifyRegistration succeeds, so an abandoned
// signup expires on its own instead of squatting a username forever.
// The normalized phone is returned so the client can carry it to the verify
// screen without having to re-derive the canonical form.
func (s *AuthService) StartRegistration(ctx context.Context, req RegisterRequest) (string, error) {
	if req.Username == "" || req.PhoneNumber == "" || req.Password == "" {
		return "", errors.New("missing required fields")
	}
	if err := utilities.ValidatePassword(req.Password); err != nil {
		return "", err
	}
	phone, err := utilities.ValidateAndNormalizeEthiopianPhone(req.PhoneNumber)
	if err != nil {
		return "", err
	}

	s.sweepExpiredOTPs(ctx)

	if _, err := s.q.GetUserByUsername(ctx, req.Username); err == nil {
		return "", ErrUserExists
	}
	if _, err := s.q.GetUserByPhone(ctx, phone); err == nil {
		return "", ErrUserExists
	}
	// A live pending signup holds its username the same way a real account
	// does, so the collision is reported here — before any SMS is paid for —
	// rather than after the user has verified their phone.
	if existing, err := s.q.GetPendingRegistrationByUsername(ctx, req.Username); err == nil && existing.PhoneNumber != phone {
		return "", ErrUserExists
	}

	var referrerCode sql.NullString
	if req.ReferralCode != "" {
		refUser, err := s.q.GetUserByReferralCode(ctx, req.ReferralCode)
		if err != nil {
			return "", ErrInvalidReferral
		}
		referrerCode = sql.NullString{String: refUser.ReferralCode, Valid: true}
	}

	passHash, err := utilities.HashPassword(req.Password)
	if err != nil {
		return "", err
	}
	code, err := utilities.GenerateOTPCode()
	if err != nil {
		return "", err
	}

	if err := s.consumeSendAllowance(ctx, phone, PurposeRegister); err != nil {
		return "", err
	}

	// One pending row per phone: restarting a signup replaces the previous
	// attempt rather than stacking up rows that each hold a username.
	if err := s.q.DeletePendingRegistrationByPhone(ctx, phone); err != nil {
		return "", err
	}
	if err := s.q.CreatePendingRegistration(ctx, repository.CreatePendingRegistrationParams{
		Username:     req.Username,
		PhoneNumber:  phone,
		PasswordHash: passHash,
		ReferredBy:   referrerCode,
		Code:         code,
		ExpiresAt:    time.Now().Add(otpCodeTTL).Unix(),
	}); err != nil {
		return "", err
	}

	if err := s.sendCode(ctx, phone, code, PurposeRegister); err != nil {
		// The user was told nothing happened, so leave nothing behind — a
		// surviving row would hold their username against their own retry.
		_ = s.q.DeletePendingRegistrationByPhone(ctx, phone)
		return "", err
	}
	return phone, nil
}

// VerifyRegistration turns a proven pending signup into a real account and
// signs the new player in. The returned token is empty if the account was
// created but the session write failed — the caller should still report
// success, since the account exists and the password they just chose works.
func (s *AuthService) VerifyRegistration(ctx context.Context, phone, code string) (*repository.User, string, error) {
	normalized, err := utilities.ValidateAndNormalizeEthiopianPhone(phone)
	if err != nil {
		return nil, "", err
	}

	pending, err := s.q.GetPendingRegistrationByPhone(ctx, normalized)
	if err != nil {
		return nil, "", ErrNoPendingRegistration
	}
	if err := s.checkCode(ctx, code, pending.Code, pending.Attempts, pending.ExpiresAt, func() {
		_ = s.q.IncrementPendingRegistrationAttempts(ctx, pending.ID)
	}, func() {
		_ = s.q.DeletePendingRegistrationByID(ctx, pending.ID)
	}); err != nil {
		return nil, "", err
	}

	// Re-check uniqueness: the reservation makes this window milliseconds
	// wide, but a real account could have completed inside it.
	if _, err := s.q.GetUserByUsername(ctx, pending.Username); err == nil {
		_ = s.q.DeletePendingRegistrationByID(ctx, pending.ID)
		return nil, "", ErrUserExists
	}
	if _, err := s.q.GetUserByPhone(ctx, pending.PhoneNumber); err == nil {
		_ = s.q.DeletePendingRegistrationByID(ctx, pending.ID)
		return nil, "", ErrUserExists
	}

	refCode, err := utilities.GenerateReferralCode()
	if err != nil {
		return nil, "", err
	}

	// Creating the user and clearing the pending row have to agree: a crash
	// between them would leave a live code for an account that already exists,
	// and the retry would fail on the UNIQUE constraint.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.q.WithTx(tx)
	id, err := qtx.CreateUser(ctx, repository.CreateUserParams{
		Username:     pending.Username,
		PhoneNumber:  pending.PhoneNumber,
		PasswordHash: pending.PasswordHash,
		Wallet:       0,
		ReferralCode: refCode,
		ReferredBy:   pending.ReferredBy,
		Role:         "player",
	})
	if err != nil {
		return nil, "", err
	}
	if err := qtx.DeletePendingRegistrationByID(ctx, pending.ID); err != nil {
		return nil, "", err
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	user, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		return nil, "", err
	}

	token, err := s.issueSession(ctx, user.ID)
	if err != nil {
		log.Printf("verify registration: account %d created but session failed: %v", user.ID, err)
		return &user, "", nil
	}
	return &user, token, nil
}

// ResendOTP mints a fresh code for an in-flight flow and invalidates the old
// one, so the most recent SMS is always the one that works — the only rule a
// user can actually follow when messages arrive out of order.
func (s *AuthService) ResendOTP(ctx context.Context, phone string, purpose OTPPurpose) error {
	normalized, err := utilities.ValidateAndNormalizeEthiopianPhone(phone)
	if err != nil {
		return err
	}
	s.sweepExpiredOTPs(ctx)

	code, err := utilities.GenerateOTPCode()
	if err != nil {
		return err
	}
	expires := time.Now().Add(otpCodeTTL).Unix()

	switch purpose {
	case PurposeReset:
		row, err := s.q.GetPasswordResetCodeByPhone(ctx, normalized)
		if err != nil {
			return ErrNoPendingReset
		}
		if err := s.consumeSendAllowance(ctx, normalized, PurposeReset); err != nil {
			return err
		}
		if err := s.q.RefreshPasswordResetCode(ctx, repository.RefreshPasswordResetCodeParams{
			Code: code, ExpiresAt: expires, ID: row.ID,
		}); err != nil {
			return err
		}
	default:
		row, err := s.q.GetPendingRegistrationByPhone(ctx, normalized)
		if err != nil {
			return ErrNoPendingRegistration
		}
		if err := s.consumeSendAllowance(ctx, normalized, PurposeRegister); err != nil {
			return err
		}
		if err := s.q.RefreshPendingRegistrationCode(ctx, repository.RefreshPendingRegistrationCodeParams{
			Code: code, ExpiresAt: expires, ID: row.ID,
		}); err != nil {
			return err
		}
	}

	// No rollback here: unlike a first send, the pending record predates this
	// call and deleting it would destroy work the user has already done. The
	// old code is gone either way, so they resend.
	return s.sendCode(ctx, normalized, code, purpose)
}

// StartPasswordReset sends a reset code. It answers honestly when no account
// exists: register already discloses the same fact, so a vague response here
// would cost usability without closing enumeration.
func (s *AuthService) StartPasswordReset(ctx context.Context, phone string) (string, error) {
	normalized, err := utilities.ValidateAndNormalizeEthiopianPhone(phone)
	if err != nil {
		return "", err
	}
	s.sweepExpiredOTPs(ctx)

	user, err := s.q.GetUserByPhone(ctx, normalized)
	if err != nil {
		return "", ErrUserNotFound
	}

	code, err := utilities.GenerateOTPCode()
	if err != nil {
		return "", err
	}
	if err := s.consumeSendAllowance(ctx, normalized, PurposeReset); err != nil {
		return "", err
	}

	if err := s.q.DeletePasswordResetCodeByPhone(ctx, normalized); err != nil {
		return "", err
	}
	if err := s.q.CreatePasswordResetCode(ctx, repository.CreatePasswordResetCodeParams{
		UserID:      user.ID,
		PhoneNumber: normalized,
		Code:        code,
		ExpiresAt:   time.Now().Add(otpCodeTTL).Unix(),
	}); err != nil {
		return "", err
	}

	if err := s.sendCode(ctx, normalized, code, PurposeReset); err != nil {
		_ = s.q.DeletePasswordResetCodeByPhone(ctx, normalized)
		return "", err
	}
	return normalized, nil
}

// VerifyResetOTP spends the code and mints the token that authorizes the
// password change. Only the token's SHA-256 digest is stored — 32 random bytes
// have the entropy for hashing to mean something, unlike a six-digit code.
func (s *AuthService) VerifyResetOTP(ctx context.Context, phone, code string) (string, error) {
	normalized, err := utilities.ValidateAndNormalizeEthiopianPhone(phone)
	if err != nil {
		return "", err
	}

	row, err := s.q.GetPasswordResetCodeByPhone(ctx, normalized)
	if err != nil {
		return "", ErrNoPendingReset
	}
	// A spent code leaves its row alive to carry the token it minted. Bail out
	// before the expiry check below, which would otherwise read the burned
	// code's zeroed clock as "expired" and delete the row — letting anyone who
	// replays a used code destroy a reset that is already in progress.
	if row.ResetToken.Valid {
		return "", ErrResetCodeSpent
	}
	if err := s.checkCode(ctx, code, row.Code, row.Attempts, row.ExpiresAt, func() {
		_ = s.q.IncrementPasswordResetAttempts(ctx, row.ID)
	}, func() {
		_ = s.q.DeletePasswordResetCodeByID(ctx, row.ID)
	}); err != nil {
		return "", err
	}

	token, digest, err := utilities.GenerateResetToken()
	if err != nil {
		return "", err
	}
	if err := s.q.SetPasswordResetToken(ctx, repository.SetPasswordResetTokenParams{
		ResetToken:     sql.NullString{String: digest, Valid: true},
		TokenExpiresAt: time.Now().Add(resetTokenTTL).Unix(),
		ID:             row.ID,
	}); err != nil {
		return "", err
	}
	return token, nil
}

// ResetPassword sets the new password and signs the user in on this device
// only. Every other session is destroyed: the reason people reset passwords is
// that someone else may hold theirs, and a reset that leaves the attacker's
// session alive accomplishes nothing.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) (*repository.User, string, error) {
	if err := utilities.ValidatePassword(newPassword); err != nil {
		return nil, "", err
	}
	if token == "" {
		return nil, "", ErrInvalidResetToken
	}

	// The account comes from the token's own row. Taking it from the request
	// would let a token issued for one phone change another user's password.
	row, err := s.q.GetPasswordResetCodeByToken(ctx, sql.NullString{String: utilities.HashResetToken(token), Valid: true})
	if err != nil {
		return nil, "", ErrInvalidResetToken
	}
	if time.Now().Unix() > row.TokenExpiresAt {
		_ = s.q.DeletePasswordResetCodeByID(ctx, row.ID)
		return nil, "", ErrInvalidResetToken
	}

	passHash, err := utilities.HashPassword(newPassword)
	if err != nil {
		return nil, "", err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.q.WithTx(tx)
	if err := qtx.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{PasswordHash: passHash, ID: row.UserID}); err != nil {
		return nil, "", err
	}
	if err := qtx.DeletePasswordResetCodeByID(ctx, row.ID); err != nil {
		return nil, "", err
	}
	if err := qtx.DeleteSessionTokensByUser(ctx, row.UserID); err != nil {
		return nil, "", err
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	user, err := s.q.GetUserByID(ctx, row.UserID)
	if err != nil {
		return nil, "", err
	}

	// Issued after the wipe, so the device that performed the reset stays in.
	token, err = s.issueSession(ctx, user.ID)
	if err != nil {
		log.Printf("reset password: password changed for %d but session failed: %v", user.ID, err)
		return &user, "", nil
	}
	return &user, token, nil
}

// checkCode applies the shared rules both flows use: expiry, the attempt cap,
// and a constant-time comparison. onWrong records a failed attempt; onExpired
// disposes of a record that can no longer be used for anything.
func (s *AuthService) checkCode(ctx context.Context, supplied, stored string, attempts int32, expiresAt int64, onWrong, onExpired func()) error {
	_ = ctx
	if time.Now().Unix() > expiresAt {
		onExpired()
		return ErrOTPExpired
	}
	// A burned code keeps its record: resend needs the payload attached to it,
	// and a fresh code earns a fresh five tries.
	if attempts >= otpMaxAttempts {
		return ErrOTPBurned
	}
	if !utilities.CodesMatch(supplied, stored) {
		onWrong()
		if attempts+1 >= otpMaxAttempts {
			return ErrOTPBurned
		}
		return ErrInvalidOTP
	}
	return nil
}

func (s *AuthService) issueSession(ctx context.Context, userID int64) (string, error) {
	token := utilities.GenerateSessionToken()
	if err := s.q.CreateSessionToken(ctx, repository.CreateSessionTokenParams{
		SessionToken: token,
		UserID:       userID,
		ExpiresAt:    time.Now().Add(SessionIdleTTL),
	}); err != nil {
		return "", err
	}
	return token, nil
}

func (s *AuthService) sendCode(ctx context.Context, phone, code string, purpose OTPPurpose) error {
	sendCtx, cancel := context.WithTimeout(ctx, otpSendTimeout)
	defer cancel()

	if err := s.sender.Send(sendCtx, phone, RenderOTPMessage(purpose, code)); err != nil {
		log.Printf("otp send failed (%s): %v", purpose, err)
		return ErrSendFailed
	}
	return nil
}

// consumeSendAllowance records a send against the per-phone budget and refuses
// one that is too soon or over the hourly cap. It is called before the send and
// never rolled back: an attacker who can induce send failures must not be able
// to reset the counter by causing them.
func (s *AuthService) consumeSendAllowance(ctx context.Context, phone string, purpose OTPPurpose) error {
	now := time.Now()

	row, err := s.q.GetOTPThrottle(ctx, repository.GetOTPThrottleParams{
		PhoneNumber: phone,
		Purpose:     string(purpose),
	})
	if err != nil {
		return s.q.CreateOTPThrottle(ctx, repository.CreateOTPThrottleParams{
			PhoneNumber:     phone,
			Purpose:         string(purpose),
			WindowStartedAt: now.Unix(),
			LastSentAt:      now.Unix(),
		})
	}

	if wait := otpResendCooldown - now.Sub(time.Unix(row.LastSentAt, 0)); wait > 0 {
		return &ThrottleError{
			RetryAfter: wait,
			message:    fmt.Sprintf("Please wait %d seconds before requesting another code.", int(wait.Seconds())+1),
		}
	}

	windowStart := time.Unix(row.WindowStartedAt, 0)
	if now.Sub(windowStart) >= otpSendWindow {
		windowStart = now
		return s.q.UpdateOTPThrottle(ctx, repository.UpdateOTPThrottleParams{
			WindowStartedAt: windowStart.Unix(),
			LastSentAt:      now.Unix(),
			SendCount:       1,
			ID:              row.ID,
		})
	}

	if row.SendCount >= otpMaxSendsPerWindow {
		wait := otpSendWindow - now.Sub(windowStart)
		return &ThrottleError{
			RetryAfter: wait,
			message:    fmt.Sprintf("Too many code requests for this number. Try again in %d minutes.", int(wait.Minutes())+1),
		}
	}

	return s.q.UpdateOTPThrottle(ctx, repository.UpdateOTPThrottleParams{
		WindowStartedAt: windowStart.Unix(),
		LastSentAt:      now.Unix(),
		SendCount:       row.SendCount + 1,
		ID:              row.ID,
	})
}

// sweepExpiredOTPs deletes dead rows at the top of every send path. This is a
// precondition rather than housekeeping: an expired pending signup still holds
// a UNIQUE claim on its username, and the call most likely to collide with it
// is the one happening right now.
func (s *AuthService) sweepExpiredOTPs(ctx context.Context) {
	now := time.Now().Unix()
	_ = s.q.DeleteExpiredPendingRegistrations(ctx, now)
	_ = s.q.DeleteExpiredPasswordResetCodes(ctx, repository.DeleteExpiredPasswordResetCodesParams{
		ExpiresAt:      now,
		TokenExpiresAt: now,
	})
	// Throttle rows outlive their window by design; drop them only once they
	// can no longer affect a decision.
	_ = s.q.DeleteStaleOTPThrottles(ctx, time.Now().Add(-2*otpSendWindow).Unix())
}
