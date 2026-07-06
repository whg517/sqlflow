package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/service"
	"github.com/whg517/sqlflow/internal/testutil"
)

// noExpiry is the zero time.Time used to create tokens that never expire.
var noExpiry time.Time

// setupTokenTest builds a fresh Echo instance, TokenService and TokenHandler
// backed by a per-test migrated SQLite DB, plus a seeded user.
func setupTokenTest(t *testing.T) (*echo.Echo, *service.TokenService, *TokenHandler, int64) {
	t.Helper()
	d := testutil.NewDB(t)

	authSvc := service.NewAuthService(d, "test-jwt-secret-32byteslong!", 0)
	tokenSvc := service.NewTokenService(d)
	h := NewTokenHandler(tokenSvc)
	e := echo.New()

	// Seed a user to own tokens. role=admin is harmless here since token
	// issuance does not gate on role at the handler layer (middleware does).
	ctx := contextWithTimeout(t)
	user, err := authSvc.CreateUser(ctx, "tokenowner", "Password123!", "developer")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return e, tokenSvc, h, user.ID
}

func newTokenContext(e *echo.Echo, method, path, body string, userID int64) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, userID, "tokenowner", "developer")
	return c, rec
}

func TestTokenHandler_CreateToken_Success(t *testing.T) {
	e, _, h, userID := setupTokenTest(t)

	c, rec := newTokenContext(e, http.MethodPost, "/api/tokens",
		`{"name":"ci-token","scopes":["read:query"],"expires_days":30}`, userID)
	c.SetPath("/api/tokens")

	if err := h.CreateToken(c); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v; body=%s", err, rec.Body.String())
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data field: %v", resp)
	}
	if data["name"] != "ci-token" {
		t.Errorf("name = %v, want ci-token", data["name"])
	}
	if data["token"] == nil || data["token"] == "" {
		t.Error("expected non-empty plain token returned at creation")
	}
	if data["token_prefix"] == nil || data["token_prefix"] == "" {
		t.Error("expected non-empty token_prefix")
	}
}

