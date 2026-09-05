-- PostgreSQL schema. This file is the only definition of the database: sqlc
-- reads it to generate repository/, and the server executes it at start-up
-- (db/schema.go embeds it), so there is no second copy to drift out of sync.
--
-- Every statement is IF NOT EXISTS and safe to re-run. Columns added after a
-- release land in the migrations list in db/schema.go, because CREATE TABLE IF
-- NOT EXISTS never alters a table that already exists.

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    phone_number VARCHAR(32) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    wallet BIGINT NOT NULL DEFAULT 0,
    referral_code CHAR(6) NOT NULL UNIQUE,
    referred_by CHAR(6) DEFAULT NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'player',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_role ON users (role);

CREATE TABLE IF NOT EXISTS transactions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount BIGINT NOT NULL,
    type VARCHAR(24) NOT NULL DEFAULT 'other',
    reason VARCHAR(64) NOT NULL,
    transaction_id VARCHAR(10) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tx_user ON transactions (user_id);
CREATE INDEX IF NOT EXISTS idx_tx_type ON transactions (type);
CREATE INDEX IF NOT EXISTS idx_tx_created ON transactions (created_at);

CREATE TABLE IF NOT EXISTS game_sessions (
    id BIGSERIAL PRIMARY KEY,
    table_id VARCHAR(32) NOT NULL,
    hand_number INTEGER NOT NULL,
    pot_amount BIGINT NOT NULL,
    commission_amount BIGINT NOT NULL,
    winner_user_id BIGINT DEFAULT NULL,
    winner_name VARCHAR(64) NOT NULL,
    hand_name VARCHAR(64) NOT NULL,
    details_json TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sessions_table ON game_sessions (table_id);

CREATE TABLE IF NOT EXISTS user_sessions (
    session_token VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions (user_id);

-- poker_game and poker_player hold live table state. storage.go is their only
-- writer; the generated queries for them exist but nothing calls them.
CREATE TABLE IF NOT EXISTS poker_game (
    id BIGSERIAL PRIMARY KEY,
    table_id VARCHAR(32) NOT NULL UNIQUE,
    phase VARCHAR(32) NOT NULL DEFAULT 'preflop',
    pot INTEGER NOT NULL DEFAULT 0,
    current_bet INTEGER NOT NULL DEFAULT 0,
    last_raise INTEGER NOT NULL DEFAULT 20,
    small_blind INTEGER NOT NULL DEFAULT 10,
    big_blind INTEGER NOT NULL DEFAULT 20,
    raises_this_round INTEGER NOT NULL DEFAULT 0,
    dealer_orbit_count INTEGER NOT NULL DEFAULT -1,
    game_started BOOLEAN NOT NULL DEFAULT FALSE,
    game_finished BOOLEAN NOT NULL DEFAULT FALSE,
    open_cards_mode BOOLEAN NOT NULL DEFAULT FALSE,
    spectator_mode BOOLEAN NOT NULL DEFAULT FALSE,
    initial_dealer_name VARCHAR(64) DEFAULT NULL,
    deck TEXT NOT NULL DEFAULT '[]',
    card_graveyard TEXT NOT NULL DEFAULT '[]',
    community_cards TEXT NOT NULL DEFAULT '[]',
    current_player_index INTEGER NOT NULL DEFAULT 0,
    total_hands INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version INTEGER NOT NULL DEFAULT 0,
    notifications TEXT NOT NULL DEFAULT '[]',
    intermission BOOLEAN NOT NULL DEFAULT FALSE,
    last_winner_json TEXT NOT NULL DEFAULT '{}',
    pot_awards_json TEXT NOT NULL DEFAULT '[]',
    intermission_started_at BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS poker_player (
    id BIGSERIAL PRIMARY KEY,
    game_id BIGINT NOT NULL REFERENCES poker_game(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL DEFAULT 0,
    name VARCHAR(64) NOT NULL,
    seat_index INTEGER NOT NULL,
    is_bot BOOLEAN NOT NULL DEFAULT FALSE,
    chips INTEGER NOT NULL DEFAULT 2000,
    round_bet INTEGER NOT NULL DEFAULT 0,
    total_bet INTEGER NOT NULL DEFAULT 0,
    folded BOOLEAN NOT NULL DEFAULT FALSE,
    all_in BOOLEAN NOT NULL DEFAULT FALSE,
    is_dealer BOOLEAN NOT NULL DEFAULT FALSE,
    is_small_blind BOOLEAN NOT NULL DEFAULT FALSE,
    is_big_blind BOOLEAN NOT NULL DEFAULT FALSE,
    card1 VARCHAR(4) DEFAULT NULL,
    card2 VARCHAR(4) DEFAULT NULL,
    win_probability DOUBLE PRECISION DEFAULT NULL,
    sitting_out BOOLEAN NOT NULL DEFAULT FALSE,
    stats_data TEXT NOT NULL DEFAULT '{}',
    UNIQUE (game_id, seat_index)
);
CREATE INDEX IF NOT EXISTS idx_poker_player_game ON poker_player (game_id);

-- poker_config is a small key/value store for engine settings that outlive a
-- process. "key" is not reserved in PostgreSQL, so it needs no quoting.
CREATE TABLE IF NOT EXISTS poker_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS poker_rooms (
    id BIGSERIAL PRIMARY KEY,
    room_code VARCHAR(16) NOT NULL UNIQUE,
    room_name VARCHAR(64) NOT NULL,
    room_type VARCHAR(16) NOT NULL DEFAULT 'public',
    host_user_id BIGINT DEFAULT NULL,
    buy_in BIGINT NOT NULL DEFAULT 20,
    small_blind INTEGER NOT NULL DEFAULT 10,
    big_blind INTEGER NOT NULL DEFAULT 20,
    max_players INTEGER NOT NULL DEFAULT 6,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_room_type_status ON poker_rooms (room_type, status);

CREATE TABLE IF NOT EXISTS site_settings (
    id INTEGER PRIMARY KEY DEFAULT 1,
    rake_mode VARCHAR(16) NOT NULL DEFAULT 'percentage',
    rake_percentage DOUBLE PRECISION NOT NULL DEFAULT 5,
    referral_percentage_pct_mode DOUBLE PRECISION NOT NULL DEFAULT 10,
    referral_percentage_sb_mode DOUBLE PRECISION NOT NULL DEFAULT 10,
    countdown_seconds INTEGER NOT NULL DEFAULT 15,
    -- Real deposits are off by default: a fresh install has no bank account
    -- configured, and a deposit form demanding a receipt nobody can produce is
    -- worse than play money.
    real_deposits_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    deposit_account_name VARCHAR(128) NOT NULL DEFAULT '',
    deposit_account_number VARCHAR(32) NOT NULL DEFAULT '',
    -- PostgreSQL has no ON UPDATE CURRENT_TIMESTAMP, so UpdateSiteSettings
    -- sets this column explicitly.
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- bank_deposits is the record of every CBE receipt anyone has pasted, credited
-- or not.
--
-- The UNIQUE on reference is the whole double-spend defence, and it belongs on
-- reference *alone* rather than on (receipt_url, reference): one transfer has
-- more than one receipt URL — the link in the sender's SMS and the
-- encodedReceipt link the bank returns for it differ — and both resolve to the
-- same reference. Keyed on the pair, the second URL would credit the same
-- money twice.
--
-- reference is NULL for a receipt queued while the verifier was unreachable:
-- nothing has read the bank's side of it yet. PostgreSQL allows repeated NULLs
-- in a UNIQUE column, so queued rows do not collide with each other. The URL
-- carries its own UNIQUE, which is what stops the same paste from queuing
-- twice.
CREATE TABLE IF NOT EXISTS bank_deposits (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(16) NOT NULL DEFAULT 'cbe',
    receipt_url VARCHAR(255) NOT NULL UNIQUE,
    reference VARCHAR(64) DEFAULT NULL UNIQUE,
    amount BIGINT NOT NULL DEFAULT 0,
    payer_name VARCHAR(128) NOT NULL DEFAULT '',
    payer_account VARCHAR(32) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'pending_review',
    note VARCHAR(255) NOT NULL DEFAULT '',
    transaction_id VARCHAR(10) DEFAULT NULL,
    -- attempts and last_attempt_at pace the background retry of queued
    -- receipts: without them a permanently unresolvable link would be
    -- re-checked on every sweep forever.
    attempts INTEGER NOT NULL DEFAULT 0,
    last_attempt_at BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_bank_deposit_status ON bank_deposits (status);
CREATE INDEX IF NOT EXISTS idx_bank_deposit_user ON bank_deposits (user_id);

-- OTP tables. Their timestamps are BIGINT unix seconds rather than TIMESTAMPTZ
-- on purpose: these are deadlines compared in Go as often as in SQL, and
-- integer seconds mean the two comparisons cannot disagree about a time zone.

-- pending_registrations holds a signup that has not proved its phone yet. No
-- users row exists until the code checks out, so username and phone are UNIQUE
-- here too — starting a signup reserves the name for the life of the code, and
-- a collision is reported before any SMS is sent.
CREATE TABLE IF NOT EXISTS pending_registrations (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    phone_number VARCHAR(32) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    referred_by CHAR(6) DEFAULT NULL,
    code CHAR(6) NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    expires_at BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pending_expires ON pending_registrations (expires_at);

-- password_reset_codes carries one reset attempt through both of its phases:
-- the code, then the token that verifying the code mints. Both live on one row
-- so "has this attempt been spent?" is a single read, and the row is keyed by
-- phone so a new request replaces the old attempt rather than stacking.
CREATE TABLE IF NOT EXISTS password_reset_codes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    phone_number VARCHAR(32) NOT NULL UNIQUE,
    code CHAR(6) NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    expires_at BIGINT NOT NULL,
    reset_token CHAR(64) DEFAULT NULL UNIQUE,
    token_expires_at BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reset_expires ON password_reset_codes (expires_at);

-- otp_send_throttle counts sends per phone per purpose. It is deliberately not
-- part of either table above: those rows are deleted when an attempt fails,
-- succeeds, or expires, and a counter that dies with the thing it counts is not
-- a counter. Nothing rolls this back.
CREATE TABLE IF NOT EXISTS otp_send_throttle (
    id BIGSERIAL PRIMARY KEY,
    phone_number VARCHAR(32) NOT NULL,
    purpose VARCHAR(16) NOT NULL,
    window_started_at BIGINT NOT NULL,
    last_sent_at BIGINT NOT NULL,
    send_count INTEGER NOT NULL DEFAULT 0,
    UNIQUE (phone_number, purpose)
);
CREATE INDEX IF NOT EXISTS idx_throttle_window ON otp_send_throttle (window_started_at);
