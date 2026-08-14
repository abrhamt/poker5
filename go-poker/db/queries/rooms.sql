-- name: CreateRoom :execresult
INSERT INTO poker_rooms (room_code, room_name, room_type, host_user_id, buy_in, small_blind, big_blind, max_players, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetRoomByCode :one
SELECT id, room_code, room_name, room_type, host_user_id, buy_in, small_blind, big_blind, max_players, status, created_at
FROM poker_rooms
WHERE room_code = ? LIMIT 1;

-- name: ListActivePublicRooms :many
SELECT id, room_code, room_name, room_type, host_user_id, buy_in, small_blind, big_blind, max_players, status, created_at
FROM poker_rooms
WHERE room_type = 'public' AND status = 'active'
ORDER BY id DESC LIMIT 20;

-- name: UpdateRoomStatus :exec
UPDATE poker_rooms
SET status = ?
WHERE room_code = ?;
