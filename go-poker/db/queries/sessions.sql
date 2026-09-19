-- name: CreateSessionToken :exec
INSERT INTO user_sessions (session_token, user_id, expires_at)
VALUES ($1, $2, $3);

-- name: GetSessionWithUser :one
-- Returns the signed-in user together with the session's own timestamps, so a
-- single round trip can both authenticate the request and decide whether the
-- session is due to be slid forward (see AuthService.GetUserByToken).
SELECT u.id, u.username, u.phone_number, u.password_hash, u.wallet, u.referral_code, u.referred_by, u.role, u.created_at,
       s.expires_at AS session_expires_at,
       s.created_at AS session_created_at
FROM user_sessions s
JOIN users u ON s.user_id = u.id
WHERE s.session_token = $1 AND s.expires_at > NOW() LIMIT 1;

-- name: ExtendSessionToken :exec
UPDATE user_sessions SET expires_at = $1 WHERE session_token = $2;

-- name: DeleteSessionToken :exec
DELETE FROM user_sessions WHERE session_token = $1;

-- name: CreateGameSession :one
INSERT INTO game_sessions (table_id, hand_number, pot_amount, commission_amount, winner_user_id, winner_name, hand_name, details_json)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: GetGameSessionsByTable :many
SELECT id, table_id, hand_number, pot_amount, commission_amount, winner_user_id, winner_name, hand_name, details_json, created_at
FROM game_sessions
WHERE table_id = $1
ORDER BY id DESC LIMIT 20;

-- name: DeleteSessionTokensByUser :exec
-- Used when a password is reset: every other device is signed out, so an
-- attacker who already holds a session cannot survive the victim's remediation.
DELETE FROM user_sessions WHERE user_id = $1;
