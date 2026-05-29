package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/service"
)

// ─── Test Setup ──────────────────────────────────────────────────────────────

// setupNotifyTest creates a fresh Echo, services, and NotifyHandler for testing.
func setupNotifyTest(t *testing.T) (*echo.Echo, *service.NotifyService, *service.AIReviewService, *NotifyHandler) {
	t.Helper()

	notifySvc := service.NewNotifyService("", "")
	aiReviewSvc := service.NewAIReviewService(nil, "openai", "gpt-4", "test-api-key-12345678", "https://api.openai.com/v1", 10*time.Second)
	handler := NewNotifyHandler(notifySvc, aiReviewSvc)

	e := echo.New()
	return e, notifySvc, aiReviewSvc, handler
}

// decodeNotifyResponse decodes the top-level response envelope.
func decodeNotifyResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

// extractNotifyData extracts the "data" field as a map from the response.
func extractNotifyData(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	result := decodeNotifyResponse(t, rec)
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data is not an object; body=%s", rec.Body.String())
	}
	return data
}

// ─── GetSettings Tests ──────────────────────────────────────────────────────

func TestNotifyHandler_GetSettings(t *testing.T) {
	e, _, _, h := setupNotifyTest(t)

	t.Run("returns_both_webhook_and_ai_config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.GetSettings(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		data := extractNotifyData(t, rec)

		// Should contain "webhook" and "ai" sections
		if _, ok := data["webhook"]; !ok {
			t.Error("missing 'webhook' in response data")
		}
		if _, ok := data["ai"]; !ok {
			t.Error("missing 'ai' in response data")
		}
	})

	t.Run("webhook_disabled_by_default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.GetSettings(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractNotifyData(t, rec)
		dingtalk, ok := data["webhook"].(map[string]interface{})
		if !ok {
			t.Fatal("webhook is not an object")
		}
		if enabled, _ := dingtalk["enabled"].(bool); enabled {
			t.Error("dingtalk should be disabled by default (empty webhook)")
		}
	})

	t.Run("ai_enabled_with_api_key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.GetSettings(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractNotifyData(t, rec)
		ai, ok := data["ai"].(map[string]interface{})
		if !ok {
			t.Fatal("ai is not an object")
		}
		if enabled, _ := ai["enabled"].(bool); !enabled {
			t.Error("ai should be enabled when API key is provided")
		}
	})
}

// ─── UpdateNotifyConfig Tests ────────────────────────────────────────────────

