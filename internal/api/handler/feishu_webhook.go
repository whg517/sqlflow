package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// FeishuWebhookHandler handles Feishu webhook CRUD API.
type FeishuWebhookHandler struct {
	svc *service.FeishuWebhookService
}

// NewFeishuWebhookHandler creates a new handler.
func NewFeishuWebhookHandler(svc *service.FeishuWebhookService) *FeishuWebhookHandler {
	return &FeishuWebhookHandler{svc: svc}
}

// Create handles POST /api/settings/feishu/webhooks — creates a new Feishu webhook.
//
// @Summary 创建飞书 Webhook
// @Description 管理员创建新的飞书 Webhook 通知通道
// @Tags 飞书通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body service.FeishuWebhookCreateRequest true "Webhook 配置"
// @Success 200 {object} resp.SuccessResponse "创建成功"
// @Failure 400 {object} resp.ErrorResponse "请求错误"
// @Router /settings/feishu/webhooks [post]
func (h *FeishuWebhookHandler) Create(c echo.Context) error {
	var req service.FeishuWebhookCreateRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	if req.Name == "" {
		return resp.BadRequest(c, "name 不能为空")
	}
	if req.WebhookURL == "" {
		return resp.BadRequest(c, "webhook_url 不能为空")
	}

	// Get admin username from JWT context
	createdBy := "admin"
	if username, ok := c.Get("username").(string); ok && username != "" {
		createdBy = username
	}

	wh, err := h.svc.Create(c.Request().Context(), req, createdBy)
	if err != nil {
		return resp.BadRequest(c, err.Error())
	}

	return resp.OK(c, wh)
}

// List handles GET /api/settings/feishu/webhooks — lists all Feishu webhooks.
//
// @Summary 列出飞书 Webhook
// @Description 管理员获取所有飞书 Webhook 配置列表
// @Tags 飞书通知
// @Produce json
// @Security BearerAuth
// @Param full_url query bool false "是否显示完整 URL（仅管理员）"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /settings/feishu/webhooks [get]
func (h *FeishuWebhookHandler) List(c echo.Context) error {
	showFullURL := c.QueryParam("full_url") == "true"
	items, err := h.svc.List(c.Request().Context(), showFullURL)
	if err != nil {
		return resp.BadRequest(c, err.Error())
	}
	if items == nil {
		items = []map[string]interface{}{}
	}
	return resp.OK(c, items)
}

// Get handles GET /api/settings/feishu/webhooks/:id — gets a single webhook.
//
// @Summary 获取飞书 Webhook 详情
// @Description 管理员获取单个飞书 Webhook 配置详情
// @Tags 飞书通知
// @Produce json
// @Security BearerAuth
// @Param id path int true "Webhook ID"
// @Param full_url query bool false "是否显示完整 URL"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 404 {object} resp.ErrorResponse "不存在"
// @Router /settings/feishu/webhooks/{id} [get]
func (h *FeishuWebhookHandler) Get(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	wh, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return resp.BadRequest(c, err.Error())
	}

	showFullURL := c.QueryParam("full_url") == "true"
	decryptedURL, _ := h.svc.DecryptURL(wh.EncryptedURL)

	displayURL := "****"
	if showFullURL && decryptedURL != "" {
		displayURL = decryptedURL
	} else if decryptedURL != "" {
		displayURL = service.MaskURL(decryptedURL)
	}

	result := map[string]interface{}{
		"id":             wh.ID,
		"name":           wh.Name,
		"webhook_url":    displayURL,
		"scene":          wh.Scene,
		"enabled":        wh.Enabled,
		"rate_limit_rps": wh.RateLimitRPS,
		"created_by":     wh.CreatedBy,
		"created_at":     wh.CreatedAt,
		"updated_at":     wh.UpdatedAt,
	}

	return resp.OK(c, result)
}

// Update handles PUT /api/settings/feishu/webhooks/:id — updates a webhook.
//
// @Summary 更新飞书 Webhook
// @Description 管理员更新飞书 Webhook 配置
// @Tags 飞书通知
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Webhook ID"
// @Param body body service.FeishuWebhookUpdateRequest true "更新字段"
// @Success 200 {object} resp.SuccessResponse "更新成功"
// @Failure 400 {object} resp.ErrorResponse "请求错误"
// @Router /settings/feishu/webhooks/{id} [put]
func (h *FeishuWebhookHandler) Update(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	var req service.FeishuWebhookUpdateRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	wh, err := h.svc.Update(c.Request().Context(), id, req)
	if err != nil {
		return resp.BadRequest(c, err.Error())
	}

	return resp.OK(c, wh)
}

// Delete handles DELETE /api/settings/feishu/webhooks/:id — deletes a webhook.
//
// @Summary 删除飞书 Webhook
// @Description 管理员删除飞书 Webhook 配置（同时清理死信队列）
// @Tags 飞书通知
// @Produce json
// @Security BearerAuth
// @Param id path int true "Webhook ID"
// @Success 200 {object} resp.SuccessResponse "删除成功"
// @Failure 400 {object} resp.ErrorResponse "请求错误"
// @Router /settings/feishu/webhooks/{id} [delete]
func (h *FeishuWebhookHandler) Delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	return resp.OKWithMessage(c, "已删除", nil)
}

// ListDeadLetters handles GET /api/settings/feishu/webhooks/dead-letters — lists dead letter entries.
//
// @Summary 查看死信队列
// @Description 管理员查看发送失败的飞书通知（死信队列）
// @Tags 飞书通知
// @Produce json
// @Security BearerAuth
// @Param webhook_id query int false "按 Webhook ID 过滤"
// @Param limit query int false "返回数量（默认 50）"
// @Success 200 {object} resp.SuccessResponse "成功"
// @Router /settings/feishu/webhooks/dead-letters [get]
func (h *FeishuWebhookHandler) ListDeadLetters(c echo.Context) error {
	var webhookID int64
	if wid := c.QueryParam("webhook_id"); wid != "" {
		var err error
		webhookID, err = strconv.ParseInt(wid, 10, 64)
		if err != nil {
			return resp.BadRequest(c, "无效的 webhook_id")
		}
	}

	limit := 50
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	items, err := h.svc.ListDeadLetters(c.Request().Context(), webhookID, limit)
	if err != nil {
		return resp.BadRequest(c, err.Error())
	}
	if items == nil {
		items = []service.FeishuDeadLetter{}
	}
	return resp.OK(c, items)
}
