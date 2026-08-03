-- name: CreateUser :execresult
INSERT INTO users (username, phone_number, password_hash, wallet, referral_code, referred_by)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetUserByID :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, created_at
FROM users
WHERE id = ? LIMIT 1;

-- name: GetUserByUsername :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, created_at
FROM users
WHERE username = ? LIMIT 1;

-- name: GetUserByPhone :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, created_at
FROM users
WHERE phone_number = ? LIMIT 1;

-- name: GetUserByReferralCode :one
SELECT id, username, phone_number, password_hash, wallet, referral_code, referred_by, created_at
FROM users
WHERE referral_code = ? LIMIT 1;

-- name: UpdateUserWallet :exec
UPDATE users
SET wallet = wallet + ?
WHERE id = ?;
