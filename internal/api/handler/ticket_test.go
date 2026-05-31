package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/service"
)

// ─── Test Setup ──────────────────────────────────────────────────────────────

// setupTicketHandlerTest creates a fresh DB, services, and TicketHandler for testing.
func setupTicketHandlerTest(t *testing.T) (*echo.Echo, *TicketHandler, *db.DB) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditSvc := service.NewAuditService(database, 10, 5*time.Second)
	t.Cleanup(func() { auditSvc.Close() })

	ticketSvc := service.NewTicketService(database.DB, auditSvc, nil)
	handler := NewTicketHandler(ticketSvc)

	e := echo.New()
	return e, handler, database
}

// setTicketAuthContext sets user identity on the echo context (simulates JWT middleware).
func setTicketAuthContext(c echo.Context, userID int64, username, role string) {
	c.Set(middleware.ContextKeyUserID, userID)
	c.Set(middleware.ContextKeyUsername, username)
	c.Set(middleware.ContextKeyRole, role)
}

// seedTicketTestUser inserts a user directly into DB and returns the user ID.
func seedTicketTestUser(t *testing.T, database *db.DB, username, role string) int64 {
	t.Helper()
	ctx := contextWithTimeout(t)
	res, err := database.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES (?, 'testhash', ?)`,
		username, role,
	)
	if err != nil {
		t.Fatalf("seed user %q: %v", username, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedTicketTestDatasource inserts a datasource directly into DB and returns the ID.
func seedTicketTestDatasource(t *testing.T, database *db.DB, name string) int64 {
	t.Helper()
	ctx := contextWithTimeout(t)
	res, err := database.ExecContext(ctx,
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, status) VALUES (?, 'mysql', 'localhost', 3306, 'root', '', 'active')`,
		name,
	)
	if err != nil {
		t.Fatalf("seed datasource %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// setTicketStatusDB directly sets a ticket's status in the DB.
func setTicketStatusDB(t *testing.T, database *db.DB, ticketID int64, status model.TicketStatus) {
	t.Helper()
	ctx := contextWithTimeout(t)
	_, err := database.ExecContext(ctx,
		`UPDATE tickets SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		status, ticketID,
	)
	if err != nil {
		t.Fatalf("setTicketStatus(%d, %s) error: %v", ticketID, status, err)
	}
}

// createTicketViaDB inserts a ticket directly into DB and returns its ID.
func createTicketViaDB(t *testing.T, database *db.DB, submitterID, dsID int64, sqlContent string) int64 {
	t.Helper()
	ctx := contextWithTimeout(t)
	res, err := database.ExecContext(ctx,
		`INSERT INTO tickets (submitter_id, datasource_id, database, sql_content, sql_summary, db_type, change_reason, status, risk_level, ai_review_result)
		 VALUES (?, ?, 'mydb', ?, 'summary', 'mysql', 'test', 'SUBMITTED', 'low', '')`,
		submitterID, dsID, sqlContent,
	)
	if err != nil {
		t.Fatalf("createTicketViaDB: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// ─── CreateTicket Handler Tests ──────────────────────────────────────────────

func TestTicketHandler_CreateTicket_Success(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	userID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"mydb","sql":"ALTER TABLE users ADD COLUMN phone VARCHAR(20)","db_type":"mysql","change_reason":"add phone","risk_level":"medium","ai_review_result":"{}"}`, dsID)
	req := httptest.NewRequest(http.MethodPost, "/api/tickets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setTicketAuthContext(c, userID, "dev1", "developer")

	if err := h.CreateTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object; body=%s", rec.Body.String())
	}
	if data["status"] != "SUBMITTED" {
		t.Errorf("status = %v, want SUBMITTED", data["status"])
	}
	if data["submitter_id"] != float64(userID) {
		t.Errorf("submitter_id = %v, want %d", data["submitter_id"], userID)
	}
}

func TestTicketHandler_CreateTicket_Validation(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	userID := seedTicketTestUser(t, database, "dev1", "developer")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			"missing_datasource_id",
			`{"sql":"SELECT 1","database":"mydb"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"zero_datasource_id",
			`{"datasource_id":0,"sql":"SELECT 1","database":"mydb"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"missing_sql",
			`{"datasource_id":1,"database":"mydb"}`,
			http.StatusBadRequest,
			"SQL内容不能为空",
		},
		{
			"empty_sql",
			`{"datasource_id":1,"sql":"","database":"mydb"}`,
			http.StatusBadRequest,
			"SQL内容不能为空",
		},
		{
			"invalid_json",
			`{bad json}`,
			http.StatusBadRequest,
			"请求格式错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/tickets", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setTicketAuthContext(c, userID, "dev1", "developer")

			if err := h.CreateTicket(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			result := decodeJSONResponse(t, rec)
			msg, _ := result["message"].(string)
			if msg != tt.wantMsg {
				t.Errorf("message = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

// ─── GetTicket Handler Tests ─────────────────────────────────────────────────

func TestTicketHandler_GetTicket_Success(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	userID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, userID, dsID, "ALTER TABLE t ADD c INT")

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, userID, "dev1", "developer")

	if err := h.GetTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object; body=%s", rec.Body.String())
	}
	if data["id"] != float64(ticketID) {
		t.Errorf("id = %v, want %d", data["id"], ticketID)
	}
	if data["submitter_name"] != "dev1" {
		t.Errorf("submitter_name = %v, want dev1", data["submitter_name"])
	}
}

func TestTicketHandler_GetTicket_NotFound(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	userID := seedTicketTestUser(t, database, "dev1", "developer")

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setTicketAuthContext(c, userID, "dev1", "developer")

	if err := h.GetTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "工单不存在" {
		t.Errorf("message = %q, want %q", msg, "工单不存在")
	}
}

func TestTicketHandler_GetTicket_InvalidID(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("notanumber")

	if err := h.GetTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "无效的工单ID" {
		t.Errorf("message = %q, want %q", msg, "无效的工单ID")
	}
}

// ─── ListTickets Handler Tests ───────────────────────────────────────────────

func TestTicketHandler_ListTickets_Success(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	userID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	// Create 3 tickets
	for i := 0; i < 3; i++ {
		createTicketViaDB(t, database, userID, dsID, fmt.Sprintf("ALTER TABLE t%d ADD c INT", i))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tickets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setTicketAuthContext(c, userID, "dev1", "developer")

	if err := h.ListTickets(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array; body=%s", rec.Body.String())
	}
	if len(data) != 3 {
		t.Errorf("len(data) = %d, want 3", len(data))
	}
	if total, _ := result["total"].(float64); total != 3 {
		t.Errorf("total = %v, want 3", total)
	}
}

func TestTicketHandler_ListTickets_Empty(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/tickets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setTicketAuthContext(c, 1, "dev1", "developer")

	if err := h.ListTickets(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array; body=%s", rec.Body.String())
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
	if total, _ := result["total"].(float64); total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}

func TestTicketHandler_ListTickets_Pagination(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	userID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	// Create 5 tickets
	for i := 0; i < 5; i++ {
		createTicketViaDB(t, database, userID, dsID, fmt.Sprintf("SELECT %d", i))
	}

	tests := []struct {
		name     string
		query    string
		wantLen  int
		wantPage int64
	}{
		{"page1_size2", "page=1&page_size=2", 2, 1},
		{"page2_size2", "page=2&page_size=2", 2, 2},
		{"page3_size2", "page=3&page_size=2", 1, 3},
		{"default", "", 5, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/tickets"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setTicketAuthContext(c, userID, "dev1", "developer")

			if err := h.ListTickets(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			result := decodeJSONResponse(t, rec)
			data, ok := result["data"].([]interface{})
			if !ok {
				t.Fatalf("data is not an array; body=%s", rec.Body.String())
			}
			if len(data) != tt.wantLen {
				t.Errorf("len(data) = %d, want %d", len(data), tt.wantLen)
			}
			if page, _ := result["page"].(float64); int64(page) != tt.wantPage {
				t.Errorf("page = %v, want %d", page, tt.wantPage)
			}
		})
	}
}

func TestTicketHandler_ListTickets_FilterByStatus(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	userID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	// Create 3 tickets, set one to APPROVED
	ticketID := createTicketViaDB(t, database, userID, dsID, "ALTER TABLE t1 ADD c INT")
	createTicketViaDB(t, database, userID, dsID, "ALTER TABLE t2 ADD c INT")
	createTicketViaDB(t, database, userID, dsID, "ALTER TABLE t3 ADD c INT")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusApproved)

	req := httptest.NewRequest(http.MethodGet, "/api/tickets?status=SUBMITTED", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setTicketAuthContext(c, userID, "dev1", "developer")

	if err := h.ListTickets(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	result := decodeJSONResponse(t, rec)
	data, _ := result["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("len(data) = %d, want 2 (SUBMITTED tickets)", len(data))
	}
}

// ─── ApproveTicket Handler Tests ─────────────────────────────────────────────

func TestTicketHandler_ApproveTicket_Success(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dbaID := seedTicketTestUser(t, database, "dba1", "dba")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusPendingApproval)

	body := `{"comment":"looks good"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/approve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, dbaID, "dba1", "dba")

	if err := h.ApproveTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object; body=%s", rec.Body.String())
	}
	if data["status"] != "APPROVED" {
		t.Errorf("status = %v, want APPROVED", data["status"])
	}
	if data["reviewer_name"] != "dba1" {
		t.Errorf("reviewer_name = %v, want dba1", data["reviewer_name"])
	}
}

func TestTicketHandler_ApproveTicket_InvalidID(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	body := `{"comment":"ok"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/approve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	setTicketAuthContext(c, 1, "dba1", "dba")

	if err := h.ApproveTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTicketHandler_ApproveTicket_NotFound(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	body := `{"comment":"ok"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/approve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setTicketAuthContext(c, 1, "dba1", "dba")

	if err := h.ApproveTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestTicketHandler_ApproveTicket_NoPermission(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusPendingApproval)

	body := `{"comment":"ok"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/approve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.ApproveTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "没有操作权限" {
		t.Errorf("message = %q, want %q", msg, "没有操作权限")
	}
}

func TestTicketHandler_ApproveTicket_InvalidStatus(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dbaID := seedTicketTestUser(t, database, "dba1", "dba")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	// Ticket is still in SUBMITTED status, not PENDING_APPROVAL
	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")

	body := `{"comment":"ok"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/approve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, dbaID, "dba1", "dba")

	if err := h.ApproveTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "无效的工单状态变更" {
		t.Errorf("message = %q, want %q", msg, "无效的工单状态变更")
	}
}

func TestTicketHandler_ApproveTicket_InvalidBody(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/approve", strings.NewReader(`{bad}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setTicketAuthContext(c, 1, "dba1", "dba")

	if err := h.ApproveTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "请求格式错误" {
		t.Errorf("message = %q, want %q", msg, "请求格式错误")
	}
}

// ─── RejectTicket Handler Tests ──────────────────────────────────────────────

func TestTicketHandler_RejectTicket_Success(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dbaID := seedTicketTestUser(t, database, "dba1", "dba")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "DELETE FROM users")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusPendingApproval)

	body := `{"reason":"too dangerous"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, dbaID, "dba1", "dba")

	if err := h.RejectTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object; body=%s", rec.Body.String())
	}
	if data["status"] != "REJECTED" {
		t.Errorf("status = %v, want REJECTED", data["status"])
	}
	if data["review_comment"] != "too dangerous" {
		t.Errorf("review_comment = %v, want 'too dangerous'", data["review_comment"])
	}
}

func TestTicketHandler_RejectTicket_MissingReason(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	body := `{"reason":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setTicketAuthContext(c, 1, "dba1", "dba")

	if err := h.RejectTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "驳回原因不能为空" {
		t.Errorf("message = %q, want %q", msg, "驳回原因不能为空")
	}
}

func TestTicketHandler_RejectTicket_NoPermission(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "DELETE FROM users")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusPendingApproval)

	body := `{"reason":"no good"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.RejectTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestTicketHandler_RejectTicket_InvalidID(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	body := `{"reason":"bad"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("xyz")
	setTicketAuthContext(c, 1, "dba1", "dba")

	if err := h.RejectTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTicketHandler_RejectTicket_InvalidBody(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/reject", strings.NewReader(`{bad}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setTicketAuthContext(c, 1, "dba1", "dba")

	if err := h.RejectTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ─── CancelTicket Handler Tests ──────────────────────────────────────────────

func TestTicketHandler_CancelTicket_Success(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")

	body := `{"reason":"changed my mind"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.CancelTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object; body=%s", rec.Body.String())
	}
	if data["status"] != "CANCELLED" {
		t.Errorf("status = %v, want CANCELLED", data["status"])
	}
}

func TestTicketHandler_CancelTicket_MissingReason(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	body := `{"reason":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setTicketAuthContext(c, 1, "dev1", "developer")

	if err := h.CancelTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "取消原因不能为空" {
		t.Errorf("message = %q, want %q", msg, "取消原因不能为空")
	}
}

func TestTicketHandler_CancelTicket_NoPermission(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	otherID := seedTicketTestUser(t, database, "dev2", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")

	body := `{"reason":"cancel it"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, otherID, "dev2", "developer")

	if err := h.CancelTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestTicketHandler_CancelTicket_NotCancellable(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")
	// Set to DONE which is terminal and not cancellable
	setTicketStatusDB(t, database, ticketID, model.TicketStatusDone)

	body := `{"reason":"cancel"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.CancelTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "当前状态不可取消" {
		t.Errorf("message = %q, want %q", msg, "当前状态不可取消")
	}
}

func TestTicketHandler_CancelTicket_InvalidID(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	body := `{"reason":"cancel"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("notanumber")
	setTicketAuthContext(c, 1, "dev1", "developer")

	if err := h.CancelTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTicketHandler_CancelTicket_DBACanCancel(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dbaID := seedTicketTestUser(t, database, "dba1", "dba")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")

	body := `{"reason":"not needed anymore"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, dbaID, "dba1", "dba")

	if err := h.CancelTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// ─── ExecuteTicket Handler Tests ─────────────────────────────────────────────

func TestTicketHandler_ExecuteTicket_Success(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusPendingApproval)
	setTicketStatusDB(t, database, ticketID, model.TicketStatusApproved)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/execute", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.ExecuteTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Execute fails without datasource service configured
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestTicketHandler_ExecuteTicket_DBACanExecute(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dbaID := seedTicketTestUser(t, database, "dba1", "dba")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusApproved)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/execute", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, dbaID, "dba1", "dba")

	if err := h.ExecuteTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Execute fails without datasource service configured
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestTicketHandler_ExecuteTicket_NotApproved(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")
	// Still in SUBMITTED status

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/execute", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.ExecuteTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "工单未审批通过，无法执行" {
		t.Errorf("message = %q, want %q", msg, "工单未审批通过，无法执行")
	}
}

func TestTicketHandler_ExecuteTicket_NoPermission(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	otherID := seedTicketTestUser(t, database, "dev2", "developer")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "ALTER TABLE t ADD c INT")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusApproved)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/execute", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, otherID, "dev2", "developer")

	if err := h.ExecuteTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestTicketHandler_ExecuteTicket_InvalidID(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/execute", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("notanumber")
	setTicketAuthContext(c, 1, "dev1", "developer")

	if err := h.ExecuteTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTicketHandler_ExecuteTicket_NotFound(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/execute", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setTicketAuthContext(c, 1, "dev1", "developer")

	if err := h.ExecuteTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// ─── CreateTicket Service Error Branches ─────────────────────────────────────

func TestTicketHandler_CreateTicket_NonExistentDatasource(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	userID := seedTicketTestUser(t, database, "dev1", "developer")

	body := `{"datasource_id":99999,"database":"mydb","sql":"ALTER TABLE t ADD c INT","db_type":"mysql","change_reason":"test","risk_level":"low"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setTicketAuthContext(c, userID, "dev1", "developer")

	if err := h.CreateTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Service accepts any datasource_id and creates the ticket
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object; body=%s", rec.Body.String())
	}
	if data["datasource_id"] != float64(99999) {
		t.Errorf("datasource_id = %v, want 99999", data["datasource_id"])
	}
}

// ─── RejectTicket Additional Tests ──────────────────────────────────────────

func TestTicketHandler_RejectTicket_NotFound(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	body := `{"reason":"bad ticket"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setTicketAuthContext(c, 1, "dba1", "dba")

	if err := h.RejectTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "工单不存在" {
		t.Errorf("message = %q, want %q", msg, "工单不存在")
	}
}

func TestTicketHandler_RejectTicket_InvalidStatus(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dbaID := seedTicketTestUser(t, database, "dba1", "dba")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	// Create ticket and set to DONE (terminal status)
	ticketID := createTicketViaDB(t, database, devID, dsID, "DELETE FROM users")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusDone)

	body := `{"reason":"too late"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, dbaID, "dba1", "dba")

	if err := h.RejectTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "无效的工单状态变更" {
		t.Errorf("message = %q, want %q", msg, "无效的工单状态变更")
	}
}

// ─── CancelTicket Additional Tests ──────────────────────────────────────────

func TestTicketHandler_CancelTicket_NotFound(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	body := `{"reason":"cancel it"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setTicketAuthContext(c, 1, "dev1", "developer")

	if err := h.CancelTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "工单不存在" {
		t.Errorf("message = %q, want %q", msg, "工单不存在")
	}
}

func TestTicketHandler_CancelTicket_InvalidBody(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel", strings.NewReader(`{bad}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setTicketAuthContext(c, 1, "dev1", "developer")

	if err := h.CancelTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "请求格式错误" {
		t.Errorf("message = %q, want %q", msg, "请求格式错误")
	}
}

// ─── Full Workflow Handler Test ──────────────────────────────────────────────

func TestTicketHandler_FullWorkflow(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dbaID := seedTicketTestUser(t, database, "dba1", "dba")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	// Step 1: Create ticket
	createBody := fmt.Sprintf(`{"datasource_id":%d,"database":"mydb","sql":"ALTER TABLE users ADD COLUMN phone VARCHAR(20)","db_type":"mysql","change_reason":"add phone","risk_level":"medium"}`, dsID)
	req := httptest.NewRequest(http.MethodPost, "/api/tickets", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.CreateTicket(c); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	createResult := decodeJSONResponse(t, rec)
	createData := createResult["data"].(map[string]interface{})
	ticketID := int64(createData["id"].(float64))

	// Step 2: Set to PENDING_APPROVAL (simulate AI review)
	setTicketStatusDB(t, database, ticketID, model.TicketStatusPendingApproval)

	// Step 3: DBA approves
	approveBody := `{"comment":"looks good"}`
	req = httptest.NewRequest(http.MethodPost, "/api/tickets/:id/approve", strings.NewReader(approveBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, dbaID, "dba1", "dba")

	if err := h.ApproveTicket(c); err != nil {
		t.Fatalf("approve ticket: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("approve status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Step 4: Developer executes
	req = httptest.NewRequest(http.MethodPost, "/api/tickets/:id/execute", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.ExecuteTicket(c); err != nil {
		t.Fatalf("execute ticket: %v", err)
	}
	// Execute fails without datasource service
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("execute status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

// ─── Reject Workflow Handler Test ────────────────────────────────────────────

func TestTicketHandler_RejectWorkflow(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	dbaID := seedTicketTestUser(t, database, "dba1", "dba")
	dsID := seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, dsID, "DELETE FROM users")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusPendingApproval)

	body := `{"reason":"too dangerous without WHERE"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, dbaID, "dba1", "dba")

	if err := h.RejectTicket(c); err != nil {
		t.Fatalf("reject ticket: %v", err)
	}

	result := decodeJSONResponse(t, rec)
	data := result["data"].(map[string]interface{})
	if data["status"] != "REJECTED" {
		t.Errorf("status = %v, want REJECTED", data["status"])
	}
}

func TestTicketHandler_ScheduleTicket(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, 1, "SELECT 1")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusApproved)

	// Schedule for a future time
	futureTime := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	body := `{"scheduled_at":"` + futureTime + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/schedule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.ScheduleTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestTicketHandler_ScheduleTicket_InvalidID(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	futureTime := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	body := `{"scheduled_at":"` + futureTime + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tickets/abc/schedule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	setTicketAuthContext(c, 1, "dev1", "developer")

	if err := h.ScheduleTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestTicketHandler_ScheduleTicket_EmptyBody(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, 1, "SELECT 1")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusApproved)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/schedule", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.ScheduleTicket(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestTicketHandler_CancelSchedule(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, 1, "SELECT 1")
	setTicketStatusDB(t, database, ticketID, model.TicketStatusScheduled)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel-schedule", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.CancelSchedule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestTicketHandler_CancelSchedule_InvalidID(t *testing.T) {
	e, h, _ := setupTicketHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/abc/cancel-schedule", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	setTicketAuthContext(c, 1, "dev1", "developer")

	if err := h.CancelSchedule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestTicketHandler_CancelSchedule_NotScheduled(t *testing.T) {
	e, h, database := setupTicketHandlerTest(t)
	devID := seedTicketTestUser(t, database, "dev1", "developer")
	seedTicketTestDatasource(t, database, "test-ds")

	ticketID := createTicketViaDB(t, database, devID, 1, "SELECT 1")
	// Keep default SUBMITTED status, not SCHEDULED

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/:id/cancel-schedule", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ticketID))
	setTicketAuthContext(c, devID, "dev1", "developer")

	if err := h.CancelSchedule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
