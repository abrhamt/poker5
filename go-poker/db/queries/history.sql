-- A player's own hand history and statistics, read from hand_players (one row
-- per real player per recorded hand) joined to the hand itself in game_sessions.

-- name: CreateHandPlayer :exec
INSERT INTO hand_players (session_id, user_id, room_code, player_name, seat_index, card1, card2, invested, won, net, folded, showed_down, hand_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: ListUserHistoryRooms :many
-- Every room the user has been dealt into, most recently played first, with
-- the room's own details where the room still exists.
SELECT hp.room_code,
       COALESCE(r.room_name, '')::text AS room_name,
       COALESCE(r.room_type, '')::text AS room_type,
       COALESCE(r.small_blind, 0)::integer AS small_blind,
       COALESCE(r.big_blind, 0)::integer AS big_blind,
       COUNT(*)::bigint AS hands,
       COUNT(*) FILTER (WHERE hp.net > 0)::bigint AS hands_won,
       COALESCE(SUM(hp.net), 0)::bigint AS net,
       MIN(hp.created_at)::timestamptz AS first_played,
       MAX(hp.created_at)::timestamptz AS last_played
FROM hand_players hp
LEFT JOIN poker_rooms r ON r.room_code = hp.room_code
WHERE hp.user_id = $1
GROUP BY hp.room_code, r.room_name, r.room_type, r.small_blind, r.big_blind
ORDER BY last_played DESC;

-- name: ListUserHandsInRoom :many
SELECT hp.id, hp.session_id, hp.player_name, hp.seat_index, hp.card1, hp.card2,
       hp.invested, hp.won, hp.net, hp.folded, hp.showed_down, hp.hand_name, hp.created_at,
       gs.hand_number, gs.pot_amount, gs.commission_amount, gs.winner_name,
       gs.hand_name AS winning_hand, gs.details_json
FROM hand_players hp
JOIN game_sessions gs ON gs.id = hp.session_id
WHERE hp.user_id = $1 AND hp.room_code = $2
ORDER BY hp.created_at DESC, hp.id DESC
LIMIT $3 OFFSET $4;

-- name: CountUserHandsInRoom :one
SELECT COUNT(*) FROM hand_players WHERE user_id = $1 AND room_code = $2;

-- name: GetUserHandTotals :one
SELECT COUNT(*)::bigint AS hands,
       COUNT(*) FILTER (WHERE net > 0)::bigint AS hands_won,
       COUNT(*) FILTER (WHERE folded)::bigint AS hands_folded,
       COUNT(*) FILTER (WHERE showed_down)::bigint AS showdowns,
       COUNT(*) FILTER (WHERE showed_down AND net > 0)::bigint AS showdowns_won,
       COALESCE(SUM(net), 0)::bigint AS net,
       COALESCE(SUM(invested), 0)::bigint AS invested,
       COALESCE(SUM(won), 0)::bigint AS won,
       COALESCE(MAX(net), 0)::bigint AS biggest_win,
       COALESCE(MIN(net), 0)::bigint AS biggest_loss,
       COUNT(DISTINCT room_code)::bigint AS rooms
FROM hand_players
WHERE user_id = $1;

-- name: ListUserRecentNets :many
-- The newest hands first; the caller reverses them to draw a running total.
SELECT net, created_at
FROM hand_players
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: ListUserWinningHandNames :many
-- The hands the user won with at showdown. Grouped by the full description
-- ("Two Pair (Kings & Eights)"); the caller folds those into categories.
SELECT hand_name, COUNT(*)::bigint AS times
FROM hand_players
WHERE user_id = $1 AND showed_down AND net > 0 AND hand_name <> ''
GROUP BY hand_name;
