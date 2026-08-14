-- name: CreateUser :execresult
INSERT INTO users (username, phone_number, password_hash, wallet, referral_code, referred_by, role)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetUserByID :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE id = ? LIMIT 1;

-- name: GetUserByUsername :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE username = ? LIMIT 1;

-- name: GetUserByPhone :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE phone_number = ? LIMIT 1;

-- name: GetUserByReferralCode :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE referral_code = ? LIMIT 1;

-- name: UpdateUserWallet :exec
UPDATE users
SET wallet = wallet + ?
WHERE id = ?;

-- name: UpdateUserRole :exec
UPDATE users
SET role = ?
WHERE id = ?;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = ?
WHERE id = ?;

-- name: UpdateUserPhone :exec
UPDATE users
SET phone_number = ?
WHERE id = ?;

-- name: ListUsers :many
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE role != 'house'
ORDER BY id DESC
LIMIT ? OFFSET ?;

-- name: SearchUsers :many
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, role, created_at
FROM users
WHERE role != 'house' AND (username LIKE ? OR phone_number LIKE ?)
ORDER BY id DESC
LIMIT ? OFFSET ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users WHERE role != 'house';
