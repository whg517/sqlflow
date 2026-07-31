package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRegisterCoverageRoutesWithoutDatabaseExposesDisabledStatus(t *testing.T) {
	e := echo.New()
	passthrough := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(c)
		}
	}

	RegisterCoverageRoutes(e, passthrough, passthrough, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/coverage/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Enabled bool   `json:"enabled"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Enabled {
		t.Fatal("enabled = true, want false")
	}
	if body.Reason == "" {
		t.Fatal("reason is empty")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/coverage/sqlflow/summary", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled data route status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
