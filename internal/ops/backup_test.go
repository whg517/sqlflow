package ops

import (
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/testutil"
)

// requirePgDump fails the test when pg_dump is not installed.
//
// Fatal rather than skipped: these tests already require a running PostgreSQL,
// and a silent skip would report a backup feature as covered on a machine where
// it cannot possibly work.
func requirePgDump(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Fatalf("pg_dump 不在 PATH 上：备份走的是 pg_dump，测试需要客户端工具 (%v)", err)
	}
}

// newBackupService returns a service pointed at a schema of its own.
func newBackupService(t *testing.T, cfg config.BackupConfig) (*BackupService, string) {
	t.Helper()
	requirePgDump(t)

	_, dsn := testutil.NewDBWithDSN(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	cfg.Dir = backupDir
	return NewBackupService(dsn, cfg), backupDir
}

// defaultBackupConfig is the shape most cases want: enabled, no scheduler tick
// within the test's lifetime, keeping five.
func defaultBackupConfig() config.BackupConfig {
	return config.BackupConfig{
		Enabled:  true,
		Interval: 999 * 24 * time.Hour,
		Keep:     5,
		Compress: false,
	}
}

// readBackup returns a backup file's contents, decompressing when needed.
func readBackup(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = f.Close() }()

	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("open gzip: %v", err)
		}
		defer func() { _ = gz.Close() }()
		r = gz
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	return string(data)
}

// TestBackupService_RunBackup checks that the dump can actually rebuild the
// database, not merely that a file appeared.
//
// The previous version asserted only that some file existed and was non-empty,
// which a one-line error message satisfies just as well as a real dump.
func TestBackupService_RunBackup(t *testing.T) {
	svc, backupDir := newBackupService(t, defaultBackupConfig())

	if err := svc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("backups = %d, want 1", len(backups))
	}

	content := readBackup(t, filepath.Join(backupDir, backups[0].Filename))
	for _, want := range []string{"CREATE TABLE", "audit_logs", "tickets", "users"} {
		if !strings.Contains(content, want) {
			t.Errorf("dump does not contain %q — it cannot restore the platform", want)
		}
	}
}

// TestBackupService_RunBackup_CoversOnlyItsOwnSchema pins the --schema flag.
//
// Several deployments put the platform in a named schema of a shared database.
// Dumping the whole database would carry away whatever else lives there — other
// applications' tables, in a file an operator hands around as "the SQLFlow
// backup".
func TestBackupService_RunBackup_CoversOnlyItsOwnSchema(t *testing.T) {
	database, dsn := testutil.NewDBWithDSN(t)
	requirePgDump(t)

	// A table outside the platform's schema, which the dump must not pick up.
	if _, err := database.Exec(`CREATE SCHEMA IF NOT EXISTS backup_outsider`); err != nil {
		t.Fatalf("create outside schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DROP SCHEMA IF EXISTS backup_outsider CASCADE`)
	})
	if _, err := database.Exec(`CREATE TABLE backup_outsider.secrets (id int)`); err != nil {
		t.Fatalf("create outside table: %v", err)
	}

	cfg := defaultBackupConfig()
	cfg.Dir = filepath.Join(t.TempDir(), "backups")
	svc := NewBackupService(dsn, cfg)
	if err := svc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	backups, _ := svc.ListBackups()
	if len(backups) != 1 {
		t.Fatalf("backups = %d, want 1", len(backups))
	}
	content := readBackup(t, filepath.Join(cfg.Dir, backups[0].Filename))
	if strings.Contains(content, "backup_outsider") {
		t.Error("dump reaches outside the platform's schema")
	}
	if !strings.Contains(content, "audit_logs") {
		t.Error("dump is missing the platform's own tables")
	}
}

func TestBackupService_RunBackup_Compressed(t *testing.T) {
	cfg := defaultBackupConfig()
	cfg.Compress = true
	svc, backupDir := newBackupService(t, cfg)

	if err := svc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("files in backup dir = %d, want 1", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasSuffix(name, ".gz") {
		t.Fatalf("backup %q is not compressed", name)
	}

	// The point of compressing is that it still restores. A truncated gzip
	// stream — which is what a missing Close() produces — reads as a valid file
	// until something tries to decompress it.
	content := readBackup(t, filepath.Join(backupDir, name))
	if !strings.Contains(content, "CREATE TABLE") {
		t.Error("decompressed dump has no DDL")
	}
}

// TestBackupService_RunBackup_TwiceInSameSecond guards a backup against being
// destroyed by the next one.
//
// The filename carried a second-resolution timestamp, so a manual trigger
// landing in the same second as the scheduled run produced the same name and
// silently overwrote the earlier backup — the failure mode is invisible until a
// restore is needed.
func TestBackupService_RunBackup_TwiceInSameSecond(t *testing.T) {
	svc, _ := newBackupService(t, defaultBackupConfig())

	for i := range 2 {
		if err := svc.RunBackup(); err != nil {
			t.Fatalf("RunBackup[%d]: %v", i, err)
		}
	}

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Errorf("backups = %d, want 2 — one run overwrote the other", len(backups))
	}
}

