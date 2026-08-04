package query

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
	_ "modernc.org/sqlite"
)

// newExportTestDB creates an in-memory SQLite database with audit_logs, tickets, and users schemas.
// newExportTestDB returns a migrated database scoped to this test.
//
// It used to open a temp-file SQLite and hand-write CREATE TABLE for the tables
// it needed, which let the test schema drift from the migrations it was meant
// to stand in for.
func newExportTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testutil.NewDB(t).DB
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

	svc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	for i := 0; i < count; i++ {
		svc.Write(context.Background(), auditlog.Record{
			UserID:          int64(i%2 + 1),
			Action:          "query_execute",
			DatasourceID:    int64(i%3 + 1),
			Database:        fmt.Sprintf("db_%d", i%3+1),
			SQLContent:      fmt.Sprintf("SELECT * FROM table_%d WHERE id = %d", i%3+1, i),
			SQLSummary:      fmt.Sprintf("SELECT * FROM table_%d ...", i%3+1),
			ResultRows:      int64(i * 10),
			ExecutionTimeMs: int64(i*5 + 10),
			IPAddress:       "10.0.0.1",
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
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())`,
			1, i%2+1, fmt.Sprintf("db_%d", i%2+1),
			fmt.Sprintf("ALTER TABLE users ADD COLUMN col_%d INT", i),
			"ALTER TABLE users ADD ...",
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
	svc := NewExportService(testutil.WrapSQL(t, db), audit.NewService(testutil.WrapSQL(t, db), 0, 0))

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
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

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
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 5)

	_, err := svc.ExportAuditLogs(context.Background(), 2, "developer1", "developer", AuditExportFilters{})
	if err != ErrExportNoPermission {
		t.Errorf("expected ErrExportNoPermission, got %v", err)
	}
}

func TestExportService_ExportAuditLogsWithFilters(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

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
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

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
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

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
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

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
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

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
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

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
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

	seedTickets(t, db, ExportMaxRows+1)

	_, err := svc.ExportTickets(context.Background(), 1, "admin", "admin", TicketExportFilters{})
	if err != ErrExportExceedsLimit {
		t.Errorf("expected ErrExportExceedsLimit, got %v", err)
	}
}

func TestExportService_ExportAuditLogs_Empty(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

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
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

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

func TestStreamExportAuditLogs_CSVOutput(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	svc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc.Write(context.Background(), auditlog.Record{
		UserID:     1,
		Action:     "query_execute",
		SQLContent: "SELECT 1",
		SQLSummary: "SELECT 1",
	})

	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	exportSvc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	_, err = exportSvc.StreamExportAuditLogs(context.Background(), &buf, "alice", AuditExportFilters{})
	if err != nil {
		t.Fatalf("StreamExportAuditLogs: %v", err)
	}

	csv := buf.String()

	// Verify header
	if !strings.Contains(csv, "ID,时间,用户,操作") {
		t.Error("expected CSV header row")
	}
}

func TestExportService_StreamExportAuditLogs(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 5)

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	written, err := svc.StreamExportAuditLogs(context.Background(), &buf, "admin", AuditExportFilters{})
	if err != nil {
		t.Fatalf("StreamExportAuditLogs: %v", err)
	}
	if written != 5 {
		t.Errorf("written = %d, want 5", written)
	}

	csv := buf.String()
	if !strings.Contains(csv, "ID,时间,用户,操作") {
		t.Error("expected CSV header")
	}
	// Should have BOM
	if !strings.HasPrefix(csv, "\xEF\xBB\xBF") {
		t.Error("expected BOM header")
	}
}

func TestExportService_StreamExportTickets(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

	seedTickets(t, db, 3)

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	written, err := svc.StreamExportTickets(context.Background(), &buf, "admin", TicketExportFilters{})
	if err != nil {
		t.Fatalf("StreamExportTickets: %v", err)
	}
	if written != 3 {
		t.Errorf("written = %d, want 3", written)
	}

	csv := buf.String()
	if !strings.Contains(csv, "ID,提交人,提交人ID") {
		t.Error("expected CSV header")
	}
}

func TestExportService_ValidateExport(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 5)

	t.Run("admin can validate audit export", func(t *testing.T) {
		total, err := svc.ValidateExport(context.Background(), "admin", ExportTypeAudit, AuditExportFilters{})
		if err != nil {
			t.Fatalf("ValidateExport: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
	})

	t.Run("developer cannot validate audit export", func(t *testing.T) {
		_, err := svc.ValidateExport(context.Background(), "developer", ExportTypeAudit, AuditExportFilters{})
		if err != ErrExportNoPermission {
			t.Errorf("expected ErrExportNoPermission, got %v", err)
		}
	})

	t.Run("developer can validate ticket export", func(t *testing.T) {
		_, err := svc.ValidateExport(context.Background(), "developer", ExportTypeTicket, TicketExportFilters{})
		if err != nil {
			t.Fatalf("ValidateExport: %v", err)
		}
	})
}

func TestExportService_ValidateExport_ExceedsLimit(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, ExportMaxRows+1)

	total, err := svc.ValidateExport(context.Background(), "admin", ExportTypeAudit, AuditExportFilters{})
	if err != ErrExportExceedsLimit {
		t.Errorf("expected ErrExportExceedsLimit, got %v", err)
	}
	if total != ExportMaxRows+1 {
		t.Errorf("total = %d, want %d", total, ExportMaxRows+1)
	}
}

func TestExportService_StreamExport_ContextCancellation(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := NewExportService(testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf strings.Builder
	_, err := svc.StreamExportAuditLogs(ctx, &buf, "admin", AuditExportFilters{})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestEscapeCSVFormula(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"=SUM(A1:A10)", "'=SUM(A1:A10)"},
		{"+cmd", "'+cmd"},
		{"-cmd", "'-cmd"},
		{"@SUM", "'@SUM"},
		{"\ttab", "'\ttab"},
		{"\rreturn", "'\rreturn"},
		{"normal text", "normal text"},
		{"", ""},
	}

	for _, tt := range tests {
		got := escapeCSVFormula(tt.input)
		if got != tt.want {
			t.Errorf("escapeCSVFormula(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
