package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/testutil"
)

// pingDriver answers Ping with whatever it was told to.
type pingDriver struct {
	driver.Driver
	err error
}

func (p *pingDriver) Ping(context.Context) error { return p.err }
func (p *pingDriver) Close() error               { return nil }
func (p *pingDriver) Type() string               { return "stub" }

func readyzChecks(t *testing.T, h *HealthHandler) (int, map[string]string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	if err := h.Readyz(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var body HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return rec.Code, body.Checks
}

// TestReadyzReportsDatasourcesItActuallyPinged pins the difference between
// checking and counting.
//
// The datasource entry was produced by walking connection maps that no
// production code fills, so it was permanently "ok", and the driver pool entry
// only counted map entries without touching any of them. Every governed
// database could be unreachable and the probe still answered
// "datasources": "ok", "driver_pool": "ok (3 connections)".
func TestReadyzReportsDatasourcesItActuallyPinged(t *testing.T) {
	database := testutil.NewDB(t)
	h := NewHealthHandler(database.DB)

	pm := driver.NewPoolManager()
	pm.InjectForTest(1, &pingDriver{})
	pm.InjectForTest(2, &pingDriver{err: errors.New("connection refused")})
	h.SetPoolManager(pm)

	code, checks := readyzChecks(t, h)

	status, ok := checks["datasource_2"]
	if !ok {
		t.Fatalf("no per-datasource entry; checks = %v", checks)
	}
	if !strings.Contains(status, "connection refused") {
		t.Errorf("datasource_2 = %q, want the ping failure", status)
	}
	if got := checks["datasource_1"]; got != "ok" {
		t.Errorf("datasource_1 = %q, want ok", got)
	}

	// A governed database being down is not this instance being unready: it can
	// still serve tickets, audit and administration. Failing readiness here
	// would pull the whole instance out of rotation over someone else's outage.
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200 — an unreachable target must not fail readiness", code)
	}
}

// TestReadyzFailsWhenThePlatformStoreIsDown keeps the one dependency that does
// gate readiness.
func TestReadyzFailsWhenThePlatformStoreIsDown(t *testing.T) {
	database := testutil.NewDB(t)
	h := NewHealthHandler(database.DB)
	_ = database.DB.Close()

	code, checks := readyzChecks(t, h)

	if code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when the platform store is unreachable", code)
	}
	if _, ok := checks["platform_db"]; !ok {
		t.Errorf("no platform_db entry; checks = %v — the label used to say sqlite", checks)
	}
}

// TestReadyzWithNoPooledConnections covers a freshly started process, which has
// opened nothing yet and is nonetheless ready.
func TestReadyzWithNoPooledConnections(t *testing.T) {
	database := testutil.NewDB(t)
	h := NewHealthHandler(database.DB)
	h.SetPoolManager(driver.NewPoolManager())

	code, checks := readyzChecks(t, h)

	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	for name := range checks {
		if strings.HasPrefix(name, "datasource_") {
			t.Errorf("reported %s with nothing pooled", name)
		}
	}
}
