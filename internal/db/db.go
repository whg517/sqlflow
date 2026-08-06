// Package db opens the platform metadata database and runs its migrations.
//
// The platform store is PostgreSQL (ADR-0009). This package is the only place
// allowed to use database/sql directly against it: migrations, connection setup
// and health checks all happen before an ent client exists. Every domain package
// goes through ent (ADR-0010).
package db

import (
	"database/sql"
	"embed"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/whg517/sqlflow/internal/db/ent"

	"sort"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Default pool sizing. PostgreSQL has no single-writer restriction, so unlike
// the SQLite era there is no reason to cap the pool at one connection — that cap
// was what made holding a rows cursor while writing deadlock until ctx timeout.
const (
	defaultMaxOpenConns = 25
	defaultMaxIdleConns = 5
)

// DB carries both the ent client and the raw connection the migration runner
// needs.
//
// The embedded *sql.DB is still reachable from domain packages during the
// migration to ent; ADR-0010 records the exit condition that removes it.
type DB struct {
	*sql.DB
	client *ent.Client
}

// Client returns the ent client for type-safe database operations.
func (db *DB) Client() *ent.Client {
	return db.client
}

// Open connects to PostgreSQL and initializes the ent client on the same pool.
//
// dsn is a libpq-style connection string, e.g.
// "postgres://user:pass@localhost:5432/sqlflow?sslmode=disable".
func Open(dsn string) (*DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	conn.SetMaxOpenConns(defaultMaxOpenConns)
	conn.SetMaxIdleConns(defaultMaxIdleConns)

	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return WrapSQL(conn)
}

// WrapSQL builds a DB around an existing connection.
//
// Tests use it to attach ent to a connection that is already scoped to a
// per-test schema.
func WrapSQL(conn *sql.DB) (*DB, error) {
	drv := entsql.OpenDB(dialect.Postgres, conn)
	return &DB{DB: conn, client: ent.NewClient(ent.Driver(drv))}, nil
}

// Migrate runs all pending schema migrations.
func (db *DB) Migrate() error {
	return MigrateDB(db.DB)
}

// Close closes the ent client and the underlying pool.
func (db *DB) Close() error {
	if err := db.client.Close(); err != nil {
		return fmt.Errorf("close ent client: %w", err)
	}
	return db.DB.Close()
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrateDB applies the embedded migrations to conn.
//
// DDL lives in these files rather than in ent auto-migration: the migration
// files are the single source of truth, and ent's schema is kept in step with
// them by hand. ADR-0002's original reason for this — SQLite's ALTER TABLE
// rebuilding tables — no longer applies, but the explicit-DDL decision was kept
// on its own merits when that ADR was split.
func MigrateDB(conn *sql.DB) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	driver, err := migratepg.WithInstance(conn, &migratepg.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// ApplySchema executes the migration SQL directly, skipping golang-migrate's
// version table and advisory lock.
//
// It exists for tests, which build a fresh schema per case: going through the
// migration runner cost roughly five seconds each, almost all of it lock
// acquisition and version bookkeeping that a brand-new schema has no use for.
//
// Production must keep using MigrateDB — this skips exactly the machinery that
// makes migrating an existing database safe. The equivalence it relies on is
// that MigrateDB against an empty schema is just the up-migrations run in
// order, which is what this does.
func ApplySchema(conn *sql.DB) error {
	names, err := SchemaFiles()
	if err != nil {
		return err
	}
	for _, name := range names {
		stmt, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := conn.Exec(string(stmt)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

// SchemaFiles lists the up-migrations in version order.
//
// The ordering is the whole contract: migrations are named NNNNNN_*.up.sql and
// a later one may depend on an earlier one having run. Sorting by name gives
// the same sequence golang-migrate would apply, which is what makes ApplySchema
// equivalent to MigrateDB on an empty schema.
//
// An empty set is an error rather than a no-op: it means the embedded
// filesystem did not pick the migrations up, and a caller would otherwise get a
// schema-less database reported as ready.
func SchemaFiles() ([]string, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var ups []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	if len(ups) == 0 {
		return nil, fmt.Errorf("no up-migrations found in the embedded migrations directory")
	}
	return ups, nil
}
