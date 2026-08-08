package query

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/security"
	"github.com/whg517/sqlflow/internal/testutil"
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

// adminActor and developerActor are the two callers the export tests contrast:
// the seeded policy gives admin a wildcard grant and developer only select, so
// one holds the platform-wide export right and the other holds none.
//
// The ids match the users seedAuditLogs and setupExportTest insert.
var (
	adminActor     = ExportActor{UserID: 1, Username: "admin", Role: "admin"}
	developerActor = ExportActor{UserID: 2, Username: "developer1", Role: "developer"}
)

// newExportServiceForTest builds an ExportService with a real policy engine
// behind it.
//
// Passing nil there would make every grant question answer "no" — safe, but it
// would stop these tests from exercising the seeded policy the production
// wiring actually runs on, which is the thing the export boundary now rests on.
func newExportServiceForTest(t *testing.T, database *db.DB, auditSvc auditlog.Writer) *ExportService {
	t.Helper()
	permSvc, err := security.NewService(database)
	if err != nil {
		t.Fatalf("security.NewService: %v", err)
	}
	return NewExportService(database, permSvc, auditSvc)
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

// TestExportService_CanExportAll pins who may export a whole record class.
//
// This case used to assert hasExportPermission("developer", ticket) == true and
// call that the permission model. It was pinning a horizontal privilege
// escalation: the ticket branch returned true for every role — including the
// empty one — and the query behind it applied no owner predicate, so the
// assertion was really "any authenticated user may download every ticket".
// The unconditional yes is gone; what a developer keeps is a *scoped* export,
// covered by TestExportService_TicketExportIsScopedToSubmitter.
func TestExportService_CanExportAll(t *testing.T) {
	svc := newExportServiceForTest(t, testutil.NewDB(t), nil)

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
		{"developer", ExportTypeTicket, false},
		// An unrecognized role carries no grant. The old switch answered true.
		{"auditor", ExportTypeTicket, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.role, tt.exportType), func(t *testing.T) {
			got, err := svc.canExportAll(context.Background(), ExportActor{UserID: 1, Role: tt.role}, tt.exportType)
			if err != nil {
				t.Fatalf("canExportAll: %v", err)
			}
			if got != tt.want {
				t.Errorf("canExportAll(%q, %q) = %v, want %v", tt.role, tt.exportType, got, tt.want)
			}
		})
	}

	// An empty role is a malformed tuple rather than a policy miss, so it comes
	// back as an error. What matters is that it is never a grant — the old
	// switch returned true for it.
	t.Run("empty_role_is_never_a_grant", func(t *testing.T) {
		for _, exportType := range []ExportType{ExportTypeAudit, ExportTypeTicket} {
			got, err := svc.canExportAll(context.Background(), ExportActor{UserID: 1}, exportType)
			if got {
				t.Errorf("canExportAll(\"\", %q) granted the platform-wide export", exportType)
			}
			if err == nil {
				t.Errorf("canExportAll(\"\", %q) silently returned false; expected the malformed subject to surface", exportType)
			}
		}
	})

	// An unknown export type must not fall through to a grant either.
	t.Run("unknown_type_is_never_a_grant", func(t *testing.T) {
		got, err := svc.canExportAll(context.Background(), adminActor, ExportType("share"))
		if got || err != ErrExportTypeInvalid {
			t.Errorf("canExportAll(admin, \"share\") = (%v, %v), want (false, ErrExportTypeInvalid)", got, err)
		}
	})
}

