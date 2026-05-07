package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/service"
)

// setupEnforcer creates a Casbin enforcer with a proper RBAC model and test policies.
func setupEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()

	m, err := model.NewModelFromString(`
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && r.dom == p.dom && r.obj == p.obj && r.act == p.act
`)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	e, err := casbin.NewEnforcer(m)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	// role "developer" can "select" on datasource "1" table "users"
	_, _ = e.AddPolicy("developer", "1", "users", "select")
	// role "admin" can "select" on datasource "1" table "users"
	_, _ = e.AddPolicy("admin", "1", "users", "select")

	return e
}

func okHandler(c echo.Context) error {
	return c.String(http.StatusOK, "ok")
}

func TestPermission_FromPathParams(t *testing.T) {
	enforcer := setupEnforcer(t)

	tests := []struct {
		name        string
		paramNames  []string
		paramValues []string
		role        string
		wantStatus  int
	}{
		{
			name:        "path param datasource_id + table matches policy",
			paramNames:  []string{"datasource_id", "table"},
			paramValues: []string{"1", "users"},
			role:        "developer",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "path param id for datasource + table",
			paramNames:  []string{"id", "table"},
			paramValues: []string{"1", "users"},
			role:        "developer",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "path param id=999 denied",
			paramNames:  []string{"id", "table"},
			paramValues: []string{"999", "users"},
			role:        "developer",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "only datasource param, missing table -> denied",
			paramNames:  []string{"id"},
			paramValues: []string{"1"},
			role:        "developer",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "admin with matching policy passes",
			paramNames:  []string{"id", "table"},
			paramValues: []string{"1", "users"},
			role:        "admin",
			wantStatus:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(ContextKeyRole, tt.role)
			c.SetParamNames(tt.paramNames...)
			c.SetParamValues(tt.paramValues...)

			handler := Permission(enforcer, "select")(okHandler)
			err := handler(c)

			if err != nil {
				he, ok := err.(*echo.HTTPError)
				if !ok {
					t.Fatalf("unexpected error type: %v", err)
				}
				if he.Code != tt.wantStatus {
					t.Errorf("got status %d, want %d", he.Code, tt.wantStatus)
				}
			} else if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestPermission_FromJSONBody(t *testing.T) {
	e := echo.New()
	enforcer := setupEnforcer(t)

	tests := []struct {
		name       string
		body       map[string]interface{}
		role       string
		wantStatus int
	}{
		{
			name: "body datasource_id and table_name match",
			body: map[string]interface{}{
				"datasource_id": float64(1),
				"table_name":    "users",
			},
			role:       "developer",
			wantStatus: http.StatusOK,
		},
		{
			name: "body datasource_id mismatch",
			body: map[string]interface{}{
				"datasource_id": float64(999),
				"table_name":    "users",
			},
			role:       "developer",
			wantStatus: http.StatusForbidden,
		},
		{
			name: "body database as fallback for table",
			body: map[string]interface{}{
				"datasource_id": float64(1),
				"database":      "users",
			},
			role:       "developer",
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty body falls back to empty dom/obj",
			body:       map[string]interface{}{},
			role:       "developer",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/query/execute", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(ContextKeyRole, tt.role)

			handler := Permission(enforcer, "select")(okHandler)
			err := handler(c)

			if err != nil {
				he, ok := err.(*echo.HTTPError)
				if !ok {
					t.Fatalf("unexpected error type: %v", err)
				}
				if he.Code != tt.wantStatus {
					t.Errorf("got status %d, want %d", he.Code, tt.wantStatus)
				}
			} else if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestPermission_FromQueryParams(t *testing.T) {
	e := echo.New()
	enforcer := setupEnforcer(t)

	tests := []struct {
		name       string
		query      string
		role       string
		wantStatus int
	}{
		{
			name:       "query param matches policy",
			query:      "?datasource=1&table=users",
			role:       "developer",
			wantStatus: http.StatusOK,
		},
		{
			name:       "query param no match",
			query:      "?datasource=999&table=users",
			role:       "developer",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "no params denied",
			query:      "",
			role:       "developer",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/test"+tt.query, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(ContextKeyRole, tt.role)

			handler := Permission(enforcer, "select")(okHandler)
			err := handler(c)

			if err != nil {
				he, ok := err.(*echo.HTTPError)
				if !ok {
					t.Fatalf("unexpected error type: %v", err)
				}
				if he.Code != tt.wantStatus {
					t.Errorf("got status %d, want %d", he.Code, tt.wantStatus)
				}
			} else if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestPermission_BodyRestoredForDownstream(t *testing.T) {
	e := echo.New()
	enforcer := setupEnforcer(t)

	bodyMap := map[string]interface{}{
		"datasource_id": float64(1),
		"table_name":    "users",
		"sql":           "SELECT 1",
	}
	bodyBytes, _ := json.Marshal(bodyMap)
	req := httptest.NewRequest(http.MethodPost, "/api/query/execute", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextKeyRole, "developer")

	var downstreamBody map[string]interface{}
	handler := Permission(enforcer, "select")(func(c echo.Context) error {
		if err := json.NewDecoder(c.Request().Body).Decode(&downstreamBody); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "body read failed: "+err.Error())
		}
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if downstreamBody["sql"] != "SELECT 1" {
		t.Errorf("downstream body not restored correctly, got %v", downstreamBody)
	}
}

func TestPeekBody_CachesResult(t *testing.T) {
	e := echo.New()
	bodyMap := map[string]interface{}{"datasource_id": float64(42)}
	bodyBytes, _ := json.Marshal(bodyMap)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	result1 := peekBody(c)
	if result1 == nil {
		t.Fatal("expected non-nil body")
	}
	if id := bodyField(result1, "datasource_id"); id != "42" {
		t.Errorf("expected datasource_id=42, got %s", id)
	}

	// Second call returns cached result
	result2 := peekBody(c)
	if result2 == nil {
		t.Fatal("expected cached non-nil body")
	}
	if bodyField(result1, "datasource_id") != bodyField(result2, "datasource_id") {
		t.Error("expected same cached values")
	}
}

func TestPeekBody_NonJSONReturnsNil(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if result := peekBody(c); result != nil {
		t.Error("expected nil for non-JSON content type")
	}
}

func TestPeekBody_EmptyBodyReturnsNil(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if result := peekBody(c); result != nil {
		t.Error("expected nil for empty body")
	}
}

func TestBodyField(t *testing.T) {
	tests := []struct {
		name string
		body map[string]interface{}
		key  string
		want string
	}{
		{
			name: "string value",
			body: map[string]interface{}{"key": "value"},
			key:  "key",
			want: "value",
		},
		{
			name: "float64 integer value",
			body: map[string]interface{}{"key": float64(42)},
			key:  "key",
			want: "42",
		},
		{
			name: "float64 decimal value",
			body: map[string]interface{}{"key": float64(3.14)},
			key:  "key",
			want: "3.14",
		},
		{
			name: "missing key",
			body: map[string]interface{}{"other": "value"},
			key:  "key",
			want: "",
		},
		{
			name: "nil value",
			body: map[string]interface{}{"key": nil},
			key:  "key",
			want: "",
		},
		{
			name: "bool value returns empty",
			body: map[string]interface{}{"key": true},
			key:  "key",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bodyField(tt.body, tt.key)
			if got != tt.want {
				t.Errorf("bodyField() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// JWT middleware tests
// ---------------------------------------------------------------------------

// newTestAuthSvc creates a real AuthService backed by a temp SQLite DB.
func newTestAuthSvc(t *testing.T) *service.AuthService {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return service.NewAuthService(database.DB, "test-secret-key", 1*time.Hour)
}

// generateToken creates a signed JWT with the given claims and secret.
func generateToken(t *testing.T, secret []byte, userID int64, username, role string, expiresAt, issuedAt time.Time) string {
	t.Helper()
	claims := &service.Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

func TestJWT_MissingAuthHeader(t *testing.T) {
	authSvc := newTestAuthSvc(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWT(authSvc)(okHandler)
	err := handler(c)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWT_InvalidFormat(t *testing.T) {
	authSvc := newTestAuthSvc(t)

	tests := []struct {
		name   string
		header string
	}{
		{"no bearer prefix", "Token abc123"},
		{"missing token part", "Bearer"},
		{"empty bearer", "Bearer "},
		{"basic auth", "Basic dXNlcjpwYXNz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", tt.header)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			handler := JWT(authSvc)(okHandler)
			err := handler(c)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got status %d, want %d for header %q", rec.Code, http.StatusUnauthorized, tt.header)
			}
		})
	}
}

func TestJWT_InvalidToken(t *testing.T) {
	authSvc := newTestAuthSvc(t)

	tests := []struct {
		name  string
		token string
	}{
		{"garbage string", "not.a.real.token"},
		{"empty string", ""},
		{"random base64", "dGhpcyBpcyByYW5kb20"},
		{"signed with wrong secret", generateToken(t, []byte("wrong-secret"), 1, "admin", "admin", time.Now().Add(time.Hour), time.Now())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			handler := JWT(authSvc)(okHandler)
			err := handler(c)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got status %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestJWT_ExpiredToken(t *testing.T) {
	authSvc := newTestAuthSvc(t)

	token := generateToken(t, []byte("test-secret-key"), 1, "admin", "admin",
		time.Now().Add(-time.Hour), // expired 1 hour ago
		time.Now().Add(-2*time.Hour),
	)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWT(authSvc)(okHandler)
	err := handler(c)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWT_ValidToken(t *testing.T) {
	authSvc := newTestAuthSvc(t)

	token := generateToken(t, []byte("test-secret-key"), 42, "testuser", "admin",
		time.Now().Add(time.Hour),
		time.Now(),
	)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// handler that captures context values
	var gotUserID, gotUsername, gotRole interface{}
	handler := JWT(authSvc)(func(c echo.Context) error {
		gotUserID = c.Get(ContextKeyUserID)
		gotUsername = c.Get(ContextKeyUsername)
		gotRole = c.Get(ContextKeyRole)
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if gotUserID != int64(42) {
		t.Errorf("user_id = %v, want 42", gotUserID)
	}
	if gotUsername != "testuser" {
		t.Errorf("username = %v, want testuser", gotUsername)
	}
	if gotRole != "admin" {
		t.Errorf("role = %v, want admin", gotRole)
	}
}

func TestJWT_BearerCaseInsensitive(t *testing.T) {
	authSvc := newTestAuthSvc(t)

	token := generateToken(t, []byte("test-secret-key"), 1, "user", "developer",
		time.Now().Add(time.Hour), time.Now(),
	)

	for _, prefix := range []string{"Bearer", "bearer", "BEARER"} {
		t.Run(prefix, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", prefix+" "+token)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			handler := JWT(authSvc)(okHandler)
			err := handler(c)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("got status %d, want %d for prefix %q", rec.Code, http.StatusOK, prefix)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Admin middleware tests
// ---------------------------------------------------------------------------

func TestAdmin_AllowsAdminRole(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextKeyRole, "admin")

	handler := Admin()(okHandler)
	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAdmin_RejectsNonAdminRole(t *testing.T) {
	tests := []struct {
		name string
		role string
	}{
		{"developer role", "developer"},
		{"dba role", "dba"},
		{"empty role", ""},
		{"unknown role", "superuser"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(ContextKeyRole, tt.role)

			handler := Admin()(okHandler)
			err := handler(c)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("got status %d, want %d for role %q", rec.Code, http.StatusForbidden, tt.role)
			}
		})
	}
}

func TestAdmin_NoRoleSet(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	// deliberately do NOT set ContextKeyRole

	handler := Admin()(okHandler)
	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAdmin_RoleSetToNonString(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextKeyRole, 12345) // int, not string

	handler := Admin()(okHandler)
	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusForbidden)
	}
}
