CREATE TABLE IF NOT EXISTS users (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    phone_number VARCHAR(32) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    wallet DECIMAL(15,2) NOT NULL DEFAULT 0.00,
    referral_code VARCHAR(16) NOT NULL UNIQUE,
    referred_by VARCHAR(16) DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_users_username (username),
    INDEX idx_users_phone (phone_number),
    INDEX idx_users_ref_code (referral_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS transactions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    amount DECIMAL(15,2) NOT NULL,
    reason VARCHAR(64) NOT NULL,
    transaction_id VARCHAR(10) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_tx_user (user_id),
    INDEX idx_tx_code (transaction_id),
    CONSTRAINT fk_tx_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS game_sessions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    table_id VARCHAR(32) NOT NULL,
    hand_number INT NOT NULL,
    pot_amount DECIMAL(15,2) NOT NULL,
    commission_amount DECIMAL(15,2) NOT NULL,
    winner_user_id BIGINT DEFAULT NULL,
    winner_name VARCHAR(64) NOT NULL,
    hand_name VARCHAR(64) NOT NULL,
    details_json TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_sessions_table (table_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_sessions (
    session_token VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_sess_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS poker_game (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
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
    deck TEXT NOT NULL,
    card_graveyard TEXT NOT NULL,
    community_cards TEXT NOT NULL,
    current_player_index INT NOT NULL DEFAULT 0,
    total_hands INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    version INT NOT NULL DEFAULT 0,
    notifications TEXT NOT NULL,
    intermission TINYINT(1) NOT NULL DEFAULT 0,
    last_winner_json TEXT NOT NULL,
    pot_awards_json TEXT NOT NULL,
    intermission_started_at BIGINT NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS poker_player (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
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
    stats_data TEXT NOT NULL,
    UNIQUE KEY uq_game_seat (game_id, seat_index),
    CONSTRAINT fk_player_game FOREIGN KEY (game_id) REFERENCES poker_game(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