// TestExportService_TicketExportIsScopedToSubmitter checks the boundary at the
// service seam the async worker also goes through, not just at the handler.
func TestExportService_TicketExportIsScopedToSubmitter(t *testing.T) {
	database := testutil.NewDB(t)
	svc := newExportServiceForTest(t, database, nil)

	seedTickets(t, database.DB, 4) // all submitted by user 1
	if _, err := database.Exec(
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, created_at, updated_at)
		 VALUES (2, 1, 'db_1', 'DROP TABLE mine', 'DROP TABLE mine', 'mysql', 'mine', 'SUBMITTED', 'high', now(), now())`,
	); err != nil {
		t.Fatalf("insert ticket for user 2: %v", err)
	}

	dev := ExportActor{UserID: 2, Username: "developer", Role: "developer"}

	total, err := svc.countTickets(context.Background(), dev, TicketExportFilters{})
	if err != nil {
		t.Fatalf("countTickets: %v", err)
	}
	if total != 1 {
		t.Errorf("developer counted %d tickets, want 1 (only their own)", total)
	}

	var buf strings.Builder
	written, err := svc.StreamExportTickets(context.Background(), &buf, dev, TicketExportFilters{}, nil)
	if err != nil {
		t.Fatalf("StreamExportTickets: %v", err)
	}
	if written != 1 {
		t.Errorf("developer streamed %d rows, want 1", written)
	}
	if strings.Contains(buf.String(), "ALTER TABLE users ADD COLUMN col_0") {
		t.Errorf("stream leaked another submitter's ticket:\n%s", buf.String())
	}

	// The count the caller is shown and the rows it gets must agree; a count
	// that ignored the boundary would drive the async switchover off the wrong
	// number.
	if int64(written) != total {
		t.Errorf("streamed %d rows but counted %d", written, total)
	}
}

// TestExportService_AuditExportRefusesWithoutGrant proves the refusal is at the
// query, not only at the entry point: a caller that reaches the stream directly
// gets an error rather than the table.
func TestExportService_AuditExportRefusesWithoutGrant(t *testing.T) {
	database := testutil.NewDB(t)
	svc := newExportServiceForTest(t, database, nil)

	seedAuditLogs(t, database.DB, 3)

	var buf strings.Builder
	_, err := svc.StreamExportAuditLogs(context.Background(), &buf, developerActor, AuditExportFilters{}, nil)
	if err != ErrExportNoPermission {
		t.Fatalf("StreamExportAuditLogs as developer = %v, want ErrExportNoPermission", err)
	}
	if buf.Len() > 0 && strings.Contains(buf.String(), "query_execute") {
		t.Errorf("refused export still wrote rows:\n%s", buf.String())
	}
}

func TestExportService_ExportAuditLogs_AdminSuccess(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 5)

	result, err := svc.ExportAuditLogs(context.Background(), adminActor, AuditExportFilters{})
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 5)

	_, err := svc.ExportAuditLogs(context.Background(), developerActor, AuditExportFilters{})
	if err != ErrExportNoPermission {
		t.Errorf("expected ErrExportNoPermission, got %v", err)
	}
}

func TestExportService_ExportAuditLogsWithFilters(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 10)

	t.Run("filter by action", func(t *testing.T) {
		result, err := svc.ExportAuditLogs(context.Background(), adminActor, AuditExportFilters{Action: "query_execute"})
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 3)

	result, err := svc.ExportAuditLogs(context.Background(), adminActor, AuditExportFilters{})
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	// Seed more than ExportMaxRows
	seedAuditLogs(t, db, ExportMaxRows+1)

	_, err := svc.ExportAuditLogs(context.Background(), adminActor, AuditExportFilters{})
	if err != ErrExportExceedsLimit {
		t.Errorf("expected ErrExportExceedsLimit, got %v", err)
	}
}

func TestExportService_ExportTickets_AuthenticatedSuccess(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedTickets(t, db, 5)

	result, err := svc.ExportTickets(context.Background(), adminActor, TicketExportFilters{})
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedTickets(t, db, 10)

	t.Run("filter by status", func(t *testing.T) {
		result, err := svc.ExportTickets(context.Background(), adminActor, TicketExportFilters{Status: "SUBMITTED"})
		if err != nil {
			t.Fatalf("ExportTickets: %v", err)
		}
		if result.TotalRows != 10 {
			t.Errorf("expected 10 rows for SUBMITTED, got %d", result.TotalRows)
		}
	})

	t.Run("filter by risk_level", func(t *testing.T) {
		result, err := svc.ExportTickets(context.Background(), adminActor, TicketExportFilters{RiskLevel: "high"})
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedTickets(t, db, 2)

	result, err := svc.ExportTickets(context.Background(), adminActor, TicketExportFilters{})
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedTickets(t, db, ExportMaxRows+1)

	_, err := svc.ExportTickets(context.Background(), adminActor, TicketExportFilters{})
	if err != ErrExportExceedsLimit {
		t.Errorf("expected ErrExportExceedsLimit, got %v", err)
	}
}

func TestExportService_ExportAuditLogs_Empty(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	result, err := svc.ExportAuditLogs(context.Background(), adminActor, AuditExportFilters{})
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	result, err := svc.ExportTickets(context.Background(), adminActor, TicketExportFilters{})
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
	exportSvc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	_, err = exportSvc.StreamExportAuditLogs(context.Background(), &buf, ExportActor{UserID: 1, Username: "alice", Role: "admin"}, AuditExportFilters{}, nil)
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 5)

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	written, err := svc.StreamExportAuditLogs(context.Background(), &buf, adminActor, AuditExportFilters{}, nil)
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedTickets(t, db, 3)

	var buf strings.Builder
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	written, err := svc.StreamExportTickets(context.Background(), &buf, adminActor, TicketExportFilters{}, nil)
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 5)

	t.Run("admin can validate audit export", func(t *testing.T) {
		total, err := svc.ValidateExport(context.Background(), adminActor, ExportTypeAudit, AuditExportFilters{})
		if err != nil {
			t.Fatalf("ValidateExport: %v", err)
		}
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
	})

	t.Run("developer cannot validate audit export", func(t *testing.T) {
		_, err := svc.ValidateExport(context.Background(), developerActor, ExportTypeAudit, AuditExportFilters{})
		if err != ErrExportNoPermission {
			t.Errorf("expected ErrExportNoPermission, got %v", err)
		}
	})

	// A developer keeps the ticket export — the count it validates is just the
	// scoped one. Every ticket here belongs to user 1, so the developer's own
	// slice is empty while the admin sees all three.
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(
			`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, created_at, updated_at)
			 VALUES (1, 1, 'appdb', 'ALTER TABLE t ADD COLUMN c INT', 'ALTER', 'mysql', 'why', 'SUBMITTED', 'low', now(), now())`,
		); err != nil {
			t.Fatalf("insert ticket %d: %v", i, err)
		}
	}

	t.Run("developer validates only their own tickets", func(t *testing.T) {
		total, err := svc.ValidateExport(context.Background(), developerActor, ExportTypeTicket, TicketExportFilters{})
		if err != nil {
			t.Fatalf("ValidateExport: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0 (the developer submitted none of them)", total)
		}
	})

	t.Run("admin validates every ticket", func(t *testing.T) {
		total, err := svc.ValidateExport(context.Background(), adminActor, ExportTypeTicket, TicketExportFilters{})
		if err != nil {
			t.Fatalf("ValidateExport: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
	})

	t.Run("an unidentified caller cannot export tickets", func(t *testing.T) {
		anon := ExportActor{Role: "developer"}
		if _, err := svc.ValidateExport(context.Background(), anon, ExportTypeTicket, TicketExportFilters{}); err != ErrExportNoPermission {
			t.Errorf("expected ErrExportNoPermission for a caller with no user id, got %v", err)
		}
	})
}

func TestExportService_ValidateExport_ExceedsLimit(t *testing.T) {
	db := newExportTestDB(t)
	defer db.Close()
	auditSvc := audit.NewService(testutil.WrapSQL(t, db), 0, 0)
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, ExportMaxRows+1)

	total, err := svc.ValidateExport(context.Background(), adminActor, ExportTypeAudit, AuditExportFilters{})
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
	svc := newExportServiceForTest(t, testutil.WrapSQL(t, db), auditSvc)

	seedAuditLogs(t, db, 5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf strings.Builder
	_, err := svc.StreamExportAuditLogs(ctx, &buf, adminActor, AuditExportFilters{}, nil)
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
