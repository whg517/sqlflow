package ticket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/testutil"
)

// setupSLATest builds a fresh Echo, SLAService (with nil NotifyService —
// notifications are no-ops in tests) and SLAHandler backed by a per-test
// migrated SQLite DB.
func setupSLATest(t *testing.T) (*echo.Echo, *SLAService, *SLAHandler) {
	t.Helper()
	d := testutil.NewDB(t)
	slaSvc := NewSLAService(d, nil)
	return echo.New(), slaSvc, NewSLAHandler(slaSvc)
}

// newSLAContext builds an authenticated admin echo.Context.
func newSLAContext(e *echo.Echo, method, path, body string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")
	return c, rec
}

// TestSLAHandler_CreateSLAConfig_Success verifies the 201 happy path and that
// the returned config echoes the requested fields.
func TestSLAHandler_CreateSLAConfig_Success(t *testing.T) {
	e, _, h := setupSLATest(t)

	body := `{"priority":"high","timeout_minutes":60,"reminder_percent":80,"escalate_to_role":"dba","enabled":true,"auto_reject_enabled":false}`
	c, rec := newSLAContext(e, http.MethodPost, "/api/settings/sla", body)
	c.SetPath("/api/settings/sla")

	if err := h.CreateSLAConfig(c); err != nil {
		t.Fatalf("CreateSLAConfig: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v; body=%s", err, rec.Body.String())
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data: %v", resp)
	}
	if data["priority"] != "high" {
		t.Errorf("priority = %v, want high", data["priority"])
	}
	if got, _ := data["timeout_minutes"].(float64); got != 60 {
		t.Errorf("timeout_minutes = %v, want 60", data["timeout_minutes"])
	}
	if data["enabled"] != true {
		t.Errorf("enabled = %v, want true", data["enabled"])
	}
}

