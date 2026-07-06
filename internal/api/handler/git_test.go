package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/service"
	"github.com/whg517/sqlflow/internal/testutil"
)

func setupGitTest(t *testing.T) (*echo.Echo, *service.GitService, *GitHandler) {
	t.Helper()
	d := testutil.NewDB(t)
	// Seed a user so created_by FK is satisfied.
	if _, err := d.DB.Exec(`INSERT INTO users (id, username, password_hash, role) VALUES (1, 'alice', 'x', 'developer')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	gitSvc := service.NewGitService(d)
	h := NewGitHandler(gitSvc)
	return echo.New(), gitSvc, h
}

func gitReq(e *echo.Echo, method, path, body string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestGitHandler_CreateGitLink_ValidationErrors(t *testing.T) {
	e, _, h := setupGitTest(t)

	cases := []struct {
		name string
		body string
	}{
		{"invalid entity_type", `{"entity_type":"bogus","entity_id":1,"link_type":"commit","commit_hash":"abc"}`},
		{"zero entity_id", `{"entity_type":"ticket","entity_id":0,"link_type":"commit","commit_hash":"abc"}`},
		{"invalid link_type", `{"entity_type":"ticket","entity_id":1,"link_type":"bogus","commit_hash":"abc"}`},
		{"missing commit_hash and pr_number", `{"entity_type":"ticket","entity_id":1,"link_type":"commit"}`},
		{"malformed JSON", `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := gitReq(e, http.MethodPost, "/api/git-links", tc.body)
			if err := h.CreateGitLink(c); err != nil {
				t.Fatalf("CreateGitLink returned error: %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestGitHandler_CreateGitLink_Success(t *testing.T) {
	e, _, h := setupGitTest(t)

	// A ticket must exist for FK if the service enforces it; create one via raw SQL.
	c, rec := gitReq(e, http.MethodPost, "/api/git-links",
		`{"entity_type":"audit_log","entity_id":1,"link_type":"commit","commit_hash":"deadbeef","commit_message":"fix","author_name":"alice","repo_url":"https://example.com/repo","branch":"main"}`)
	setContextUser(c, 1, "alice", "developer")
	if err := h.CreateGitLink(c); err != nil {
		t.Fatalf("CreateGitLink: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data: %v", resp)
	}
	if data["commit_hash"] != "deadbeef" {
		t.Errorf("commit_hash = %v, want deadbeef", data["commit_hash"])
	}
}

func TestGitHandler_ListGitLinks_ValidationErrors(t *testing.T) {
	e, _, h := setupGitTest(t)

	cases := []struct {
		name     string
		query    string
		wantCode int
	}{
		{"missing entity_type", "/api/git-links?entity_id=1", http.StatusBadRequest},
		{"missing entity_id", "/api/git-links?entity_type=ticket", http.StatusBadRequest},
		{"invalid entity_id", "/api/git-links?entity_type=ticket&entity_id=abc", http.StatusBadRequest},
		{"zero entity_id", "/api/git-links?entity_type=ticket&entity_id=0", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.query, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if err := h.ListGitLinks(c); err != nil {
				t.Fatalf("ListGitLinks returned error: %v", err)
			}
			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
		})
	}
}

func TestGitHandler_DeleteGitLink_InvalidID(t *testing.T) {
	e, _, h := setupGitTest(t)

	c, rec := gitReq(e, http.MethodDelete, "/api/git-links/abc", "")
	c.SetParamNames("id")
	c.SetParamValues("abc")
	if err := h.DeleteGitLink(c); err != nil {
		t.Fatalf("DeleteGitLink returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid id)", rec.Code, http.StatusBadRequest)
	}
}

func TestGitHandler_DeleteGitLink_NotFound(t *testing.T) {
	e, _, h := setupGitTest(t)

	c, rec := gitReq(e, http.MethodDelete, "/api/git-links/99999", "")
	c.SetParamNames("id")
	c.SetParamValues("99999")
	if err := h.DeleteGitLink(c); err != nil {
		t.Fatalf("DeleteGitLink returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not found); body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
