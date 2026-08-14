package repository

import (
	"database/sql"
)

const defaultSchema = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username VARCHAR(64) NOT NULL UNIQUE,
    phone_number VARCHAR(32) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    wallet INTEGER NOT NULL DEFAULT 0,
    referral_code CHAR(6) NOT NULL UNIQUE,
    referred_by CHAR(6) DEFAULT NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'player',
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id BIGINT NOT NULL,
    amount INTEGER NOT NULL,
    type VARCHAR(24) NOT NULL DEFAULT 'other',
    reason VARCHAR(64) NOT NULL,
    transaction_id VARCHAR(10) NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS user_sessions (
    session_token VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS poker_rooms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    room_code VARCHAR(16) NOT NULL UNIQUE,
    room_name VARCHAR(64) NOT NULL,
    room_type VARCHAR(16) NOT NULL DEFAULT 'public',
    host_user_id BIGINT DEFAULT NULL,
    buy_in INTEGER NOT NULL DEFAULT 20,
    small_blind INT NOT NULL DEFAULT 10,
    big_blind INT NOT NULL DEFAULT 20,
    max_players INT NOT NULL DEFAULT 6,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS game_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    table_id VARCHAR(32) NOT NULL,
    hand_number INT NOT NULL,
    pot_amount INTEGER NOT NULL,
    commission_amount INTEGER NOT NULL,
    winner_user_id BIGINT DEFAULT NULL,
    winner_name VARCHAR(64) NOT NULL,
    hand_name VARCHAR(64) NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS poker_game (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    table_id VARCHAR(32) NOT NULL UNIQUE,
    phase VARCHAR(32) NOT NULL DEFAULT 'preflop',
    pot INT NOT NULL DEFAULT 0,
    current_bet INT NOT NULL DEFAULT 0,
    last_raise INT NOT NULL DEFAULT 20,
    small_blind INT NOT NULL DEFAULT 10,
    big_blind INT NOT NULL DEFAULT 20,
    raises_this_round INT NOT NULL DEFAULT 0,
    dealer_orbit_count INT NOT NULL DEFAULT -1,
    game_started TINYINT(1) NOT NULL DEFAULT 0,
    game_finished TINYINT(1) NOT NULL DEFAULT 0,
    open_cards_mode TINYINT(1) NOT NULL DEFAULT 0,
    spectator_mode TINYINT(1) NOT NULL DEFAULT 0,
    initial_dealer_name VARCHAR(64) DEFAULT NULL,
    deck TEXT NOT NULL DEFAULT '[]',
    card_graveyard TEXT NOT NULL DEFAULT '[]',
    community_cards TEXT NOT NULL DEFAULT '[]',
    current_player_index INT NOT NULL DEFAULT 0,
    total_hands INT NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    version INT NOT NULL DEFAULT 0,
    notifications TEXT NOT NULL DEFAULT '[]',
    intermission TINYINT(1) NOT NULL DEFAULT 0,
    last_winner_json TEXT NOT NULL DEFAULT '{}',
    pot_awards_json TEXT NOT NULL DEFAULT '[]',
    intermission_started_at BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS poker_player (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id BIGINT NOT NULL,
    user_id BIGINT DEFAULT NULL,
    name VARCHAR(64) NOT NULL,
    seat_index INT NOT NULL,
    is_bot TINYINT(1) NOT NULL DEFAULT 0,
    chips INT NOT NULL DEFAULT 2000,
    round_bet INT NOT NULL DEFAULT 0,
    total_bet INT NOT NULL DEFAULT 0,
    folded TINYINT(1) NOT NULL DEFAULT 0,
    all_in TINYINT(1) NOT NULL DEFAULT 0,
    is_dealer TINYINT(1) NOT NULL DEFAULT 0,
    is_small_blind TINYINT(1) NOT NULL DEFAULT 0,
    is_big_blind TINYINT(1) NOT NULL DEFAULT 0,
    card1 VARCHAR(4) DEFAULT NULL,
    card2 VARCHAR(4) DEFAULT NULL,
    win_probability DOUBLE DEFAULT NULL,
    stats_data TEXT NOT NULL DEFAULT '{}',
    UNIQUE (game_id, seat_index)
);

CREATE TABLE IF NOT EXISTS site_settings (
    id INTEGER PRIMARY KEY DEFAULT 1,
    rake_mode VARCHAR(16) NOT NULL DEFAULT 'percentage',
    rake_percentage REAL NOT NULL DEFAULT 5,
    referral_percentage_pct_mode REAL NOT NULL DEFAULT 10,
    referral_percentage_sb_mode REAL NOT NULL DEFAULT 10,
    countdown_seconds INT NOT NULL DEFAULT 15,
    updated_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
`

func InitDBSchema(db *sql.DB) error {
	if _, err := db.Exec(defaultSchema); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO site_settings (id) VALUES (1);`)
	return err
}
