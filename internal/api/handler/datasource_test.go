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
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/service"
)

// ─── Test Setup ──────────────────────────────────────────────────────────────

// setupDatasourceTest creates a fresh Echo, DB, DatasourceService, and DatasourceHandler for testing.
func setupDatasourceTest(t *testing.T) (*echo.Echo, *service.DatasourceService, *DatasourceHandler) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256
	connMgr := connpool.NewManager()
	dsSvc := service.NewDatasourceService(database.DB, encKey, connMgr)
	handler := NewDatasourceHandler(dsSvc)

	e := echo.New()
	return e, dsSvc, handler
}

// createTestDatasource is a helper that creates a datasource directly via service.
func createTestDatasource(t *testing.T, dsSvc *service.DatasourceService, name, dsType, host string, port int) *model.DataSource {
	t.Helper()
	ctx := contextWithTimeout(t)
	ds := &model.DataSource{
		Name:              name,
		Type:              dsType,
		Host:              host,
		Port:              port,
		Username:          "root",
		PasswordEncrypted: "testpassword",
		Database:          "testdb",
	}
	if err := dsSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("create test datasource %q: %v", name, err)
	}
	return ds
}

// decodeDatasourceResponse decodes the top-level response envelope.
func decodeDatasourceResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

// extractDatasourceData extracts the "data" field as a map from the response.
func extractDatasourceData(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	result := decodeDatasourceResponse(t, rec)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data is not an object; body=%s", rec.Body.String())
	}
	return data
}

// ─── CreateDatasource Tests ─────────────────────────────────────────────────

