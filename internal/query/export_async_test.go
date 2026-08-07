package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

// newExportAsyncTestDB returns a real migrated schema plus a scratch directory
// for the exported files.
//
// It used to hand-write CREATE TABLE for four production tables in a temp-file
// SQLite database. Those definitions had already drifted — the DDL carried
// DEFAULT now(), which SQLite has no such function for — and a schema written
// out by hand cannot catch the column rename it exists to protect against.
func newExportAsyncTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	return testutil.NewDB(t).DB, t.TempDir()
}

func TestExportAsyncService_CreateAndRetrieve(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	exportSvc := NewExportService(testutil.WrapSQL(t, db), auditSvc)
	asyncSvc := NewAsyncExportService(testutil.WrapSQL(t, db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	// Insert a test user
	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")

	// Insert some audit logs
	auditSvc.Write(context.Background(), auditlog.Record{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT 1",
		SQLSummary: "SELECT 1",
	})

	filters := AuditExportFilters{}
	filtersJSON, _ := json.Marshal(filters)

	task, err := asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin", "audit", string(filtersJSON), "csv", nil)
	if err != nil {
		t.Fatalf("CreateAsyncExport: %v", err)
	}

	if task.ID == 0 {
		t.Error("expected non-zero task ID")
	}
	if task.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", task.Status)
	}

	// Wait for the async export to complete (with timeout)
	deadline := time.After(5 * time.Second)
	for {
		retrieved, err := asyncSvc.GetTask(context.Background(), task.ID, 1)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if retrieved.Status == "completed" || retrieved.Status == "failed" {
			if retrieved.Status != "completed" {
				t.Errorf("expected completed status, got %q, error: %s", retrieved.Status, retrieved.ErrorMsg)
			}
			if retrieved.TotalRows != 1 {
				t.Errorf("expected 1 total row, got %d", retrieved.TotalRows)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for async export to complete")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func TestExportAsyncService_ListTasks(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	exportSvc := NewExportService(testutil.WrapSQL(t, db), auditSvc)
	asyncSvc := NewAsyncExportService(testutil.WrapSQL(t, db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")
	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('dev', 'hash', 'developer')")

	tasks, err := asyncSvc.ListTasks(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks initially, got %d", len(tasks))
	}

	// Create a task
	filtersJSON, _ := json.Marshal(AuditExportFilters{})
	_, err = asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin", "audit", string(filtersJSON), "csv", nil)
	if err != nil {
		t.Fatalf("CreateAsyncExport: %v", err)
	}

	tasks, err = asyncSvc.ListTasks(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListTasks after create: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}

	// Different user should see 0 tasks
	tasks2, _ := asyncSvc.ListTasks(context.Background(), 2)
	if len(tasks2) != 0 {
		t.Errorf("expected 0 tasks for different user, got %d", len(tasks2))
	}
}

func TestExportAsyncService_PermissionDenied(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	exportSvc := NewExportService(testutil.WrapSQL(t, db), auditSvc)
	asyncSvc := NewAsyncExportService(testutil.WrapSQL(t, db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('dev', 'hash', 'developer')")

	filtersJSON, _ := json.Marshal(AuditExportFilters{})
	_, err := asyncSvc.CreateAsyncExport(context.Background(), 2, "dev", "developer", "audit", string(filtersJSON), "csv", nil)
	if err != ErrExportNoPermission {
		t.Errorf("expected ErrExportNoPermission, got %v", err)
	}
}

func TestExportAsyncService_DownloadFile(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	exportSvc := NewExportService(testutil.WrapSQL(t, db), auditSvc)
	asyncSvc := NewAsyncExportService(testutil.WrapSQL(t, db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")
	auditSvc.Write(context.Background(), auditlog.Record{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT 1",
		SQLSummary: "SELECT 1",
	})

	filtersJSON, _ := json.Marshal(AuditExportFilters{})
	task, err := asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin", "audit", string(filtersJSON), "csv", nil)
	if err != nil {
		t.Fatalf("CreateAsyncExport: %v", err)
	}

	// Wait for completion
	deadline := time.After(5 * time.Second)
	for {
		retrieved, _ := asyncSvc.GetTask(context.Background(), task.ID, 1)
		if retrieved.Status == "completed" || retrieved.Status == "failed" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Download the file
	reader, filename, err := asyncSvc.DownloadFile(context.Background(), task.ID, 1)
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	defer reader.Close()

	if filename == "" {
		t.Error("expected non-empty filename")
	}

	// Verify file content exists
	stat, _ := os.Stat(filepath.Join(dataDir, ExportDir, filename))
	if stat == nil {
		t.Error("expected file to exist on disk")
	}
}

func TestExportAsyncService_NotFound(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	exportSvc := NewExportService(testutil.WrapSQL(t, db), auditSvc)
	asyncSvc := NewAsyncExportService(testutil.WrapSQL(t, db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	_, err := asyncSvc.GetTask(context.Background(), 99999, 1)
	if err != ErrExportNotFound {
		t.Errorf("expected ErrExportNotFound, got %v", err)
	}
}

func TestGenerateExportFilename(t *testing.T) {
	fname := generateExportFilename("audit", "csv")
	if fname == "" {
		t.Error("expected non-empty filename")
	}
	if filepath.Ext(fname) != ".csv" {
		t.Errorf("expected .csv extension, got %q", filepath.Ext(fname))
	}
}

func TestExportAsyncService_CleanupExpiredFiles(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	exportSvc := NewExportService(testutil.WrapSQL(t, db), auditSvc)
	asyncSvc := NewAsyncExportService(testutil.WrapSQL(t, db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")
	auditSvc.Write(context.Background(), auditlog.Record{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT 1",
		SQLSummary: "SELECT 1",
	})

	filtersJSON, _ := json.Marshal(AuditExportFilters{})
	task, _ := asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin", "audit", string(filtersJSON), "csv", nil)

	// Wait for completion
	deadline := time.After(5 * time.Second)
	for {
		retrieved, _ := asyncSvc.GetTask(context.Background(), task.ID, 1)
		if retrieved.Status == "completed" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Manually set completed_at to the past to simulate expiry
	past := time.Now().Add(-ExportFileTTL - 1*time.Hour)
	_, _ = db.Exec(`UPDATE export_tasks SET completed_at = $1 WHERE id = $2`, past, task.ID)

	// Run cleanup
	asyncSvc.cleanupExpiredFiles()

	// File should be removed
	_, err := os.Stat(task.FilePath)
	if !os.IsNotExist(err) {
		t.Error("expected export file to be cleaned up")
	}

	// Task should be marked as failed
	retrieved, _ := asyncSvc.GetTask(context.Background(), task.ID, 1)
	if retrieved.Status != "failed" {
		t.Errorf("expected failed status after cleanup, got %q", retrieved.Status)
	}
}
