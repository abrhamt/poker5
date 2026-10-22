-- name: CreateTransaction :one
INSERT INTO transactions (user_id, amount, type, reason, transaction_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: DebitWalletWithTransaction :one
-- Takes money out of a wallet and records why, as one statement: the balance
-- check, the debit and the ledger row cannot be split by a concurrent debit or
-- a failure between them. No row back means the balance was short.
WITH debited AS (
    UPDATE users
    SET wallet = wallet - sqlc.arg(amount)::bigint
    WHERE users.id = sqlc.arg(user_id)::bigint AND users.wallet >= sqlc.arg(amount)::bigint
    RETURNING users.id AS debited_user_id
)
INSERT INTO transactions (user_id, amount, type, reason, transaction_id)
SELECT debited.debited_user_id, -sqlc.arg(amount)::bigint, sqlc.arg(type), sqlc.arg(reason), sqlc.arg(transaction_id)
FROM debited
RETURNING transactions.id;

-- name: ListTransactionsByUserID :many
SELECT id, user_id, amount, type, reason, transaction_id, created_at
FROM transactions
WHERE user_id = $1
ORDER BY id DESC
LIMIT $2 OFFSET $3;

-- name: CountTransactionsByUserID :one
SELECT COUNT(*) AS total
FROM transactions
WHERE user_id = $1;

-- name: FilterTransactions :many
SELECT t.id, t.user_id, t.amount, t.type, t.reason, t.transaction_id, t.created_at, u.username
FROM transactions t
JOIN users u ON u.id = t.user_id
WHERE u.username ILIKE sqlc.arg(username)::text
  AND (sqlc.arg(type_filter)::text = '' OR t.type = sqlc.arg(type_filter)::text)
ORDER BY t.id DESC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountFilterTransactions :one
SELECT COUNT(*)
FROM transactions t
JOIN users u ON u.id = t.user_id
WHERE u.username ILIKE sqlc.arg(username)::text
  AND (sqlc.arg(type_filter)::text = '' OR t.type = sqlc.arg(type_filter)::text);

-- name: SumTransactionAmountByTypeSince :one
-- The cast is load-bearing: SUM() over a bigint column is numeric in
-- PostgreSQL, which arrives in Go as a byte slice rather than an integer.
SELECT COALESCE(SUM(amount), 0)::BIGINT AS total
FROM transactions
WHERE type = $1 AND created_at >= $2;