func TestDatasourceHandler_CreateDatasource(t *testing.T) {
	e, _, h := setupDatasourceTest(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			"success_mysql",
			`{"name":"prod-mysql","type":"mysql","host":"10.0.0.1","port":3306,"username":"root","password":"secret","database":"app_db"}`,
			http.StatusCreated,
			"created",
		},
		{
			"success_mongodb",
			`{"name":"dev-mongo","type":"mongodb","host":"10.0.0.2","port":27017,"username":"admin","password":"secret","database":"testdb"}`,
			http.StatusCreated,
			"created",
		},
		{
			"missing_name",
			`{"type":"mysql","host":"10.0.0.1","port":3306,"username":"root","password":"secret"}`,
			http.StatusBadRequest,
			"数据源名称不能为空",
		},
		{
			"empty_name",
			`{"name":"","type":"mysql","host":"10.0.0.1","port":3306,"username":"root","password":"secret"}`,
			http.StatusBadRequest,
			"数据源名称不能为空",
		},
		{
			"invalid_type",
			`{"name":"bad-type","type":"postgres","host":"10.0.0.1","port":5432,"username":"root","password":"secret"}`,
			http.StatusBadRequest,
			"数据源类型必须是 mysql 或 mongodb",
		},
		{
			"missing_host",
			`{"name":"no-host","type":"mysql","port":3306,"username":"root","password":"secret"}`,
			http.StatusBadRequest,
			"主机地址不能为空",
		},
		{
			"empty_host",
			`{"name":"no-host","type":"mysql","host":"","port":3306,"username":"root","password":"secret"}`,
			http.StatusBadRequest,
			"主机地址不能为空",
		},
		{
			"missing_port",
			`{"name":"no-port","type":"mysql","host":"10.0.0.1","username":"root","password":"secret"}`,
			http.StatusBadRequest,
			"端口不能为空",
		},
		{
			"zero_port",
			`{"name":"zero-port","type":"mysql","host":"10.0.0.1","port":0,"username":"root","password":"secret"}`,
			http.StatusBadRequest,
			"端口不能为空",
		},
		{
			"missing_password",
			`{"name":"no-pw","type":"mysql","host":"10.0.0.1","port":3306,"username":"root"}`,
			http.StatusBadRequest,
			"密码不能为空",
		},
		{
			"empty_password",
			`{"name":"no-pw","type":"mysql","host":"10.0.0.1","port":3306,"username":"root","password":""}`,
			http.StatusBadRequest,
			"密码不能为空",
		},
		{
			"empty_body",
			``,
			http.StatusBadRequest,
			"数据源名称不能为空",
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
			req := httptest.NewRequest(http.MethodPost, "/api/datasources", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := h.CreateDatasource(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			result := decodeDatasourceResponse(t, rec)
			msg, _ := result["message"].(string)
			if msg != tt.wantMsg {
				t.Errorf("message = %q, want %q", msg, tt.wantMsg)
			}
		})
	}

	// Verify success response shape
	t.Run("success_response_shape", func(t *testing.T) {
		body := `{"name":"shape-test","type":"mysql","host":"10.0.0.1","port":3306,"username":"root","password":"secret","database":"mydb","max_open":20,"max_idle":10,"max_lifetime":1800,"max_idle_time":300}`
		req := httptest.NewRequest(http.MethodPost, "/api/datasources", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.CreateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
		}

		data := extractDatasourceData(t, rec)

		// Verify all expected fields present
		for _, field := range []string{"id", "name", "type", "host", "port", "username", "database", "max_open", "max_idle", "max_lifetime", "max_idle_time", "status", "created_at", "updated_at"} {
			if _, ok := data[field]; !ok {
				t.Errorf("missing field %q in response", field)
			}
		}

		if data["name"] != "shape-test" {
			t.Errorf("name = %v, want shape-test", data["name"])
		}
		if data["type"] != "mysql" {
			t.Errorf("type = %v, want mysql", data["type"])
		}
		if data["host"] != "10.0.0.1" {
			t.Errorf("host = %v, want 10.0.0.1", data["host"])
		}
		if data["port"] != float64(3306) {
			t.Errorf("port = %v, want 3306", data["port"])
		}
		if data["database"] != "mydb" {
			t.Errorf("database = %v, want mydb", data["database"])
		}
		if data["max_open"] != float64(20) {
			t.Errorf("max_open = %v, want 20", data["max_open"])
		}
		if data["max_idle"] != float64(10) {
			t.Errorf("max_idle = %v, want 10", data["max_idle"])
		}
		if data["max_lifetime"] != float64(1800) {
			t.Errorf("max_lifetime = %v, want 1800", data["max_lifetime"])
		}
		if data["max_idle_time"] != float64(300) {
			t.Errorf("max_idle_time = %v, want 300", data["max_idle_time"])
		}
		if data["status"] != "active" {
			t.Errorf("status = %v, want active", data["status"])
		}
	})

	// Verify defaults applied when pool config is zero
	t.Run("default_pool_config", func(t *testing.T) {
		body := `{"name":"defaults-test","type":"mysql","host":"10.0.0.1","port":3306,"username":"root","password":"secret"}`
		req := httptest.NewRequest(http.MethodPost, "/api/datasources", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.CreateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractDatasourceData(t, rec)
		if data["max_open"] != float64(10) {
			t.Errorf("max_open = %v, want 10 (default)", data["max_open"])
		}
		if data["max_idle"] != float64(5) {
			t.Errorf("max_idle = %v, want 5 (default)", data["max_idle"])
		}
		if data["max_lifetime"] != float64(3600) {
			t.Errorf("max_lifetime = %v, want 3600 (default)", data["max_lifetime"])
		}
		if data["max_idle_time"] != float64(600) {
			t.Errorf("max_idle_time = %v, want 600 (default)", data["max_idle_time"])
		}
	})

	// Duplicate name should fail
	t.Run("duplicate_name", func(t *testing.T) {
		_, dsSvc, h := setupDatasourceTest(t)
		createTestDatasource(t, dsSvc, "dup-name", "mysql", "10.0.0.1", 3306)

		body := `{"name":"dup-name","type":"mysql","host":"10.0.0.2","port":3306,"username":"root","password":"secret"}`
		req := httptest.NewRequest(http.MethodPost, "/api/datasources", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.CreateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
		}
	})
}

// ─── ListDatasources Tests ──────────────────────────────────────────────────

func TestDatasourceHandler_ListDatasources(t *testing.T) {
	e, dsSvc, h := setupDatasourceTest(t)
	ctx := contextWithTimeout(t)

	// Create multiple datasources
	names := []string{"ds-alpha", "ds-beta", "ds-gamma"}
	for i, name := range names {
		ds := &model.DataSource{
			Name:              name,
			Type:              "mysql",
			Host:              "10.0.0.1",
			Port:              3306 + i,
			Username:          "root",
			PasswordEncrypted: "secret",
		}
		if err := dsSvc.CreateDataSource(ctx, ds); err != nil {
			t.Fatalf("create datasource %q: %v", name, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/datasources", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListDatasources(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeDatasourceResponse(t, rec)
	items, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array; body=%s", rec.Body.String())
	}

	if len(items) != 3 {
		t.Errorf("len(data) = %d, want 3", len(items))
	}
}

func TestDatasourceHandler_ListDatasources_Empty(t *testing.T) {
	e, _, h := setupDatasourceTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/datasources", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListDatasources(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeDatasourceResponse(t, rec)
	items, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array; body=%s", rec.Body.String())
	}

	if len(items) != 0 {
		t.Errorf("len(data) = %d, want 0", len(items))
	}
}

func TestDatasourceHandler_ListDatasources_NoPasswords(t *testing.T) {
	_, dsSvc, h := setupDatasourceTest(t)
	ctx := contextWithTimeout(t)

	ds := &model.DataSource{
		Name:              "pw-test",
		Type:              "mysql",
		Host:              "10.0.0.1",
		Port:              3306,
		Username:          "root",
		PasswordEncrypted: "supersecret",
	}
	if err := dsSvc.CreateDataSource(ctx, ds); err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/datasources", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListDatasources(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Response body should not contain the raw password
	body := rec.Body.String()
	if strings.Contains(body, "supersecret") {
		t.Error("response body should not contain the raw password")
	}
}

// ─── GetDatasource Tests ────────────────────────────────────────────────────

func TestDatasourceHandler_GetDatasource(t *testing.T) {
	e, dsSvc, h := setupDatasourceTest(t)
	ds := createTestDatasource(t, dsSvc, "get-test", "mysql", "10.0.0.1", 3306)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.GetDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		data := extractDatasourceData(t, rec)
		if data["name"] != "get-test" {
			t.Errorf("name = %v, want get-test", data["name"])
		}
		if data["type"] != "mysql" {
			t.Errorf("type = %v, want mysql", data["type"])
		}
		if data["host"] != "10.0.0.1" {
			t.Errorf("host = %v, want 10.0.0.1", data["host"])
		}
		if data["port"] != float64(3306) {
			t.Errorf("port = %v, want 3306", data["port"])
		}
	})

	t.Run("not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("99999")

		if err := h.GetDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("notanumber")

		if err := h.GetDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("no_password_in_response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.GetDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		body := rec.Body.String()
		if strings.Contains(body, "testpassword") {
			t.Error("response should not contain the raw password")
		}
	})
}

