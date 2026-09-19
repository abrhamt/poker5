package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	// pgx's database/sql driver, registered as "pgx". The repository layer is
	// the only place that names a driver.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// ErrNoDatabaseURL is returned when DATABASE_URL is unset. There is no
// fallback: a default connection string would let a misconfigured deployment
// come up pointed at the wrong database instead of failing at start-up.
var ErrNoDatabaseURL = errors.New("DATABASE_URL is not set")

const (
	// maxOpenConns bounds the pool. PostgreSQL's own max_connections is the
	// real ceiling, and an unbounded pool turns a traffic spike into
	// "sorry, too many clients already" for every request at once.
	maxOpenConns = 25
	maxIdleConns = 25

	// connMaxLifetime keeps connections young enough that a failover or a
	// restarted database is picked up without a bounce.
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute

	connectTimeout = 10 * time.Second
)

// Connect opens the pool from DATABASE_URL.
func Connect(ctx context.Context) (*sql.DB, error) {
	url := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if url == "" {
		return nil, ErrNoDatabaseURL
	}
	return ConnectURL(ctx, url)
}

// ConnectURL opens the pool from an explicit DSN, verifying it before handing
// it back: sql.Open does not talk to the server, so without the ping a bad
// DSN or an unreachable database would only surface on the first request.
func ConnectURL(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return db, nil
}

// IsUniqueViolation reports whether err is PostgreSQL's unique_violation
// (SQLSTATE 23505). Callers use it to tell "someone already claimed this" from
// a real failure, which is a distinction the database makes and the
// application must not guess at.
//
// The check reads the message rather than unwrapping to *pgconn.PgError so
// that nothing above this package has to import the driver.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "23505") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
