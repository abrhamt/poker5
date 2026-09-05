-- name: CreateGame :one
INSERT INTO poker_game (table_id, phase, pot, current_bet, last_raise, small_blind, big_blind, raises_this_round, dealer_orbit_count, game_started, game_finished, open_cards_mode, spectator_mode, initial_dealer_name, deck, card_graveyard, community_cards, current_player_index, total_hands, version, notifications, intermission, last_winner_json, pot_awards_json, intermission_started_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
RETURNING id;

-- name: GetGameByTableID :one
SELECT id, table_id, phase, pot, current_bet, last_raise, small_blind, big_blind, raises_this_round, dealer_orbit_count, game_started, game_finished, open_cards_mode, spectator_mode, initial_dealer_name, deck, card_graveyard, community_cards, current_player_index, total_hands, created_at, updated_at, version, notifications, intermission, last_winner_json, pot_awards_json, intermission_started_at
FROM poker_game
WHERE table_id = $1 LIMIT 1;

-- name: UpdateGame :exec
UPDATE poker_game
SET phase = $1, pot = $2, current_bet = $3, last_raise = $4, small_blind = $5, big_blind = $6, raises_this_round = $7, dealer_orbit_count = $8, game_started = $9, game_finished = $10, open_cards_mode = $11, spectator_mode = $12, initial_dealer_name = $13, deck = $14, card_graveyard = $15, community_cards = $16, current_player_index = $17, total_hands = $18, version = version + 1, notifications = $19, intermission = $20, last_winner_json = $21, pot_awards_json = $22, intermission_started_at = $23, updated_at = NOW()
WHERE id = $24 AND version = $25;

-- name: DeletePlayersByGameID :exec
DELETE FROM poker_player WHERE game_id = $1;

-- name: CreatePlayer :one
INSERT INTO poker_player (game_id, user_id, name, seat_index, is_bot, chips, round_bet, total_bet, folded, all_in, is_dealer, is_small_blind, is_big_blind, card1, card2, win_probability, sitting_out, stats_data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING id;

-- name: GetPlayersByGameID :many
SELECT id, game_id, user_id, name, seat_index, is_bot, chips, round_bet, total_bet, folded, all_in, is_dealer, is_small_blind, is_big_blind, card1, card2, win_probability, sitting_out, stats_data
FROM poker_player
WHERE game_id = $1
ORDER BY seat_index ASC;
