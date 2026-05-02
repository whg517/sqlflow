package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a database/sql connection for SQLite.
type DB struct {
	*sql.DB
}

// Open creates a new SQLite connection and enables WAL mode.
func Open(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Enable foreign keys.
	if _, err := conn.Exec("PRAGMA foreign_keys=ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	return &DB{conn}, nil
}

// Migrate runs schema migrations for all tables.
func (db *DB) Migrate() error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    username    TEXT    NOT NULL UNIQUE,
    password_hash TEXT  NOT NULL,
    role        TEXT    NOT NULL DEFAULT 'developer',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate users: %w", err)
	}
	return nil
}
