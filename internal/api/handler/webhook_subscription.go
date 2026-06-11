package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// WebhookSubscriptionHandler handles outbound webhook subscription CRUD API.
type WebhookSubscriptionHandler struct {
	svc *service.WebhookSubscriptionService
}

// NewWebhookSubscriptionHandler creates a new handler.
func NewWebhookSubscriptionHandler(svc *service.WebhookSubscriptionService) *WebhookSubscriptionHandler {
	return &WebhookSubscriptionHandler{svc: svc}
}

// mapError maps service errors to appropriate HTTP status codes.
func mapWebhookError(err error) (int, string) {
	if service.IsNotFoundError(err) {
		return http.StatusNotFound, err.Error()
	}
	if service.IsValidationError(err) {
		return http.StatusBadRequest, err.Error()
	}
	// Default: internal error for unexpected failures
	return http.StatusInternalServerError, err.Error()
}

// Create handles POST /api/admin/webhooks/subscriptions
func (h *WebhookSubscriptionHandler) Create(c echo.Context) error {
	var req service.CreateSubscriptionRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	username := "admin"
	if u, ok := c.Get("username").(string); ok && u != "" {
		username = u
	}

	sub, plainSecret, err := h.svc.Create(c.Request().Context(), req, username)
	if err != nil {
		code, msg := mapWebhookError(err)
		if code == http.StatusBadRequest {
			return resp.BadRequest(c, msg)
		}
		return resp.InternalError(c, msg)
	}

	result := service.FormatSubscriptionForResponse(sub)
	result["secret"] = plainSecret // Only returned once on creation
	return resp.OK(c, result)
}

// List handles GET /api/admin/webhooks/subscriptions
func (h *WebhookSubscriptionHandler) List(c echo.Context) error {
	subs, err := h.svc.List(c.Request().Context())
	if err != nil {
		return resp.InternalError(c, "获取订阅列表失败")
	}

	items := make([]map[string]interface{}, 0, len(subs))
	for _, sub := range subs {
		items = append(items, service.FormatSubscriptionForResponse(sub))
	}

	return resp.OK(c, items)
}

// Get handles GET /api/admin/webhooks/subscriptions/:id
func (h *WebhookSubscriptionHandler) Get(c echo.Context) error {
	id, err := service.ParseWebhookID(c.Param("id"))
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	sub, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		code, msg := mapWebhookError(err)
		if code == http.StatusNotFound {
			return c.JSON(http.StatusNotFound, resp.ErrorResponse{Code: http.StatusNotFound, Message: msg})
		}
		return resp.InternalError(c, msg)
	}

	return resp.OK(c, service.FormatSubscriptionForResponse(sub))
}

// Update handles PUT /api/admin/webhooks/subscriptions/:id
func (h *WebhookSubscriptionHandler) Update(c echo.Context) error {
	id, err := service.ParseWebhookID(c.Param("id"))
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	var req service.UpdateSubscriptionRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	username := "admin"
	if u, ok := c.Get("username").(string); ok && u != "" {
		username = u
	}

	sub, err := h.svc.Update(c.Request().Context(), id, req, username)
	if err != nil {
		code, msg := mapWebhookError(err)
		if code == http.StatusNotFound {
			return c.JSON(http.StatusNotFound, resp.ErrorResponse{Code: http.StatusNotFound, Message: msg})
		}
		if code == http.StatusBadRequest {
			return resp.BadRequest(c, msg)
		}
		return resp.InternalError(c, msg)
	}

	return resp.OK(c, service.FormatSubscriptionForResponse(sub))
}

// Delete handles DELETE /api/admin/webhooks/subscriptions/:id
func (h *WebhookSubscriptionHandler) Delete(c echo.Context) error {
	id, err := service.ParseWebhookID(c.Param("id"))
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	username := "admin"
	if u, ok := c.Get("username").(string); ok && u != "" {
		username = u
	}

	if err := h.svc.Delete(c.Request().Context(), id, username); err != nil {
		code, msg := mapWebhookError(err)
		if code == http.StatusNotFound {
			return c.JSON(http.StatusNotFound, resp.ErrorResponse{Code: http.StatusNotFound, Message: msg})
		}
		return resp.InternalError(c, msg)
	}

	return resp.OK(c, "删除成功")
}

// Toggle handles POST /api/admin/webhooks/subscriptions/:id/toggle
func (h *WebhookSubscriptionHandler) Toggle(c echo.Context) error {
	id, err := service.ParseWebhookID(c.Param("id"))
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	username := "admin"
	if u, ok := c.Get("username").(string); ok && u != "" {
		username = u
	}

	sub, err := h.svc.Toggle(c.Request().Context(), id, username)
	if err != nil {
		code, msg := mapWebhookError(err)
		if code == http.StatusNotFound {
			return c.JSON(http.StatusNotFound, resp.ErrorResponse{Code: http.StatusNotFound, Message: msg})
		}
		return resp.InternalError(c, msg)
	}

	return resp.OK(c, service.FormatSubscriptionForResponse(sub))
}

// TestSend handles POST /api/admin/webhooks/subscriptions/:id/test
func (h *WebhookSubscriptionHandler) TestSend(c echo.Context) error {
	id, err := service.ParseWebhookID(c.Param("id"))
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	if err := h.svc.TestSend(c.Request().Context(), id); err != nil {
		code, msg := mapWebhookError(err)
		if code == http.StatusNotFound {
			return c.JSON(http.StatusNotFound, resp.ErrorResponse{Code: http.StatusNotFound, Message: msg})
		}
		return resp.BadRequest(c, "测试发送失败: "+msg)
	}

	return resp.OK(c, "测试发送成功")
}
