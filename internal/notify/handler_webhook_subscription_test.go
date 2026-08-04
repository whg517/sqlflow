package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/testutil"
)

func setupWebhookSubscriptionTest(t *testing.T) (*echo.Echo, *WebhookSubscriptionService, *SubscriptionHandler) {
	t.Helper()
	d := testutil.NewDB(t)
	svc := NewWebhookSubscriptionService(d, "test-encryption-key-32byte-len!!")
	h := NewSubscriptionHandler(svc)
	return echo.New(), svc, h
}

func TestWebhookSubscriptionHandler_Create_Success(t *testing.T) {
	e, _, h := setupWebhookSubscriptionTest(t)

	body := `{"name":"ci-sub","url":"https://example.com/hook","events":["ticket.created","ticket.approved"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/webhooks/subscriptions", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data, _ := resp["data"].(map[string]interface{})
	if data == nil {
		t.Fatal("expected data object in response")
	}
	// The plain secret is returned exactly once at creation.
	if data["secret"] == nil || data["secret"] == "" {
		t.Error("expected non-empty secret returned at creation")
	}
	if data["name"] != "ci-sub" {
		t.Errorf("name = %v, want ci-sub", data["name"])
	}
}

func TestWebhookSubscriptionHandler_Create_MalformedJSON(t *testing.T) {
	e, _, h := setupWebhookSubscriptionTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/webhooks/subscriptions", strings.NewReader(`{not-json`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")
	if err := h.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (malformed); body=%s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestWebhookSubscriptionHandler_List_Empty(t *testing.T) {
	e, _, h := setupWebhookSubscriptionTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/webhooks/subscriptions", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")
	if err := h.List(c); err != nil {
		t.Fatalf("List: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	data, _ := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty list, got %d", len(data))
	}
}

func TestWebhookSubscriptionHandler_Get_InvalidID(t *testing.T) {
	e, _, h := setupWebhookSubscriptionTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/webhooks/subscriptions/abc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	if err := h.Get(c); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid id)", rec.Code, http.StatusBadRequest)
	}
}

func TestWebhookSubscriptionHandler_Delete_InvalidID(t *testing.T) {
	e, _, h := setupWebhookSubscriptionTest(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/webhooks/subscriptions/abc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")
	if err := h.Delete(c); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid id)", rec.Code, http.StatusBadRequest)
	}
}
