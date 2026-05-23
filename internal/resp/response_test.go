package resp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// helper to create an echo context for testing
func newTestContext() (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

func TestOK(t *testing.T) {
	c, rec := newTestContext()
	data := map[string]string{"key": "value"}

	err := OK(c, data)
	if err != nil {
		t.Fatalf("OK: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body SuccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Code != 0 {
		t.Errorf("code = %d, want 0", body.Code)
	}
	if body.Message != "ok" {
		t.Errorf("message = %q, want %q", body.Message, "ok")
	}
	if body.Data == nil {
		t.Error("data is nil")
	}
}

func TestOKWithMessage(t *testing.T) {
	c, rec := newTestContext()

	err := OKWithMessage(c, "custom message", nil)
	if err != nil {
		t.Fatalf("OKWithMessage: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body SuccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Message != "custom message" {
		t.Errorf("message = %q, want %q", body.Message, "custom message")
	}
}

func TestCreated(t *testing.T) {
	c, rec := newTestContext()
	data := map[string]string{"id": "123"}

	err := Created(c, data)
	if err != nil {
		t.Fatalf("Created: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body SuccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Message != "created" {
		t.Errorf("message = %q, want %q", body.Message, "created")
	}
}

func TestOKPage(t *testing.T) {
	c, rec := newTestContext()
	data := []string{"a", "b"}

	err := OKPage(c, data, 1, 10, 100)
	if err != nil {
		t.Fatalf("OKPage: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body PageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Page != 1 {
		t.Errorf("page = %d, want 1", body.Page)
	}
	if body.PageSize != 10 {
		t.Errorf("page_size = %d, want 10", body.PageSize)
	}
	if body.Total != 100 {
		t.Errorf("total = %d, want 100", body.Total)
	}
	if body.Message != "ok" {
		t.Errorf("message = %q, want %q", body.Message, "ok")
	}
}

func TestBadRequest(t *testing.T) {
	c, rec := newTestContext()

	err := BadRequest(c, "invalid input")
	if err != nil {
		t.Fatalf("BadRequest: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Code != 400 {
		t.Errorf("code = %d, want 400", body.Code)
	}
	if body.Message != "invalid input" {
		t.Errorf("message = %q, want %q", body.Message, "invalid input")
	}
}

func TestUnauthorized(t *testing.T) {
	c, rec := newTestContext()

	err := Unauthorized(c, "please login")
	if err != nil {
		t.Fatalf("Unauthorized: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Code != 401 {
		t.Errorf("code = %d, want 401", body.Code)
	}
}

func TestForbidden(t *testing.T) {
	c, rec := newTestContext()

	err := Forbidden(c, "access denied")
	if err != nil {
		t.Fatalf("Forbidden: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Code != 403 {
		t.Errorf("code = %d, want 403", body.Code)
	}
}

func TestNotFound(t *testing.T) {
	c, rec := newTestContext()

	err := NotFound(c, "resource not found")
	if err != nil {
		t.Fatalf("NotFound: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Code != 404 {
		t.Errorf("code = %d, want 404", body.Code)
	}
}

func TestInternalError(t *testing.T) {
	c, rec := newTestContext()

	err := InternalError(c, "something went wrong")
	if err != nil {
		t.Fatalf("InternalError: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Code != 500 {
		t.Errorf("code = %d, want 500", body.Code)
	}
	if body.Message != "something went wrong" {
		t.Errorf("message = %q, want %q", body.Message, "something went wrong")
	}
}
