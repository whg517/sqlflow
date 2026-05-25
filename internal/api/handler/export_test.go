package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/service"
)

// setupExportTest creates a fresh Echo, DB, AuditService, ExportService, and ExportHandler for testing.
func setupExportTest(t *testing.T) (*echo.Echo, *service.AuditService, *service.ExportService, *ExportHandler) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert users for the JOIN
	_, err = database.Exec("INSERT INTO users (username, password_hash, role) VALUES ('admin', 'hash', 'admin')")
	if err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	_, err = database.Exec("INSERT INTO users (username, password_hash, role) VALUES ('developer', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert developer: %v", err)
	}

	auditSvc := service.NewAuditService(database.DB, 0, 0)
	exportSvc := service.NewExportService(database.DB, auditSvc)
	handler := NewExportHandler(exportSvc)

	e := echo.New()
	return e, auditSvc, exportSvc, handler
}

// setContextUser sets the user context values (simulating JWT middleware).
func setContextUser(c echo.Context, userID int64, username, role string) {
	c.Set("user_id", userID)
	c.Set("username", username)
	c.Set("role", role)
}

func TestExportHandler_ExportAuditLogs_Admin(t *testing.T) {
	e, auditSvc, _, h := setupExportTest(t)

	// Seed 5 audit logs
	for i := 0; i < 5; i++ {
		auditSvc.Write(context.Background(), service.AuditRecord{
			UserID:       1,
			Action:       "query_execute",
			SQLContent:   "SELECT 1",
			SQLSummary:   "SELECT 1",
			ExecutionTimeMs: int64(i * 10),
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/export/audit", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "admin", "admin")

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
	e, _, _, h := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/export/audit", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 2, "developer", "developer")

	err := h.ExportAuditLogs(c)
	if err != nil {
		t.Fatalf("ExportAuditLogs: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body=%s", rec.Code, rec.Body.String())
	}

	// Verify error message
	var result map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	if msg, ok := result["message"].(string); !ok || !strings.Contains(msg, "导出权限") {
		t.Errorf("expected permission error message, got %v", result["message"])
	}
}

func TestExportHandler_ExportAuditLogs_ExceedsLimit(t *testing.T) {
	e, auditSvc, _, h := setupExportTest(t)

	// Seed more than 10000 records
	for i := 0; i < service.ExportMaxRows+1; i++ {
		auditSvc.Write(context.Background(), service.AuditRecord{
			UserID:     1,
			Action:     "query_execute",
			SQLContent: "SELECT 1",
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/export/audit", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "admin", "admin")

	err := h.ExportAuditLogs(c)
	if err != nil {
		t.Fatalf("ExportAuditLogs: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var result map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	if msg, ok := result["message"].(string); !ok || !strings.Contains(msg, "10000") {
		t.Errorf("expected row limit error message, got %v", result["message"])
	}
}

func TestExportHandler_ExportTickets_Success(t *testing.T) {
	t.Skip("ticket seed requires DB access - tested via service tests")
}

func TestExportHandler_ExportTickets_DeveloperAllowed(t *testing.T) {
	e, _, _, h := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/export/tickets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 2, "developer", "developer")

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

func TestExportHandler_ExportTickets_Watermark(t *testing.T) {
	e, _, _, h := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/export/tickets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "admin", "admin")

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
	e, _, _, h := setupExportTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	<-ctx.Done()

	req := httptest.NewRequest(http.MethodGet, "/api/export/audit", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "admin", "admin")

	// Should not panic, even with expired context
	_ = h.ExportAuditLogs(c)
}