// TestBackupService_RunBackup_LeavesNoPartialFile checks that a failed dump does
// not leave something that looks like a backup.
func TestBackupService_RunBackup_LeavesNoPartialFile(t *testing.T) {
	requirePgDump(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	cfg := defaultBackupConfig()
	cfg.Dir = backupDir

	// A database that does not exist: pg_dump connects, fails, and writes
	// nothing to stdout.
	svc := NewBackupService("postgres://nobody:nobody@127.0.0.1:1/does_not_exist?sslmode=disable", cfg)

	if err := svc.RunBackup(); err == nil {
		t.Fatal("RunBackup succeeded against an unreachable database")
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("failed backup left %d file(s) behind: %v", len(entries), entries[0].Name())
	}
}

func TestBackupService_ListBackups(t *testing.T) {
	svc, _ := newBackupService(t, defaultBackupConfig())

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("backups before any run = %d, want 0", len(backups))
	}

	if err := svc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	backups, err = svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("backups = %d, want 1", len(backups))
	}
	b := backups[0]
	if b.Filename == "" || b.Size == 0 || b.Compressed {
		t.Errorf("backup info = %+v, want a named, non-empty, uncompressed entry", b)
	}
}

func TestBackupService_DeleteBackup(t *testing.T) {
	svc, _ := newBackupService(t, defaultBackupConfig())

	if err := svc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	backups, _ := svc.ListBackups()
	if len(backups) == 0 {
		t.Fatal("no backup to delete")
	}

	if err := svc.DeleteBackup(backups[0].Filename); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	backups, _ = svc.ListBackups()
	if len(backups) != 0 {
		t.Errorf("backups after delete = %d, want 0", len(backups))
	}
}

func TestBackupService_DeleteBackup_NotFound(t *testing.T) {
	svc, _ := newBackupService(t, defaultBackupConfig())

	if err := svc.DeleteBackup("sqlflow-nonexistent.sql"); err == nil {
		t.Error("deleting a non-existent backup reported success")
	}
}

// TestBackupService_DeleteBackup_InvalidFilename checks that the filename from
// the request cannot escape the backup directory.
//
// The handler passes it through, so this guard is what stands between a
// delete-backup call and an arbitrary file on the host.
func TestBackupService_DeleteBackup_InvalidFilename(t *testing.T) {
	svc, backupDir := newBackupService(t, defaultBackupConfig())

	outside := filepath.Join(filepath.Dir(backupDir), "outside.txt")
	if err := os.WriteFile(outside, []byte("keep me"), 0600); err != nil {
		t.Fatalf("write bait file: %v", err)
	}

	tests := []struct {
		name string
		file string
	}{
		{"no_prefix", "random-file.sql"},
		{"path_traversal", "sqlflow-../outside.txt"},
		{"slash", "sqlflow-test/file.sql"},
		{"backslash", "sqlflow-test\\file.sql"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.DeleteBackup(tt.file); err == nil {
				t.Errorf("DeleteBackup(%q) reported success", tt.file)
			}
		})
	}

	if _, err := os.Stat(outside); err != nil {
		t.Errorf("a file outside the backup directory was removed: %v", err)
	}
}

// TestBackupService_Rotation checks that rotation keeps exactly Keep backups.
//
// The old assertion was "at most 3", which a single file also satisfies — and a
// single file is what the second-resolution filename actually produced, so the
// test passed while rotation was never exercised.
func TestBackupService_Rotation(t *testing.T) {
	cfg := defaultBackupConfig()
	cfg.Keep = 3
	svc, _ := newBackupService(t, cfg)

	for i := range 5 {
		if err := svc.RunBackup(); err != nil {
			t.Fatalf("RunBackup[%d]: %v", i, err)
		}
	}

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 3 {
		t.Fatalf("backups after 5 runs with Keep=3 = %d, want 3", len(backups))
	}
	// Rotation must drop the oldest, not an arbitrary three: ListBackups sorts
	// newest first, and the filenames sort the same way.
	names := []string{backups[0].Filename, backups[1].Filename, backups[2].Filename}
	if names[0] <= names[1] || names[1] <= names[2] {
		t.Errorf("backups are not the newest three in order: %v", names)
	}
}

func TestBackupService_BackupDir(t *testing.T) {
	svc, backupDir := newBackupService(t, defaultBackupConfig())

	if svc.BackupDir() != backupDir {
		t.Errorf("BackupDir() = %q, want %q", svc.BackupDir(), backupDir)
	}
}

// TestBackupService_Start_Disabled checks that a disabled scheduler takes no
// backups, rather than only that Start does not panic.
func TestBackupService_Start_Disabled(t *testing.T) {
	cfg := defaultBackupConfig()
	cfg.Enabled = false
	cfg.Interval = 10 * time.Millisecond
	svc, backupDir := newBackupService(t, cfg)

	svc.Start()
	defer svc.Stop()
	time.Sleep(100 * time.Millisecond)

	if entries, err := os.ReadDir(backupDir); err == nil && len(entries) > 0 {
		t.Errorf("a disabled scheduler produced %d backup(s)", len(entries))
	}
}

// TestBackupService_Stop_WithoutStart checks that shutdown is safe on a service
// that never ran — the path taken when startup fails partway through.
func TestBackupService_Stop_WithoutStart(t *testing.T) {
	svc, _ := newBackupService(t, defaultBackupConfig())
	svc.Stop()
	svc.Stop() // idempotent: the container may unwind more than once
}
