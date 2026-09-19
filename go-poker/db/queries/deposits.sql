-- name: CreateBankDeposit :one
INSERT INTO bank_deposits (
    user_id, provider, receipt_url, reference, amount,
    payer_name, payer_account, status, note, transaction_id, resolved_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: GetBankDepositByID :one
SELECT * FROM bank_deposits WHERE id = $1 LIMIT 1;

-- name: GetBankDepositByReference :one
SELECT * FROM bank_deposits WHERE reference = $1 LIMIT 1;

-- name: GetBankDepositByURL :one
SELECT * FROM bank_deposits WHERE receipt_url = $1 LIMIT 1;

-- name: ListBankDepositsForReview :many
SELECT sqlc.embed(d), u.username FROM bank_deposits d
JOIN users u ON u.id = d.user_id
WHERE d.status = $1
ORDER BY d.created_at DESC, d.id DESC
LIMIT $2 OFFSET $3;

-- name: CountBankDepositsByStatus :one
SELECT COUNT(*) FROM bank_deposits WHERE status = $1;

-- name: ListBankDepositsByUser :many
SELECT * FROM bank_deposits
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: ResolveBankDeposit :exec
UPDATE bank_deposits
SET reference = $1,
    amount = $2,
    payer_name = $3,
    payer_account = $4,
    status = $5,
    note = $6,
    transaction_id = $7,
    resolved_at = $8
WHERE id = $9;

-- name: DeleteBankDeposit :exec
DELETE FROM bank_deposits WHERE id = $1;

-- name: ListBankDepositsToRetry :many
SELECT * FROM bank_deposits
WHERE status = $1
  AND attempts < $2
  AND last_attempt_at <= $3
ORDER BY last_attempt_at ASC, id ASC
LIMIT $4;

-- name: NoteBankDepositAttempt :exec
UPDATE bank_deposits
SET note = $1
WHERE id = $2;

-- name: TouchBankDepositAttempt :exec
UPDATE bank_deposits
SET attempts = attempts + 1,
    last_attempt_at = $1,
    note = $2
WHERE id = $3;
