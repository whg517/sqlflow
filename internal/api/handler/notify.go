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
//
// @Summary 获取系统设置
// @Description 管理员获取钉钉通知和AI配置
// @Tags 设置
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /settings [get]
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
//
// @Summary 更新钉钉通知配置
// @Description 管理员更新钉钉Webhook配置
// @Tags 设置
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body updateNotifyConfigRequest true "钉钉配置请求"
// @Success 200 {object} resp.SuccessResponse "更新成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Router /settings/dingtalk [put]
func (h *NotifyHandler) UpdateNotifyConfig(c echo.Context) error {
	var req updateNotifyConfigRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	h.notifySvc.UpdateConfig(req.WebhookURL, req.Secret)
	return resp.OK(c, h.notifySvc.GetConfig())
}

// TestNotify handles POST /api/settings/dingtalk/test — sends a test notification.
//
// @Summary 测试钉钉通知
// @Description 管理员发送一条测试钉钉通知消息
// @Tags 设置
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "测试消息已发送"
// @Failure 400 {object} resp.ErrorResponse "钉钉通知未启用"
// @Router /settings/dingtalk/test [post]
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
//
// @Summary 更新AI配置
// @Description 管理员更新AI审核服务的配置
// @Tags 设置
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body updateAIConfigRequest true "AI配置请求"
// @Success 200 {object} resp.SuccessResponse "更新成功"
// @Failure 400 {object} resp.ErrorResponse "请求格式错误"
// @Router /settings/ai [put]
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
