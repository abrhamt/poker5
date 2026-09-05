-- name: CreateUser :one
INSERT INTO users (username, phone_number, password_hash, wallet, referral_code, referred_by, role)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: GetUserByID :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE id = $1 LIMIT 1;

-- name: GetUserByUsername :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE username = $1 LIMIT 1;

-- name: GetUserByPhone :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE phone_number = $1 LIMIT 1;

-- name: GetUserByReferralCode :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE referral_code = $1 LIMIT 1;

-- name: UpdateUserWallet :exec
UPDATE users
SET wallet = wallet + $1
WHERE id = $2;

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
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE role != 'house'
ORDER BY id DESC
LIMIT $1 OFFSET $2;

-- name: SearchUsers :many
-- ILIKE, not LIKE: PostgreSQL's LIKE is case-sensitive, so an admin searching
-- for "Alice" would miss the user "alice" entirely.
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE role != 'house' AND (username ILIKE $1 OR phone_number ILIKE $2)
ORDER BY id DESC
LIMIT $3 OFFSET $4;

-- name: CountUsers :one
SELECT COUNT(*) FROM users WHERE role != 'house';
