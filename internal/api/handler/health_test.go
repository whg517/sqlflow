package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/pkg/metrics"
)

func TestHealthHandler_Health(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	h := NewHealthHandler(database.DB)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Health(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
	if resp.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", resp.Version, "1.0.0")
	}
	if resp.DB != "ok" {
		t.Errorf("db = %q, want %q", resp.DB, "ok")
	}
	if resp.Uptime < 0 {
		t.Errorf("uptime = %d, want >= 0", resp.Uptime)
	}
}

func TestHealthHandler_Health_DBError(t *testing.T) {
	// Close the DB to simulate a connection failure
	tmpDir := t.TempDir()
	database, err := db.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.Close() // close to cause Ping failure

	h := NewHealthHandler(database.DB)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Health(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.DB != "error" {
		t.Errorf("db = %q, want %q", resp.DB, "error")
	}
	if resp.Status != "ok" {
		t.Errorf("overall status = %q, want %q (service still reports ok even when DB is down)", resp.Status, "ok")
	}
}

func TestHealthHandler_Metrics(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	h := NewHealthHandler(database.DB)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Metrics(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Prometheus metrics format should contain some standard metrics
	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("metrics response should not be empty")
	}
	// Prometheus text format typically ends with a newline
	if body[len(body)-1] != '\n' {
		t.Error("metrics response should end with newline")
	}
}

func TestHealthHandler_NewHealthHandler(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	h := NewHealthHandler(database.DB)

	if h == nil {
		t.Fatal("NewHealthHandler returned nil")
	}
	if h.version != "1.0.0" {
		t.Errorf("version = %q, want %q", h.version, "1.0.0")
	}
	if h.started.IsZero() {
		t.Error("started time should not be zero")
	}
	if time.Since(h.started) > time.Second {
		t.Error("started time should be recent")
	}
}

// Suppress unused import
var _ = metrics.PromhttpHandler
