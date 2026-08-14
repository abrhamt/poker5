-- name: CreateTransaction :execresult
INSERT INTO transactions (user_id, amount, type, reason, transaction_id)
VALUES (?, ?, ?, ?, ?);

-- name: GetTransactionsByUserID :many
SELECT id, user_id, amount, type, reason, transaction_id, created_at
FROM transactions
WHERE user_id = ?
ORDER BY id DESC LIMIT 50;

-- name: FilterTransactions :many
SELECT t.id, t.user_id, t.amount, t.type, t.reason, t.transaction_id, t.created_at, u.username
FROM transactions t
JOIN users u ON u.id = t.user_id
WHERE u.username LIKE ?
  AND (sqlc.arg(type_filter) = '' OR t.type = sqlc.arg(type_filter))
ORDER BY t.id DESC
LIMIT ? OFFSET ?;

-- name: SumTransactionAmountByTypeSince :one
SELECT CAST(COALESCE(SUM(amount), 0) AS SIGNED) AS total
FROM transactions
WHERE type = ? AND created_at >= ?;
