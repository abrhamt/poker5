-- Pending registrations ------------------------------------------------------

-- name: DeleteExpiredPendingRegistrations :exec
-- Run at the top of every send path. This is not housekeeping: an expired row
-- still holds its UNIQUE claim on a username, so deleting it is a precondition
-- of the insert that follows, not a background chore.
DELETE FROM pending_registrations WHERE expires_at < $1;

-- name: DeletePendingRegistrationByPhone :exec
DELETE FROM pending_registrations WHERE phone_number = $1;

-- name: DeletePendingRegistrationByID :exec
DELETE FROM pending_registrations WHERE id = $1;

-- name: CreatePendingRegistration :exec
INSERT INTO pending_registrations (username, phone_number, password_hash, referred_by, code, attempts, expires_at)
VALUES ($1, $2, $3, $4, $5, 0, $6);

-- name: GetPendingRegistrationByPhone :one
SELECT id, username, phone_number, password_hash, referred_by, code, attempts, expires_at, created_at
FROM pending_registrations
WHERE phone_number = $1 LIMIT 1;

-- name: GetPendingRegistrationByUsername :one
SELECT id, username, phone_number, password_hash, referred_by, code, attempts, expires_at, created_at
FROM pending_registrations
WHERE username = $1 LIMIT 1;

-- name: IncrementPendingRegistrationAttempts :exec
UPDATE pending_registrations SET attempts = attempts + 1 WHERE id = $1;

-- name: RefreshPendingRegistrationCode :exec
-- Resend mints a new code and clears the attempt counter, so the most recent
-- SMS is always the one that works and a fresh code earns a fresh five tries.
UPDATE pending_registrations SET code = $1, expires_at = $2, attempts = 0 WHERE id = $3;

-- Password reset codes -------------------------------------------------------

-- name: DeleteExpiredPasswordResetCodes :exec
-- A row survives until both of its clocks are dead: the code may be spent while
-- the token it minted is still live.
DELETE FROM password_reset_codes WHERE expires_at < $1 AND token_expires_at < $2;

-- name: DeletePasswordResetCodeByPhone :exec
DELETE FROM password_reset_codes WHERE phone_number = $1;

-- name: DeletePasswordResetCodeByID :exec
DELETE FROM password_reset_codes WHERE id = $1;

-- name: CreatePasswordResetCode :exec
INSERT INTO password_reset_codes (user_id, phone_number, code, attempts, expires_at, reset_token, token_expires_at)
VALUES ($1, $2, $3, 0, $4, NULL, 0);

-- name: GetPasswordResetCodeByPhone :one
SELECT id, user_id, phone_number, code, attempts, expires_at, reset_token, token_expires_at, created_at
FROM password_reset_codes
WHERE phone_number = $1 LIMIT 1;

-- name: GetPasswordResetCodeByToken :one
-- The user this reset acts on comes from here, never from the request body.
SELECT id, user_id, phone_number, code, attempts, expires_at, reset_token, token_expires_at, created_at
FROM password_reset_codes
WHERE reset_token = $1 LIMIT 1;

-- name: IncrementPasswordResetAttempts :exec
UPDATE password_reset_codes SET attempts = attempts + 1 WHERE id = $1;

-- name: RefreshPasswordResetCode :exec
UPDATE password_reset_codes SET code = $1, expires_at = $2, attempts = 0, reset_token = NULL, token_expires_at = 0 WHERE id = $3;

-- name: SetPasswordResetToken :exec
-- Verifying the code burns it (code is blanked) and mints the token in one write.
UPDATE password_reset_codes SET reset_token = $1, token_expires_at = $2, code = '', expires_at = 0 WHERE id = $3;

-- OTP send throttle ----------------------------------------------------------

-- name: GetOTPThrottle :one
SELECT id, phone_number, purpose, window_started_at, last_sent_at, send_count
FROM otp_send_throttle
WHERE phone_number = $1 AND purpose = $2 LIMIT 1;

-- name: CreateOTPThrottle :exec
INSERT INTO otp_send_throttle (phone_number, purpose, window_started_at, last_sent_at, send_count)
VALUES ($1, $2, $3, $4, 1);

-- name: UpdateOTPThrottle :exec
UPDATE otp_send_throttle SET window_started_at = $1, last_sent_at = $2, send_count = $3 WHERE id = $4;

-- name: DeleteStaleOTPThrottles :exec
DELETE FROM otp_send_throttle WHERE window_started_at < $1;