// ─── UpdateDatasource Tests ─────────────────────────────────────────────────

func TestDatasourceHandler_UpdateDatasource(t *testing.T) {
	e, dsSvc, h := setupDatasourceTest(t)
	ds := createTestDatasource(t, dsSvc, "update-test", "mysql", "10.0.0.1", 3306)

	t.Run("success", func(t *testing.T) {
		body := `{"name":"updated-name","type":"mysql","host":"10.0.0.2","port":3307,"username":"admin","database":"newdb","max_open":20,"max_idle":10,"max_lifetime":1800,"max_idle_time":300}`
		req := httptest.NewRequest(http.MethodPut, "/api/datasources/:id", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.UpdateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		data := extractDatasourceData(t, rec)
		if data["name"] != "updated-name" {
			t.Errorf("name = %v, want updated-name", data["name"])
		}
		if data["host"] != "10.0.0.2" {
			t.Errorf("host = %v, want 10.0.0.2", data["host"])
		}
		if data["port"] != float64(3307) {
			t.Errorf("port = %v, want 3307", data["port"])
		}
		if data["database"] != "newdb" {
			t.Errorf("database = %v, want newdb", data["database"])
		}
		if data["max_open"] != float64(20) {
			t.Errorf("max_open = %v, want 20", data["max_open"])
		}
	})

	t.Run("invalid_type", func(t *testing.T) {
		body := `{"name":"bad","type":"postgres","host":"10.0.0.1","port":5432,"username":"root"}`
		req := httptest.NewRequest(http.MethodPut, "/api/datasources/:id", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.UpdateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		body := `{"name":"x","type":"mysql","host":"10.0.0.1","port":3306,"username":"root"}`
		req := httptest.NewRequest(http.MethodPut, "/api/datasources/:id", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("99999")

		if err := h.UpdateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		body := `{"name":"x","type":"mysql","host":"10.0.0.1","port":3306,"username":"root"}`
		req := httptest.NewRequest(http.MethodPut, "/api/datasources/:id", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("abc")

		if err := h.UpdateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("empty_body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/datasources/:id", strings.NewReader(``))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.UpdateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})
}

// ─── DisableDatasource Tests ────────────────────────────────────────────────

func TestDatasourceHandler_DisableDatasource(t *testing.T) {
	e, dsSvc, h := setupDatasourceTest(t)
	ds := createTestDatasource(t, dsSvc, "disable-test", "mysql", "10.0.0.1", 3306)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.DisableDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodeDatasourceResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "数据源已禁用" {
			t.Errorf("message = %q, want %q", msg, "数据源已禁用")
		}

		// Verify status changed in DB
		ctx := contextWithTimeout(t)
		updated, err := dsSvc.GetDataSource(ctx, ds.ID)
		if err != nil {
			t.Fatalf("get datasource: %v", err)
		}
		if updated.Status != "disabled" {
			t.Errorf("status = %q, want disabled", updated.Status)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("99999")

		if err := h.DisableDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("invalid")

		if err := h.DisableDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("already_disabled", func(t *testing.T) {
		// Disabling an already-disabled datasource should still succeed (idempotent)
		req := httptest.NewRequest(http.MethodDelete, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.DisableDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})
}

// ─── TestConnection Tests ───────────────────────────────────────────────────

func TestDatasourceHandler_TestConnection(t *testing.T) {
	e, dsSvc, h := setupDatasourceTest(t)
	ds := createTestDatasource(t, dsSvc, "test-conn", "mysql", "10.0.0.1", 3306)

	t.Run("connection_fails_gracefully", func(t *testing.T) {
		// The connection will fail because there's no real MySQL server, but it should
		// return 200 with success=false, not a 500 error.
		req := httptest.NewRequest(http.MethodPost, "/api/datasources/:id/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.TestConnection(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		data := extractDatasourceData(t, rec)
		success, _ := data["success"].(bool)
		if success {
			t.Error("expected success=false since no real MySQL server is running")
		}
		if msg, _ := data["message"].(string); msg != "连接失败" {
			t.Errorf("message = %q, want %q", msg, "连接失败")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/datasources/:id/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("99999")

		if err := h.TestConnection(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/datasources/:id/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("abc")

		if err := h.TestConnection(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})
}

// ─── GetTables Tests ────────────────────────────────────────────────────────

func TestDatasourceHandler_GetTables(t *testing.T) {
	e, dsSvc, h := setupDatasourceTest(t)
	ds := createTestDatasource(t, dsSvc, "tables-test", "mysql", "10.0.0.1", 3306)

	t.Run("connection_error", func(t *testing.T) {
		// No real MySQL server, so this should fail with internal error
		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id/tables", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.GetTables(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		// Should be 500 because the connection fails (no real server)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id/tables", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("99999")

		if err := h.GetTables(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id/tables", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("abc")

		if err := h.GetTables(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("disabled_datasource", func(t *testing.T) {
		// Disable the datasource first
		ctx := contextWithTimeout(t)
		if err := dsSvc.DisableDataSource(ctx, ds.ID); err != nil {
			t.Fatalf("disable datasource: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id/tables", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", ds.ID))

		if err := h.GetTables(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}

		result := decodeDatasourceResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "数据源已禁用" {
			t.Errorf("message = %q, want %q", msg, "数据源已禁用")
		}
	})
}

// ─── parseDatasourceID Tests ────────────────────────────────────────────────

func TestParseDatasourceID(t *testing.T) {
	e := echo.New()

	tests := []struct {
		name    string
		idParam string
		wantID  int64
		wantErr bool
	}{
		{"valid", "123", 123, false},
		{"one", "1", 1, false},
		{"large", "9999999999", 9999999999, false},
		{"invalid_string", "abc", 0, true},
		{"empty", "", 0, true},
		{"negative", "-1", -1, false},
		{"float_string", "1.5", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(tt.idParam)

			id, err := parseDatasourceID(c)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDatasourceID(%q) error = %v, wantErr %v", tt.idParam, err, tt.wantErr)
			}
			if !tt.wantErr && id != tt.wantID {
				t.Errorf("parseDatasourceID(%q) = %d, want %d", tt.idParam, id, tt.wantID)
			}
		})
	}
}

// ─── Integration: CRUD lifecycle ────────────────────────────────────────────

func TestDatasourceHandler_CRUDLifecycle(t *testing.T) {
	e, dsSvc, h := setupDatasourceTest(t)

	// Step 1: List (empty)
	t.Run("list_empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListDatasources(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeDatasourceResponse(t, rec)
		items, _ := result["data"].([]interface{})
		if len(items) != 0 {
			t.Errorf("expected empty list, got %d items", len(items))
		}
	})

	// Step 2: Create
	var createdID float64
	t.Run("create", func(t *testing.T) {
		body := `{"name":"lifecycle-ds","type":"mysql","host":"10.0.0.1","port":3306,"username":"root","password":"secret","database":"app"}`
		req := httptest.NewRequest(http.MethodPost, "/api/datasources", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.CreateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractDatasourceData(t, rec)
		createdID = data["id"].(float64)
		if createdID <= 0 {
			t.Errorf("expected positive id, got %v", createdID)
		}
	})

	// Step 3: Get
	t.Run("get", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%.0f", createdID))

		if err := h.GetDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractDatasourceData(t, rec)
		if data["name"] != "lifecycle-ds" {
			t.Errorf("name = %v, want lifecycle-ds", data["name"])
		}
		if data["status"] != "active" {
			t.Errorf("status = %v, want active", data["status"])
		}
	})

	// Step 4: Update
	t.Run("update", func(t *testing.T) {
		body := `{"name":"lifecycle-updated","type":"mysql","host":"10.0.0.2","port":3307,"username":"admin","database":"newapp"}`
		req := httptest.NewRequest(http.MethodPut, "/api/datasources/:id", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%.0f", createdID))

		if err := h.UpdateDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractDatasourceData(t, rec)
		if data["name"] != "lifecycle-updated" {
			t.Errorf("name = %v, want lifecycle-updated", data["name"])
		}
	})

	// Step 5: List (should have 1 item)
	t.Run("list_with_item", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListDatasources(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeDatasourceResponse(t, rec)
		items, _ := result["data"].([]interface{})
		if len(items) != 1 {
			t.Errorf("expected 1 item, got %d", len(items))
		}
	})

	// Step 6: Disable
	t.Run("disable", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%.0f", createdID))

		if err := h.DisableDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	// Step 7: Verify disabled via Get
	t.Run("verify_disabled", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasources/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%.0f", createdID))

		if err := h.GetDatasource(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractDatasourceData(t, rec)
		if data["status"] != "disabled" {
			t.Errorf("status = %v, want disabled", data["status"])
		}
	})

	// Suppress unused import warnings
	_ = time.Second
	_ = contextWithTimeout
	_ = dsSvc.GetDataSource
}

// ─── toDatasourceResponse Tests ─────────────────────────────────────────────

func TestToDatasourceResponse(t *testing.T) {
	ds := &model.DataSource{
		ID:         42,
		Name:       "test-ds",
		Type:       "mysql",
		Host:       "127.0.0.1",
		Port:       3306,
		Username:   "root",
		Database:   "appdb",
		MaxOpen:    10,
		MaxIdle:    5,
		MaxLifetime: 3600,
		MaxIdleTime: 600,
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	resp := toDatasourceResponse(ds)

	if resp.ID != ds.ID {
		t.Errorf("ID = %d, want %d", resp.ID, ds.ID)
	}
	if resp.Name != ds.Name {
		t.Errorf("Name = %s, want %s", resp.Name, ds.Name)
	}
	if resp.Type != ds.Type {
		t.Errorf("Type = %s, want %s", resp.Type, ds.Type)
	}
	if resp.Host != ds.Host {
		t.Errorf("Host = %s, want %s", resp.Host, ds.Host)
	}
	if resp.Port != ds.Port {
		t.Errorf("Port = %d, want %d", resp.Port, ds.Port)
	}
	if resp.Username != ds.Username {
		t.Errorf("Username = %s, want %s", resp.Username, ds.Username)
	}
	if resp.Database != ds.Database {
		t.Errorf("Database = %s, want %s", resp.Database, ds.Database)
	}
	if resp.MaxOpen != ds.MaxOpen {
		t.Errorf("MaxOpen = %d, want %d", resp.MaxOpen, ds.MaxOpen)
	}
	if resp.MaxIdle != ds.MaxIdle {
		t.Errorf("MaxIdle = %d, want %d", resp.MaxIdle, ds.MaxIdle)
	}
	if resp.MaxLifetime != ds.MaxLifetime {
		t.Errorf("MaxLifetime = %d, want %d", resp.MaxLifetime, ds.MaxLifetime)
	}
	if resp.MaxIdleTime != ds.MaxIdleTime {
		t.Errorf("MaxIdleTime = %d, want %d", resp.MaxIdleTime, ds.MaxIdleTime)
	}
	if resp.Status != ds.Status {
		t.Errorf("Status = %s, want %s", resp.Status, ds.Status)
	}
	if resp.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if resp.UpdatedAt == "" {
		t.Error("UpdatedAt should not be empty")
	}
}
