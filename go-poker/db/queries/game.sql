-- name: CreateGame :execresult
INSERT INTO poker_game (table_id, phase, pot, current_bet, last_raise, small_blind, big_blind, raises_this_round, dealer_orbit_count, game_started, game_finished, open_cards_mode, spectator_mode, initial_dealer_name, deck, card_graveyard, community_cards, current_player_index, total_hands, version, notifications, intermission, last_winner_json, pot_awards_json, intermission_started_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetGameByTableID :one
SELECT id, table_id, phase, pot, current_bet, last_raise, small_blind, big_blind, raises_this_round, dealer_orbit_count, game_started, game_finished, open_cards_mode, spectator_mode, initial_dealer_name, deck, card_graveyard, community_cards, current_player_index, total_hands, created_at, updated_at, version, notifications, intermission, last_winner_json, pot_awards_json, intermission_started_at
FROM poker_game
WHERE table_id = ? LIMIT 1;

-- name: UpdateGame :exec
UPDATE poker_game
SET phase = ?, pot = ?, current_bet = ?, last_raise = ?, small_blind = ?, big_blind = ?, raises_this_round = ?, dealer_orbit_count = ?, game_started = ?, game_finished = ?, open_cards_mode = ?, spectator_mode = ?, initial_dealer_name = ?, deck = ?, card_graveyard = ?, community_cards = ?, current_player_index = ?, total_hands = ?, version = version + 1, notifications = ?, intermission = ?, last_winner_json = ?, pot_awards_json = ?, intermission_started_at = ?
WHERE id = ? AND version = ?;

-- name: DeletePlayersByGameID :exec
DELETE FROM poker_player WHERE game_id = ?;

-- name: CreatePlayer :execresult
INSERT INTO poker_player (game_id, user_id, name, seat_index, is_bot, chips, round_bet, total_bet, folded, all_in, is_dealer, is_small_blind, is_big_blind, card1, card2, win_probability, stats_data)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPlayersByGameID :many
SELECT id, game_id, user_id, name, seat_index, is_bot, chips, round_bet, total_bet, folded, all_in, is_dealer, is_small_blind, is_big_blind, card1, card2, win_probability, stats_data
FROM poker_player
WHERE game_id = ?
ORDER BY seat_index ASC;
