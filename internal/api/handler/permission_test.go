package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/service"
)

// ─── Test Setup ──────────────────────────────────────────────────────────────

// setupPermissionTest creates a fresh Echo, DB, PermissionService, and PermissionHandler for testing.
// It seeds the casbin_rule table directly to avoid dependency on the policy.csv file location.
func setupPermissionTest(t *testing.T) (*echo.Echo, *service.PermissionService, *PermissionHandler) {
	t.Helper()

	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Seed initial policies directly into casbin_rule so that
	// PermissionService.seedsIfEmpty sees count > 0 and skips file read.
	seedPolicies := []struct{ ptype, v0, v1, v2, v3 string }{
		{"p", "admin", "*", "*", "*"},
		{"p", "dba", "*", "*", "select"},
		{"p", "dba", "*", "*", "update"},
		{"p", "dba", "*", "*", "delete"},
		{"p", "dba", "*", "*", "ddl"},
		{"p", "dba", "*", "*", "export"},
		{"p", "dba", "*", "*", "desensitize:bypass"},
		{"p", "developer", "*", "*", "select"},
	}
	for _, sp := range seedPolicies {
		_, err := database.Exec(
			`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3) VALUES (?, ?, ?, ?, ?)`,
			sp.ptype, sp.v0, sp.v1, sp.v2, sp.v3,
		)
		if err != nil {
			t.Fatalf("seed policy: %v", err)
		}
	}

	permSvc, err := service.NewPermissionService(database.DB)
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}

	handler := NewPermissionHandler(permSvc)

	e := echo.New()
	return e, permSvc, handler
}

// addTestPolicy is a helper that adds a policy directly via service.
func addTestPolicy(t *testing.T, permSvc *service.PermissionService, sub, dom, obj, act string) {
	t.Helper()
	if err := permSvc.AddPolicy(sub, dom, obj, act); err != nil {
		t.Fatalf("add test policy (%s, %s, %s, %s): %v", sub, dom, obj, act, err)
	}
}

// decodePermResponse decodes the top-level response envelope.
func decodePermResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

// extractPermData extracts the "data" field as a map from the response.
func extractPermData(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	result := decodePermResponse(t, rec)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data is not an object; body=%s", rec.Body.String())
	}
	return data
}

// ─── ListRoles Tests ────────────────────────────────────────────────────────

func TestPermissionHandler_ListRoles(t *testing.T) {
	e, _, h := setupPermissionTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/roles", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListRoles(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodePermResponse(t, rec)
	items, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array; body=%s", rec.Body.String())
	}

	// Should return the 3 built-in roles: admin, dba, developer
	if len(items) != 3 {
		t.Errorf("len(data) = %d, want 3", len(items))
	}

	names := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("item is not a map: %v", item)
		}
		name, _ := m["name"].(string)
		names = append(names, name)
	}

	wantNames := []string{"admin", "dba", "developer"}
	for _, want := range wantNames {
		found := false
		for _, got := range names {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing role %q in response; got %v", want, names)
		}
	}
}

// ─── GetRole Tests ──────────────────────────────────────────────────────────

func TestPermissionHandler_GetRole(t *testing.T) {
	e, _, h := setupPermissionTest(t)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/roles/:role", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("role")
		c.SetParamValues("admin")

		if err := h.GetRole(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		data := extractPermData(t, rec)
		if data["name"] != "admin" {
			t.Errorf("name = %v, want admin", data["name"])
		}

		policies, ok := data["policies"].([]interface{})
		if !ok {
			t.Fatalf("policies is not an array; body=%s", rec.Body.String())
		}
		// Seeded admin should have wildcard policies
		if len(policies) == 0 {
			t.Error("expected admin role to have at least one policy")
		}
	})

	t.Run("empty_role", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/roles/:role", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("role")
		c.SetParamValues("")

		if err := h.GetRole(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "角色名称不能为空" {
			t.Errorf("message = %q, want %q", msg, "角色名称不能为空")
		}
	})

	t.Run("role_with_no_policies", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/roles/:role", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("role")
		c.SetParamValues("nonexistent_role")

		if err := h.GetRole(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		data := extractPermData(t, rec)
		if data["name"] != "nonexistent_role" {
			t.Errorf("name = %v, want nonexistent_role", data["name"])
		}

		// policies may be nil (serialized as null) for a role with no policies
		policiesRaw := data["policies"]
		if policiesRaw == nil {
			// null is acceptable for no policies
		} else {
			policies, ok := policiesRaw.([]interface{})
			if !ok {
				t.Fatalf("policies is not an array; body=%s", rec.Body.String())
			}
			if len(policies) != 0 {
				t.Errorf("expected empty policies for nonexistent role, got %d", len(policies))
			}
		}
	})
}

