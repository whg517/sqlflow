package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps a database/sql connection for SQLite.
type DB struct {
	*sql.DB
}

// Open creates a new SQLite connection and enables WAL mode.
func Open(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// Use file: URI with WAL pragma so every connection from the pool
	// automatically gets the correct settings. modernc.org/sqlite
	// requires this because PRAGMAs set via Exec only apply to the
	// single connection that executed them, not to pooled connections.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", dbPath)

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite only allows one writer at a time. Limit the pool to 1
	// open connection to avoid "database is locked" / I/O errors
	// when multiple goroutines write concurrently.
	conn.SetMaxOpenConns(1)

	return &DB{conn}, nil
}

// Migrate runs all pending schema migrations using golang-migrate.
// For existing databases, version 000001 uses CREATE TABLE IF NOT EXISTS
// so it is safe to run against a database that already has tables.
func (db *DB) Migrate() error {
	return MigrateDB(db.DB)
}

// MigrateDB runs migrations on the given *sql.DB connection.
// This is the shared entry point used by both production and test code.
func MigrateDB(conn *sql.DB) error {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	driver, err := sqlite.WithInstance(conn, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", d, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	// --- shared_results (SF-FEAT0038: query result sharing) ---
	_, err = conn.Exec(`
CREATE TABLE IF NOT EXISTS shared_results (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL,
    username        TEXT    NOT NULL DEFAULT '',
    token           TEXT    NOT NULL UNIQUE,
    columns_json    TEXT    NOT NULL DEFAULT '[]',
    rows_json       TEXT    NOT NULL DEFAULT '[]',
    row_count       INTEGER NOT NULL DEFAULT 0,
    expires_at      DATETIME NOT NULL,
    password_hash   TEXT    NOT NULL DEFAULT '',
    sql_summary     TEXT    NOT NULL DEFAULT '',
    datasource_name TEXT    NOT NULL DEFAULT '',
    revoked         INTEGER NOT NULL DEFAULT 0,
    revoked_at      DATETIME,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate shared_results: %w", err)
	}

	_, err = conn.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_shared_results_token ON shared_results(token)`)
	if err != nil {
		return fmt.Errorf("migrate shared_results token index: %w", err)
	}

	_, err = conn.Exec(`CREATE INDEX IF NOT EXISTS idx_shared_results_user ON shared_results(user_id)`)
	if err != nil {
		return fmt.Errorf("migrate shared_results user index: %w", err)
	}

	_, err = conn.Exec(`CREATE INDEX IF NOT EXISTS idx_shared_results_expires ON shared_results(expires_at)`)
	if err != nil {
		return fmt.Errorf("migrate shared_results expires index: %w", err)
	}

	return nil
}
