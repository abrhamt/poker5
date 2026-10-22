-- name: CreateUser :one
INSERT INTO users (username, phone_number, password_hash, wallet, referral_code, referred_by, role)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: GetUserByID :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at, avatar
FROM users
WHERE id = $1 LIMIT 1;

-- name: GetUserByUsername :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at, avatar
FROM users
WHERE username = $1 LIMIT 1;

-- name: GetUserByPhone :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at, avatar
FROM users
WHERE phone_number = $1 LIMIT 1;

-- name: GetUserByReferralCode :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at, avatar
FROM users
WHERE referral_code = $1 LIMIT 1;

-- name: UpdateUserWallet :exec
UPDATE users
SET wallet = wallet + $1
WHERE id = $2;

-- name: DebitUserWalletIfSufficient :execrows
-- The balance check and the debit are one statement, so two debits racing each
-- other cannot both pass a check that only one of them can afford. Zero rows
-- affected means the balance was short.
UPDATE users
SET wallet = wallet - sqlc.arg(amount)
WHERE id = sqlc.arg(id) AND wallet >= sqlc.arg(amount);

-- name: UpdateUserRole :exec
UPDATE users
SET role = $1
WHERE id = $2;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $1
WHERE id = $2;

-- name: UpdateUserPhone :exec
UPDATE users
SET phone_number = $1
WHERE id = $2;

-- name: ListUsers :many
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at, avatar
FROM users
WHERE role != 'house'
ORDER BY id DESC
LIMIT $1 OFFSET $2;

-- name: SearchUsers :many
-- ILIKE, not LIKE: PostgreSQL's LIKE is case-sensitive, so an admin searching
-- for "Alice" would miss the user "alice" entirely.
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at, avatar
FROM users
WHERE role != 'house' AND (username ILIKE $1 OR phone_number ILIKE $2)
ORDER BY id DESC
LIMIT $3 OFFSET $4;

-- name: CountUsers :one
SELECT COUNT(*) FROM users WHERE role != 'house';

-- name: CountSearchUsers :one
SELECT COUNT(*) FROM users
WHERE role != 'house' AND (username ILIKE $1 OR phone_number ILIKE $2);

-- name: RecordPasswordReset :exec
INSERT INTO password_resets (user_id, reset_at)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET reset_at = EXCLUDED.reset_at;

-- name: GetPasswordResetAt :one
SELECT reset_at FROM password_resets WHERE user_id = $1;

-- name: UpdateUserAvatar :exec
UPDATE users
SET avatar = $1
WHERE id = $2;

-- name: UpdateUsername :exec
UPDATE users
SET username = $1
WHERE id = $2;