// ─── AddPolicy Tests ────────────────────────────────────────────────────────

func TestPermissionHandler_AddPolicy(t *testing.T) {
	_, _, h := setupPermissionTest(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			"success",
			`{"sub":"developer","dom":"domain1","obj":"table1","act":"select"}`,
			http.StatusCreated,
			"created",
		},
		{
			"missing_sub",
			`{"sub":"","dom":"domain1","obj":"table1","act":"select"}`,
			http.StatusBadRequest,
			"sub, dom, obj, act 均不能为空",
		},
		{
			"missing_dom",
			`{"sub":"developer","dom":"","obj":"table1","act":"select"}`,
			http.StatusBadRequest,
			"sub, dom, obj, act 均不能为空",
		},
		{
			"missing_obj",
			`{"sub":"developer","dom":"domain1","obj":"","act":"select"}`,
			http.StatusBadRequest,
			"sub, dom, obj, act 均不能为空",
		},
		{
			"missing_act",
			`{"sub":"developer","dom":"domain1","obj":"table1","act":""}`,
			http.StatusBadRequest,
			"sub, dom, obj, act 均不能为空",
		},
		{
			"all_missing",
			`{}`,
			http.StatusBadRequest,
			"sub, dom, obj, act 均不能为空",
		},
		{
			"invalid_json",
			`{bad json}`,
			http.StatusBadRequest,
			"请求格式错误",
		},
		{
			"empty_body",
			``,
			http.StatusBadRequest,
			"sub, dom, obj, act 均不能为空",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/api/policies", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := h.AddPolicy(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			result := decodePermResponse(t, rec)
			msg, _ := result["message"].(string)
			if msg != tt.wantMsg {
				t.Errorf("message = %q, want %q", msg, tt.wantMsg)
			}
		})
	}

	// Verify success response shape
	t.Run("success_response_shape", func(t *testing.T) {
		e := echo.New()
		body := `{"sub":"test_sub","dom":"test_dom","obj":"test_obj","act":"test_act"}`
		req := httptest.NewRequest(http.MethodPost, "/api/policies", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.AddPolicy(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
		}

		data := extractPermData(t, rec)
		msg, _ := data["message"].(string)
		if msg != "策略添加成功" {
			t.Errorf("data.message = %q, want %q", msg, "策略添加成功")
		}
	})

	// Duplicate policy should fail
	t.Run("duplicate_policy", func(t *testing.T) {
		e := echo.New()
		body := `{"sub":"developer","dom":"domain1","obj":"table1","act":"select"}`
		req := httptest.NewRequest(http.MethodPost, "/api/policies", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.AddPolicy(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "策略已存在" {
			t.Errorf("message = %q, want %q", msg, "策略已存在")
		}
	})
}

// ─── ListPolicies Tests ─────────────────────────────────────────────────────

func TestPermissionHandler_ListPolicies(t *testing.T) {
	e, permSvc, h := setupPermissionTest(t)

	// Add a few policies for listing
	addTestPolicy(t, permSvc, "list_sub_1", "list_dom_1", "list_obj_1", "select")
	addTestPolicy(t, permSvc, "list_sub_2", "list_dom_2", "list_obj_2", "insert")
	addTestPolicy(t, permSvc, "list_sub_3", "list_dom_3", "list_obj_3", "update")

	t.Run("success_default_pagination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/policies", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListPolicies(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatalf("data is not an array; body=%s", rec.Body.String())
		}

		// Should have at least the 3 we added plus seeded ones
		if len(data) < 3 {
			t.Errorf("expected at least 3 policies, got %d", len(data))
		}

		// Verify pagination fields
		page, _ := result["page"].(float64)
		pageSize, _ := result["page_size"].(float64)
		total, _ := result["total"].(float64)

		if page != 1 {
			t.Errorf("page = %v, want 1", page)
		}
		if pageSize != 50 {
			t.Errorf("page_size = %v, want 50", pageSize)
		}
		if total < 3 {
			t.Errorf("total = %v, want >= 3", total)
		}
	})

	t.Run("custom_pagination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/policies?page=1&page_size=2", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListPolicies(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatalf("data is not an array; body=%s", rec.Body.String())
		}

		if len(data) > 2 {
			t.Errorf("expected at most 2 items with page_size=2, got %d", len(data))
		}

		pageSize, _ := result["page_size"].(float64)
		if pageSize != 2 {
			t.Errorf("page_size = %v, want 2", pageSize)
		}
	})

	t.Run("filter_by_sub", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/policies?sub=list_sub_1", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListPolicies(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
		data, ok := result["data"].([]interface{})
		if !ok {
			t.Fatalf("data is not an array; body=%s", rec.Body.String())
		}

		// Should have exactly 1 policy matching list_sub_1
		if len(data) != 1 {
			t.Errorf("expected 1 policy for sub=list_sub_1, got %d", len(data))
		}
	})

	t.Run("invalid_page_defaults_to_1", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/policies?page=-1&page_size=-1", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListPolicies(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
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
		req := httptest.NewRequest(http.MethodGet, "/api/policies?page=abc&page_size=xyz", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListPolicies(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
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

func TestPermissionHandler_ListPolicies_Empty(t *testing.T) {
	e, _, h := setupPermissionTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/policies", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListPolicies(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	result := decodePermResponse(t, rec)
	_, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array; body=%s", rec.Body.String())
	}

	// May still have seeded policies depending on whether seed succeeded
	// Just verify the response is valid
	total, _ := result["total"].(float64)
	if total < 0 {
		t.Errorf("total = %v, want >= 0", total)
	}
}

// ─── DeletePolicy Tests ─────────────────────────────────────────────────────

func TestPermissionHandler_DeletePolicy(t *testing.T) {
	e, permSvc, h := setupPermissionTest(t)

	t.Run("success", func(t *testing.T) {
		// Add a policy and get its ID via ListPolicies
		addTestPolicy(t, permSvc, "del_sub", "del_dom", "del_obj", "delete")

		ctx := contextWithTimeout(t)
		policies, _, err := permSvc.GetPolicies(ctx, 1, 100, "", "del_sub")
		if err != nil {
			t.Fatalf("get policies: %v", err)
		}
		if len(policies) == 0 {
			t.Fatal("expected to find the policy we just added")
		}

		policyID := policies[0].ID

		req := httptest.NewRequest(http.MethodDelete, "/api/policies/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%d", policyID))

		if err := h.DeletePolicy(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "策略已删除" {
			t.Errorf("message = %q, want %q", msg, "策略已删除")
		}

		// Verify policy is actually deleted
		policies2, _, err := permSvc.GetPolicies(ctx, 1, 100, "", "del_sub")
		if err != nil {
			t.Fatalf("get policies after delete: %v", err)
		}
		if len(policies2) != 0 {
			t.Errorf("expected 0 policies for del_sub after deletion, got %d", len(policies2))
		}
	})

	t.Run("not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/policies/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("99999")

		if err := h.DeletePolicy(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "策略不存在" {
			t.Errorf("message = %q, want %q", msg, "策略不存在")
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/policies/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("notanumber")

		if err := h.DeletePolicy(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "无效的策略ID" {
			t.Errorf("message = %q, want %q", msg, "无效的策略ID")
		}
	})

	t.Run("negative_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/policies/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("-1")

		if err := h.DeletePolicy(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		// Negative IDs are valid int64 but won't exist in DB
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	t.Run("zero_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/policies/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("0")

		if err := h.DeletePolicy(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

// ─── SyncPolicies Tests ─────────────────────────────────────────────────────

func TestPermissionHandler_SyncPolicies(t *testing.T) {
	e, _, h := setupPermissionTest(t)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/policies/sync", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.SyncPolicies(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodePermResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "策略同步成功" {
			t.Errorf("message = %q, want %q", msg, "策略同步成功")
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		// Syncing multiple times should still succeed
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodPost, "/api/policies/sync", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := h.SyncPolicies(c); err != nil {
				t.Fatalf("sync %d: handler error: %v", i, err)
			}

			if rec.Code != http.StatusOK {
				t.Errorf("sync %d: status = %d, want %d; body = %s", i, rec.Code, http.StatusOK, rec.Body.String())
			}
		}
	})
}

// ─── Integration: Full Permission CRUD Lifecycle ────────────────────────────

func TestPermissionHandler_CRUDLifecycle(t *testing.T) {
	e, permSvc, h := setupPermissionTest(t)

	// Step 1: ListRoles — should return 3 built-in roles
	t.Run("list_roles", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/roles", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListRoles(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodePermResponse(t, rec)
		items, _ := result["data"].([]interface{})
		if len(items) != 3 {
			t.Errorf("expected 3 roles, got %d", len(items))
		}
	})

	// Step 2: GetRole for admin
	t.Run("get_admin_role", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/roles/:role", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("role")
		c.SetParamValues("admin")

		if err := h.GetRole(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractPermData(t, rec)
		if data["name"] != "admin" {
			t.Errorf("name = %v, want admin", data["name"])
		}
	})

	// Step 3: AddPolicy
	t.Run("add_policy", func(t *testing.T) {
		body := `{"sub":"lifecycle_sub","dom":"lifecycle_dom","obj":"lifecycle_obj","act":"select"}`
		req := httptest.NewRequest(http.MethodPost, "/api/policies", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.AddPolicy(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
		}
	})

	// Step 4: ListPolicies — should include the added policy
	var policyID float64
	t.Run("list_policies_with_filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/policies?sub=lifecycle_sub", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListPolicies(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodePermResponse(t, rec)
		data, _ := result["data"].([]interface{})
		if len(data) != 1 {
			t.Fatalf("expected 1 policy, got %d", len(data))
		}

		policy, _ := data[0].(map[string]interface{})
		policyID = policy["id"].(float64)
		if policyID <= 0 {
			t.Errorf("expected positive policy ID, got %v", policyID)
		}
	})

	// Step 5: GetRole for lifecycle_sub — should show the policy
	t.Run("get_role_with_policy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/roles/:role", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("role")
		c.SetParamValues("lifecycle_sub")

		if err := h.GetRole(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractPermData(t, rec)
		policies, _ := data["policies"].([]interface{})
		if len(policies) != 1 {
			t.Errorf("expected 1 policy for lifecycle_sub, got %d", len(policies))
		}
	})

	// Step 6: DeletePolicy
	t.Run("delete_policy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/policies/:id", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(fmt.Sprintf("%.0f", policyID))

		if err := h.DeletePolicy(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})

	// Step 7: Verify deleted — ListPolicies should not find it
	t.Run("verify_deleted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/policies?sub=lifecycle_sub", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.ListPolicies(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		result := decodePermResponse(t, rec)
		data, _ := result["data"].([]interface{})
		if len(data) != 0 {
			t.Errorf("expected 0 policies after deletion, got %d", len(data))
		}
	})

	// Step 8: SyncPolicies
	t.Run("sync_policies", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/policies/sync", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.SyncPolicies(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})

	// Suppress unused warnings
	_ = permSvc.LoadPolicy
}

// ─── SyncPolicies Error Path ────────────────────────────────────────────────

func TestPermissionHandler_SyncPolicies_LoadPolicyError(t *testing.T) {
	database, err := db.Open(t.TempDir() + "/test_sync_err.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Seed a policy so the service initializes correctly
	_, _ = database.Exec(
		`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3) VALUES (?, ?, ?, ?, ?)`,
		"p", "admin", "*", "*", "*",
	)

	permSvc, err := service.NewPermissionService(database.DB)
	if err != nil {
		t.Fatalf("create permission service: %v", err)
	}

	handler := NewPermissionHandler(permSvc)

	// Close the database to force LoadPolicy to fail
	database.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/policies/sync", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.SyncPolicies(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	result := decodePermResponse(t, rec)
	msg, _ := result["message"].(string)
	if msg != "同步策略失败" {
		t.Errorf("message = %q, want %q", msg, "同步策略失败")
	}
}
