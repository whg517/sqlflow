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

// newAuditFTSHandlerTest creates a fully initialized test setup with FTS5 support.
func newAuditFTSHandlerTest(t *testing.T) (*echo.Echo, *service.AuditService, *AuditHandler) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert a user for the JOIN.
	_, err = database.Exec("INSERT INTO users (username, password_hash, role) VALUES ('alice', 'hash', 'developer')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = database.Exec("INSERT INTO users (username, password_hash, role) VALUES ('bob', 'hash', 'admin')")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	auditSvc := service.NewAuditService(database.DB, 0, 0)
	handler := NewAuditHandler(auditSvc)

	e := echo.New()
	return e, auditSvc, handler
}

// seedFTSAuditLogs inserts audit logs with FTS-indexed content.
func seedFTSAuditLogs(t *testing.T, auditSvc *service.AuditService, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		auditSvc.Write(contextWithTimeout(t), service.AuditRecord{
			UserID:       int64(i%2 + 1), // alternating user 1 and 2
			Action:       "query_execute",
			DatasourceID: int64(i % 3),
			SQLContent:   fmt.Sprintf("SELECT * FROM orders_%d WHERE status = 'active'", i),
			SQLSummary:   fmt.Sprintf("Query orders_%d", i),
		})
	}
}

func TestAuditHandler_SearchAuditLogs(t *testing.T) {
	e, auditSvc, h := newAuditFTSHandlerTest(t)
	seedFTSAuditLogs(t, auditSvc, 15)

	t.Run("basic_keyword_search", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs/search?keyword=orders", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.SearchAuditLogs(c); err != nil {
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
		if len(data) < 1 {
			t.Errorf("expected at least 1 result for 'orders', got %d", len(data))
		}

		total, _ := result["total"].(float64)
		if total < 1 {
			t.Errorf("total = %v, want >= 1", total)
		}
	})

	t.Run("missing_keyword_returns_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs/search", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.SearchAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("filter_by_action", func(t *testing.T) {
		// Add a different action type.
		auditSvc.Write(contextWithTimeout(t), service.AuditRecord{
			UserID:     1,
			Action:     "export",
			SQLContent: "SELECT * FROM orders WHERE id > 100",
		})

		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs/search?keyword=orders&action=export", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.SearchAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		total, _ := result["total"].(float64)
		if total != 1 {
			t.Errorf("expected 1 for action=export with orders keyword, got total = %v", total)
		}
	})

	t.Run("filter_by_user_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs/search?keyword=orders&user_id=2", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.SearchAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		total, _ := result["total"].(float64)
		if total < 1 {
			t.Errorf("expected at least 1 for user_id=2 with orders keyword, got total = %v", total)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs/search?keyword=orders&page=1&page_size=5", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.SearchAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatalf("data is not an array; body=%s", rec.Body.String())
		}
		if len(data) > 5 {
			t.Errorf("expected at most 5 items (page_size=5), got %d", len(data))
		}
	})

	t.Run("no_results", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs/search?keyword=nonexistent_xyz", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.SearchAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		total, _ := result["total"].(float64)
		if total != 0 {
			t.Errorf("expected 0 for nonexistent keyword, got total = %v", total)
		}
	})

	t.Run("highlight_fields_present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit-logs/search?keyword=orders&page_size=1", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.SearchAuditLogs(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodeAuditResponse(t, rec)
		data, ok := result["data"].([]interface{})
		if !ok || len(data) == 0 {
			t.Fatal("expected at least 1 result")
		}

		first := data[0].(map[string]interface{})
		// Check that highlight fields exist (may be empty string if no match in that field).
		if _, hasHighlight := first["highlight_sql_content"]; !hasHighlight {
			t.Error("expected highlight_sql_content field in response")
		}
		if _, hasHighlight := first["highlight_sql_summary"]; !hasHighlight {
			t.Error("expected highlight_sql_summary field in response")
		}
		if _, hasRank := first["rank"]; !hasRank {
			t.Error("expected rank field in response")
		}
	})
}

func TestAuditHandler_SearchAuditLogs_EmptyDB(t *testing.T) {
	e, _, h := newAuditFTSHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/audit-logs/search?keyword=test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.SearchAuditLogs(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodeAuditResponse(t, rec)
	total, _ := result["total"].(float64)
	if total != 0 {
		t.Errorf("expected 0 from empty DB, got total = %v", total)
	}
}

// verifySearchResponseStructure checks the JSON structure of a search response.
func verifySearchResponseStructure(t *testing.T, body []byte) {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode: %v; body=%s", err, string(body))
	}

	// Required top-level fields.
	for _, field := range []string{"code", "message", "data", "page", "page_size", "total"} {
		if _, ok := result[field]; !ok {
			t.Errorf("missing field %q in response", field)
		}
	}
}
