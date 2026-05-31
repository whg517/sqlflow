package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/whg517/sqlflow/internal/db/ent"
	_ "modernc.org/sqlite"

	// Preserve golang-migrate for Phase 1 dual-track compatibility.
	// Phase 3 will remove this import and all migration files.
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// DB wraps both a raw *sql.DB and an ent.Client for dual-track operation.
// During Phase 1-2, both are available. Phase 3 will remove the raw SQL path.
type DB struct {
	*sql.DB
	client *ent.Client
}

// Client returns the ent client for type-safe database operations.
func (db *DB) Client() *ent.Client {
	return db.client
}

// Open creates a new SQLite connection with WAL mode and initializes the ent client.
func Open(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", dbPath)

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite only allows one writer at a time. Limit the pool to 1
	// open connection to avoid "database is locked" / I/O errors.
	conn.SetMaxOpenConns(1)

	// Initialize ent client backed by the same *sql.DB connection pool.
	drv := entsql.OpenDB(dialect.SQLite, conn)
	client := ent.NewClient(ent.Driver(drv))

	return &DB{DB: conn, client: client}, nil
}

// WrapSQL wraps an existing *sql.DB connection with an ent client.
// This is useful for tests that already have a *sql.DB and need to
// pass a *DB to services that require ent client access.
func WrapSQL(conn *sql.DB) (*DB, error) {
	drv := entsql.OpenDB(dialect.SQLite, conn)
	client := ent.NewClient(ent.Driver(drv))
	return &DB{DB: conn, client: client}, nil
}

// Migrate runs all pending schema migrations using golang-migrate.
//
// Phase 1: golang-migrate manages all DDL. ent schemas are defined but
// ent auto-migrate is NOT run — SQLite ALTER TABLE limitations cause ent
// to DROP+recreate tables, losing SQL-level DEFAULT expressions like
// datetime('now'). (See Marcus CR feedback on commit 884eb10.)
//
// Phase 3: Will switch to ent auto-migrate and remove golang-migrate.
func (db *DB) Migrate() error {
	return MigrateDB(db.DB)
}

// Close closes both the ent client and the underlying *sql.DB.
func (db *DB) Close() error {
	if err := db.client.Close(); err != nil {
		return fmt.Errorf("close ent client: %w", err)
	}
	return db.DB.Close()
}

// --- Legacy golang-migrate functions preserved for Phase 1 ---

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrateDB runs golang-migrate on the given *sql.DB connection.
// Preserved for Phase 1 dual-track. Phase 3 will remove this.
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
