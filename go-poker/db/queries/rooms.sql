-- name: CreateRoom :one
INSERT INTO poker_rooms (room_code, room_name, room_type, host_user_id, buy_in, small_blind, big_blind, max_players, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: GetRoomByCode :one
SELECT id, room_code, room_name, room_type, host_user_id, buy_in, small_blind, big_blind, max_players, status, created_at
FROM poker_rooms
WHERE room_code = $1 LIMIT 1;

-- name: ListActivePublicRooms :many
-- Ordered by stake first so a busy site can never starve a whole blind level
-- out of the lobby: a flat "newest 20" would hide every table in the oldest
-- tier once enough rooms exist.
SELECT id, room_code, room_name, room_type, host_user_id, buy_in, small_blind, big_blind, max_players, status, created_at
FROM poker_rooms
WHERE room_type = 'public' AND status = 'active'
ORDER BY small_blind ASC, id DESC LIMIT 60;

-- name: UpdateRoomStatus :exec
UPDATE poker_rooms
SET status = $1
WHERE room_code = $2;