func TestTokenHandler_CreateToken_ValidationErrors(t *testing.T) {
	e, _, h, userID := setupTokenTest(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty name", `{"name":"","scopes":["read:query"]}`, http.StatusBadRequest},
		{"missing scopes", `{"name":"no-scopes","scopes":[]}`, http.StatusBadRequest},
		{"name too long", `{"name":"` + strings.Repeat("x", 51) + `","scopes":["read:query"]}`, http.StatusBadRequest},
		{"invalid JSON body", `{not-json`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newTokenContext(e, http.MethodPost, "/api/tokens", tc.body, userID)
			c.SetPath("/api/tokens")
			if err := h.CreateToken(c); err != nil {
				t.Fatalf("CreateToken returned error: %v", err)
			}
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestTokenHandler_CreateToken_DuplicateName_Conflict(t *testing.T) {
	e, tokenSvc, h, userID := setupTokenTest(t)
	ctx := contextWithTimeout(t)

	// Pre-create a token with the same name directly via the service.
	if _, _, err := tokenSvc.CreateToken(ctx, userID, "dup", "", []string{"read:query"}, noExpiry); err != nil {
		t.Fatalf("seed first token: %v", err)
	}

	c, rec := newTokenContext(e, http.MethodPost, "/api/tokens",
		`{"name":"dup","scopes":["read:query"]}`, userID)
	c.SetPath("/api/tokens")
	if err := h.CreateToken(c); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (duplicate name conflict); body=%s",
			rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestTokenHandler_CreateToken_InvalidScope(t *testing.T) {
	e, _, h, userID := setupTokenTest(t)

	c, rec := newTokenContext(e, http.MethodPost, "/api/tokens",
		`{"name":"bad-scope","scopes":["bogus:scope"]}`, userID)
	c.SetPath("/api/tokens")
	if err := h.CreateToken(c); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid scope); body=%s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestTokenHandler_ListMyTokens_ReturnsCreated(t *testing.T) {
	e, tokenSvc, h, userID := setupTokenTest(t)
	ctx := contextWithTimeout(t)

	if _, _, err := tokenSvc.CreateToken(ctx, userID, "t1", "", []string{"read:query"}, noExpiry); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	c, rec := newTokenContext(e, http.MethodGet, "/api/tokens", "", userID)
	c.SetPath("/api/tokens")
	if err := h.ListMyTokens(c); err != nil {
		t.Fatalf("ListMyTokens: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data, _ := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("expected 1 token, got %d", len(data))
	}
}

func TestTokenHandler_GetTokenStats(t *testing.T) {
	e, tokenSvc, h, userID := setupTokenTest(t)
	ctx := contextWithTimeout(t)

	// Create one active token.
	if _, _, err := tokenSvc.CreateToken(ctx, userID, "stat-token", "", []string{"read:query", "read:audit"}, noExpiry); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	c, rec := newTokenContext(e, http.MethodGet, "/api/tokens/stats", "", userID)
	c.SetPath("/api/tokens/stats")
	if err := h.GetTokenStats(c); err != nil {
		t.Fatalf("GetTokenStats: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data, _ := resp["data"].(map[string]interface{})
	if data["total_tokens"] == nil {
		t.Fatal("missing total_tokens")
	}
	// total_tokens is json-encoded as a float64 by encoding/json.
	if data["total_tokens"].(float64) < 1 {
		t.Errorf("total_tokens = %v, want >= 1", data["total_tokens"])
	}
}

func TestTokenHandler_RevokeMyToken_NotFound(t *testing.T) {
	e, _, h, userID := setupTokenTest(t)

	c, rec := newTokenContext(e, http.MethodDelete, "/api/tokens/99999", "", userID)
	c.SetPath("/api/tokens/:id")
	c.SetParamNames("id")
	c.SetParamValues("99999")

	if err := h.RevokeMyToken(c); err != nil {
		t.Fatalf("RevokeMyToken: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (not found); body=%s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestTokenHandler_RevokeMyToken_InvalidID(t *testing.T) {
	e, _, h, userID := setupTokenTest(t)

	c, rec := newTokenContext(e, http.MethodDelete, "/api/tokens/abc", "", userID)
	c.SetPath("/api/tokens/:id")
	c.SetParamNames("id")
	c.SetParamValues("abc")

	if err := h.RevokeMyToken(c); err != nil {
		t.Fatalf("RevokeMyToken: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid id); body=%s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestTokenHandler_RevokeMyToken_Success(t *testing.T) {
	e, tokenSvc, h, userID := setupTokenTest(t)
	ctx := contextWithTimeout(t)

	_, tok, err := tokenSvc.CreateToken(ctx, userID, "revoke-me", "", []string{"read:query"}, noExpiry)
	if err != nil {
		t.Fatalf("seed token: %v", err)
	}

	c, rec := newTokenContext(e, http.MethodDelete, "/api/tokens/"+strconv.FormatInt(tok.ID, 10), "", userID)
	c.SetPath("/api/tokens/:id")
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(tok.ID, 10))

	if err := h.RevokeMyToken(c); err != nil {
		t.Fatalf("RevokeMyToken: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestTokenHandler_ListAllTokens_PaginationDefaults(t *testing.T) {
	e, tokenSvc, h, userID := setupTokenTest(t)
	ctx := contextWithTimeout(t)

	// Seed a few tokens across users.
	for i := 0; i < 3; i++ {
		if _, _, err := tokenSvc.CreateToken(ctx, userID, "tk"+strconv.Itoa(i), "", []string{"read:query"}, noExpiry); err != nil {
			t.Fatalf("seed token %d: %v", i, err)
		}
	}

	// No page/page_size query → defaults (page=1, page_size=20).
	c, rec := newTokenContext(e, http.MethodGet, "/api/admin/tokens", "", userID)
	c.SetPath("/api/admin/tokens")
	if err := h.ListAllTokens(c); err != nil {
		t.Fatalf("ListAllTokens: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	// Response should be a page envelope with total >= 3.
	body := rec.Body.String()
	if !strings.Contains(body, "total") {
		t.Errorf("expected page envelope with 'total', got: %s", body)
	}
}
