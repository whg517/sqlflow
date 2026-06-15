package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newExportAsyncTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	dsn := "file:" + dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	// Create necessary tables
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'developer', created_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME NOT NULL DEFAULT (datetime('now')));
CREATE TABLE IF NOT EXISTS audit_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL, action TEXT NOT NULL DEFAULT '', datasource_id INTEGER NOT NULL DEFAULT 0, database TEXT NOT NULL DEFAULT '', sql_content TEXT NOT NULL DEFAULT '', sql_summary TEXT NOT NULL DEFAULT '', result_rows INTEGER NOT NULL DEFAULT 0, affected_rows INTEGER NOT NULL DEFAULT 0, execution_time_ms INTEGER NOT NULL DEFAULT 0, error_message TEXT NOT NULL DEFAULT '', desensitized_fields TEXT NOT NULL DEFAULT '', ip_address TEXT NOT NULL DEFAULT '', ai_review_result TEXT NOT NULL DEFAULT '', ticket_id INTEGER NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT (datetime('now')));
CREATE TABLE IF NOT EXISTS tickets (id INTEGER PRIMARY KEY AUTOINCREMENT, submitter_id INTEGER NOT NULL, datasource_id INTEGER NOT NULL, database TEXT NOT NULL DEFAULT '', sql_content TEXT NOT NULL, sql_summary TEXT NOT NULL DEFAULT '', db_type TEXT NOT NULL DEFAULT 'mysql', change_reason TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'SUBMITTED', risk_level TEXT NOT NULL DEFAULT '', ai_review_result TEXT NOT NULL DEFAULT '', reviewer_id INTEGER NOT NULL DEFAULT 0, review_comment TEXT NOT NULL DEFAULT '', scheduled_at DATETIME, executed_at DATETIME, created_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME NOT NULL DEFAULT (datetime('now')));
CREATE TABLE IF NOT EXISTS export_tasks (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL, username TEXT NOT NULL DEFAULT '', export_type TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending', filename TEXT NOT NULL DEFAULT '', file_path TEXT NOT NULL DEFAULT '', file_format TEXT NOT NULL DEFAULT '', total_rows INTEGER NOT NULL DEFAULT 0, file_bytes INTEGER NOT NULL DEFAULT 0, filters_json TEXT NOT NULL DEFAULT '{}', error_msg TEXT NOT NULL DEFAULT '', created_at DATETIME NOT NULL DEFAULT (datetime('now')), completed_at DATETIME);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	return db, dir
}

func TestExportAsyncService_CreateAndRetrieve(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	exportSvc := NewExportService(mustWrapDB(db), auditSvc)
	asyncSvc := NewExportAsyncService(mustWrapDB(db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	// Insert a test user
	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")

	// Insert some audit logs
	auditSvc.Write(context.Background(), AuditRecord{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT 1",
		SQLSummary: "SELECT 1",
	})

	filters := AuditExportFilters{}
	filtersJSON, _ := json.Marshal(filters)

	task, err := asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin", "audit", string(filtersJSON), "csv")
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
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	exportSvc := NewExportService(mustWrapDB(db), auditSvc)
	asyncSvc := NewExportAsyncService(mustWrapDB(db), exportSvc, auditSvc, dataDir)
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
	_, err = asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin", "audit", string(filtersJSON), "csv")
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
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	exportSvc := NewExportService(mustWrapDB(db), auditSvc)
	asyncSvc := NewExportAsyncService(mustWrapDB(db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('dev', 'hash', 'developer')")

	filtersJSON, _ := json.Marshal(AuditExportFilters{})
	_, err := asyncSvc.CreateAsyncExport(context.Background(), 2, "dev", "developer", "audit", string(filtersJSON), "csv")
	if err != ErrExportNoPermission {
		t.Errorf("expected ErrExportNoPermission, got %v", err)
	}
}

func TestExportAsyncService_DownloadFile(t *testing.T) {
	db, dataDir := newExportAsyncTestDB(t)
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	exportSvc := NewExportService(mustWrapDB(db), auditSvc)
	asyncSvc := NewExportAsyncService(mustWrapDB(db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")
	auditSvc.Write(context.Background(), AuditRecord{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT 1",
		SQLSummary: "SELECT 1",
	})

	filtersJSON, _ := json.Marshal(AuditExportFilters{})
	task, err := asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin", "audit", string(filtersJSON), "csv")
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
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	exportSvc := NewExportService(mustWrapDB(db), auditSvc)
	asyncSvc := NewExportAsyncService(mustWrapDB(db), exportSvc, auditSvc, dataDir)
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
	auditSvc := NewAuditService(mustWrapDB(db), 0, 0)
	exportSvc := NewExportService(mustWrapDB(db), auditSvc)
	asyncSvc := NewExportAsyncService(mustWrapDB(db), exportSvc, auditSvc, dataDir)
	defer asyncSvc.Close()

	_, _ = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")
	auditSvc.Write(context.Background(), AuditRecord{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT 1",
		SQLSummary: "SELECT 1",
	})

	filtersJSON, _ := json.Marshal(AuditExportFilters{})
	task, _ := asyncSvc.CreateAsyncExport(context.Background(), 1, "admin", "admin", "audit", string(filtersJSON), "csv")

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
	_, _ = db.Exec(`UPDATE export_tasks SET completed_at = ? WHERE id = ?`, past, task.ID)

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
