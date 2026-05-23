package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestLogger(t *testing.T) {
	e := echo.New()
	handler := Logger()(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler(c)
	if err != nil {
		t.Fatalf("Logger middleware: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRecovery(t *testing.T) {
	e := echo.New()
	handler := Recovery()(func(c echo.Context) error {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Should recover from panic without propagating
	err := handler(c)
	// Recovery middleware returns error about the panic
	_ = err

	// Status should be 500 (internal server error) after panic recovery
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d after panic", rec.Code, http.StatusInternalServerError)
	}
}
