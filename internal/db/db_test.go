// Package db_test is an external test package so it can use internal/testutil,
// which imports internal/db.
package db_test

import (
	"database/sql"
	"testing"

	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/testutil"
)

func TestOpen(t *testing.T) {
	database, err := db.Open(testutil.TestDSN())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if database.Client() == nil {
		t.Error("Client() is nil — ent was not attached to the pool")
	}
	if err := database.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestOpen_UnreachableDSN(t *testing.T) {
	// Port 1 is reserved and never listening, so this fails at connect rather
	// than at parse — Open must surface that instead of returning a handle that
	// only fails later, on the first query.
	_, err := db.Open("postgres://nobody:nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("Open on an unreachable DSN returned no error")
	}
}

func TestOpen_MalformedDSN(t *testing.T) {
	if _, err := db.Open("this is not a dsn"); err == nil {
		t.Fatal("Open on a malformed DSN returned no error")
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	database := testutil.NewDB(t) // already migrated once
	if err := database.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var n int
	if err := database.QueryRow(
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema()`,
	).Scan(&n); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if n == 0 {
		t.Error("no tables after migration")
	}
}

// TestPoolIsNotCappedAtOne guards against reintroducing the SQLite-era
// SetMaxOpenConns(1).
//
// That cap was a workaround for SQLite's single writer, and it made holding a
// rows cursor while writing deadlock until the context expired — a real
// scheduler failure. PostgreSQL has no such restriction, so a pool of one would
// be a silent throughput ceiling with no reason behind it.
func TestPoolIsNotCappedAtOne(t *testing.T) {
	database, err := db.Open(testutil.TestDSN())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if got := database.Stats().MaxOpenConnections; got <= 1 {
		t.Errorf("MaxOpenConnections = %d, want > 1", got)
	}
}

// TestConcurrentReadWhileWriting is the regression test for the deadlock the
// single-connection pool used to cause: read a cursor and write on another
// connection before draining it.
func TestConcurrentReadWhileWriting(t *testing.T) {
	database := testutil.NewDB(t)
	ctx := t.Context()

	seed := testutil.SeedUser(t, database.DB, "cursor_probe", "developer")
	if seed == 0 {
		t.Fatal("seed user returned id 0")
	}

	rows, err := database.QueryContext(ctx, `SELECT id FROM users`)
	if err != nil {
		t.Fatalf("open cursor: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("cursor returned no rows")
	}

	// Under SQLite with MaxOpenConns(1) this blocked until ctx timeout.
	if _, err := database.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES ($1, 'x', 'developer')`,
		"written_while_reading",
	); err != nil {
		t.Fatalf("write while a cursor is open: %v", err)
	}
	if err := rows.Err(); err != nil {
		t.Errorf("cursor error: %v", err)
	}
}

var _ = sql.ErrNoRows // keep database/sql imported for future assertions
