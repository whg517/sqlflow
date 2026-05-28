package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestSafeContextExtraction_NilContext verifies that getContextUserID/Username/Role
// return zero values when context has no auth values set (no panic).
func TestSafeContextExtraction_NilContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// No Set() calls — values are nil
	if id := getContextUserID(c); id != 0 {
		t.Errorf("expected 0, got %d", id)
	}
	if name := getContextUsername(c); name != "" {
		t.Errorf("expected empty, got %q", name)
	}
	if role := getContextRole(c); role != "" {
		t.Errorf("expected empty, got %q", role)
	}
}

// TestSafeContextExtraction_WrongType verifies no panic when wrong type is stored.
func TestSafeContextExtraction_WrongType(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Set wrong types
	c.Set("user_id", "not-an-int64")
	c.Set("username", 12345)
	c.Set("role", 42)

	if id := getContextUserID(c); id != 0 {
		t.Errorf("expected 0 for wrong type, got %d", id)
	}
	if name := getContextUsername(c); name != "" {
		t.Errorf("expected empty for wrong type, got %q", name)
	}
	if role := getContextRole(c); role != "" {
		t.Errorf("expected empty for wrong type, got %q", role)
	}
}

// TestSafeContextExtraction_CorrectValues verifies correct values are extracted.
func TestSafeContextExtraction_CorrectValues(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("user_id", int64(42))
	c.Set("username", "testuser")
	c.Set("role", "admin")

	if id := getContextUserID(c); id != 42 {
		t.Errorf("expected 42, got %d", id)
	}
	if name := getContextUsername(c); name != "testuser" {
		t.Errorf("expected testuser, got %q", name)
	}
	if role := getContextRole(c); role != "admin" {
		t.Errorf("expected admin, got %q", role)
	}
}

// TestRequireAuth_MissingValues verifies requireAuth returns error when values missing.
func TestRequireAuth_MissingValues(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_, _, _, err := requireAuth(c)
	if err == nil {
		t.Error("expected error when auth values missing")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Errorf("expected HTTPError, got %T", err)
	} else if he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", he.Code)
	}
}

// TestRequireAuth_ValidValues verifies requireAuth succeeds with valid values.
func TestRequireAuth_ValidValues(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("user_id", int64(1))
	c.Set("username", "admin")
	c.Set("role", "admin")

	userID, username, role, err := requireAuth(c)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if userID != 1 || username != "admin" || role != "admin" {
		t.Errorf("got userID=%d username=%q role=%q", userID, username, role)
	}
}

// TestNoPanicWithExpiredContext verifies that handlers don't panic with expired context.
// This is the core regression test for the export timeout panic issue.
func TestNoPanicWithExpiredContext(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked with expired context: %v", r)
		}
	}()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Simulate expired/nil context — no Set() calls at all
	// Previously, this would panic on c.Get(...).(int64)
	_ = getContextUserID(c)
	_ = getContextUsername(c)
	_ = getContextRole(c)
}
