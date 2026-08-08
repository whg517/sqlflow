package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/model"
	"github.com/whg517/sqlflow/internal/platform/httpx"
	"github.com/whg517/sqlflow/internal/testutil"
)

func setupPerformanceTest(t *testing.T) (*echo.Echo, *HistoryService, *PerformanceHandler) {
	t.Helper()
	d := testutil.NewDB(t)
	histSvc := NewHistoryService(d)
	h := NewPerformanceHandler(histSvc)
	return echo.New(), histSvc, h
}

func TestPerformanceHandler_ListSlowQueries_Empty(t *testing.T) {
	e, _, h := setupPerformanceTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/query/performance/slow", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	asSeededOwner(c)
	if err := h.ListSlowQueries(c); err != nil {
		t.Fatalf("ListSlowQueries: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	// Response is a page envelope; data must be a non-nil (empty) array.
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty list, got %d items", len(data))
	}
}

func TestPerformanceHandler_ListSlowQueries_DefaultThreshold(t *testing.T) {
	e, histSvc, h := setupPerformanceTest(t)
	ctx := context.Background()

	// Seed one slow (50ms) and one fast (1ms) query.
	for _, execMs := range []int64{50, 1} {
		if _, err := histSvc.CreateHistory(ctx, &model.QueryHistory{
			UserID:        1,
			DatasourceID:  1,
			SQLContent:    "SELECT 1",
			ExecutionTime: execMs,
		}); err != nil {
			t.Fatalf("seed history: %v", err)
		}
	}

	// No threshold query param → default 1000ms → nothing qualifies as slow.
	req := httptest.NewRequest(http.MethodGet, "/api/query/performance/slow", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	asSeededOwner(c)
	if err := h.ListSlowQueries(c); err != nil {
		t.Fatalf("ListSlowQueries: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	data, _ := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("default threshold 1000ms: expected 0 slow, got %d", len(data))
	}

	// threshold=10 → only the 50ms query qualifies.
	req = httptest.NewRequest(http.MethodGet, "/api/query/performance/slow?threshold=10", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	asSeededOwner(c)
	if err := h.ListSlowQueries(c); err != nil {
		t.Fatalf("ListSlowQueries(threshold=10): %v", err)
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	data, _ = resp["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("threshold=10: expected 1 slow query, got %d", len(data))
	}
}

func TestPerformanceHandler_GetPerformanceStats_DefaultDays(t *testing.T) {
	e, _, h := setupPerformanceTest(t)

	// No days param → default 7.
	req := httptest.NewRequest(http.MethodGet, "/api/query/performance/stats", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	asSeededOwner(c)
	if err := h.GetPerformanceStats(c); err != nil {
		t.Fatalf("GetPerformanceStats: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["data"] == nil {
		t.Error("expected non-nil stats data")
	}
}

func TestPerformanceHandler_GetPerformanceStats_CustomDays(t *testing.T) {
	e, _, h := setupPerformanceTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/query/performance/stats?days=30", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	asSeededOwner(c)
	if err := h.GetPerformanceStats(c); err != nil {
		t.Fatalf("GetPerformanceStats: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// asSeededOwner authenticates the request as the user these fixtures record
// their history under.
//
// The slow-query read is scoped to the caller now, so a context with no
// identity reads zero rows — and every assertion here about thresholds and
// windows would pass for the wrong reason.
func asSeededOwner(c echo.Context) {
	c.Set(httpx.ContextKeyUserID, int64(1))
	c.Set(httpx.ContextKeyUsername, "perf_owner")
	c.Set(httpx.ContextKeyRole, "developer")
}
