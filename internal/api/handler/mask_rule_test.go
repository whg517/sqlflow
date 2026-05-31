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
	"github.com/whg517/sqlflow/internal/service"
)

// ─── Test Setup ──────────────────────────────────────────────────────────────

// setupMaskRuleHandlerTest creates a fresh DB, services, and MaskRuleHandler for testing.
func setupMaskRuleHandlerTest(t *testing.T) (*echo.Echo, *MaskRuleHandler, *db.DB) {
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

	maskRuleSvc := service.NewMaskRuleService(database, nil, auditSvc)
	handler := NewMaskRuleHandler(maskRuleSvc)

	e := echo.New()
	return e, handler, database
}

// setMaskRuleAuthContext sets user identity on the echo context (simulates JWT middleware).
func setMaskRuleAuthContext(c echo.Context, userID int64, username, role string) {
	c.Set(middleware.ContextKeyUserID, userID)
	c.Set(middleware.ContextKeyUsername, username)
	c.Set(middleware.ContextKeyRole, role)
}

// seedMaskRuleDatasource inserts a datasource directly into DB and returns the ID.
func seedMaskRuleDatasource(t *testing.T, database *db.DB, name string) int64 {
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

// seedMaskRule inserts a mask rule directly into DB and returns the ID.
func seedMaskRule(t *testing.T, database *db.DB, dsID int64, db_, table, field, maskType string) int64 {
	t.Helper()
	ctx := contextWithTimeout(t)
	res, err := database.ExecContext(ctx,
		`INSERT INTO mask_rules (datasource_id, database, table_name, field, mask_type, custom_regex, custom_template) VALUES (?, ?, ?, ?, ?, '', '')`,
		dsID, db_, table, field, maskType,
	)
	if err != nil {
		t.Fatalf("seed mask rule: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedSensitiveTable inserts a sensitive table directly into DB and returns the ID.
func seedSensitiveTable(t *testing.T, database *db.DB, dsID int64, db_, table, level string) int64 {
	t.Helper()
	ctx := contextWithTimeout(t)
	res, err := database.ExecContext(ctx,
		`INSERT INTO sensitive_tables (datasource_id, database, table_name, sensitivity_level) VALUES (?, ?, ?, ?)`,
		dsID, db_, table, level,
	)
	if err != nil {
		t.Fatalf("seed sensitive table: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// ─── CreateMaskRule Handler Tests ────────────────────────────────────────────

func TestMaskRuleHandler_CreateMaskRule_Success(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"mydb","table_name":"users","field":"phone","mask_type":"phone"}`, dsID)
	req := httptest.NewRequest(http.MethodPost, "/api/mask-rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.CreateMaskRule(c); err != nil {
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
	if data["table_name"] != "users" {
		t.Errorf("table_name = %v, want users", data["table_name"])
	}
	if data["field"] != "phone" {
		t.Errorf("field = %v, want phone", data["field"])
	}
	if data["mask_type"] != "phone" {
		t.Errorf("mask_type = %v, want phone", data["mask_type"])
	}
}

func TestMaskRuleHandler_CreateMaskRule_CustomType(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"mydb","table_name":"users","field":"email","mask_type":"custom","custom_regex":"(?<=@).*","custom_template":"***"}`, dsID)
	req := httptest.NewRequest(http.MethodPost, "/api/mask-rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.CreateMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestMaskRuleHandler_CreateMaskRule_Validation(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			"missing_datasource_id",
			`{"table_name":"users","field":"phone","mask_type":"phone"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"zero_datasource_id",
			`{"datasource_id":0,"table_name":"users","field":"phone","mask_type":"phone"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"missing_table_name",
			fmt.Sprintf(`{"datasource_id":%d,"field":"phone","mask_type":"phone"}`, dsID),
			http.StatusBadRequest,
			"表名不能为空",
		},
		{
			"empty_table_name",
			fmt.Sprintf(`{"datasource_id":%d,"table_name":"","field":"phone","mask_type":"phone"}`, dsID),
			http.StatusBadRequest,
			"表名不能为空",
		},
		{
			"missing_field",
			fmt.Sprintf(`{"datasource_id":%d,"table_name":"users","mask_type":"phone"}`, dsID),
			http.StatusBadRequest,
			"字段名不能为空",
		},
		{
			"empty_field",
			fmt.Sprintf(`{"datasource_id":%d,"table_name":"users","field":"","mask_type":"phone"}`, dsID),
			http.StatusBadRequest,
			"字段名不能为空",
		},
		{
			"missing_mask_type",
			fmt.Sprintf(`{"datasource_id":%d,"table_name":"users","field":"phone"}`, dsID),
			http.StatusBadRequest,
			"脱敏类型不能为空",
		},
		{
			"invalid_mask_type",
			fmt.Sprintf(`{"datasource_id":%d,"table_name":"users","field":"phone","mask_type":"invalid"}`, dsID),
			http.StatusBadRequest,
			"无效的脱敏类型",
		},
		{
			"custom_without_regex",
			fmt.Sprintf(`{"datasource_id":%d,"table_name":"users","field":"phone","mask_type":"custom"}`, dsID),
			http.StatusBadRequest,
			"自定义脱敏类型必须提供正则表达式",
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
			req := httptest.NewRequest(http.MethodPost, "/api/mask-rules", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setMaskRuleAuthContext(c, 1, "admin", "admin")

			if err := h.CreateMaskRule(c); err != nil {
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

func TestMaskRuleHandler_CreateMaskRule_Duplicate(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	// Create first rule
	body := fmt.Sprintf(`{"datasource_id":%d,"database":"mydb","table_name":"users","field":"phone","mask_type":"phone"}`, dsID)
	req := httptest.NewRequest(http.MethodPost, "/api/mask-rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")
	if err := h.CreateMaskRule(c); err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Try duplicate
	req = httptest.NewRequest(http.MethodPost, "/api/mask-rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.CreateMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "该字段已存在脱敏规则" {
		t.Errorf("message = %q, want %q", msg, "该字段已存在脱敏规则")
	}
}

// ─── GetMaskRule Handler Tests ───────────────────────────────────────────────

func TestMaskRuleHandler_GetMaskRule_Success(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")
	ruleID := seedMaskRule(t, database, dsID, "mydb", "users", "phone", "phone")

	req := httptest.NewRequest(http.MethodGet, "/api/mask-rules/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ruleID))
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.GetMaskRule(c); err != nil {
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
	if data["id"] != float64(ruleID) {
		t.Errorf("id = %v, want %d", data["id"], ruleID)
	}
	if data["table_name"] != "users" {
		t.Errorf("table_name = %v, want users", data["table_name"])
	}
	if data["field"] != "phone" {
		t.Errorf("field = %v, want phone", data["field"])
	}
}

func TestMaskRuleHandler_GetMaskRule_NotFound(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/mask-rules/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.GetMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "脱敏规则不存在" {
		t.Errorf("message = %q, want %q", msg, "脱敏规则不存在")
	}
}

func TestMaskRuleHandler_GetMaskRule_InvalidID(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/mask-rules/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("notanumber")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.GetMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "无效的规则ID" {
		t.Errorf("message = %q, want %q", msg, "无效的规则ID")
	}
}

// ─── ListMaskRules Handler Tests ─────────────────────────────────────────────

func TestMaskRuleHandler_ListMaskRules_Success(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	for i := 0; i < 3; i++ {
		seedMaskRule(t, database, dsID, "mydb", "users", fmt.Sprintf("field%d", i), "phone")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mask-rules", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.ListMaskRules(c); err != nil {
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

func TestMaskRuleHandler_ListMaskRules_Empty(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/mask-rules", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.ListMaskRules(c); err != nil {
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

func TestMaskRuleHandler_ListMaskRules_Pagination(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	for i := 0; i < 5; i++ {
		seedMaskRule(t, database, dsID, "mydb", "users", fmt.Sprintf("field%d", i), "phone")
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
			url := "/api/mask-rules"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setMaskRuleAuthContext(c, 1, "admin", "admin")

			if err := h.ListMaskRules(c); err != nil {
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

func TestMaskRuleHandler_ListMaskRules_FilterByDatasource(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	ds1 := seedMaskRuleDatasource(t, database, "ds1")
	ds2 := seedMaskRuleDatasource(t, database, "ds2")

	seedMaskRule(t, database, ds1, "mydb", "users", "phone", "phone")
	seedMaskRule(t, database, ds1, "mydb", "users", "email", "email")
	seedMaskRule(t, database, ds2, "mydb", "orders", "card", "bank_card")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/mask-rules?datasource_id=%d", ds1), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.ListMaskRules(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	result := decodeJSONResponse(t, rec)
	data, _ := result["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("len(data) = %d, want 2 (filtered by ds1)", len(data))
	}
	if total, _ := result["total"].(float64); total != 2 {
		t.Errorf("total = %v, want 2", total)
	}
}

// ─── UpdateMaskRule Handler Tests ────────────────────────────────────────────

func TestMaskRuleHandler_UpdateMaskRule_Success(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")
	ruleID := seedMaskRule(t, database, dsID, "mydb", "users", "phone", "phone")

	body := `{"table_name":"customers","field":"mobile","mask_type":"phone"}`
	req := httptest.NewRequest(http.MethodPut, "/api/mask-rules/:id", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ruleID))
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.UpdateMaskRule(c); err != nil {
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
	if data["table_name"] != "customers" {
		t.Errorf("table_name = %v, want customers", data["table_name"])
	}
	if data["field"] != "mobile" {
		t.Errorf("field = %v, want mobile", data["field"])
	}
}

func TestMaskRuleHandler_UpdateMaskRule_NotFound(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	body := `{"table_name":"users","field":"phone","mask_type":"phone"}`
	req := httptest.NewRequest(http.MethodPut, "/api/mask-rules/:id", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.UpdateMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "脱敏规则不存在" {
		t.Errorf("message = %q, want %q", msg, "脱敏规则不存在")
	}
}

func TestMaskRuleHandler_UpdateMaskRule_InvalidID(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	body := `{"table_name":"users","field":"phone","mask_type":"phone"}`
	req := httptest.NewRequest(http.MethodPut, "/api/mask-rules/:id", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("notanumber")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.UpdateMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "无效的规则ID" {
		t.Errorf("message = %q, want %q", msg, "无效的规则ID")
	}
}

func TestMaskRuleHandler_UpdateMaskRule_InvalidMaskType(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")
	ruleID := seedMaskRule(t, database, dsID, "mydb", "users", "phone", "phone")

	body := `{"mask_type":"invalid_type"}`
	req := httptest.NewRequest(http.MethodPut, "/api/mask-rules/:id", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ruleID))
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.UpdateMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "无效的脱敏类型" {
		t.Errorf("message = %q, want %q", msg, "无效的脱敏类型")
	}
}

func TestMaskRuleHandler_UpdateMaskRule_InvalidBody(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/mask-rules/:id", strings.NewReader(`{bad}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.UpdateMaskRule(c); err != nil {
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

// ─── DeleteMaskRule Handler Tests ────────────────────────────────────────────

func TestMaskRuleHandler_DeleteMaskRule_Success(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")
	ruleID := seedMaskRule(t, database, dsID, "mydb", "users", "phone", "phone")

	req := httptest.NewRequest(http.MethodDelete, "/api/mask-rules/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", ruleID))
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.DeleteMaskRule(c); err != nil {
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
	if data["message"] != "删除成功" {
		t.Errorf("message = %v, want 删除成功", data["message"])
	}
}

func TestMaskRuleHandler_DeleteMaskRule_NotFound(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/mask-rules/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.DeleteMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "脱敏规则不存在" {
		t.Errorf("message = %q, want %q", msg, "脱敏规则不存在")
	}
}

func TestMaskRuleHandler_DeleteMaskRule_InvalidID(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/mask-rules/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("notanumber")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.DeleteMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "无效的规则ID" {
		t.Errorf("message = %q, want %q", msg, "无效的规则ID")
	}
}

// ─── CreateSensitiveTable Handler Tests ──────────────────────────────────────

func TestMaskRuleHandler_CreateSensitiveTable_Success(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	body := fmt.Sprintf(`{"datasource_id":%d,"database":"mydb","table_name":"users","sensitivity_level":"high"}`, dsID)
	req := httptest.NewRequest(http.MethodPost, "/api/sensitive-tables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.CreateSensitiveTable(c); err != nil {
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
	if data["table_name"] != "users" {
		t.Errorf("table_name = %v, want users", data["table_name"])
	}
	if data["sensitivity_level"] != "high" {
		t.Errorf("sensitivity_level = %v, want high", data["sensitivity_level"])
	}
}

func TestMaskRuleHandler_CreateSensitiveTable_DefaultLevel(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	// No sensitivity_level provided — should default to "medium"
	body := fmt.Sprintf(`{"datasource_id":%d,"database":"mydb","table_name":"orders"}`, dsID)
	req := httptest.NewRequest(http.MethodPost, "/api/sensitive-tables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.CreateSensitiveTable(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	data, _ := result["data"].(map[string]interface{})
	if data["sensitivity_level"] != "medium" {
		t.Errorf("sensitivity_level = %v, want medium (default)", data["sensitivity_level"])
	}
}

func TestMaskRuleHandler_CreateSensitiveTable_Validation(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			"missing_datasource_id",
			`{"table_name":"users","sensitivity_level":"high"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"zero_datasource_id",
			`{"datasource_id":0,"table_name":"users","sensitivity_level":"high"}`,
			http.StatusBadRequest,
			"数据源ID不能为空",
		},
		{
			"missing_table_name",
			fmt.Sprintf(`{"datasource_id":%d,"sensitivity_level":"high"}`, dsID),
			http.StatusBadRequest,
			"表名不能为空",
		},
		{
			"empty_table_name",
			fmt.Sprintf(`{"datasource_id":%d,"table_name":"","sensitivity_level":"high"}`, dsID),
			http.StatusBadRequest,
			"表名不能为空",
		},
		{
			"invalid_sensitivity_level",
			fmt.Sprintf(`{"datasource_id":%d,"table_name":"users","sensitivity_level":"critical"}`, dsID),
			http.StatusBadRequest,
			"无效的敏感等级，可选: low, medium, high",
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
			req := httptest.NewRequest(http.MethodPost, "/api/sensitive-tables", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setMaskRuleAuthContext(c, 1, "admin", "admin")

			if err := h.CreateSensitiveTable(c); err != nil {
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

func TestMaskRuleHandler_CreateSensitiveTable_Duplicate(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	// Create first
	body := fmt.Sprintf(`{"datasource_id":%d,"database":"mydb","table_name":"users","sensitivity_level":"high"}`, dsID)
	req := httptest.NewRequest(http.MethodPost, "/api/sensitive-tables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")
	if err := h.CreateSensitiveTable(c); err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Try duplicate
	req = httptest.NewRequest(http.MethodPost, "/api/sensitive-tables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.CreateSensitiveTable(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "该表已标记为敏感表" {
		t.Errorf("message = %q, want %q", msg, "该表已标记为敏感表")
	}
}

// ─── ListSensitiveTables Handler Tests ───────────────────────────────────────

func TestMaskRuleHandler_ListSensitiveTables_Success(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	seedSensitiveTable(t, database, dsID, "mydb", "users", "high")
	seedSensitiveTable(t, database, dsID, "mydb", "orders", "medium")
	seedSensitiveTable(t, database, dsID, "mydb", "products", "low")

	req := httptest.NewRequest(http.MethodGet, "/api/sensitive-tables", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.ListSensitiveTables(c); err != nil {
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

func TestMaskRuleHandler_ListSensitiveTables_Empty(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sensitive-tables", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.ListSensitiveTables(c); err != nil {
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
}

func TestMaskRuleHandler_ListSensitiveTables_Pagination(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	for i := 0; i < 5; i++ {
		seedSensitiveTable(t, database, dsID, "mydb", fmt.Sprintf("table%d", i), "medium")
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
			url := "/api/sensitive-tables"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setMaskRuleAuthContext(c, 1, "admin", "admin")

			if err := h.ListSensitiveTables(c); err != nil {
				t.Fatalf("handler error: %v", err)
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

func TestMaskRuleHandler_ListSensitiveTables_FilterByDatasource(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	ds1 := seedMaskRuleDatasource(t, database, "ds1")
	ds2 := seedMaskRuleDatasource(t, database, "ds2")

	seedSensitiveTable(t, database, ds1, "mydb", "users", "high")
	seedSensitiveTable(t, database, ds1, "mydb", "accounts", "medium")
	seedSensitiveTable(t, database, ds2, "mydb", "logs", "low")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/sensitive-tables?datasource_id=%d", ds1), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.ListSensitiveTables(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	result := decodeJSONResponse(t, rec)
	data, _ := result["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("len(data) = %d, want 2 (filtered by ds1)", len(data))
	}
}

// ─── DeleteSensitiveTable Handler Tests ──────────────────────────────────────

func TestMaskRuleHandler_DeleteSensitiveTable_Success(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")
	stID := seedSensitiveTable(t, database, dsID, "mydb", "users", "high")

	req := httptest.NewRequest(http.MethodDelete, "/api/sensitive-tables/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", stID))
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.DeleteSensitiveTable(c); err != nil {
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
	if data["message"] != "删除成功" {
		t.Errorf("message = %v, want 删除成功", data["message"])
	}
}

func TestMaskRuleHandler_DeleteSensitiveTable_NotFound(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/sensitive-tables/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.DeleteSensitiveTable(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "敏感表记录不存在" {
		t.Errorf("message = %q, want %q", msg, "敏感表记录不存在")
	}
}

func TestMaskRuleHandler_DeleteSensitiveTable_InvalidID(t *testing.T) {
	e, h, _ := setupMaskRuleHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/sensitive-tables/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("notanumber")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := h.DeleteSensitiveTable(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "无效的记录ID" {
		t.Errorf("message = %q, want %q", msg, "无效的记录ID")
	}
}

// ─── DeleteMaskRule Error Path ────────────────────────────────────────────────

func TestMaskRuleHandler_DeleteMaskRule_DBError(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/test_del_rule_err.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditSvc := service.NewAuditService(database, 10, 5*time.Second)
	maskRuleSvc := service.NewMaskRuleService(database, nil, auditSvc)
	handler := NewMaskRuleHandler(maskRuleSvc)
	e := echo.New()

	// Close the database to force a non-ErrMaskRuleNotFound error
	database.Close()
	auditSvc.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/mask-rules/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := handler.DeleteMaskRule(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "删除脱敏规则失败" {
		t.Errorf("message = %q, want %q", msg, "删除脱敏规则失败")
	}
}

// ─── DeleteSensitiveTable Error Path ─────────────────────────────────────────

func TestMaskRuleHandler_DeleteSensitiveTable_DBError(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/test_del_st_err.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditSvc := service.NewAuditService(database, 10, 5*time.Second)
	maskRuleSvc := service.NewMaskRuleService(database, nil, auditSvc)
	handler := NewMaskRuleHandler(maskRuleSvc)
	e := echo.New()

	// Close the database to force a non-ErrSensitiveTableNotFound error
	database.Close()
	auditSvc.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/sensitive-tables/:id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	setMaskRuleAuthContext(c, 1, "admin", "admin")

	if err := handler.DeleteSensitiveTable(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	result := decodeJSONResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "删除敏感表失败" {
		t.Errorf("message = %q, want %q", msg, "删除敏感表失败")
	}
}

// ─── Sensitivity Level Tests (all valid levels) ─────────────────────────────

func TestMaskRuleHandler_CreateSensitiveTable_AllLevels(t *testing.T) {
	e, h, database := setupMaskRuleHandlerTest(t)
	dsID := seedMaskRuleDatasource(t, database, "test-ds")

	tests := []struct {
		level string
	}{
		{"low"},
		{"medium"},
		{"high"},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			body := fmt.Sprintf(`{"datasource_id":%d,"database":"mydb","table_name":"tbl_%s","sensitivity_level":"%s"}`, dsID, tt.level, tt.level)
			req := httptest.NewRequest(http.MethodPost, "/api/sensitive-tables", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setMaskRuleAuthContext(c, 1, "admin", "admin")

			if err := h.CreateSensitiveTable(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}

			if rec.Code != http.StatusCreated {
				t.Errorf("level=%s: status = %d, want %d; body = %s", tt.level, rec.Code, http.StatusCreated, rec.Body.String())
			}

			result := decodeJSONResponse(t, rec)
			data, _ := result["data"].(map[string]interface{})
			if data["sensitivity_level"] != tt.level {
				t.Errorf("sensitivity_level = %v, want %s", data["sensitivity_level"], tt.level)
			}
		})
	}
}