// TestSLAHandler_CreateSLAConfig_ValidationErrors covers each 400 path.
func TestSLAHandler_CreateSLAConfig_ValidationErrors(t *testing.T) {
	e, _, h := setupSLATest(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty priority", `{"priority":"","timeout_minutes":60}`, http.StatusBadRequest},
		{"zero timeout", `{"priority":"low","timeout_minutes":0}`, http.StatusBadRequest},
		{"negative timeout", `{"priority":"low","timeout_minutes":-5}`, http.StatusBadRequest},
		{"malformed JSON", `{not-json`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newSLAContext(e, http.MethodPost, "/api/settings/sla", tc.body)
			c.SetPath("/api/settings/sla")
			if err := h.CreateSLAConfig(c); err != nil {
				t.Fatalf("CreateSLAConfig returned error: %v", err)
			}
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestSLAHandler_ListSLAConfigs_ReturnsCreated creates two configs and confirms
// List returns them, ordered by timeout_minutes ASC.
func TestSLAHandler_ListSLAConfigs_ReturnsCreated(t *testing.T) {
	e, _, h := setupSLATest(t)

	// Create high (60m) then low (30m). Service orders by timeout_minutes ASC,
	// so low should appear first.
	for _, body := range []string{
		`{"priority":"high","timeout_minutes":60}`,
		`{"priority":"low","timeout_minutes":30}`,
	} {
		c, rec := newSLAContext(e, http.MethodPost, "/api/settings/sla", body)
		c.SetPath("/api/settings/sla")
		if err := h.CreateSLAConfig(c); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed status=%d body=%s", rec.Code, rec.Body.String())
		}
	}

	c, rec := newSLAContext(e, http.MethodGet, "/api/settings/sla", "")
	c.SetPath("/api/settings/sla")
	if err := h.ListSLAConfigs(c); err != nil {
		t.Fatalf("ListSLAConfigs: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := resp["data"].([]interface{})
	if len(data) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(data))
	}
	// First element should be the 30-minute config.
	first := data[0].(map[string]interface{})
	if first["priority"] != "low" {
		t.Errorf("first priority = %v, want low (ordered by timeout ASC)", first["priority"])
	}
}

// TestSLAHandler_ListSLAConfigs_Empty returns an empty (or null) list — never
// an error — when no configs exist.
func TestSLAHandler_ListSLAConfigs_Empty(t *testing.T) {
	e, _, h := setupSLATest(t)

	c, rec := newSLAContext(e, http.MethodGet, "/api/settings/sla", "")
	c.SetPath("/api/settings/sla")
	if err := h.ListSLAConfigs(c); err != nil {
		t.Fatalf("ListSLAConfigs: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestSLAHandler_UpdateSLAConfig covers invalid id and success.
func TestSLAHandler_UpdateSLAConfig(t *testing.T) {
	e, _, h := setupSLATest(t)

	t.Run("invalid id", func(t *testing.T) {
		c, rec := newSLAContext(e, http.MethodPut, "/api/settings/sla/abc", `{"priority":"x","timeout_minutes":10}`)
		c.SetPath("/api/settings/sla/:id")
		c.SetParamNames("id")
		c.SetParamValues("abc")
		if err := h.UpdateSLAConfig(c); err != nil {
			t.Fatalf("UpdateSLAConfig: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("success", func(t *testing.T) {
		// Seed a config to update.
		seedC, seedRec := newSLAContext(e, http.MethodPost, "/api/settings/sla",
			`{"priority":"med","timeout_minutes":45}`)
		seedC.SetPath("/api/settings/sla")
		if err := h.CreateSLAConfig(seedC); err != nil {
			t.Fatalf("seed: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(seedRec.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("decode seed: %v", err)
		}
		idStr := strconv.FormatInt(int64(parsed["data"].(map[string]interface{})["id"].(float64)), 10)

		body := `{"priority":"med","timeout_minutes":90,"reminder_percent":75}`
		c, rec := newSLAContext(e, http.MethodPut, "/api/settings/sla/"+idStr, body)
		c.SetPath("/api/settings/sla/:id")
		c.SetParamNames("id")
		c.SetParamValues(idStr)
		if err := h.UpdateSLAConfig(c); err != nil {
			t.Fatalf("UpdateSLAConfig: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})
}

// TestSLAHandler_DeleteSLAConfig covers invalid id and success.
func TestSLAHandler_DeleteSLAConfig(t *testing.T) {
	e, _, h := setupSLATest(t)

	t.Run("invalid id", func(t *testing.T) {
		c, rec := newSLAContext(e, http.MethodDelete, "/api/settings/sla/xyz", "")
		c.SetPath("/api/settings/sla/:id")
		c.SetParamNames("id")
		c.SetParamValues("xyz")
		if err := h.DeleteSLAConfig(c); err != nil {
			t.Fatalf("DeleteSLAConfig: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("success", func(t *testing.T) {
		// Seed then delete.
		seedC, seedRec := newSLAContext(e, http.MethodPost, "/api/settings/sla",
			`{"priority":"del","timeout_minutes":15}`)
		seedC.SetPath("/api/settings/sla")
		if err := h.CreateSLAConfig(seedC); err != nil {
			t.Fatalf("seed: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(seedRec.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("decode seed: %v", err)
		}
		idStr := strconv.FormatInt(int64(parsed["data"].(map[string]interface{})["id"].(float64)), 10)

		c, rec := newSLAContext(e, http.MethodDelete, "/api/settings/sla/"+idStr, "")
		c.SetPath("/api/settings/sla/:id")
		c.SetParamNames("id")
		c.SetParamValues(idStr)
		if err := h.DeleteSLAConfig(c); err != nil {
			t.Fatalf("DeleteSLAConfig: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

// TestSLAHandler_GetTicketSLAStatuses covers the missing-param and no-valid-ids
// 400 branches.
func TestSLAHandler_GetTicketSLAStatuses_ValidationErrors(t *testing.T) {
	e, _, h := setupSLATest(t)

	t.Run("missing ticket_ids param", func(t *testing.T) {
		c, rec := newSLAContext(e, http.MethodGet, "/api/tickets/sla-status", "")
		c.SetPath("/api/tickets/sla-status")
		if err := h.GetTicketSLAStatuses(c); err != nil {
			t.Fatalf("GetTicketSLAStatuses: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("no valid ids", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/tickets/sla-status?ticket_ids=abc,xyz", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/api/tickets/sla-status")
		c.QueryParams().Set("ticket_ids", "abc,xyz")
		testutil.SetContextUser(c, 1, "admin", "admin")
		if err := h.GetTicketSLAStatuses(c); err != nil {
			t.Fatalf("GetTicketSLAStatuses: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

// TestSLAHandler_GetTicketSLAStatuses_Success sets up a fresh handler+DB, seeds
// a ticket row, then asserts the batch status endpoint returns a 200 with a map
// keyed by ticket id.
func TestSLAHandler_GetTicketSLAStatuses_Success(t *testing.T) {
	d := testutil.NewDB(t)
	slaSvc := NewSLAService(d, nil)
	h := NewSLAHandler(slaSvc)
	e := echo.New()

	// Seed one ticket with an SLA deadline set.
	if _, err := d.Exec(`INSERT INTO tickets (submitter_id, datasource_id, sql_content, status, sla_status, sla_deadline) VALUES (1, 1, 'SELECT 1', 'PENDING_APPROVAL', 'normal', (now() + interval '+1 hour'))`); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/sla-status?ticket_ids=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/tickets/sla-status")
	c.QueryParams().Set("ticket_ids", "1")
	testutil.SetContextUser(c, 1, "admin", "admin")

	if err := h.GetTicketSLAStatuses(c); err != nil {
		t.Fatalf("GetTicketSLAStatuses: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map keyed by ticket id; got %v", resp["data"])
	}
	entry, ok := data["1"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected key '1' in SLA status map; got %v", data)
	}
	if entry["sla_status"] != "normal" {
		t.Errorf("sla_status = %v, want normal", entry["sla_status"])
	}
}

// TestSLAHandler_ListSLANotifications_Empty hits the notifications list with no
// seeded rows; it should return 200 with an empty page.
func TestSLAHandler_ListSLANotifications_Empty(t *testing.T) {
	e, _, h := setupSLATest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sla-notifications?page=1&page_size=10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/sla-notifications")
	c.QueryParams().Set("page", "1")
	c.QueryParams().Set("page_size", "10")
	testutil.SetContextUser(c, 1, "admin", "admin")

	if err := h.ListSLANotifications(c); err != nil {
		t.Fatalf("ListSLANotifications: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	// Page envelope must carry total=0.
	if !strings.Contains(rec.Body.String(), `"total":0`) {
		t.Errorf("expected total:0 in body, got: %s", rec.Body.String())
	}
}
