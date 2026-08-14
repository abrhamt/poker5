-- name: CreateSessionToken :exec
INSERT INTO user_sessions (session_token, user_id, expires_at)
VALUES (?, ?, ?);

-- name: GetUserBySessionToken :one
SELECT u.id, u.username, u.phone_number, u.password_hash, u.wallet, u.referral_code, u.referred_by, u.role, u.created_at
FROM user_sessions s
JOIN users u ON s.user_id = u.id
WHERE s.session_token = ? AND s.expires_at > CURRENT_TIMESTAMP LIMIT 1;


-- name: DeleteSessionToken :exec
DELETE FROM user_sessions WHERE session_token = ?;

-- name: CreateGameSession :execresult
INSERT INTO game_sessions (table_id, hand_number, pot_amount, commission_amount, winner_user_id, winner_name, hand_name, details_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetGameSessionsByTable :many
SELECT id, table_id, hand_number, pot_amount, commission_amount, winner_user_id, winner_name, hand_name, details_json, created_at
FROM game_sessions
WHERE table_id = ?
ORDER BY id DESC LIMIT 20;
