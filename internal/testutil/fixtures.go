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
	var id int64
	if err := conn.QueryRow(
		`INSERT INTO users (username, password_hash, role) VALUES ($1, 'hashed', 'developer') RETURNING id`,
		username,
	).Scan(&id); err != nil {
		t.Fatalf("testutil: create user %q: %v", username, err)
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
	// RETURNING rather than LastInsertId: PostgreSQL's driver does not implement
	// the latter, since the value only exists for the sequence the row used.
	var id int64
	if err := conn.QueryRow(
		`INSERT INTO users (username, password_hash, role, created_at, updated_at)
		 VALUES ($1, $2, $3, now(), now()) RETURNING id`,
		username, "$2a$10$testhash", role,
	).Scan(&id); err != nil {
		t.Fatalf("testutil: seed user %s: %v", username, err)
	}
	return id
}

// DatasourceDatabase is the database every seeded datasource is configured for.
//
// It is named rather than inlined because the platform derives a query's or a
// ticket's scope from this column and refuses a request naming anything else. A
// fixture with no database models a datasource nothing can be executed against,
// which is why the seeders below all set one; a test that passes a different
// name is testing the refusal, not the workflow.
const DatasourceDatabase = "appdb"

// SeedDatasource inserts an active MySQL datasource and returns its ID.
//
// The connection details are placeholders: this exists for tests that need a
// datasource row to satisfy a foreign key or a permission lookup, not for tests
// that actually connect.
func SeedDatasource(t *testing.T, conn *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	if err := conn.QueryRow(
		`INSERT INTO datasources (name, type, host, port, username, password_encrypted, database, status, created_at, updated_at)
		 VALUES ($1, 'mysql', 'localhost', 3306, 'root', '', $2, 'active', now(), now()) RETURNING id`,
		name, DatasourceDatabase,
	).Scan(&id); err != nil {
		t.Fatalf("testutil: seed datasource %s: %v", name, err)
	}
	return id
}
