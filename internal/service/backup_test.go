package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/db"
)

func setupBackupServiceTest(t *testing.T) (*BackupService, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := database.Migrate(); err != nil {
		database.Close()
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.BackupConfig{
		Enabled:  true,
		Dir:      backupDir,
		Interval: 999 * 24 * time.Hour,
		Keep:     5,
		Compress: false,
	}

	svc := NewBackupService(database.DB, dbPath, cfg)
	cleanup := func() {
		database.Close()
	}

	return svc, backupDir, cleanup
}

func TestBackupService_RunBackup(t *testing.T) {
	svc, backupDir, cleanup := setupBackupServiceTest(t)
	defer cleanup()

	err := svc.RunBackup()
	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}

	found := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "sqlflow-") {
			found = true
			info, _ := entry.Info()
			if info != nil && info.Size() == 0 {
				t.Errorf("backup file %s is empty", entry.Name())
			}
		}
	}
	if !found {
		t.Error("no backup file created")
	}
}

func TestBackupService_RunBackup_Compressed(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.BackupConfig{
		Enabled:  true,
		Dir:      backupDir,
		Interval: 999 * 24 * time.Hour,
		Keep:     5,
		Compress: true,
	}

	svc := NewBackupService(database.DB, dbPath, cfg)

	err = svc.RunBackup()
	if err != nil {
		t.Fatalf("RunBackup compressed: %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}

	found := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "sqlflow-") && strings.HasSuffix(entry.Name(), ".gz") {
			found = true
		}
	}
	if !found {
		t.Error("no compressed backup file created")
	}

	// Uncompressed file should not exist
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "sqlflow-") && !strings.HasSuffix(entry.Name(), ".gz") {
			t.Errorf("uncompressed backup file should not exist: %s", entry.Name())
		}
	}
}

func TestBackupService_ListBackups(t *testing.T) {
	svc, _, cleanup := setupBackupServiceTest(t)
	defer cleanup()

	// No backups initially
	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}

	// Create a backup
	if err := svc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	backups, err = svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}

	b := backups[0]
	if b.Filename == "" {
		t.Error("filename should not be empty")
	}
	if b.Size == 0 {
		t.Error("backup size should not be zero")
	}
	if b.Compressed {
		t.Error("expected uncompressed backup")
	}
}

func TestBackupService_DeleteBackup(t *testing.T) {
	svc, _, cleanup := setupBackupServiceTest(t)
	defer cleanup()

	// Create a backup first
	if err := svc.RunBackup(); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}

	backups, _ := svc.ListBackups()
	if len(backups) == 0 {
		t.Fatal("expected at least 1 backup")
	}

	filename := backups[0].Filename

	err := svc.DeleteBackup(filename)
	if err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}

	// Verify deleted
	backups, _ = svc.ListBackups()
	if len(backups) != 0 {
		t.Errorf("expected 0 backups after delete, got %d", len(backups))
	}
}

func TestBackupService_DeleteBackup_NotFound(t *testing.T) {
	svc, _, cleanup := setupBackupServiceTest(t)
	defer cleanup()

	err := svc.DeleteBackup("sqlflow-nonexistent.db")
	if err == nil {
		t.Error("expected error for non-existent backup")
	}
}

func TestBackupService_DeleteBackup_InvalidFilename(t *testing.T) {
	svc, _, cleanup := setupBackupServiceTest(t)
	defer cleanup()

	tests := []struct {
		name string
		file string
	}{
		{"no_prefix", "random-file.db"},
		{"path_traversal", "sqlflow-../../../etc/passwd"},
		{"slash", "sqlflow-test/file.db"},
		{"backslash", "sqlflow-test\\file.db"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.DeleteBackup(tt.file)
			if err == nil {
				t.Errorf("expected error for invalid filename %q", tt.file)
			}
		})
	}
}

func TestBackupService_Rotation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	backupDir := filepath.Join(tmpDir, "backups")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.BackupConfig{
		Enabled:  true,
		Dir:      backupDir,
		Interval: 999 * 24 * time.Hour,
		Keep:     3,
		Compress: false,
	}

	svc := NewBackupService(database.DB, dbPath, cfg)

	// Create 5 backups
	for i := 0; i < 5; i++ {
		if err := svc.RunBackup(); err != nil {
			t.Fatalf("RunBackup[%d]: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond) // ensure different timestamps
	}

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}

	// Should keep only 3 most recent
	if len(backups) > 3 {
		t.Errorf("expected at most 3 backups after rotation, got %d", len(backups))
	}
}

func TestBackupService_BackupDir(t *testing.T) {
	svc, backupDir, cleanup := setupBackupServiceTest(t)
	defer cleanup()

	if svc.BackupDir() != backupDir {
		t.Errorf("BackupDir() = %q, want %q", svc.BackupDir(), backupDir)
	}
}

func TestBackupService_Start_Disabled(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.BackupConfig{
		Enabled:  false,
		Dir:      filepath.Join(tmpDir, "backups"),
		Interval: 1 * time.Second,
		Keep:     5,
		Compress: false,
	}

	svc := NewBackupService(database.DB, dbPath, cfg)

	// Start should return immediately when disabled
	svc.Start()
	// No crash means success
}

func TestBackupService_Stop(t *testing.T) {
	svc, _, cleanup := setupBackupServiceTest(t)
	defer cleanup()

	// Stop without start should be safe
	svc.Stop()
}

func TestBackupInfo(t *testing.T) {
	info := BackupInfo{
		Filename:   "sqlflow-20240101-120000.db",
		Filepath:   "/backups/sqlflow-20240101-120000.db",
		Size:       1024,
		Compressed: false,
	}

	if info.Filename != "sqlflow-20240101-120000.db" {
		t.Errorf("Filename = %q", info.Filename)
	}
	if info.Size != 1024 {
		t.Errorf("Size = %d", info.Size)
	}
	if info.Compressed {
		t.Error("expected Compressed=false")
	}
}
