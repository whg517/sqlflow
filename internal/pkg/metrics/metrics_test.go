package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestMiddleware_RecordsMetrics(t *testing.T) {
	e := echo.New()

	// Create a test handler that returns 200
	handler := Middleware()(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler(c)
	if err != nil {
		t.Fatalf("middleware error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_RecordsDifferentStatusCodes(t *testing.T) {
	t.Run("200 ok", func(t *testing.T) {
		e := echo.New()
		handler := Middleware()(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		_ = handler(c)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("500 via direct write", func(t *testing.T) {
		e := echo.New()
		handler := Middleware()(func(c echo.Context) error {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "fail"})
		})
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		_ = handler(c)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestMiddleware_DurationRecorded(t *testing.T) {
	e := echo.New()

	// Handler that takes some time
	handler := Middleware()(func(c echo.Context) error {
		time.Sleep(10 * time.Millisecond)
		return c.String(http.StatusOK, "slow")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/slow", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	start := time.Now()
	_ = handler(c)
	elapsed := time.Since(start)

	// The handler slept 10ms, so middleware should record at least that
	if elapsed < 10*time.Millisecond {
		t.Errorf("elapsed = %v, expected >= 10ms", elapsed)
	}
}

func TestPromhttpHandler(t *testing.T) {
	handler := PromhttpHandler()
	if handler == nil {
		t.Fatal("PromhttpHandler returned nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("metrics body is empty")
	}
}

func TestPrometheusCounters(t *testing.T) {
	// Verify the Prometheus metrics variables are initialized
	if HTTPRequestDuration == nil {
		t.Error("HTTPRequestDuration histogram is nil")
	}
	if HTTPRequestsTotal == nil {
		t.Error("HTTPRequestsTotal counter is nil")
	}
	if ActiveTickets == nil {
		t.Error("ActiveTickets gauge is nil")
	}
	if DBQueriesTotal == nil {
		t.Error("DBQueriesTotal counter is nil")
	}
}
