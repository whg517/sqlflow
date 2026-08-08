package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

// setupExportTest creates a fresh Echo, DB, AuditService, ExportService, and ExportHandler for testing.
func setupExportTest(t *testing.T) (*echo.Echo, *audit.Service, *ExportService, *AsyncExportService, *ExportHandler, *db.DB) {
	t.Helper()

	database := testutil.NewDB(t)

	// Insert users for the JOIN
	_, err := database.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")
	if err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	_, err = database.Exec("INSERT INTO users (username, password_hash, role) VALUES ('developer', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert developer: %v", err)
	}

	auditSvc := audit.NewService(database, 0, 0)
	exportSvc := newExportServiceForTest(t, database, auditSvc)
	exportAsyncSvc := NewAsyncExportService(database, exportSvc, auditSvc, t.TempDir())
	t.Cleanup(func() { exportAsyncSvc.Close() })
	handler := NewExportHandler(exportSvc, exportAsyncSvc)

	e := echo.New()
	return e, auditSvc, exportSvc, exportAsyncSvc, handler, database
}

// setContextUser sets the user context values (simulating JWT middleware).
func TestExportHandler_ExportAuditLogs_Admin(t *testing.T) {
	e, auditSvc, _, _, h, _ := setupExportTest(t)

	// Seed 5 audit logs
	for i := 0; i < 5; i++ {
		auditSvc.Write(context.Background(), auditlog.Record{
			UserID:          1,
			Action:          "query_execute",
			SQLContent:      "SELECT 1",
			SQLSummary:      "SELECT 1",
			ExecutionTimeMs: int64(i * 10),
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/export/audit", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")

	err := h.ExportAuditLogs(c)
	if err != nil {
		t.Fatalf("ExportAuditLogs: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	// Verify BOM header
	body := rec.Body.Bytes()
	if len(body) < 3 || body[0] != 0xEF || body[1] != 0xBB || body[2] != 0xBF {
		t.Error("expected UTF-8 BOM header")
	}

	// Verify watermark
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "导出水印:") {
		t.Error("expected watermark in CSV")
	}
	if !strings.Contains(bodyStr, "导出人=admin") {
		t.Error("expected '导出人=admin' in watermark")
	}

	// Verify Content-Disposition header
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("expected Content-Disposition with attachment, got %q", cd)
	}
}

func TestExportHandler_ExportAuditLogs_DeveloperDenied(t *testing.T) {
	e, _, _, _, h, _ := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/export/audit", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 2, "developer", "developer")

	err := h.ExportAuditLogs(c)
	if err != nil {
		t.Fatalf("ExportAuditLogs: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body=%s", rec.Code, rec.Body.String())
	}

	// Verify error message
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON response body: %v; body=%s", err, rec.Body.String())
	}
	if msg, ok := result["message"].(string); !ok || !strings.Contains(msg, "导出权限") {
		t.Errorf("expected permission error message, got %v", result["message"])
	}
}

func TestExportHandler_ExportAuditLogs_ExceedsLimit(t *testing.T) {
	e, auditSvc, _, _, h, _ := setupExportTest(t)

	// Seed more than 10000 records
	for i := 0; i < ExportMaxRows+1; i++ {
		auditSvc.Write(context.Background(), auditlog.Record{
			UserID:     1,
			Action:     "query_execute",
			SQLContent: "SELECT 1",
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/export/audit", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")

	err := h.ExportAuditLogs(c)
	if err != nil {
		t.Fatalf("ExportAuditLogs: %v", err)
	}

	// When rows exceed the sync limit, the handler auto-switches to async export (202)
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202 (async export created), got %d; body=%s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON response body: %v; body=%s", err, rec.Body.String())
	}
	if msg, ok := result["message"].(string); !ok || !strings.Contains(msg, "后台生成") {
		t.Errorf("expected async export message, got %v", result["message"])
	}
	data, ok := result["data"].(map[string]interface{})
	if !ok || data["task_id"] == nil {
		t.Errorf("expected task_id in response data, got %v", result["data"])
	}
}

func TestExportHandler_ExportTickets_DeveloperAllowed(t *testing.T) {
	e, _, _, _, h, _ := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/export/tickets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 2, "developer", "developer")

	err := h.ExportTickets(c)
	if err != nil {
		t.Fatalf("ExportTickets: %v", err)
	}

	// Developer should be able to export tickets — even if empty
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	// Verify BOM header
	body := rec.Body.Bytes()
	if len(body) < 3 || body[0] != 0xEF || body[1] != 0xBB || body[2] != 0xBF {
		t.Error("expected UTF-8 BOM header")
	}
}

// seedTicketFor inserts one ticket owned by the given submitter and returns the
// SQL text that identifies it in an export.
func seedTicketFor(t *testing.T, database *db.DB, submitterID int64, marker string) string {
	t.Helper()
	sqlContent := "ALTER TABLE t ADD COLUMN " + marker + " INT"
	_, err := database.Exec(
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, created_at, updated_at)
		 VALUES ($1, 1, 'appdb', $2, $3, 'mysql', $4, 'SUBMITTED', 'low', now(), now())`,
		submitterID, sqlContent, marker, "reason "+marker,
	)
	if err != nil {
		t.Fatalf("insert ticket for submitter %d: %v", submitterID, err)
	}
	return sqlContent
}

// TestExportHandler_ExportTickets_DeveloperSeesOnlyOwn is the regression test for
// a horizontal privilege escalation: the export path used to answer "may this
// role export tickets?" with an unconditional yes and then apply no owner
// predicate, so any authenticated developer could download every ticket in the
// system — SQL, change reason, submitter and reviewer identity included.
//
// The ticket list endpoint has always restricted a non-governance role to its
// own tickets. Export is a second entrance to the same rows and must draw the
// same boundary.
func TestExportHandler_ExportTickets_DeveloperSeesOnlyOwn(t *testing.T) {
	e, _, _, _, h, database := setupExportTest(t)

	adminSQL := seedTicketFor(t, database, 1, "admin_secret_col")
	devSQL := seedTicketFor(t, database, 2, "dev_own_col")

	req := httptest.NewRequest(http.MethodGet, "/api/export/tickets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 2, "developer", "developer")

	if err := h.ExportTickets(c); err != nil {
		t.Fatalf("ExportTickets: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, devSQL) {
		t.Errorf("developer's own ticket missing from export; body=%s", body)
	}
	if strings.Contains(body, adminSQL) {
		t.Errorf("export leaked another user's ticket SQL %q; body=%s", adminSQL, body)
	}
}

// TestExportHandler_ExportTickets_GovernanceSeesAll pins the other half of the
// boundary: narrowing the export must not blind the roles whose job is to see
// every ticket.
func TestExportHandler_ExportTickets_GovernanceSeesAll(t *testing.T) {
	e, _, _, _, h, database := setupExportTest(t)

	adminSQL := seedTicketFor(t, database, 1, "admin_own_col")
	devSQL := seedTicketFor(t, database, 2, "dev_other_col")

	req := httptest.NewRequest(http.MethodGet, "/api/export/tickets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")

	if err := h.ExportTickets(c); err != nil {
		t.Fatalf("ExportTickets: %v", err)
	}

	body := rec.Body.String()
	for _, want := range []string{adminSQL, devSQL} {
		if !strings.Contains(body, want) {
			t.Errorf("admin export missing ticket %q; body=%s", want, body)
		}
	}
}

func TestExportHandler_ExportTickets_Watermark(t *testing.T) {
	e, _, _, _, h, _ := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/export/tickets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")

	err := h.ExportTickets(c)
	if err != nil {
		t.Fatalf("ExportTickets: %v", err)
	}

	bodyStr := rec.Body.String()
	if !strings.Contains(bodyStr, "导出水印:") {
		t.Error("expected watermark in CSV")
	}
	if !strings.Contains(bodyStr, "导出人=admin") {
		t.Error("expected '导出人=admin' in watermark")
	}
}

// Ensure ExportHandler doesn't leak resources
func TestExportHandler_ContextTimeout(t *testing.T) {
	e, _, _, _, h, _ := setupExportTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	<-ctx.Done()

	req := httptest.NewRequest(http.MethodGet, "/api/export/audit", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")

	// Should not panic, even with expired context
	_ = h.ExportAuditLogs(c)
}
