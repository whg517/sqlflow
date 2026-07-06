package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/service"
	"github.com/whg517/sqlflow/internal/testutil"
)

func setupSQLTemplateTest(t *testing.T) (*echo.Echo, *service.TemplateService, *SQLTemplateHandler) {
	t.Helper()
	d := testutil.NewDB(t)
	svc := service.NewSQLTemplateService(d)
	h := NewSQLTemplateHandler(svc)
	return echo.New(), svc, h
}

func TestSQLTemplateHandler_CreateTemplate_ValidationErrors(t *testing.T) {
	e, _, h := setupSQLTemplateTest(t)

	cases := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":"","sql_content":"SELECT 1"}`},
		{"name too long", `{"name":"` + strings.Repeat("x", 101) + `","sql_content":"SELECT 1"}`},
		{"empty sql_content", `{"name":"t1","sql_content":""}`},
		{"malformed JSON", `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/sql-templates", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			setContextUser(c, 1, "alice", "developer")
			if err := h.CreateTemplate(c); err != nil {
				t.Fatalf("CreateTemplate returned error: %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestSQLTemplateHandler_CreateTemplate_Success(t *testing.T) {
	e, _, h := setupSQLTemplateTest(t)

	body := `{"name":"get-user","description":"lookup","sql_content":"SELECT * FROM users WHERE id = {{.id}}","db_type":"mysql","category":"general","is_public":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/sql-templates", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "alice", "developer")
	if err := h.CreateTemplate(c); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data, _ := resp["data"].(map[string]interface{})
	if data == nil || data["name"] != "get-user" {
		t.Errorf("expected data.name=get-user, got %v", resp["data"])
	}
}

func TestSQLTemplateHandler_CreateTemplate_DuplicateName(t *testing.T) {
	e, svc, h := setupSQLTemplateTest(t)
	ctx := context.Background()
	// Pre-create via service.
	if _, err := svc.CreateTemplate(ctx, 1, "dup", "", "SELECT 1", "mysql", "general", true); err != nil {
		t.Fatalf("seed: %v", err)
	}
	body := `{"name":"dup","sql_content":"SELECT 2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sql-templates", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "alice", "developer")
	if err := h.CreateTemplate(c); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (duplicate); body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestSQLTemplateHandler_GetTemplate_NotFound(t *testing.T) {
	e, _, h := setupSQLTemplateTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/sql-templates/99999", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	if err := h.GetTemplate(c); err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not found); body=%s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestSQLTemplateHandler_GetTemplate_InvalidID(t *testing.T) {
	e, _, h := setupSQLTemplateTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/sql-templates/abc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	if err := h.GetTemplate(c); err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid id)", rec.Code, http.StatusBadRequest)
	}
}

func TestSQLTemplateHandler_ListTemplates_Empty(t *testing.T) {
	e, _, h := setupSQLTemplateTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/sql-templates", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "alice", "developer")
	if err := h.ListTemplates(c); err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestSQLTemplateHandler_DeleteTemplate_NotFound(t *testing.T) {
	e, _, h := setupSQLTemplateTest(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/sql-templates/99999", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	setContextUser(c, 1, "alice", "developer")
	if err := h.DeleteTemplate(c); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not found); body=%s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestSQLTemplateHandler_RenderTemplate_NotFound(t *testing.T) {
	e, _, h := setupSQLTemplateTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/sql-templates/99999/render",
		strings.NewReader(`{"params":{}}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	if err := h.RenderTemplate(c); err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not found); body=%s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestSQLTemplateHandler_RenderTemplate_Success(t *testing.T) {
	e, svc, h := setupSQLTemplateTest(t)
	ctx := context.Background()
	tpl, err := svc.CreateTemplate(ctx, 1, "rt", "", "SELECT {{.col}} FROM t", "mysql", "general", true)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	idStr := strconv.FormatInt(tpl.ID, 10)
	body := `{"params":{"col":"id"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/sql-templates/"+idStr+"/render", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	if err := h.RenderTemplate(c); err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
