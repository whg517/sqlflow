package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/service"
)

// setupAuditTest creates a fresh Echo, DB, AuditService, and AuditHandler for testing.
func setupAuditTest(t *testing.T) (*echo.Echo, *service.AuditService, *AuditHandler) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert a user for the JOIN in List
	_, err = database.Exec("INSERT INTO users (username, password_hash, role) VALUES ('audituser', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	auditSvc := service.NewAuditService(database.DB, 0, 0)

	handler := NewAuditHandler(auditSvc)

	e := echo.New()
	return e, auditSvc, handler
}

// seedAuditLogs inserts n audit log records for testing.
func seedAuditLogs(t *testing.T, auditSvc *service.AuditService, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		auditSvc.Write(contextWithTimeout(t), service.AuditRecord{
			UserID:          1,
			Action:          "query_execute",
			DatasourceID:    int64(i % 3),
			Database:        fmt.Sprintf("db_%d", i%2),
			SQLContent:      fmt.Sprintf("SELECT * FROM table_%d", i),
			SQLSummary:      fmt.Sprintf("SELECT * FROM table_%d", i),
			ResultRows:      int64(i * 10),
			ExecutionTimeMs: int64(i * 100),
		})
	}
}

// decodeAuditResponse decodes the JSON response body into a map.
func decodeAuditResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return result
}

func TestAuditHandler_ListAuditLogs(t *testing.T) {
	e, auditSvc, h := setupAuditTest(t)

	// Seed 15 audit logs
	seedAuditLogs(t, auditSvc, 15)

	t.Run("default_pagination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodeAuditResponse(t, rec)
		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatalf("data is not an array; body=%s", rec.Body.String())
		}

		if len(data) != 15 {
			t.Errorf("expected 15 items (default page_size=50), got %d", len(data))
		}

		total, _ := result["total"].(float64)
		if total != 15 {
			t.Errorf("total = %v, want 15", total)
		}
	})

	t.Run("custom_pagination_page_1", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?page=1&page_size=5", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodeAuditResponse(t, rec)
		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatalf("data is not an array; body=%s", rec.Body.String())
		}

		if len(data) != 5 {
			t.Errorf("expected 5 items (page_size=5), got %d", len(data))
		}

		page, _ := result["page"].(float64)
		pageSize, _ := result["page_size"].(float64)
		total, _ := result["total"].(float64)

		if page != 1 {
			t.Errorf("page = %v, want 1", page)
		}
		if pageSize != 5 {
			t.Errorf("page_size = %v, want 5", pageSize)
		}
		if total != 15 {
			t.Errorf("total = %v, want 15", total)
		}
	})

	t.Run("custom_pagination_page_3", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?page=3&page_size=5", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatalf("data is not an array; body=%s", rec.Body.String())
		}

		if len(data) != 5 {
			t.Errorf("expected 5 items on page 3, got %d", len(data))
		}
	})

	t.Run("page_beyond_data_returns_empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?page=100&page_size=5", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatalf("data is not an array; body=%s", rec.Body.String())
		}

		if len(data) != 0 {
			t.Errorf("expected 0 items on page 100, got %d", len(data))
		}

		total, _ := result["total"].(float64)
		if total != 15 {
			t.Errorf("total = %v, want 15", total)
		}
	})

	t.Run("filter_by_action", func(t *testing.T) {
		// Add a different action type
		auditSvc.Write(contextWithTimeout(t), service.AuditRecord{
			UserID:     1,
			Action:     "export",
			SQLContent: "SELECT * FROM sensitive_table",
		})

		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?action=export", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		total, _ := result["total"].(float64)
		if total != 1 {
			t.Errorf("expected 1 export log, got total = %v", total)
		}
	})

	t.Run("filter_by_user_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?user_id=1", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		total, _ := result["total"].(float64)
		if total < 15 {
			t.Errorf("expected at least 15 for user_id=1, got total = %v", total)
		}
	})

	t.Run("filter_by_keyword", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?keyword=table_0", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		total, _ := result["total"].(float64)
		// "table_0" matches "table_0" and "table_10" etc.
		if total < 1 {
			t.Errorf("expected at least 1 match for keyword 'table_0', got total = %v", total)
		}
	})

	t.Run("empty_results", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?action=nonexistent_action", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		result := decodeAuditResponse(t, rec)
		total, _ := result["total"].(float64)
		if total != 0 {
			t.Errorf("expected 0 results for nonexistent action, got %v", total)
		}
	})

	t.Run("invalid_page_defaults_to_1", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?page=-1&page_size=-1", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		page, _ := result["page"].(float64)
		pageSize, _ := result["page_size"].(float64)

		if page != 1 {
			t.Errorf("page = %v, want 1 (default)", page)
		}
		if pageSize != 50 {
			t.Errorf("page_size = %v, want 50 (default)", pageSize)
		}
	})

	t.Run("non_numeric_page_defaults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?page=abc&page_size=xyz", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		page, _ := result["page"].(float64)
		pageSize, _ := result["page_size"].(float64)

		if page != 1 {
			t.Errorf("page = %v, want 1 (default for non-numeric)", page)
		}
		if pageSize != 50 {
			t.Errorf("page_size = %v, want 50 (default for non-numeric)", pageSize)
		}
	})
}

func TestAuditHandler_ListAuditLogs_EmptyDB(t *testing.T) {
	e, _, h := setupAuditTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/audit-logs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListAuditLogs(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeAuditResponse(t, rec)
	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array; body=%s", rec.Body.String())
	}

	if len(data) != 0 {
		t.Errorf("expected 0 items from empty DB, got %d", len(data))
	}

	total, _ := result["total"].(float64)
	if total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}

func TestAuditHandler_ListAuditLogs_FilterCombinations(t *testing.T) {
	e, auditSvc, h := setupAuditTest(t)

	// Seed records with distinct attributes for filter testing
	records := []service.AuditRecord{
		{UserID: 1, Action: "query_execute", DatasourceID: 1, SQLContent: "SELECT * FROM orders"},
		{UserID: 1, Action: "export", DatasourceID: 2, SQLContent: "SELECT * FROM users"},
		{UserID: 1, Action: "ticket_create", DatasourceID: 1, SQLContent: "UPDATE orders SET status=1"},
	}
	for _, r := range records {
		auditSvc.Write(contextWithTimeout(t), r)
	}

	tests := []struct {
		name       string
		query      string
		wantTotal  float64
	}{
		{"filter_by_datasource_id", "datasource_id=2", 1},
		{"filter_by_action_and_keyword", "action=query_execute&keyword=orders", 1},
		{"filter_nonexistent_returns_zero", "action=nonexistent", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/audit-logs?"+tt.query, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := h.ListAuditLogs(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			result := decodeAuditResponse(t, rec)
			total, _ := result["total"].(float64)
			if total != tt.wantTotal {
				t.Errorf("total = %v, want %v", total, tt.wantTotal)
			}
		})
	}
}
