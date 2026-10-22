-- name: CreateWithdrawal :one
INSERT INTO withdrawals (user_id, method, account_number, account_name, amount, debit_transaction_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetWithdrawalByID :one
SELECT * FROM withdrawals WHERE id = $1 LIMIT 1;

-- name: GetPendingWithdrawalByUser :one
SELECT * FROM withdrawals WHERE user_id = $1 AND status = 'pending' LIMIT 1;

-- name: ListWithdrawalsByUser :many
SELECT * FROM withdrawals
WHERE user_id = $1
ORDER BY id DESC
LIMIT $2;

-- name: ResolveWithdrawal :one
-- The status guard makes resolving a claim: of two admins (or two clicks)
-- acting on the same request, exactly one gets a row back, so a rejection can
-- never refund twice. The row stays locked until the caller's transaction
-- commits.
UPDATE withdrawals
SET status = sqlc.arg(status),
    note = sqlc.arg(note),
    payout_reference = sqlc.arg(payout_reference),
    resolved_by = sqlc.arg(resolved_by),
    resolved_at = sqlc.arg(resolved_at)
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: SetWithdrawalRefundTransaction :exec
UPDATE withdrawals SET refund_transaction_id = $1 WHERE id = $2;

-- name: ListWithdrawalsForAdmin :many
-- Pending requests are a queue and come oldest first, so whoever asked first
-- is paid first; every other status is history and comes newest first.
SELECT sqlc.embed(w), u.username, u.phone_number
FROM withdrawals w
JOIN users u ON u.id = w.user_id
WHERE (sqlc.arg(status)::text = '' OR w.status = sqlc.arg(status)::text)
  AND (sqlc.arg(user_id)::bigint = 0 OR w.user_id = sqlc.arg(user_id)::bigint)
ORDER BY CASE WHEN w.status = 'pending' THEN w.id END ASC, w.id DESC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountWithdrawalsForAdmin :one
SELECT COUNT(*) FROM withdrawals
WHERE (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
  AND (sqlc.arg(user_id)::bigint = 0 OR user_id = sqlc.arg(user_id)::bigint);

-- name: SumWithdrawalAmountByStatus :one
SELECT COALESCE(SUM(amount), 0)::BIGINT AS total FROM withdrawals WHERE status = $1;
