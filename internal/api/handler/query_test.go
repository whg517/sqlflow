package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/service"
)

// ─── Test Setup ──────────────────────────────────────────────────────────────

// setupQueryTest creates a fresh DB, services, and QueryHandler for testing.
// It also seeds a user and a datasource so ExecuteQuery/ExportQuery can reference them.
func setupQueryTest(t *testing.T) (*echo.Echo, *service.QueryService, *service.QueryHistoryService, *service.DatasourceService, *QueryHandler, *db.DB) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef"
	connMgr := connpool.NewManager()

	dsSvc := service.NewDatasourceService(database.DB, encKey, connMgr)
	historySvc := service.NewQueryHistoryService(database.DB)
	permSvc, _ := service.NewPermissionService(database.DB)
	// PermissionService creation may fail if policy.csv is not found;
	// that's OK for handler-level tests that don't reach the permission check.
	auditSvc := service.NewAuditService(database.DB, 10, 5*time.Second)

	querySvc := service.NewQueryService(database.DB, dsSvc, historySvc, permSvc, auditSvc, encKey, connMgr)
	handler := NewQueryHandler(querySvc, historySvc)

	e := echo.New()
	return e, querySvc, historySvc, dsSvc, handler, database
}

// seedTestUser inserts a user directly into DB and returns the user ID.
func seedTestUser(t *testing.T, database *db.DB, username, role string) int64 {
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

// seedTestDatasource creates a datasource via service and returns it.
func seedTestDatasource(t *testing.T, dsSvc *service.DatasourceService, name string) *model.DataSource {
	t.Helper()
	ctx := contextWithTimeout(t)
	ds := &model.DataSource{
		Name:     name,
		Type:     "mysql",
		Host:     "10.0.0.1",
		Port:     3306,
		Username: "root",
		Database: "testdb",
	}
	if err := dsSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("seed datasource %q: %v", name, err)
	}
	return ds
}

