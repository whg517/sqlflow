package handler

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// NotifyHandler handles notification settings.
type NotifyHandler struct {
	notifySvc   *service.NotifyService
	aiReviewSvc *service.AIReviewService
}

// NewNotifyHandler creates a new NotifyHandler.
func NewNotifyHandler(notifySvc *service.NotifyService, aiReviewSvc *service.AIReviewService) *NotifyHandler {
	return &NotifyHandler{
		notifySvc:   notifySvc,
		aiReviewSvc: aiReviewSvc,
	}
}

// GetSettings handles GET /api/settings — returns all notification and AI settings.
func (h *NotifyHandler) GetSettings(c echo.Context) error {
	result := map[string]interface{}{
		"dingtalk": h.notifySvc.GetConfig(),
		"ai":       h.aiReviewSvc.GetConfig(),
	}
	return resp.OK(c, result)
}

type updateNotifyConfigRequest struct {
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret"`
}

// UpdateNotifyConfig handles PUT /api/settings/dingtalk — updates DingTalk config.
func (h *NotifyHandler) UpdateNotifyConfig(c echo.Context) error {
	var req updateNotifyConfigRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	h.notifySvc.UpdateConfig(req.WebhookURL, req.Secret)
	return resp.OK(c, h.notifySvc.GetConfig())
}

type testNotifyRequest struct{}

// TestNotify handles POST /api/settings/dingtalk/test — sends a test notification.
func (h *NotifyHandler) TestNotify(c echo.Context) error {
	if !h.notifySvc.IsEnabled() {
		return resp.BadRequest(c, "钉钉通知未启用，请先配置 Webhook URL")
	}

	h.notifySvc.SendTestMessage()
	return resp.OKWithMessage(c, "测试消息已发送", nil)
}

type updateAIConfigRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Timeout  string `json:"timeout"`
}

// UpdateAIConfig handles PUT /api/settings/ai — updates AI review config.
func (h *NotifyHandler) UpdateAIConfig(c echo.Context) error {
	var req updateAIConfigRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	timeout := 10 * time.Second
	if req.Timeout != "" {
		d, err := time.ParseDuration(req.Timeout)
		if err != nil {
			return resp.BadRequest(c, "超时格式错误，例: 10s, 1m")
		}
		timeout = d
	}

	h.aiReviewSvc.UpdateConfig(req.Provider, req.Model, req.APIKey, req.BaseURL, timeout)
	return resp.OK(c, h.aiReviewSvc.GetConfig())
}
