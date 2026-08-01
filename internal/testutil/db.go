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
//	// for constructors that take a raw *sql.DB
//	raw := testutil.NewDB(t).DB
package testutil

import (
	"database/sql"
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
