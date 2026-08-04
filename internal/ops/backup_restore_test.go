package ops

import (
	"database/sql"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/whg517/sqlflow/internal/testutil"
)

// TestBackupRestoreRoundTrip restores a backup into a database of its own and
// checks the data is there.
//
// Everything else about backups can pass while the product still cannot
// recover: a dump that omits a sequence, or names a schema the target does not
// have, fails only when someone tries to use it — during an incident. This runs
// the recovery path the way an operator would.
func TestBackupRestoreRoundTrip(t *testing.T) {
	requirePgDump(t)
	requirePsql(t)

	source, dsn := testutil.NewDBWithDSN(t)
	userID := testutil.SeedUser(t, source.DB, "restore_probe", "developer")
	if _, err := source.Exec(
		`INSERT INTO audit_logs (user_id, action, sql_content, sql_summary, database)
		 VALUES ($1, 'query_execute', 'SELECT * FROM orders WHERE 订单状态 = 1', '查询订单明细', 'shop')`,
		userID,
	); err != nil {
		t.Fatalf("seed audit record: %v", err)
	}

	cfg := defaultBackupConfig()
	cfg.Dir = filepath.Join(t.TempDir(), "backups")
	svc := NewBackupService(dsn, cfg)
	if err := svc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	backups, err := svc.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("ListBackups = %v, %v; want exactly one backup", backups, err)
	}
	dumpPath := filepath.Join(cfg.Dir, backups[0].Filename)

	target, targetDSN := newRestoreTarget(t)

	// The dump names the source schema, which does not exist in the fresh
	// database until the dump itself creates it.
	dump, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	restore := exec.Command("psql", targetDSN, "--quiet", "--no-psqlrc", "-v", "ON_ERROR_STOP=1")
	restore.Stdin = strings.NewReader(string(dump))
	var restoreErr strings.Builder
	restore.Stderr = &restoreErr
	if err := restore.Run(); err != nil {
		t.Fatalf("restore failed: %v\n%s", err, restoreErr.String())
	}

	sourceSchema := schemaFromDSN(t, dsn)
	if _, err := target.Exec("SET search_path TO " + pq(sourceSchema)); err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	var users, audits, roles int
	if err := target.QueryRow(
		`SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM audit_logs), (SELECT count(*) FROM roles)`,
	).Scan(&users, &audits, &roles); err != nil {
		t.Fatalf("count restored rows: %v", err)
	}
	if users != 1 || audits != 1 {
		t.Errorf("restored users=%d audits=%d, want 1/1", users, audits)
	}
	// The built-in roles come from the migration's seed, so their absence would
	// mean the dump carried structure without data.
	if roles != 3 {
		t.Errorf("restored roles = %d, want the 3 built-ins", roles)
	}

	// The search function and its indexes are objects the dump has to carry as
	// much as the tables are — without them the restored platform comes up with
	// audit search broken.
	var matches int
	if err := target.QueryRow(
		`SELECT count(*) FROM audit_logs a
		 WHERE audit_search_text(a.sql_content, a.sql_summary, a.action, a.error_message, a.database) ILIKE '%订单%'`,
	).Scan(&matches); err != nil {
		t.Fatalf("search on restored database: %v", err)
	}
	if matches != 1 {
		t.Errorf("restored search matched %d records, want 1", matches)
	}

	// Sequences are the classic omission: the table restores, and the first
	// insert afterwards collides with an existing id.
	var newID int64
	if err := target.QueryRow(
		`INSERT INTO users (username, password_hash, role) VALUES ('after_restore', 'hash', 'developer') RETURNING id`,
	).Scan(&newID); err != nil {
		t.Fatalf("insert after restore: %v — the sequence did not come across", err)
	}
	if newID <= userID {
		t.Errorf("id after restore = %d, want greater than the restored %d", newID, userID)
	}
}

// requirePsql fails the test when the psql client is missing.
func requirePsql(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("psql"); err != nil {
		t.Fatalf("psql 不在 PATH 上：恢复演练需要客户端工具 (%v)", err)
	}
}

// newRestoreTarget creates an empty database to restore into, and drops it on
// cleanup.
//
// A database rather than a schema: the dump carries its own CREATE SCHEMA, so
// restoring beside the source would collide with it.
func newRestoreTarget(t *testing.T) (*sql.DB, string) {
	t.Helper()

	name := "restore_" + strings.ToLower(strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	if len(name) > 40 {
		name = name[:40]
	}

	admin, err := sql.Open("pgx", testutil.TestDSN())
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer func() { _ = admin.Close() }()

	if _, err := admin.Exec("DROP DATABASE IF EXISTS " + pq(name)); err != nil {
		t.Fatalf("drop stale restore database: %v", err)
	}
	if _, err := admin.Exec("CREATE DATABASE " + pq(name)); err != nil {
		t.Fatalf("create restore database: %v", err)
	}

	dsn := replaceDatabase(t, testutil.TestDSN(), name)
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open restore database: %v", err)
	}
	// pg_trgm is a database-level object that pg_dump does not carry, the same
	// way a fresh production database needs it created before migrating.
	if _, err := conn.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm"); err != nil {
		t.Fatalf("create pg_trgm in restore database: %v", err)
	}

	t.Cleanup(func() {
		_ = conn.Close()
		dropper, err := sql.Open("pgx", testutil.TestDSN())
		if err != nil {
			return
		}
		defer func() { _ = dropper.Close() }()
		if _, err := dropper.Exec("DROP DATABASE IF EXISTS " + pq(name)); err != nil {
			t.Logf("drop restore database %s: %v", name, err)
		}
	})

	return conn, dsn
}

// schemaFromDSN reads the search_path a test DSN carries.
func schemaFromDSN(t *testing.T, dsn string) string {
	t.Helper()
	conn, err := parsePostgresDSN(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	if len(conn.schemas) == 0 {
		t.Fatal("test DSN carries no search_path")
	}
	return conn.schemas[0]
}

// replaceDatabase swaps the database name in a DSN.
//
// Through net/url rather than string replacement: the database name is also the
// username here, and a substring replace rewrote the credentials instead.
func replaceDatabase(t *testing.T, dsn, name string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

// pq quotes an identifier for interpolation into DDL, which takes no
// placeholders.
func pq(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
