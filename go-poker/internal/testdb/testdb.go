// Package testdb gives tests a real PostgreSQL to run against.
//
// There is no in-process PostgreSQL the way there was an in-memory SQLite, and
// testing the application against a different engine than it ships on is how
// engine-specific bugs reach production. So the suite runs against the real
// thing: a throwaway container started once per test binary, with each test
// getting its own empty database inside it.
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	dbschema "github.com/zuse/poker5/go-poker/db"
	"github.com/zuse/poker5/go-poker/repository"
)

const (
	image           = "postgres:17-alpine"
	password        = "poker-test"
	readyTimeout    = 90 * time.Second
	readyPollEvery  = 250 * time.Millisecond
	dockerCmdTimout = 5 * time.Minute
)

var (
	// server is resolved once per test binary: either the DSN the operator
	// supplied or the container this package started.
	serverOnce sync.Once
	serverDSN  string
	serverSkip string

	containerID string

	nameMu  sync.Mutex
	nameSeq int
)

// RunMain wraps a package's TestMain so the container is torn down once every
// test in the binary has finished. Without it the container would outlive the
// run.
func RunMain(m *testing.M) int {
	code := m.Run()
	stopContainer()
	return code
}

// New returns a pool onto a fresh, empty database with the schema applied. The
// database is dropped when the test ends, so tests never see each other's
// rows and can run in any order.
func New(t *testing.T) *sql.DB {
	t.Helper()

	admin := adminDSN(t)
	name := uniqueName(t)

	adminDB, err := sql.Open("pgx", admin)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer adminDB.Close()

	if _, err := adminDB.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}

	db, err := repository.ConnectURL(context.Background(), replaceDatabase(admin, name))
	if err != nil {
		t.Fatalf("connect to %s: %v", name, err)
	}
	if err := dbschema.Apply(context.Background(), db); err != nil {
		_ = db.Close()
		t.Fatalf("apply schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		cleanup, err := sql.Open("pgx", admin)
		if err != nil {
			return
		}
		defer cleanup.Close()
		// WITH (FORCE) so a connection the test left open cannot keep the
		// database alive and leak it into the next run.
		_, _ = cleanup.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`)
	})

	return db
}

// adminDSN returns a DSN for the maintenance database, starting the container
// on first use.
func adminDSN(t *testing.T) string {
	t.Helper()
	serverOnce.Do(startServer)
	if serverSkip != "" {
		t.Skip(serverSkip)
	}
	return serverDSN
}

func startServer() {
	if url := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")); url != "" {
		serverDSN = url
		return
	}

	if _, err := exec.LookPath("docker"); err != nil {
		serverSkip = "docker not found: install Docker, or set TEST_DATABASE_URL to a PostgreSQL server these tests may create databases on"
		return
	}

	out, err := docker("run", "-d", "--rm",
		"-e", "POSTGRES_PASSWORD="+password,
		"-P", image,
		// Durability is pointless for a database that is deleted minutes from
		// now, and turning it off makes the suite noticeably faster.
		"-c", "fsync=off",
		"-c", "full_page_writes=off",
		"-c", "synchronous_commit=off")
	if err != nil {
		serverSkip = fmt.Sprintf("could not start a PostgreSQL container (%v): start Docker, or set TEST_DATABASE_URL", err)
		return
	}
	containerID = strings.TrimSpace(out)

	port, err := mappedPort()
	if err != nil {
		stopContainer()
		serverSkip = fmt.Sprintf("could not read the container's port: %v", err)
		return
	}

	dsn := fmt.Sprintf("postgres://postgres:%s@127.0.0.1:%s/postgres?sslmode=disable", password, port)
	if err := waitReady(dsn); err != nil {
		stopContainer()
		serverSkip = fmt.Sprintf("PostgreSQL container never became ready: %v", err)
		return
	}
	serverDSN = dsn
}

func mappedPort() (string, error) {
	out, err := docker("port", containerID, "5432/tcp")
	if err != nil {
		return "", err
	}
	// docker prints one line per address family; either maps to the same host
	// port, so the first is enough.
	line := strings.TrimSpace(strings.Split(strings.TrimSpace(out), "\n")[0])
	idx := strings.LastIndex(line, ":")
	if idx < 0 || idx == len(line)-1 {
		return "", fmt.Errorf("unexpected port mapping %q", out)
	}
	return line[idx+1:], nil
}

// waitReady polls until the server accepts a connection. A container is
// reported as running well before PostgreSQL finishes its first-boot
// initialization, so this is a real wait, not a formality.
func waitReady(dsn string) error {
	deadline := time.Now().Add(readyTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		db, err := sql.Open("pgx", dsn)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err = db.PingContext(ctx)
			cancel()
			_ = db.Close()
			if err == nil {
				return nil
			}
		}
		lastErr = err
		time.Sleep(readyPollEvery)
	}
	return lastErr
}

func stopContainer() {
	if containerID == "" {
		return
	}
	_, _ = docker("rm", "-f", containerID)
	containerID = ""
}

func docker(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerCmdTimout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// uniqueName builds a legal, unique database name from the test's own name, so
// a failure that leaves a database behind says which test owned it.
func uniqueName(t *testing.T) string {
	nameMu.Lock()
	nameSeq++
	seq := nameSeq
	nameMu.Unlock()

	var b strings.Builder
	b.WriteString("test_")
	for _, r := range strings.ToLower(t.Name()) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := b.String()
	if len(name) > 40 {
		name = name[:40]
	}
	return fmt.Sprintf("%s_%d", name, seq)
}

// replaceDatabase swaps the database name in a DSN, keeping everything else —
// host, credentials, and options such as sslmode — exactly as given.
func replaceDatabase(dsn, name string) string {
	base, query, hasQuery := strings.Cut(dsn, "?")
	slash := strings.LastIndex(base, "/")
	if slash < 0 {
		base = strings.TrimRight(base, "/") + "/" + name
	} else {
		base = base[:slash+1] + name
	}
	if hasQuery {
		return base + "?" + query
	}
	return base
}