func TestNotifyHandler_UpdateNotifyConfig(t *testing.T) {
	e, _, _, h := setupNotifyTest(t)

	t.Run("success", func(t *testing.T) {
		body := `{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=test","secret":"mysecret"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/notify/webhook", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateNotifyConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		data := extractNotifyData(t, rec)
		if data["webhook_url"] != "https://oapi.dingtalk.com/robot/send?access_token=test" {
			t.Errorf("webhook_url = %v, want the provided URL", data["webhook_url"])
		}
		if enabled, _ := data["enabled"].(bool); !enabled {
			t.Error("should be enabled after setting webhook URL")
		}
		// Secret should be masked
		secret, _ := data["secret"].(string)
		if secret == "mysecret" {
			t.Error("secret should be masked in response")
		}
	})

	t.Run("enables_notification", func(t *testing.T) {
		// First update with a webhook URL
		body := `{"webhook_url":"https://example.com/webhook","secret":"secret123"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/notify/webhook", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateNotifyConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractNotifyData(t, rec)
		if enabled, _ := data["enabled"].(bool); !enabled {
			t.Error("should be enabled after setting webhook URL")
		}
	})

	t.Run("disables_when_webhook_cleared", func(t *testing.T) {
		// First enable
		_, notifySvc, _, h := setupNotifyTest(t)
		notifySvc.UpdateConfig("https://example.com/webhook", "secret")

		// Then clear
		body := `{"webhook_url":"","secret":""}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/notify/webhook", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateNotifyConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractNotifyData(t, rec)
		if enabled, _ := data["enabled"].(bool); enabled {
			t.Error("should be disabled after clearing webhook URL")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/settings/notify/webhook", strings.NewReader(`{bad json}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateNotifyConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("empty_body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/settings/notify/webhook", strings.NewReader(``))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateNotifyConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		// Empty body with Bind still produces zero-value struct, which is valid
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})
}

// ─── TestNotify Tests ────────────────────────────────────────────────────────

func TestNotifyHandler_TestNotify(t *testing.T) {
	t.Run("success_when_enabled", func(t *testing.T) {
		_, notifySvc, _, h := setupNotifyTest(t)
		notifySvc.UpdateConfig("https://example.com/webhook", "secret")

		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/api/settings/notify/webhook/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.TestNotify(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		result := decodeNotifyResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "测试消息已发送" {
			t.Errorf("message = %q, want %q", msg, "测试消息已发送")
		}
	})

	t.Run("fails_when_disabled", func(t *testing.T) {
		_, _, _, h := setupNotifyTest(t)

		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/api/settings/notify/webhook/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.TestNotify(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("fails_without_webhook", func(t *testing.T) {
		_, _, _, h := setupNotifyTest(t)

		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/api/settings/notify/webhook/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.TestNotify(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}

		result := decodeNotifyResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "Webhook 通知未启用，请先配置 Webhook URL" {
			t.Errorf("message = %q, want %q", msg, "Webhook 通知未启用，请先配置 Webhook URL")
		}
	})

	t.Run("success_after_update_config", func(t *testing.T) {
		// Start disabled, enable via UpdateNotifyConfig, then TestNotify succeeds
		_, _, _, h := setupNotifyTest(t)
		e := echo.New()

		// Enable via handler
		body := `{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=abc","secret":"s"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/notify/webhook", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateNotifyConfig(c); err != nil {
			t.Fatalf("update config error: %v", err)
		}

		// Now test notify
		req2 := httptest.NewRequest(http.MethodPost, "/api/settings/notify/webhook/test", nil)
		rec2 := httptest.NewRecorder()
		c2 := e.NewContext(req2, rec2)

		if err := h.TestNotify(c2); err != nil {
			t.Fatalf("test notify error: %v", err)
		}

		if rec2.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec2.Code, http.StatusOK, rec2.Body.String())
		}
	})
}

// ─── UpdateAIConfig Tests ────────────────────────────────────────────────────

func TestNotifyHandler_UpdateAIConfig(t *testing.T) {
	e, _, _, h := setupNotifyTest(t)

	t.Run("success", func(t *testing.T) {
		body := `{"provider":"openai","model":"gpt-4o","api_key":"sk-test-key-1234567890","base_url":"https://api.openai.com/v1","timeout":"15s"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/ai", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateAIConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		data := extractNotifyData(t, rec)
		if data["provider"] != "openai" {
			t.Errorf("provider = %v, want openai", data["provider"])
		}
		if data["model"] != "gpt-4o" {
			t.Errorf("model = %v, want gpt-4o", data["model"])
		}
		if data["base_url"] != "https://api.openai.com/v1" {
			t.Errorf("base_url = %v, want https://api.openai.com/v1", data["base_url"])
		}
		// API key should be masked
		apiKey, _ := data["api_key"].(string)
		if apiKey == "sk-test-key-1234567890" {
			t.Error("api_key should be masked in response")
		}
		if apiKey == "" {
			t.Error("api_key should not be empty")
		}
		if enabled, _ := data["enabled"].(bool); !enabled {
			t.Error("should be enabled when api_key is provided")
		}
	})

	t.Run("disables_when_api_key_cleared", func(t *testing.T) {
		_, _, _, h := setupNotifyTest(t)
		e := echo.New()

		body := `{"provider":"openai","model":"gpt-4","api_key":"","base_url":"https://api.openai.com/v1","timeout":"10s"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/ai", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateAIConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		data := extractNotifyData(t, rec)
		if enabled, _ := data["enabled"].(bool); enabled {
			t.Error("should be disabled when api_key is empty")
		}
	})

	t.Run("invalid_timeout_format", func(t *testing.T) {
		body := `{"provider":"openai","model":"gpt-4","api_key":"test-key","base_url":"https://api.openai.com/v1","timeout":"not-a-duration"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/ai", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateAIConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}

		result := decodeNotifyResponse(t, rec)
		msg, _ := result["message"].(string)
		if msg != "超时格式错误，例: 10s, 1m" {
			t.Errorf("message = %q, want %q", msg, "超时格式错误，例: 10s, 1m")
		}
	})

	t.Run("default_timeout_when_empty", func(t *testing.T) {
		_, _, _, h := setupNotifyTest(t)
		e := echo.New()

		body := `{"provider":"openai","model":"gpt-4","api_key":"test-key","base_url":"https://api.openai.com/v1","timeout":""}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/ai", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateAIConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		data := extractNotifyData(t, rec)
		if data["timeout"] != "10s" {
			t.Errorf("timeout = %v, want 10s (default)", data["timeout"])
		}
	})

	t.Run("various_timeout_formats", func(t *testing.T) {
		tests := []struct {
			name    string
			timeout string
			want    string
		}{
			{"seconds", "30s", "30s"},
			{"minutes", "2m", "2m0s"},
			{"milliseconds", "500ms", "500ms"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, _, _, h := setupNotifyTest(t)
				e := echo.New()

				body := `{"provider":"openai","model":"gpt-4","api_key":"test-key","base_url":"https://api.openai.com/v1","timeout":"` + tt.timeout + `"}`
				req := httptest.NewRequest(http.MethodPut, "/api/settings/ai", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)

				if err := h.UpdateAIConfig(c); err != nil {
					t.Fatalf("handler error: %v", err)
				}

				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
				}

				data := extractNotifyData(t, rec)
				if data["timeout"] != tt.want {
					t.Errorf("timeout = %v, want %s", data["timeout"], tt.want)
				}
			})
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/settings/ai", strings.NewReader(`{bad json}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateAIConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("empty_body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/settings/ai", strings.NewReader(``))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h.UpdateAIConfig(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		// Empty body with Bind still produces zero-value struct, which is valid
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})
}

// ─── Integration: GetSettings after Update ───────────────────────────────────

func TestNotifyHandler_GetSettings_AfterUpdates(t *testing.T) {
	_, _, _, h := setupNotifyTest(t)
	e := echo.New()

	// Update DingTalk config
	dtBody := `{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=xyz","secret":"mylongsecret"}`
	dtReq := httptest.NewRequest(http.MethodPut, "/api/settings/notify/webhook", strings.NewReader(dtBody))
	dtReq.Header.Set("Content-Type", "application/json")
	dtRec := httptest.NewRecorder()
	dtCtx := e.NewContext(dtReq, dtRec)
	if err := h.UpdateNotifyConfig(dtCtx); err != nil {
		t.Fatalf("update webhook: %v", err)
	}

	// Update AI config
	aiBody := `{"provider":"anthropic","model":"claude-3","api_key":"sk-ant-12345678","base_url":"https://api.anthropic.com/v1","timeout":"20s"}`
	aiReq := httptest.NewRequest(http.MethodPut, "/api/settings/ai", strings.NewReader(aiBody))
	aiReq.Header.Set("Content-Type", "application/json")
	aiRec := httptest.NewRecorder()
	aiCtx := e.NewContext(aiReq, aiRec)
	if err := h.UpdateAIConfig(aiCtx); err != nil {
		t.Fatalf("update ai: %v", err)
	}

	// Now GetSettings should reflect both updates
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getRec := httptest.NewRecorder()
	getCtx := e.NewContext(getReq, getRec)

	if err := h.GetSettings(getCtx); err != nil {
		t.Fatalf("get settings: %v", err)
	}

	if getRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}

	data := extractNotifyData(t, getRec)

	// Verify DingTalk config updated
	dingtalk, ok := data["webhook"].(map[string]interface{})
	if !ok {
		t.Fatal("webhook is not an object")
	}
	if enabled, _ := dingtalk["enabled"].(bool); !enabled {
		t.Error("dingtalk should be enabled after update")
	}
	if dingtalk["webhook_url"] != "https://oapi.dingtalk.com/robot/send?access_token=xyz" {
		t.Errorf("dingtalk webhook_url = %v, want the updated URL", dingtalk["webhook_url"])
	}
	// Secret should be masked
	secret, _ := dingtalk["secret"].(string)
	if secret == "mylongsecret" {
		t.Error("dingtalk secret should be masked")
	}

	// Verify AI config updated
	ai, ok := data["ai"].(map[string]interface{})
	if !ok {
		t.Fatal("ai is not an object")
	}
	if ai["provider"] != "anthropic" {
		t.Errorf("ai provider = %v, want anthropic", ai["provider"])
	}
	if ai["model"] != "claude-3" {
		t.Errorf("ai model = %v, want claude-3", ai["model"])
	}
	if enabled, _ := ai["enabled"].(bool); !enabled {
		t.Error("ai should be enabled after update")
	}
	// API key should be masked
	apiKey, _ := ai["api_key"].(string)
	if apiKey == "sk-ant-12345678" {
		t.Error("ai api_key should be masked")
	}
}

// ─── NewNotifyHandler Tests ──────────────────────────────────────────────────

func TestNewNotifyHandler(t *testing.T) {
	notifySvc := service.NewNotifyService("", "")
	aiReviewSvc := service.NewAIReviewService(nil, "", "", "", "", 0)

	handler := NewNotifyHandler(notifySvc, aiReviewSvc)
	if handler == nil {
		t.Fatal("NewNotifyHandler returned nil")
	}
	if handler.notifySvc != notifySvc {
		t.Error("notifySvc not set correctly")
	}
	if handler.aiReviewSvc != aiReviewSvc {
		t.Error("aiReviewSvc not set correctly")
	}
}
