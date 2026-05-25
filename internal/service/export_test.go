package service

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// newExportTestDB creates an in-memory SQLite database with audit_logs, tickets, and users schemas.
func newExportTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)

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
    ai_review_result    TEXT    NOT NULL DEFAULT '',
    ticket_id           INTEGER NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS users (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    username     TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'developer',
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS tickets (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    submitter_id     INTEGER NOT NULL,
    datasource_id    INTEGER NOT NULL,
    database         TEXT    NOT NULL DEFAULT '',
    sql_content      TEXT    NOT NULL DEFAULT '',
    sql_summary      TEXT    NOT NULL DEFAULT '',
    db_type          TEXT    NOT NULL DEFAULT 'mysql',
    change_reason    TEXT    NOT NULL DEFAULT '',
    status           TEXT    NOT NULL DEFAULT 'SUBMITTED',
    risk_level       TEXT    NOT NULL DEFAULT '',
    ai_review_result TEXT    NOT NULL DEFAULT '',
    reviewer_id      INTEGER NOT NULL DEFAULT 0,
    review_comment   TEXT    NOT NULL DEFAULT '',
    scheduled_at     DATETIME,
    executed_at      DATETIME,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	return db
}

// seedAuditLogs inserts sample audit log data for testing.
func seedAuditLogs(t *testing.T, db *sql.DB, count int) {
	t.Helper()
	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('developer1', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(db, 0, 0)
	for i := 0; i < count; i++ {
		svc.Write(context.Background(), AuditRecord{
			UserID:       int64(i%2 + 1),
			Action:       "query_execute",
			DatasourceID: int64(i%3 + 1),
			Database:     fmt.Sprintf("db_%d", i%3+1),
			SQLContent:   fmt.Sprintf("SELECT * FROM table_%d WHERE id = %d", i%3+1, i),
			SQLSummary:   fmt.Sprintf("SELECT * FROM table_%d ...", i%3+1),
			ResultRows:   int64(i * 10),
			ExecutionTimeMs: int64(i * 5 + 10),
			IPAddress:    "10.0.0.1",
		})
	}
}

// seedTickets inserts sample ticket data for testing.
func seedTickets(t *testing.T, db *sql.DB, count int) {
	t.Helper()
	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	for i := 0; i < count; i++ {
		_, err := db.Exec(
			`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
			1, i%2+1, fmt.Sprintf("db_%d", i%2+1),
			fmt.Sprintf("ALTER TABLE users ADD COLUMN col_%d INT", i),
			fmt.Sprintf("ALTER TABLE users ADD ..."),
			"mysql",
			fmt.Sprintf("Adding column %d", i),
			"SUBMITTED",
			[]string{"low", "medium", "high"}[i%3],
		)
		if err != nil {
			t.Fatalf("insert ticket %d: %v", i, err)
		}
	}
}

func TestExportService_HasPermission(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	svc := NewExportService(db, NewAuditService(db, 0, 0))

	tests := []struct {
		role       string
		exportType ExportType
		want       bool
	}{
		{"admin", ExportTypeAudit, true},
		{"dba", ExportTypeAudit, true},
		{"developer", ExportTypeAudit, false},
		{"admin", ExportTypeTicket, true},
		{"dba", ExportTypeTicket, true},
		{"developer", ExportTypeTicket, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.role, tt.exportType), func(t *testing.T) {
			got := svc.hasExportPermission(tt.role, tt.exportType)
			if got != tt.want {
				t.Errorf("hasExportPermission(%q, %q) = %v, want %v", tt.role, tt.exportType, got, tt.want)
			}
		})
	}
}

func TestExportService_ExportAuditLogs_AdminSuccess(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	seedAuditLogs(t, db, 5)

	result, err := svc.ExportAuditLogs(context.Background(), 1, "admin", "admin", AuditExportFilters{})
	if err != nil {
		t.Fatalf("ExportAuditLogs: %v", err)
	}

	if result.TotalRows != 5 {
		t.Errorf("expected 5 rows, got %d", result.TotalRows)
	}
	if len(result.CSVBytes) == 0 {
		t.Error("expected non-empty CSV bytes")
	}
	// Verify BOM header
	if len(result.CSVBytes) < 3 || result.CSVBytes[0] != 0xEF || result.CSVBytes[1] != 0xBB || result.CSVBytes[2] != 0xBF {
		t.Error("expected UTF-8 BOM header")
	}
	// Verify filename
	if !strings.Contains(result.Filename, "audit_logs_") {
		t.Errorf("expected filename to contain 'audit_logs_', got %q", result.Filename)
	}
}

func TestExportService_ExportAuditLogs_DeveloperDenied(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	seedAuditLogs(t, db, 5)

	_, err := svc.ExportAuditLogs(context.Background(), 2, "developer1", "developer", AuditExportFilters{})
	if err != ErrExportNoPermission {
		t.Errorf("expected ErrExportNoPermission, got %v", err)
	}
}

func TestExportService_ExportAuditLogsWithFilters(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	seedAuditLogs(t, db, 10)

	t.Run("filter by action", func(t *testing.T) {
		result, err := svc.ExportAuditLogs(context.Background(), 1, "admin", "admin", AuditExportFilters{Action: "query_execute"})
		if err != nil {
			t.Fatalf("ExportAuditLogs: %v", err)
		}
		if result.TotalRows != 10 {
			t.Errorf("expected 10 rows, got %d", result.TotalRows)
		}
	})
}

func TestExportService_ExportAuditLogs_Watermark(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	seedAuditLogs(t, db, 3)

	result, err := svc.ExportAuditLogs(context.Background(), 1, "admin", "admin", AuditExportFilters{})
	if err != nil {
		t.Fatalf("ExportAuditLogs: %v", err)
	}

	csvStr := string(result.CSVBytes)
	if !strings.Contains(csvStr, "导出水印:") {
		t.Error("expected watermark in CSV output")
	}
	if !strings.Contains(csvStr, "导出人=admin") {
		t.Error("expected username in watermark")
	}
	if !strings.Contains(csvStr, "仅限内部使用") {
		t.Error("expected '仅限内部使用' in watermark")
	}
}

func TestExportService_ExportAuditLogs_ExceedsLimit(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	// Seed more than ExportMaxRows
	seedAuditLogs(t, db, ExportMaxRows+1)

	_, err := svc.ExportAuditLogs(context.Background(), 1, "admin", "admin", AuditExportFilters{})
	if err != ErrExportExceedsLimit {
		t.Errorf("expected ErrExportExceedsLimit, got %v", err)
	}
}

func TestExportService_ExportTickets_AuthenticatedSuccess(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	seedTickets(t, db, 5)

	result, err := svc.ExportTickets(context.Background(), 1, "admin", "admin", TicketExportFilters{})
	if err != nil {
		t.Fatalf("ExportTickets: %v", err)
	}

	if result.TotalRows != 5 {
		t.Errorf("expected 5 rows, got %d", result.TotalRows)
	}
	if len(result.CSVBytes) == 0 {
		t.Error("expected non-empty CSV bytes")
	}
	// Verify BOM header
	if result.CSVBytes[0] != 0xEF || result.CSVBytes[1] != 0xBB || result.CSVBytes[2] != 0xBF {
		t.Error("expected UTF-8 BOM header")
	}
}

func TestExportService_ExportTickets_WithFilters(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	seedTickets(t, db, 10)

	t.Run("filter by status", func(t *testing.T) {
		result, err := svc.ExportTickets(context.Background(), 1, "admin", "admin", TicketExportFilters{Status: "SUBMITTED"})
		if err != nil {
			t.Fatalf("ExportTickets: %v", err)
		}
		if result.TotalRows != 10 {
			t.Errorf("expected 10 rows for SUBMITTED, got %d", result.TotalRows)
		}
	})

	t.Run("filter by risk_level", func(t *testing.T) {
		result, err := svc.ExportTickets(context.Background(), 1, "admin", "admin", TicketExportFilters{RiskLevel: "high"})
		if err != nil {
			t.Fatalf("ExportTickets: %v", err)
		}
		// high is every 3rd record: indices 2, 5, 8 → 3 records
		if result.TotalRows != 3 {
			t.Errorf("expected 3 rows for high risk, got %d", result.TotalRows)
		}
	})
}

func TestExportService_ExportTickets_Watermark(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	seedTickets(t, db, 2)

	result, err := svc.ExportTickets(context.Background(), 1, "admin", "admin", TicketExportFilters{})
	if err != nil {
		t.Fatalf("ExportTickets: %v", err)
	}

	csvStr := string(result.CSVBytes)
	if !strings.Contains(csvStr, "导出水印:") {
		t.Error("expected watermark in CSV output")
	}
	if !strings.Contains(csvStr, "导出人=admin") {
		t.Error("expected username in watermark")
	}
}

func TestExportService_ExportTickets_ExceedsLimit(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	seedTickets(t, db, ExportMaxRows+1)

	_, err := svc.ExportTickets(context.Background(), 1, "admin", "admin", TicketExportFilters{})
	if err != ErrExportExceedsLimit {
		t.Errorf("expected ErrExportExceedsLimit, got %v", err)
	}
}

func TestExportService_ExportAuditLogs_Empty(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	result, err := svc.ExportAuditLogs(context.Background(), 1, "admin", "admin", AuditExportFilters{})
	if err != nil {
		t.Fatalf("ExportAuditLogs empty: %v", err)
	}
	if result.TotalRows != 0 {
		t.Errorf("expected 0 rows, got %d", result.TotalRows)
	}
}

func TestExportService_ExportTickets_Empty(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := NewAuditService(db, 0, 0)
	svc := NewExportService(db, auditSvc)

	result, err := svc.ExportTickets(context.Background(), 1, "admin", "admin", TicketExportFilters{})
	if err != nil {
		t.Fatalf("ExportTickets empty: %v", err)
	}
	if result.TotalRows != 0 {
		t.Errorf("expected 0 rows, got %d", result.TotalRows)
	}
}

func TestAddBOM(t *testing.T) {
	result := addBOM("hello")
	if len(result) != 8 { // 3 BOM bytes + 5 "hello" bytes
		t.Errorf("expected 8 bytes, got %d", len(result))
	}
	if result[0] != 0xEF || result[1] != 0xBB || result[2] != 0xBF {
		t.Error("BOM bytes incorrect")
	}
	if string(result[3:]) != "hello" {
		t.Errorf("content after BOM = %q, want %q", string(result[3:]), "hello")
	}
}

func TestBuildAuditCSV_WatermarkFormat(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := NewAuditService(db, 0, 0)
	svc.Write(context.Background(), AuditRecord{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT 1",
		SQLSummary: "SELECT 1",
	})

	rows, err := db.Query(`SELECT a.id, a.user_id, a.action, a.datasource_id, a.database, a.sql_content, a.sql_summary,
		a.result_rows, a.affected_rows, a.execution_time_ms, a.error_message,
		a.desensitized_fields, a.ip_address, a.ai_review_result, a.ticket_id, a.created_at,
		COALESCE(u.username, '') AS username
		FROM audit_logs a LEFT JOIN users u ON a.user_id = u.id ORDER BY a.created_at DESC LIMIT 1`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	csv := buildAuditCSV(context.Background(), rows, "alice")

	// Verify watermark line
	if !strings.Contains(csv, "导出水印:") {
		t.Error("expected watermark marker")
	}
	if !strings.Contains(csv, "导出人=alice") {
		t.Error("expected '导出人=alice' in watermark")
	}
	// Verify header
	if !strings.Contains(csv, "ID,时间,用户,操作") {
		t.Error("expected CSV header row")
	}
}
