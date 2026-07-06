// Package testutil provides shared test helpers for the SQLFlow test suite.
//
// Historically every *_test.go file declared its own setupXxxTestDB helper
// (17+ duplicated implementations) that opened an in-memory / temp-dir
// SQLite via internal/db and ran migrations. This package centralizes that
// so tests stop duplicating ~15 lines of boilerplate and stop drifting in
// behavior (some used internal/db.Open, others raw sql.Open with hand
// written DDL).
//
// Usage:
//
//	// returns *db.DB (ent + raw sql) with migrations applied + t.Cleanup
//	d := testutil.NewDB(t)
//	// returns the underlying *sql.DB for tests that need raw SQL
//	rawDB := testutil.NewSQLDB(t)
package testutil

import (
	"path/filepath"
	"testing"

	"github.com/whg517/sqlflow/internal/db"
)

// NewDB returns a migrated *db.DB backed by a per-test temp-dir SQLite file.
// The DB is closed automatically via t.Cleanup.
func NewDB(t *testing.T) *db.DB {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("testutil: open db: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("testutil: close db: %v", err)
		}
	})
	if err := database.Migrate(); err != nil {
		t.Fatalf("testutil: migrate db: %v", err)
	}
	return database
}

// NewSQLDB returns the *sql.DB of a migrated per-test SQLite database.
// Equivalent to NewDB(t).DB but convenient for tests that only need raw SQL.
// The connection is closed automatically via t.Cleanup (via the *db.DB).
func NewSQLDB(t *testing.T) *db.DB {
	return NewDB(t)
}
