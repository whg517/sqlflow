package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/service"
)

func setupDashboardTest(t *testing.T) (*echo.Echo, *DashboardHandler, *db.DB) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	svc := service.NewDashboardService(database.DB)
	handler := NewDashboardHandler(svc)
	e := echo.New()

	return e, handler, database
}

func TestDashboardHandler_GetStats(t *testing.T) {
	e, h, database := setupDashboardTest(t)

	// Seed some data
	ctx := contextWithTimeout(t)
	database.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES ('testuser', 'hash', 'developer')`,
	)
	database.ExecContext(ctx,
		`INSERT INTO datasources (name, type, host, port, status) VALUES ('ds1', 'mysql', 'localhost', 3306, 'active')`,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/stats", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.GetStats(c); err != nil {
		t.Fatalf("GetStats handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if code, ok := result["code"].(float64); !ok || code != 0 {
		t.Errorf("code = %v, want 0", result["code"])
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data missing or not an object: %v", result["data"])
	}

	if v, ok := data["total_users"].(float64); !ok || v != 1 {
		t.Errorf("total_users = %v, want 1", data["total_users"])
	}
	if v, ok := data["active_datasources"].(float64); !ok || v != 1 {
		t.Errorf("active_datasources = %v, want 1", data["active_datasources"])
	}
	if v, ok := data["pending_tickets"].(float64); !ok || v != 0 {
		t.Errorf("pending_tickets = %v, want 0", data["pending_tickets"])
	}
	if v, ok := data["recent_queries_7d"].(float64); !ok || v != 0 {
		t.Errorf("recent_queries_7d = %v, want 0", data["recent_queries_7d"])
	}
}
