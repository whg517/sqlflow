package testutil

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/whg517/sqlflow/internal/platform/httpx"
)

// ContextWithTimeout returns a bounded context for a test, cancelled on cleanup.
//
// The budget is deliberately generous: under -race with a cold bcrypt cost a
// single test that creates several users can take seconds, and a tight deadline
// turns that into a flake rather than a signal.
func ContextWithTimeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// CreateUser inserts a user row directly and returns its ID.
//
// It writes raw SQL rather than going through the auth service so that any
// package can use it: routing user creation through a service would make this
// helper depend on a domain, and domains' own tests could then not import it.
// Tests that need a real password hash or a token must use the auth service.
func CreateUser(t *testing.T, conn *sql.DB, username string) int64 {
	t.Helper()
	if _, err := conn.Exec(
		`INSERT INTO users (username, password_hash, role) VALUES (?, 'hashed', 'developer')`,
		username,
	); err != nil {
		t.Fatalf("testutil: create user %q: %v", username, err)
	}
	var id int64
	if err := conn.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id); err != nil {
		t.Fatalf("testutil: read id of user %q: %v", username, err)
	}
	return id
}

// SetContextUser populates the identity the auth middleware would have set, so
// a handler can be exercised without running the middleware chain.
func SetContextUser(c echo.Context, userID int64, username, role string) {
	c.Set(httpx.ContextKeyUserID, userID)
	c.Set(httpx.ContextKeyUsername, username)
	c.Set(httpx.ContextKeyRole, role)
}

// EncryptionKey is a fixed 32-byte AES-256 key for tests.
//
// It is a constant so that a fixture encrypted in one package can be decrypted
// in another; generating one per test would make cross-package fixtures
// impossible and buys nothing, since no test asserts on key secrecy.
const EncryptionKey = "0123456789abcdef0123456789abcdef"

// DecodeJSON unmarshals a recorded response body into a generic map.
//
// It reports the raw body on failure: a handler that returned an error page or
// an empty body otherwise shows up as an opaque unmarshal error.
func DecodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("testutil: decode response: %v; body=%s", err, rec.Body.String())
	}
	return result
}

// SeedUser inserts a user with an explicit role and returns its ID.
//
// CreateUser always writes the developer role; tests that exercise role-based
// behavior need to pick one.
func SeedUser(t *testing.T, conn *sql.DB, username, role string) int64 {
	t.Helper()
	result, err := conn.Exec(
		`INSERT INTO users (username, password_hash, role, created_at, updated_at)
		 VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		username, "$2a$10$testhash", role,
	)
	if err != nil {
		t.Fatalf("testutil: seed user %s: %v", username, err)
	}
	id, _ := result.LastInsertId()
	return id
}

// SeedDatasource inserts an active MySQL datasource and returns its ID.
//
// The connection details are placeholders: this exists for tests that need a
// datasource row to satisfy a foreign key or a permission lookup, not for tests
// that actually connect.
func SeedDatasource(t *testing.T, conn *sql.DB, name string) int64 {
	t.Helper()
	result, err := conn.Exec(
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, status, created_at, updated_at)
		 VALUES (?, 'mysql', 'localhost', 3306, 'root', '', 'active', datetime('now'), datetime('now'))`,
		name,
	)
	if err != nil {
		t.Fatalf("testutil: seed datasource %s: %v", name, err)
	}
	id, _ := result.LastInsertId()
	return id
}
