-- name: CreateTransaction :execresult
INSERT INTO transactions (user_id, amount, reason, transaction_id)
VALUES (?, ?, ?, ?);

-- name: GetTransactionsByUserID :many
SELECT id, user_id, amount, reason, transaction_id, created_at
FROM transactions
WHERE user_id = ?
ORDER BY id DESC LIMIT 50;