// seedQueryHistory inserts a query history record directly.
func seedQueryHistory(t *testing.T, database *db.DB, userID, dsID int64, sqlContent string) int64 {
	t.Helper()
	ctx := contextWithTimeout(t)
	res, err := database.ExecContext(ctx,
		`INSERT INTO query_history (user_id, datasource_id, database, sql_content, sql_summary, db_type, execution_time, result_rows, affected_rows)
		 VALUES (?, ?, 'testdb', ?, 'summary', 'mysql', 10, 5, 0)`,
		userID, dsID, sqlContent,
	)
	if err != nil {
		t.Fatalf("seed query history: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// setQueryAuthContext sets user identity on the echo context (simulates JWT middleware).
func setQueryAuthContext(c echo.Context, userID int64, username, role string) {
	c.Set(middleware.ContextKeyUserID, userID)
	c.Set(middleware.ContextKeyUsername, username)
	c.Set(middleware.ContextKeyRole, role)
}

// decodeJSONResponse unmarshals the response body into a map.
func decodeJSONResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return result
}

// ─── ExecuteQuery Tests ─────────────────────────────────────────────────────

func TestQueryHandler_ExecuteQuery_Validation(t *testing.T) {
	e, _, _, _, h, _ := setupQueryTest(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			"missing_datasource_id",
			`{"sql":"SELECT 1","database":"testdb"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"zero_datasource_id",
			`{"datasource_id":0,"sql":"SELECT 1","database":"testdb"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"missing_sql",
			`{"datasource_id":1,"database":"testdb"}`,
			http.StatusBadRequest,
			"SQL不能为空",
		},
		{
			"empty_sql",
			`{"datasource_id":1,"sql":"","database":"testdb"}`,
			http.StatusBadRequest,
			"SQL不能为空",
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
			req := httptest.NewRequest(http.MethodPost, "/api/query/execute", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setQueryAuthContext(c, 1, "testuser", "developer")

			if err := h.ExecuteQuery(c); err != nil {
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

func TestQueryHandler_ExecuteQuery_DatasourceNotFound(t *testing.T) {
	e, _, _, _, h, _ := setupQueryTest(t)

	body := `{"datasource_id":99999,"sql":"SELECT 1","database":"testdb"}`
	req := httptest.NewRequest(http.MethodPost, "/api/query/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ExecuteQuery(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Datasource 99999 doesn't exist → service returns error → handler returns 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestQueryHandler_ExecuteQuery_DisabledDatasource(t *testing.T) {
	e, _, _, dsSvc, h, _ := setupQueryTest(t)

	ds := seedTestDatasource(t, dsSvc, "disabled-ds")

	// Disable the datasource
	ctx := contextWithTimeout(t)
	if err := dsSvc.DisableDataSource(ctx, ds.ID); err != nil {
		t.Fatalf("disable datasource: %v", err)
	}

	body := fmt.Sprintf(`{"datasource_id":%d,"sql":"SELECT 1","database":"testdb"}`, ds.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/query/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ExecuteQuery(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Disabled datasource → service.ErrDatasourceDisabled → handler maps to internal error
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestQueryHandler_ExecuteQuery_NonSelectSQL(t *testing.T) {
	e, _, _, dsSvc, h, _ := setupQueryTest(t)

	ds := seedTestDatasource(t, dsSvc, "query-noselect")

	// INSERT is not blocked by static rules but is not SELECT → ErrSQLOperationForbidden → 403
	body := fmt.Sprintf(`{"datasource_id":%d,"sql":"INSERT INTO users (id, name) VALUES (1, 'test')","database":"testdb"}`, ds.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/query/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ExecuteQuery(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if !strings.Contains(msg, "工单") {
		t.Errorf("message = %q, should mention 工单", msg)
	}
}

func TestQueryHandler_ExecuteQuery_BlockedSQL(t *testing.T) {
	e, _, _, dsSvc, h, _ := setupQueryTest(t)

	ds := seedTestDatasource(t, dsSvc, "query-blocked")

	// DROP TABLE is statically blocked → wrapped ErrSQLBlocked → falls to default case → 500
	body := fmt.Sprintf(`{"datasource_id":%d,"sql":"DROP TABLE users","database":"testdb"}`, ds.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/query/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ExecuteQuery(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Wrapped ErrSQLBlocked falls through to default handler → 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestQueryHandler_ExecuteQuery_ConnectionError(t *testing.T) {
	e, _, _, dsSvc, h, _ := setupQueryTest(t)

	// Datasource points to non-existent MySQL server → connection error → 500
	ds := seedTestDatasource(t, dsSvc, "query-connerr")

	body := fmt.Sprintf(`{"datasource_id":%d,"sql":"SELECT 1","database":"testdb"}`, ds.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/query/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ExecuteQuery(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Connection failure → internal error
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

// ─── ListHistory Tests ───────────────────────────────────────────────────────

func TestQueryHandler_ListHistory_Empty(t *testing.T) {
	e, _, _, _, h, _ := setupQueryTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/query/history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ListHistory(c); err != nil {
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
	if len(data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(data))
	}

	// Verify pagination metadata
	if page, _ := result["page"].(float64); page != 1 {
		t.Errorf("page = %v, want 1", page)
	}
	if total, _ := result["total"].(float64); total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}

func TestQueryHandler_ListHistory_WithRecords(t *testing.T) {
	e, _, historySvc, dsSvc, h, database := setupQueryTest(t)

	userID := seedTestUser(t, database, "historyuser", "developer")
	ds := seedTestDatasource(t, dsSvc, "history-ds")

	// Insert 3 history records
	for i := 0; i < 3; i++ {
		ctx := contextWithTimeout(t)
		history := &model.QueryHistory{
			UserID:       userID,
			DatasourceID: ds.ID,
			Database:     "testdb",
			SQLContent:   fmt.Sprintf("SELECT %d", i),
			SQLSummary:   fmt.Sprintf("SELECT %d", i),
			DBType:       "mysql",
			ExecutionTime: 10,
			ResultRows:   1,
		}
		if err := historySvc.CreateHistory(ctx, history); err != nil {
			t.Fatalf("create history %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/query/history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, userID, "historyuser", "developer")

	if err := h.ListHistory(c); err != nil {
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
	if len(data) != 3 {
		t.Errorf("len(data) = %d, want 3", len(data))
	}
	if total, _ := result["total"].(float64); total != 3 {
		t.Errorf("total = %v, want 3", total)
	}
}

func TestQueryHandler_ListHistory_Pagination(t *testing.T) {
	e, _, historySvc, dsSvc, h, database := setupQueryTest(t)

	userID := seedTestUser(t, database, "pageuser", "developer")
	ds := seedTestDatasource(t, dsSvc, "page-ds")

	// Insert 5 history records
	for i := 0; i < 5; i++ {
		ctx := contextWithTimeout(t)
		history := &model.QueryHistory{
			UserID:       userID,
			DatasourceID: ds.ID,
			Database:     "testdb",
			SQLContent:   fmt.Sprintf("SELECT * FROM t WHERE id=%d", i),
			SQLSummary:   fmt.Sprintf("SELECT * FROM t WHERE id=%d", i),
			DBType:       "mysql",
			ExecutionTime: int64(i * 10),
			ResultRows:   1,
		}
		if err := historySvc.CreateHistory(ctx, history); err != nil {
			t.Fatalf("create history %d: %v", i, err)
		}
	}

	tests := []struct {
		name      string
		query     string
		wantLen   int
		wantPage  int64
		wantSize  int64
	}{
		{"page1_size2", "page=1&page_size=2", 2, 1, 2},
		{"page2_size2", "page=2&page_size=2", 2, 2, 2},
		{"page3_size2", "page=3&page_size=2", 1, 3, 2},
		{"default_params", "", 5, 1, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/query/history"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setQueryAuthContext(c, userID, "pageuser", "developer")

			if err := h.ListHistory(c); err != nil {
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
			if pageSize, _ := result["page_size"].(float64); int64(pageSize) != tt.wantSize {
				t.Errorf("page_size = %v, want %d", pageSize, tt.wantSize)
			}
		})
	}
}

func TestQueryHandler_ListHistory_UserIsolation(t *testing.T) {
	e, _, historySvc, dsSvc, h, database := setupQueryTest(t)

	userA := seedTestUser(t, database, "userA", "developer")
	userB := seedTestUser(t, database, "userB", "developer")
	ds := seedTestDatasource(t, dsSvc, "iso-ds")

	// User A creates 2 records
	for i := 0; i < 2; i++ {
		ctx := contextWithTimeout(t)
		history := &model.QueryHistory{
			UserID: userA, DatasourceID: ds.ID, Database: "db",
			SQLContent: fmt.Sprintf("SELECT %d", i), SQLSummary: "s", DBType: "mysql",
		}
		if err := historySvc.CreateHistory(ctx, history); err != nil {
			t.Fatalf("create history: %v", err)
		}
	}

	// User B creates 1 record
	{
		ctx := contextWithTimeout(t)
		history := &model.QueryHistory{
			UserID: userB, DatasourceID: ds.ID, Database: "db",
			SQLContent: "SELECT 999", SQLSummary: "s", DBType: "mysql",
		}
		if err := historySvc.CreateHistory(ctx, history); err != nil {
			t.Fatalf("create history: %v", err)
		}
	}

	// User A should see only 2 records
	req := httptest.NewRequest(http.MethodGet, "/api/query/history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, userA, "userA", "developer")

	if err := h.ListHistory(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	result := decodeJSONResponse(t, rec)
	data, _ := result["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("user A should see 2 records, got %d", len(data))
	}
}

// ─── DeleteHistory Tests ─────────────────────────────────────────────────────

func TestQueryHandler_DeleteHistory_Success(t *testing.T) {
	// Separate DB for this test to avoid interference
	database, err := db.Open(t.TempDir() + "/test2.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID := seedTestUser(t, database, "deluser", "developer")
	ds := &model.DataSource{
		Name: "del-ds", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", Database: "testdb",
	}
	encKey := "0123456789abcdef0123456789abcdef"
	connMgr := connpool.NewManager()
	dsSvc2 := service.NewDatasourceService(database.DB, encKey, connMgr)
	ctx := contextWithTimeout(t)
	if err := dsSvc2.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	historyID := seedQueryHistory(t, database, userID, ds.ID, "SELECT 1")

	historySvc2 := service.NewQueryHistoryService(database.DB)
	querySvc2 := service.NewQueryService(database.DB, dsSvc2, historySvc2, nil, nil, encKey, connMgr)
	h2 := NewQueryHandler(querySvc2, historySvc2)
	e := echo.New()

	req := httptest.NewRequest(http.MethodDelete, "/api/query/history/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", historyID))
	setQueryAuthContext(c, userID, "deluser", "developer")

	if err := h2.DeleteHistory(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "删除成功" {
		t.Errorf("message = %q, want %q", msg, "删除成功")
	}
}

func TestQueryHandler_DeleteHistory_InvalidID(t *testing.T) {
	e, _, _, _, h, _ := setupQueryTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/query/history/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("notanumber")
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.DeleteHistory(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestQueryHandler_DeleteHistory_NotFound(t *testing.T) {
	e, _, _, _, h, _ := setupQueryTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/query/history/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.DeleteHistory(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestQueryHandler_DeleteHistory_WrongUser(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/test3.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef"
	connMgr := connpool.NewManager()
	dsSvc := service.NewDatasourceService(database.DB, encKey, connMgr)

	userOwner := seedTestUser(t, database, "owner", "developer")
	userOther := seedTestUser(t, database, "other", "developer")

	ds := &model.DataSource{
		Name: "wronguser-ds", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", Database: "testdb",
	}
	ctx := contextWithTimeout(t)
	if err := dsSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	historyID := seedQueryHistory(t, database, userOwner, ds.ID, "SELECT 1")

	historySvc := service.NewQueryHistoryService(database.DB)
	querySvc := service.NewQueryService(database.DB, dsSvc, historySvc, nil, nil, encKey, connMgr)
	h := NewQueryHandler(querySvc, historySvc)
	e := echo.New()

	// Try to delete owner's record as "other" user
	req := httptest.NewRequest(http.MethodDelete, "/api/query/history/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", historyID))
	setQueryAuthContext(c, userOther, "other", "developer")

	if err := h.DeleteHistory(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Should fail because the record belongs to userOwner, not userOther
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// ─── ClearHistory Tests ──────────────────────────────────────────────────────

func TestQueryHandler_ClearHistory_Success(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/test4.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef"
	connMgr := connpool.NewManager()
	dsSvc := service.NewDatasourceService(database.DB, encKey, connMgr)
	historySvc := service.NewQueryHistoryService(database.DB)
	querySvc := service.NewQueryService(database.DB, dsSvc, historySvc, nil, nil, encKey, connMgr)
	h := NewQueryHandler(querySvc, historySvc)
	e := echo.New()

	userID := seedTestUser(t, database, "clearuser", "developer")
	ds := &model.DataSource{
		Name: "clear-ds", Type: "mysql", Host: "10.0.0.1", Port: 3306,
		Username: "root", Database: "testdb",
	}
	ctx := contextWithTimeout(t)
	if err := dsSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	// Seed 3 records
	for i := 0; i < 3; i++ {
		seedQueryHistory(t, database, userID, ds.ID, fmt.Sprintf("SELECT %d", i))
	}

	// Clear
	req := httptest.NewRequest(http.MethodDelete, "/api/query/history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, userID, "clearuser", "developer")

	if err := h.ClearHistory(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "已清空所有查询历史" {
		t.Errorf("message = %q, want %q", msg, "已清空所有查询历史")
	}

	// Verify records are gone via ListHistory
	req2 := httptest.NewRequest(http.MethodGet, "/api/query/history", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	setQueryAuthContext(c2, userID, "clearuser", "developer")

	if err := h.ListHistory(c2); err != nil {
		t.Fatalf("list after clear: %v", err)
	}

	result2 := decodeJSONResponse(t, rec2)
	data, _ := result2["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("after clear, expected 0 records, got %d", len(data))
	}
}

func TestQueryHandler_ClearHistory_Empty(t *testing.T) {
	e, _, _, _, h, _ := setupQueryTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/query/history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ClearHistory(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Clearing empty history should still succeed
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// ─── ExportQuery Tests ───────────────────────────────────────────────────────

func TestQueryHandler_ExportQuery_Validation(t *testing.T) {
	e, _, _, _, h, _ := setupQueryTest(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			"missing_datasource_id",
			`{"sql":"SELECT 1","format":"csv"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"zero_datasource_id",
			`{"datasource_id":0,"sql":"SELECT 1","format":"csv"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"missing_sql",
			`{"datasource_id":1,"format":"csv"}`,
			http.StatusBadRequest,
			"SQL不能为空",
		},
		{
			"empty_sql",
			`{"datasource_id":1,"sql":"","format":"csv"}`,
			http.StatusBadRequest,
			"SQL不能为空",
		},
		{
			"invalid_format",
			`{"datasource_id":1,"sql":"SELECT 1","format":"xml"}`,
			http.StatusBadRequest,
			"导出格式仅支持 csv 或 json",
		},
		{
			"missing_format",
			`{"datasource_id":1,"sql":"SELECT 1"}`,
			http.StatusBadRequest,
			"导出格式仅支持 csv 或 json",
		},
		{
			"empty_format",
			`{"datasource_id":1,"sql":"SELECT 1","format":""}`,
			http.StatusBadRequest,
			"导出格式仅支持 csv 或 json",
		},
		{
			"invalid_json",
			`{bad}`,
			http.StatusBadRequest,
			"请求格式错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/query/export", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setQueryAuthContext(c, 1, "testuser", "developer")

			if err := h.ExportQuery(c); err != nil {
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

func TestQueryHandler_ExportQuery_NonSelectSQL(t *testing.T) {
	e, _, _, dsSvc, h, _ := setupQueryTest(t)

	ds := seedTestDatasource(t, dsSvc, "export-noselect")

	// INSERT is not blocked by static rules but is not SELECT → ErrSQLOperationForbidden → 403
	body := fmt.Sprintf(`{"datasource_id":%d,"sql":"INSERT INTO t VALUES (1)","database":"testdb","format":"csv"}`, ds.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/query/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ExportQuery(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestQueryHandler_ExportQuery_ConnectionError(t *testing.T) {
	e, _, _, dsSvc, h, _ := setupQueryTest(t)

	ds := seedTestDatasource(t, dsSvc, "export-connerr")

	body := fmt.Sprintf(`{"datasource_id":%d,"sql":"SELECT 1","database":"testdb","format":"csv"}`, ds.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/query/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ExportQuery(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

// ─── CSVEscape Tests (preserved from original) ───────────────────────────────

func TestCSVEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "hello", "hello"},
		{"comma", "hello,world", `"hello,world"`},
		{"quote", `say "hi"`, `"say ""hi"""`},
		{"newline", "line1\nline2", "\"line1\nline2\""},
		{"carriage_return", "line1\rline2", "\"line1\rline2\""},
		{"empty", "", ""},
		{"number", "123", "123"},
		{"chinese", "你好世界", "你好世界"},
		{"mixed", `a,b"c\nd`, `"a,b""c\nd"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := csvEscape(tt.input)
			if got != tt.want {
				t.Errorf("csvEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ─── writeCSV / writeExportJSON unit tests ───────────────────────────────────

func TestWriteCSV(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	result := &service.QueryResult{
		Columns: []string{"id", "name", "email"},
		Rows: []map[string]interface{}{
			{"id": 1, "name": "Alice", "email": "alice@test.com"},
			{"id": 2, "name": "Bob, Jr.", "email": `bob "the builder"@test.com`},
		},
	}

	if err := writeCSV(c, result); err != nil {
		t.Fatalf("writeCSV error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Check Content-Type
	ct := rec.Header().Get("Content-Type")
	if ct != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/csv; charset=utf-8")
	}

	// Check Content-Disposition
	cd := rec.Header().Get("Content-Disposition")
	if cd != "attachment; filename=export.csv" {
		t.Errorf("Content-Disposition = %q, want %q", cd, "attachment; filename=export.csv")
	}

	body := rec.Body.String()
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 data), got %d; body=%s", len(lines), body)
	}

	// Header row
	if lines[0] != "id,name,email" {
		t.Errorf("header = %q, want %q", lines[0], "id,name,email")
	}

	// Data row with comma in value
	if lines[1] != "1,Alice,alice@test.com" {
		t.Errorf("row1 = %q, want %q", lines[1], "1,Alice,alice@test.com")
	}

	// Data row with comma and quote in values
	if lines[2] != `2,"Bob, Jr.","bob ""the builder""@test.com"` {
		t.Errorf("row2 = %q, want escaped version", lines[2])
	}
}

func TestWriteCSV_EmptyResult(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	result := &service.QueryResult{
		Columns: []string{"id", "name"},
		Rows:    []map[string]interface{}{},
	}

	if err := writeCSV(c, result); err != nil {
		t.Fatalf("writeCSV error: %v", err)
	}

	body := rec.Body.String()
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header only), got %d; body=%s", len(lines), body)
	}
	if lines[0] != "id,name" {
		t.Errorf("header = %q, want %q", lines[0], "id,name")
	}
}

func TestWriteCSV_NilValues(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	result := &service.QueryResult{
		Columns: []string{"id", "name"},
		Rows: []map[string]interface{}{
			{"id": 1, "name": nil},
			{"id": 2},
		},
	}

	if err := writeCSV(c, result); err != nil {
		t.Fatalf("writeCSV error: %v", err)
	}

	body := rec.Body.String()
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d; body=%s", len(lines), body)
	}
	if lines[1] != "1," {
		t.Errorf("row with nil = %q, want %q", lines[1], "1,")
	}
	if lines[2] != "2," {
		t.Errorf("row with missing key = %q, want %q", lines[2], "2,")
	}
}

func TestWriteExportJSON(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	result := &service.QueryResult{
		Columns: []string{"id", "name"},
		Rows: []map[string]interface{}{
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
		},
	}

	if err := writeExportJSON(c, result); err != nil {
		t.Fatalf("writeExportJSON error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	cd := rec.Header().Get("Content-Disposition")
	if cd != "attachment; filename=export.json" {
		t.Errorf("Content-Disposition = %q, want %q", cd, "attachment; filename=export.json")
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0]["name"] != "Alice" {
		t.Errorf("rows[0][name] = %v, want Alice", rows[0]["name"])
	}
}

// ─── ClearHistory Error Path ─────────────────────────────────────────────────

func TestQueryHandler_ClearHistory_Error(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/test_clear_err.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef"
	connMgr := connpool.NewManager()
	dsSvc := service.NewDatasourceService(database.DB, encKey, connMgr)
	historySvc := service.NewQueryHistoryService(database.DB)
	querySvc := service.NewQueryService(database.DB, dsSvc, historySvc, nil, nil, encKey, connMgr)
	h := NewQueryHandler(querySvc, historySvc)
	e := echo.New()

	// Close the database to force ClearHistory to fail
	database.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/query/history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ClearHistory(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "清空查询历史失败" {
		t.Errorf("message = %q, want %q", msg, "清空查询历史失败")
	}
}

// ─── ExportQuery Error Branches ──────────────────────────────────────────────

func TestQueryHandler_ExportQuery_BlockedSQL(t *testing.T) {
	e, _, _, dsSvc, h, _ := setupQueryTest(t)

	ds := seedTestDatasource(t, dsSvc, "export-blocked")

	body := fmt.Sprintf(`{"datasource_id":%d,"sql":"DROP TABLE users","database":"testdb","format":"csv"}`, ds.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/query/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ExportQuery(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Blocked SQL wraps error → falls to default handler → 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestQueryHandler_ExportQuery_HighRiskSQL(t *testing.T) {
	e, _, _, dsSvc, h, _ := setupQueryTest(t)

	ds := seedTestDatasource(t, dsSvc, "export-highrisk")

	// UPDATE without WHERE is high-risk
	body := fmt.Sprintf(`{"datasource_id":%d,"sql":"UPDATE users SET name = 'hacked'","database":"testdb","format":"csv"}`, ds.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/query/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setQueryAuthContext(c, 1, "testuser", "developer")

	if err := h.ExportQuery(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// High-risk SQL should return 403 or fall to default 500 depending on error wrapping
	if rec.Code != http.StatusInternalServerError && rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 or 500; body = %s", rec.Code, rec.Body.String())
	}
}

func TestWriteExportJSON_EmptyResult(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	result := &service.QueryResult{
		Columns: []string{"id"},
		Rows:    []map[string]interface{}{},
	}

	if err := writeExportJSON(c, result); err != nil {
		t.Fatalf("writeExportJSON error: %v", err)
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0", len(rows))
	}
}
