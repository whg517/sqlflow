package query

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/testutil"
)

// postTemplate posts a create request and returns the recorder.
func postTemplate(t *testing.T, e *echo.Echo, h *TemplateHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sql-templates", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "alice", "developer")
	if err := h.CreateTemplate(c); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	return rec
}

// TestCreateTemplateRequiresAnExplicitDBType removes the last implicit MySQL.
//
// An omitted db_type was silently filled in with "mysql". The field is not
// cosmetic: it selects the placeholder dialect the renderer emits, so a
// PostgreSQL template saved without it rendered `?` markers that PostgreSQL
// rejects — and the author found out at query time, not at save time.
//
// The browser always sends the field, so only non-browser clients were
// affected, which is precisely the case the server cannot delegate.
func TestCreateTemplateRequiresAnExplicitDBType(t *testing.T) {
	e, _, h := setupSQLTemplateTest(t)

	rec := postTemplate(t, e, h,
		`{"name":"no-type","sql_content":"SELECT * FROM users WHERE id = {{id}}"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — an omitted db_type was guessed; body = %s",
			rec.Code, rec.Body.String())
	}
}

// TestCreateTemplateKeepsTheDialectItWasGiven guards the other direction: the
// requirement is that the caller says which dialect, not that only one works.
func TestCreateTemplateKeepsTheDialectItWasGiven(t *testing.T) {
	e, _, h := setupSQLTemplateTest(t)

	for _, dbType := range []string{"mysql", "postgresql", "sqlite"} {
		t.Run(dbType, func(t *testing.T) {
			rec := postTemplate(t, e, h,
				`{"name":"tpl-`+dbType+`","sql_content":"SELECT * FROM users WHERE id = {{id}}","db_type":"`+dbType+`"}`)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
			}

			var resp struct {
				Data struct {
					DBType string `json:"db_type"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Data.DBType != dbType {
				t.Errorf("db_type = %q, want %q", resp.Data.DBType, dbType)
			}
		})
	}
}

// TestUpdateTemplateRequiresAnExplicitDBType covers the same default on the
// update path, where the consequence is worse: a template saved as PostgreSQL
// and edited by a client that omits the field would silently become MySQL.
func TestUpdateTemplateRequiresAnExplicitDBType(t *testing.T) {
	e, svc, h := setupSQLTemplateTest(t)

	tpl, err := svc.CreateTemplate(t.Context(), 1, "pg-tpl", "", "SELECT {{id}}", "postgresql", "general", false)
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/sql-templates/1",
		strings.NewReader(`{"name":"pg-tpl","sql_content":"SELECT {{id}}"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(tpl.ID, 10))
	testutil.SetContextUser(c, 1, "alice", "developer")

	if err := h.UpdateTemplate(c); err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 — an omitted db_type would have rewritten the dialect to mysql; body = %s",
			rec.Code, rec.Body.String())
	}
}
