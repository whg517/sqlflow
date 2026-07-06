package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/service"
	"github.com/whg517/sqlflow/internal/testutil"
)

func setupWebVitalsTest(t *testing.T) (*echo.Echo, *service.WebVitalsService, *WebVitalsHandler) {
	t.Helper()
	d := testutil.NewDB(t)
	svc := service.NewWebVitalsService(d)
	h := NewWebVitalsHandler(svc)
	// Loosen the rate limiter for tests so the multi-request cases don't trip it.
	h.limiter = newPerIPLimiter(1000, 0)
	return echo.New(), svc, h
}

func TestWebVitalsHandler_RecordVitals_Success(t *testing.T) {
	e, _, h := setupWebVitalsTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/metrics/web-vitals",
		strings.NewReader(`{"name":"LCP","value":2.5,"rating":"good","path":"/","navigationType":"navigate"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.RecordVitals(c); err != nil {
		t.Fatalf("RecordVitals: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["message"] != "上报成功" {
		t.Errorf("message = %v, want 上报成功", resp["message"])
	}
}

func TestWebVitalsHandler_RecordVitals_ValidationErrors(t *testing.T) {
	e, _, h := setupWebVitalsTest(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"invalid metric name", `{"name":"TTFB","value":1.0}`, http.StatusBadRequest},
		{"negative value", `{"name":"LCP","value":-1}`, http.StatusBadRequest},
		{"invalid rating", `{"name":"CLS","value":0.1,"rating":"excellent"}`, http.StatusBadRequest},
		{"malformed JSON", `{not-json`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/metrics/web-vitals", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if err := h.RecordVitals(c); err != nil {
				t.Fatalf("RecordVitals returned error: %v", err)
			}
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestWebVitalsHandler_RecordVitals_EmptyRatingAllowed(t *testing.T) {
	e, _, h := setupWebVitalsTest(t)
	// An empty rating must be accepted (only non-empty invalid ratings are rejected).
	req := httptest.NewRequest(http.MethodPost, "/api/metrics/web-vitals",
		strings.NewReader(`{"name":"INP","value":120,"rating":"","path":"/q"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.RecordVitals(c); err != nil {
		t.Fatalf("RecordVitals: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (empty rating allowed); body=%s",
			rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestPerIPLimiter(t *testing.T) {
	t.Run("allows up to max then blocks", func(t *testing.T) {
		l := newPerIPLimiter(3, time.Minute)
		for i := 0; i < 3; i++ {
			if !l.allow("1.2.3.4") {
				t.Fatalf("request %d should be allowed", i)
			}
		}
		if l.allow("1.2.3.4") {
			t.Error("4th request should be blocked")
		}
	})

	t.Run("isolates per IP", func(t *testing.T) {
		l := newPerIPLimiter(1, time.Minute)
		if !l.allow("a") {
			t.Error("first from a should pass")
		}
		if l.allow("a") {
			t.Error("second from a should be blocked")
		}
		if !l.allow("b") {
			t.Error("first from b should pass (independent of a)")
		}
	})
}
