-- name: CreateGatewayDeposit :one
INSERT INTO gateway_deposits (user_id, reference, amount, status)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: GetGatewayDepositByReference :one
SELECT * FROM gateway_deposits WHERE reference = $1 LIMIT 1;

-- name: GetGatewayDepositForUpdate :one
SELECT * FROM gateway_deposits WHERE reference = $1 FOR UPDATE;

-- name: AttachGatewayCheckout :exec
UPDATE gateway_deposits
SET router_payment_id = $1,
    checkout_url = $2
WHERE id = $3;

-- name: ResolveGatewayDeposit :exec
UPDATE gateway_deposits
SET status = $1,
    transaction_id = $2,
    last_event = $3,
    note = $4,
    resolved_at = $5
WHERE id = $6;

-- name: ListGatewayDepositsPendingSince :many
SELECT * FROM gateway_deposits
WHERE status = $1
  AND router_payment_id <> ''
  AND created_at <= $2
ORDER BY created_at ASC, id ASC
LIMIT $3;

-- name: ListGatewayDepositsByUser :many
SELECT * FROM gateway_deposits
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;
