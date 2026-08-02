package audit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/testutil"
)

// setupAuditReportTest builds a fresh Echo, DB-backed AuditReportService and handler.
// It seeds one user (id=1) and a handful of audit logs so the report queries have data.
func setupAuditReportTest(t *testing.T) (*echo.Echo, *ReportService, *ReportHandler, int64) {
	t.Helper()
	d := testutil.NewDB(t)

	reportSvc := NewReportService(d)
	h := NewReportHandler(reportSvc)
	e := echo.New()

	// Seed a user the audit logs can join to.
	uid := testutil.SeedUser(t, d.DB, "alice", "developer")

	// Seed audit logs with a timestamp anchored to "now" so they always fall inside
	// the default 7-day / 30-day report windows regardless of when the test runs.
	// (Hardcoded past dates would silently fall outside the window as time passes.)
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 3; i++ {
		if _, err := d.DB.Exec(
			`INSERT INTO audit_logs (user_id, action, database, error_message, ip_address, execution_time_ms, result_rows, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			uid, "query_execute", "appdb", "", "10.0.0.1", 500+int64(i*100), int64(i), now,
		); err != nil {
			t.Fatalf("seed audit log %d: %v", i, err)
		}
	}
	// One error log.
	if _, err := d.DB.Exec(
		`INSERT INTO audit_logs (user_id, action, database, error_message, ip_address, execution_time_ms, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uid, "query_execute", "appdb", "syntax error", "10.0.0.1", 100, now,
	); err != nil {
		t.Fatalf("seed error audit log: %v", err)
	}

	return e, reportSvc, h, uid
}

// newAuditReportCtx builds an authenticated GET echo.Context with optional query string.
func newAuditReportCtx(e *echo.Echo, path string, userID int64) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, userID, "alice", "admin")
	return c, rec
}

func TestAuditReport_GetUsageStats_Success(t *testing.T) {
	e, _, h, uid := setupAuditReportTest(t)

	c, rec := newAuditReportCtx(e, "/api/reports/usage?days=30", uid)
	if err := h.GetUsageStats(c); err != nil {
		t.Fatalf("GetUsageStats: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v; body=%s", err, rec.Body.String())
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data field: %v", resp)
	}
	// We seeded 4 audit logs in the last 30 days.
	if data["total_actions"].(float64) != 4 {
		t.Errorf("total_actions = %v, want 4", data["total_actions"])
	}
	if data["unique_users"].(float64) != 1 {
		t.Errorf("unique_users = %v, want 1", data["unique_users"])
	}
}

func TestAuditReport_GetUsageStats_EmptyDB(t *testing.T) {
	d := testutil.NewDB(t)
	h := NewReportHandler(NewReportService(d))
	e := echo.New()

	c, rec := newAuditReportCtx(e, "/api/reports/usage", 1)
	if err := h.GetUsageStats(c); err != nil {
		t.Fatalf("GetUsageStats: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["total_actions"].(float64) != 0 {
		t.Errorf("total_actions = %v, want 0 on empty DB", data["total_actions"])
	}
}

func TestAuditReport_GetErrorStats_Success(t *testing.T) {
	e, _, h, uid := setupAuditReportTest(t)

	c, rec := newAuditReportCtx(e, "/api/reports/errors?days=30", uid)
	if err := h.GetErrorStats(c); err != nil {
		t.Fatalf("GetErrorStats: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	// 1 of the 4 seeded logs has an error_message.
	if data["total_errors"].(float64) != 1 {
		t.Errorf("total_errors = %v, want 1", data["total_errors"])
	}
	// error rate = 1/4*100 = 25
	if got := data["error_rate"].(float64); got < 24.9 || got > 25.1 {
		t.Errorf("error_rate = %v, want ~25", got)
	}
}

func TestAuditReport_GetPerformanceReport_Success(t *testing.T) {
	e, _, h, uid := setupAuditReportTest(t)

	c, rec := newAuditReportCtx(e, "/api/reports/performance?days=30", uid)
	if err := h.GetPerformanceReport(c); err != nil {
		t.Fatalf("GetPerformanceReport: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	// Max execution time across seeded logs is 700 (500/600/700/100).
	if data["max_execution_ms"].(float64) != 700 {
		t.Errorf("max_execution_ms = %v, want 700", data["max_execution_ms"])
	}
}

func TestAuditReport_GetTicketReport_Success(t *testing.T) {
	e, _, h, uid := setupAuditReportTest(t)

	c, rec := newAuditReportCtx(e, "/api/reports/tickets?days=30", uid)
	if err := h.GetTicketReport(c); err != nil {
		t.Fatalf("GetTicketReport: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["total_tickets"].(float64) != 0 {
		t.Errorf("total_tickets = %v, want 0 (no tickets seeded)", data["total_tickets"])
	}
}

func TestAuditReport_GetUserAnalytics_DefaultsTo7d(t *testing.T) {
	e, _, h, uid := setupAuditReportTest(t)

	// No time_range → handler defaults to "7d"; no user_id → 0 (valid).
	c, rec := newAuditReportCtx(e, "/api/audit/user-analytics", uid)
	if err := h.GetUserAnalytics(c); err != nil {
		t.Fatalf("GetUserAnalytics: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data field: %v", resp)
	}
	// Empty time_range defaults to "7d".
	if data["time_range"] != "7d" {
		t.Errorf("time_range = %v, want 7d", data["time_range"])
	}
	// Seeded logs roll up into a single active user with 4 total actions.
	topUsers, _ := data["top_active_users"].([]interface{})
	if len(topUsers) == 0 {
		t.Fatalf("expected at least 1 top active user, got %v", topUsers)
	}
	first := topUsers[0].(map[string]interface{})
	if first["total_actions"].(float64) != 4 {
		t.Errorf("top user total_actions = %v, want 4", first["total_actions"])
	}
	if first["username"] != "alice" {
		t.Errorf("top user username = %v, want alice", first["username"])
	}
}

func TestAuditReport_GetUserAnalytics_InvalidUserID(t *testing.T) {
	e, _, h, uid := setupAuditReportTest(t)

	// ParseAnalyticsUserID rejects non-positive / non-numeric values.
	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"zero", "/api/audit/user-analytics?user_id=0", http.StatusBadRequest},
		{"negative", "/api/audit/user-analytics?user_id=-3", http.StatusBadRequest},
		{"non-numeric", "/api/audit/user-analytics?user_id=abc", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newAuditReportCtx(e, tc.query, uid)
			if err := h.GetUserAnalytics(c); err != nil {
				t.Fatalf("GetUserAnalytics returned error: %v", err)
			}
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestAuditReport_GetUserAnalytics_CustomRangeMissingDates(t *testing.T) {
	e, _, h, uid := setupAuditReportTest(t)

	// custom range without start/end fails validation → BadRequest.
	c, rec := newAuditReportCtx(e, "/api/audit/user-analytics?time_range=custom", uid)
	if err := h.GetUserAnalytics(c); err != nil {
		t.Fatalf("GetUserAnalytics returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
