// Package dbschema owns the database schema: the DDL that sqlc generates from
// and that the server applies at start-up are the same bytes, embedded here,
// so there is no second copy that can drift.
package dbschema

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var Schema string

// migrations run after the schema. Every statement in schema.sql is CREATE
// TABLE IF NOT EXISTS, which never alters a table that already exists — so a
// column introduced after a database was first created has to land here too,
// or a live deployment starts failing on the missing column.
//
// PostgreSQL has ADD COLUMN IF NOT EXISTS, so these are idempotent as written
// and need no error-message matching to stay that way.
var migrations = []string{
	`ALTER TABLE site_settings ADD COLUMN IF NOT EXISTS real_deposits_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE site_settings ADD COLUMN IF NOT EXISTS deposit_account_name VARCHAR(128) NOT NULL DEFAULT ''`,
	`ALTER TABLE site_settings ADD COLUMN IF NOT EXISTS deposit_account_number VARCHAR(32) NOT NULL DEFAULT ''`,
	`ALTER TABLE bank_deposits ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE bank_deposits ADD COLUMN IF NOT EXISTS last_attempt_at BIGINT NOT NULL DEFAULT 0`,
	`ALTER TABLE poker_player ADD COLUMN IF NOT EXISTS sitting_out BOOLEAN NOT NULL DEFAULT FALSE`,
}

// Apply creates the schema, runs the migrations, and makes sure the single
// site_settings row exists. Safe to call on every start-up.
func Apply(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, Schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	for _, statement := range migrations {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migration %q: %w", statement, err)
		}
	}
	// The settings row is a singleton the application always expects to read.
	if _, err := db.ExecContext(ctx, `INSERT INTO site_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed site_settings: %w", err)
	}
	return nil
}
