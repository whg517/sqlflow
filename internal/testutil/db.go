// Package testutil provides shared test helpers for the SQLFlow test suite.
//
// Historically every *_test.go file declared its own setupXxxTestDB helper
// (17+ duplicated implementations) that opened a temp-dir SQLite file and ran
// migrations. This package centralizes that so tests stop duplicating ~15 lines
// of boilerplate and stop drifting in behavior.
//
// Since ADR-0009 the platform store is PostgreSQL, so a per-test database can no
// longer be a temp file. Isolation is per-schema instead: each test gets its own
// PostgreSQL schema, migrated from scratch and dropped on cleanup.
//
// Usage:
//
//	// returns *db.DB (ent + raw sql) with migrations applied + t.Cleanup
//	d := testutil.NewDB(t)
//	// for constructors that take a raw *sql.DB
//	raw := testutil.NewDB(t).DB
package testutil

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/whg517/sqlflow/internal/db"
)

// TestDSNEnv names the environment variable holding the PostgreSQL connection
// string used by tests.
const TestDSNEnv = "SQLFLOW_TEST_DSN"

// defaultTestDSN matches the container started by `make dev-db`.
const defaultTestDSN = "postgres://sqlflow:sqlflow@localhost:55433/sqlflow?sslmode=disable"

// schemaCounter keeps generated schema names unique within a process.
//
// The test name alone is not enough: subtests can repeat names, and table-driven
// cases often share one.
var schemaCounter atomic.Uint64

// TestDSN returns the connection string tests should use.
func TestDSN() string {
	if dsn := os.Getenv(TestDSNEnv); dsn != "" {
		return dsn
	}
	return defaultTestDSN
}

// NewDB returns a migrated *db.DB scoped to a schema of its own.
//
// The connection's search_path is pinned to that schema, so unqualified table
// names in both ent and raw SQL resolve inside it and tests cannot see each
// other's rows. The schema is dropped on cleanup.
func NewDB(t *testing.T) *db.DB {
	t.Helper()

	schema := schemaName(t)

	// The schema has to exist before anything connects with it on the
	// search_path, so it is created over a short-lived admin connection.
	admin, err := sql.Open("pgx", TestDSN())
	if err != nil {
		t.Fatalf("testutil: open postgres: %v", err)
	}
	defer func() { _ = admin.Close() }()
	if err := admin.Ping(); err != nil {
		t.Fatalf("testutil: cannot reach PostgreSQL at %s: %v\n"+
			"tests need a running instance — try `make dev-db`, or set %s",
			TestDSN(), err, TestDSNEnv)
	}
	// Dropped first: the counter restarts with the process, so a schema left
	// behind by a crashed run would otherwise collide and, worse, be reused with
	// stale rows still in it.
	if _, err := admin.Exec("DROP SCHEMA IF EXISTS " + quoteIdent(schema) + " CASCADE"); err != nil {
		t.Fatalf("testutil: drop stale schema %s: %v", schema, err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + quoteIdent(schema)); err != nil {
		t.Fatalf("testutil: create schema %s: %v", schema, err)
	}

	// search_path goes in the DSN rather than being SET after connecting, so
	// every connection the pool opens lands in this test's schema. Setting it
	// per-session would only pin the first connection, and pinning the pool to
	// one connection instead deadlocks the migration runner: golang-migrate
	// holds an advisory lock on one connection while migrating on another.
	conn, err := sql.Open("pgx", withSearchPath(TestDSN(), schema))
	if err != nil {
		t.Fatalf("testutil: open test schema: %v", err)
	}

	database, err := db.WrapSQL(conn)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("testutil: wrap connection: %v", err)
	}

	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("testutil: close db: %v", err)
		}
		dropper, err := sql.Open("pgx", TestDSN())
		if err != nil {
			t.Logf("testutil: reopen to drop schema %s: %v", schema, err)
			return
		}
		defer func() { _ = dropper.Close() }()
		if _, err := dropper.Exec("DROP SCHEMA " + quoteIdent(schema) + " CASCADE"); err != nil {
			t.Logf("testutil: drop schema %s: %v", schema, err)
		}
	})

	if err := database.Migrate(); err != nil {
		t.Fatalf("testutil: migrate schema %s: %v", schema, err)
	}
	return database
}

// withSearchPath appends the schema to a DSN as a connection parameter.
func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + url.QueryEscape(schema)
}

// schemaName derives a valid, unique identifier from the test's name.
func schemaName(t *testing.T) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '_'
		}
	}, t.Name())
	if len(safe) > 40 {
		safe = safe[:40]
	}
	return fmt.Sprintf("t_%s_%d", safe, schemaCounter.Add(1))
}

// quoteIdent quotes an identifier for interpolation into DDL.
//
// Schema names here are derived from test names, not from user input, but DDL
// cannot take placeholders — quoting keeps a test named with a stray character
// from producing broken SQL.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// WrapSQL wraps a raw *sql.DB into a *db.DB for ent-based constructors.
//
// Tests that already hold a *sql.DB — usually because the service under test
// takes one — need the ent-backed handle for a collaborator. Failure is fatal
// rather than returned: it can only mean the driver is misconfigured, which is
// a broken test rather than a case worth asserting on.
func WrapSQL(t *testing.T, conn *sql.DB) *db.DB {
	t.Helper()
	wrapped, err := db.WrapSQL(conn)
	if err != nil {
		t.Fatalf("testutil: wrap sql db: %v", err)
	}
	return wrapped
}
