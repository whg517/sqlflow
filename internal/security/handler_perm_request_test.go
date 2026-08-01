package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/testutil"
)

func setupPermReqTest(t *testing.T) (*echo.Echo, *RequestService, *RequestHandler) {
	t.Helper()
	d := testutil.NewDB(t)
	permSvc, err := NewService(d)
	if err != nil {
		t.Fatalf("NewPermissionService: %v", err)
	}
	svc := NewRequestService(d, permSvc, auditlog.Discard)
	h := NewRequestHandler(svc)
	// Seed a user to own requests.
	if _, err := d.DB.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('dev1','x','developer')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return echo.New(), svc, h
}

func TestPermReqHandler_CreateRequest_ValidationErrors(t *testing.T) {
	e, _, h := setupPermReqTest(t)

	cases := []struct {
		name string
		body string
	}{
		{"missing datasource_id", `{"database":"db","actions":"select"}`},
		{"missing database", `{"datasource_id":1,"actions":"select"}`},
		{"missing actions", `{"datasource_id":1,"database":"db"}`},
		{"malformed JSON", `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/permission-requests", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			testutil.SetContextUser(c, 1, "dev1", "developer")
			if err := h.CreateRequest(c); err != nil {
				t.Fatalf("CreateRequest returned error: %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestPermReqHandler_CreateRequest_InvalidAction(t *testing.T) {
	e, _, h := setupPermReqTest(t)

	body := `{"datasource_id":1,"database":"db","table_name":"t","actions":"bogus_action"}`
	req := httptest.NewRequest(http.MethodPost, "/api/permission-requests", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "dev1", "developer")
	if err := h.CreateRequest(c); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid action); body=%s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPermReqHandler_CreateRequest_Success(t *testing.T) {
	e, _, h := setupPermReqTest(t)

	body := `{"datasource_id":1,"database":"db","table_name":"t","actions":"select","reason":"need it"}`
	req := httptest.NewRequest(http.MethodPost, "/api/permission-requests", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "dev1", "developer")
	if err := h.CreateRequest(c); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestPermReqHandler_GetRequest_InvalidID(t *testing.T) {
	e, _, h := setupPermReqTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/permission-requests/abc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	if err := h.GetRequest(c); err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid id)", rec.Code, http.StatusBadRequest)
	}
}

func TestPermReqHandler_GetRequest_NotFound(t *testing.T) {
	e, _, h := setupPermReqTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/permission-requests/99999", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	if err := h.GetRequest(c); err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not found); body=%s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestPermReqHandler_ApproveRequest_NotFound(t *testing.T) {
	e, _, h := setupPermReqTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/permission-requests/99999/approve",
		strings.NewReader(`{"comment":"ok"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("99999")
	testutil.SetContextUser(c, 2, "dba1", "dba")
	if err := h.ApproveRequest(c); err != nil {
		t.Fatalf("ApproveRequest: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not found); body=%s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestPermReqHandler_MyRequests_Empty(t *testing.T) {
	e, _, h := setupPermReqTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/permission-requests/mine", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "dev1", "developer")
	if err := h.MyRequests(c); err != nil {
		t.Fatalf("MyRequests: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestPermReqHandler_ListRequests_DefaultPagination(t *testing.T) {
	e, _, h := setupPermReqTest(t)
	// No page/page_size → defaults.
	req := httptest.NewRequest(http.MethodGet, "/api/permission-requests", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")
	if err := h.ListRequests(c); err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestPermReqHandler_ExpireOverdue_NoError(t *testing.T) {
	e, _, h := setupPermReqTest(t)
	req := httptest.NewRequest(http.MethodPost, "/api/permission-requests/expire", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")
	if err := h.ExpireOverdue(c); err != nil {
		t.Fatalf("ExpireOverdue: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
