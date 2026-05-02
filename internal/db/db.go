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

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS datasources (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    name                TEXT    NOT NULL UNIQUE,
    type                TEXT    NOT NULL,
    host                TEXT    NOT NULL,
    port                INTEGER NOT NULL,
    username            TEXT    NOT NULL DEFAULT '',
    password_encrypted  TEXT    NOT NULL DEFAULT '',
    database            TEXT    NOT NULL DEFAULT '',
    max_open            INTEGER NOT NULL DEFAULT 10,
    max_idle            INTEGER NOT NULL DEFAULT 5,
    max_lifetime        INTEGER NOT NULL DEFAULT 3600,
    max_idle_time       INTEGER NOT NULL DEFAULT 600,
    status              TEXT    NOT NULL DEFAULT 'active',
    created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at          DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate datasources: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS casbin_rule (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    ptype TEXT    NOT NULL DEFAULT '',
    v0    TEXT    NOT NULL DEFAULT '',
    v1    TEXT    NOT NULL DEFAULT '',
    v2    TEXT    NOT NULL DEFAULT '',
    v3    TEXT    NOT NULL DEFAULT '',
    v4    TEXT    NOT NULL DEFAULT '',
    v5    TEXT    NOT NULL DEFAULT ''
);
	`)
	if err != nil {
		return fmt.Errorf("migrate casbin_rule: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS query_history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL,
    datasource_id   INTEGER NOT NULL,
    database        TEXT    NOT NULL DEFAULT '',
    sql_content     TEXT    NOT NULL,
    sql_summary     TEXT    NOT NULL DEFAULT '',
    db_type         TEXT    NOT NULL DEFAULT 'mysql',
    execution_time  INTEGER NOT NULL DEFAULT 0,
    result_rows     INTEGER NOT NULL DEFAULT 0,
    affected_rows   INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate query_history: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_query_history_user_id ON query_history(user_id)`)
	if err != nil {
		return fmt.Errorf("migrate query_history index: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS audit_logs (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL,
    action              TEXT    NOT NULL DEFAULT '',
    datasource_id       INTEGER NOT NULL DEFAULT 0,
    database            TEXT    NOT NULL DEFAULT '',
    sql_content         TEXT    NOT NULL DEFAULT '',
    sql_summary         TEXT    NOT NULL DEFAULT '',
    result_rows         INTEGER NOT NULL DEFAULT 0,
    affected_rows       INTEGER NOT NULL DEFAULT 0,
    execution_time_ms   INTEGER NOT NULL DEFAULT 0,
    error_message       TEXT    NOT NULL DEFAULT '',
    desensitized_fields TEXT    NOT NULL DEFAULT '',
    ip_address          TEXT    NOT NULL DEFAULT '',
    created_at          DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id)`)
	if err != nil {
		return fmt.Errorf("migrate audit_logs index: %w", err)
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS mask_rules (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    datasource_id   INTEGER NOT NULL DEFAULT 0,
    database        TEXT    NOT NULL DEFAULT '',
    table_name      TEXT    NOT NULL DEFAULT '',
    field           TEXT    NOT NULL DEFAULT '',
    mask_type       TEXT    NOT NULL DEFAULT '',
    custom_regex    TEXT    NOT NULL DEFAULT '',
    custom_template TEXT    NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		return fmt.Errorf("migrate mask_rules: %w", err)
	}

	return nil
}
