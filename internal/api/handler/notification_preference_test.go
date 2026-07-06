package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/service"
	"github.com/whg517/sqlflow/internal/testutil"
)

func setupNotifPrefTest(t *testing.T) (*echo.Echo, *service.NotificationPreferenceService, *NotificationPreferenceHandler) {
	t.Helper()
	d := testutil.NewDB(t)
	svc := service.NewNotificationPreferenceService(d)
	h := NewNotificationPreferenceHandler(svc)
	return echo.New(), svc, h
}

func TestNotificationPreferenceHandler_GetPreferences_ReturnsDefaults(t *testing.T) {
	e, _, h := setupNotifPrefTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/notifications/preferences", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "alice", "developer")

	if err := h.GetPreferences(c); err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	// When no preferences are saved, the service returns built-in defaults
	// (one row per supported event type), so the array is non-empty.
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) == 0 {
		t.Error("expected default preferences, got empty array")
	}
}

func TestNotificationPreferenceHandler_UpdatePreferences_ValidationError(t *testing.T) {
	e, _, h := setupNotifPrefTest(t)

	// Empty preferences list → 400.
	req := httptest.NewRequest(http.MethodPut, "/api/notifications/preferences",
		strings.NewReader(`{"preferences":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "alice", "developer")
	if err := h.UpdatePreferences(c); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (empty list); body=%s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestNotificationPreferenceHandler_UpdatePreferences_Success(t *testing.T) {
	e, _, h := setupNotifPrefTest(t)

	body := `{"preferences":[{"event_type":"ticket_created","channels":["feishu","dingtalk"]}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/notifications/preferences", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setContextUser(c, 1, "alice", "developer")
	if err := h.UpdatePreferences(c); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Verify the preference persists via GetPreferences.
	req2 := httptest.NewRequest(http.MethodGet, "/api/notifications/preferences", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	setContextUser(c2, 1, "alice", "developer")
	if err := h.GetPreferences(c2); err != nil {
		t.Fatalf("GetPreferences after update: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(rec2.Body.Bytes(), &resp)
	data, _ := resp["data"].([]interface{})
	// The service fans out one row per channel, so we expect at least the 2
	// channels we submitted. Assert the event type is present among them.
	if len(data) < 2 {
		t.Fatalf("expected at least 2 preference rows (one per channel), got %d", len(data))
	}
	foundEvent := false
	for _, item := range data {
		row, _ := item.(map[string]interface{})
		if row["event_type"] == "ticket_created" {
			foundEvent = true
			break
		}
	}
	if !foundEvent {
		t.Errorf("ticket_created preference not found after update; data=%v", data)
	}
}
