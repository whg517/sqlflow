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

func setupFeishuWebhookTest(t *testing.T) (*echo.Echo, *FeishuService, *FeishuHandler) {
	t.Helper()
	d := testutil.NewDB(t)
	svc := NewFeishuService(d.DB, "test-encryption-key-32byte-len!!")
	h := NewFeishuHandler(svc)
	return echo.New(), svc, h
}

func TestFeishuWebhookHandler_Create_ValidationErrors(t *testing.T) {
	e, _, h := setupFeishuWebhookTest(t)

	cases := []struct {
		name string
		body string
	}{
		{"empty name", `{"name":"","webhook_url":"https://example.com/h"}`},
		{"empty webhook_url", `{"name":"wh","webhook_url":""}`},
		{"malformed JSON", `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/settings/feishu/webhooks", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			testutil.SetContextUser(c, 1, "admin", "admin")
			if err := h.Create(c); err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestFeishuWebhookHandler_Create_Success(t *testing.T) {
	e, _, h := setupFeishuWebhookTest(t)

	body := `{"name":"ci-hook","webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/x","scene":"ticket"}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings/feishu/webhooks", strings.NewReader(body))
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
	if data == nil || data["name"] != "ci-hook" {
		t.Errorf("expected data.name=ci-hook, got %v", resp["data"])
	}
}

func TestFeishuWebhookHandler_List_Empty(t *testing.T) {
	e, _, h := setupFeishuWebhookTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/settings/feishu/webhooks", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	testutil.SetContextUser(c, 1, "admin", "admin")
	if err := h.List(c); err != nil {
		t.Fatalf("List: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestFeishuWebhookHandler_Get_InvalidID(t *testing.T) {
	e, _, h := setupFeishuWebhookTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/settings/feishu/webhooks/abc", nil)
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

func TestFeishuWebhookHandler_Delete_InvalidID(t *testing.T) {
	e, _, h := setupFeishuWebhookTest(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/settings/feishu/webhooks/abc", nil)
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
