package notify

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/platform/httpx"
	"github.com/whg517/sqlflow/internal/resp"
)

// SubscriptionHandler handles outbound webhook subscription CRUD API.
type SubscriptionHandler struct {
	svc *WebhookSubscriptionService
}

// NewSubscriptionHandler creates a new handler.
func NewSubscriptionHandler(svc *WebhookSubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{svc: svc}
}

// mapError maps service errors to appropriate HTTP status codes.
func mapWebhookError(err error) (int, string) {
	if IsNotFoundError(err) {
		return http.StatusNotFound, err.Error()
	}
	if IsValidationError(err) {
		return http.StatusBadRequest, err.Error()
	}
	// Default: internal error for unexpected failures
	return http.StatusInternalServerError, err.Error()
}

// Create handles POST /api/admin/webhooks/subscriptions
func (h *SubscriptionHandler) Create(c echo.Context) error {
	var req CreateSubscriptionRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	// No fallback identity. These routes sit behind the admin group, so a
	// request without one is a wiring fault — and an audit trail that invents an
	// actor ("admin") is worse than one that records none.
	actorID, actorName := httpx.UserID(c), httpx.Username(c)

	sub, plainSecret, err := h.svc.Create(c.Request().Context(), req, actorID, actorName)
	if err != nil {
		code, msg := mapWebhookError(err)
		if code == http.StatusBadRequest {
			return resp.BadRequest(c, msg)
		}
		return resp.InternalError(c, msg)
	}

	result := FormatSubscriptionForResponse(sub)
	result["secret"] = plainSecret // Only returned once on creation
	return resp.OK(c, result)
}

// List handles GET /api/admin/webhooks/subscriptions
func (h *SubscriptionHandler) List(c echo.Context) error {
	subs, err := h.svc.List(c.Request().Context())
	if err != nil {
		return resp.InternalError(c, "获取订阅列表失败")
	}

	items := make([]map[string]interface{}, 0, len(subs))
	for _, sub := range subs {
		items = append(items, FormatSubscriptionForResponse(sub))
	}

	return resp.OK(c, items)
}

// Get handles GET /api/admin/webhooks/subscriptions/:id
func (h *SubscriptionHandler) Get(c echo.Context) error {
	id, err := ParseWebhookID(c.Param("id"))
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

	return resp.OK(c, FormatSubscriptionForResponse(sub))
}

// Update handles PUT /api/admin/webhooks/subscriptions/:id
func (h *SubscriptionHandler) Update(c echo.Context) error {
	id, err := ParseWebhookID(c.Param("id"))
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	var req UpdateSubscriptionRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}

	actorID, actorName := httpx.UserID(c), httpx.Username(c)

	sub, err := h.svc.Update(c.Request().Context(), id, req, actorID, actorName)
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

	return resp.OK(c, FormatSubscriptionForResponse(sub))
}

// Delete handles DELETE /api/admin/webhooks/subscriptions/:id
func (h *SubscriptionHandler) Delete(c echo.Context) error {
	id, err := ParseWebhookID(c.Param("id"))
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	actorID, actorName := httpx.UserID(c), httpx.Username(c)

	if err := h.svc.Delete(c.Request().Context(), id, actorID, actorName); err != nil {
		code, msg := mapWebhookError(err)
		if code == http.StatusNotFound {
			return c.JSON(http.StatusNotFound, resp.ErrorResponse{Code: http.StatusNotFound, Message: msg})
		}
		return resp.InternalError(c, msg)
	}

	return resp.OK(c, "删除成功")
}

// Toggle handles POST /api/admin/webhooks/subscriptions/:id/toggle
func (h *SubscriptionHandler) Toggle(c echo.Context) error {
	id, err := ParseWebhookID(c.Param("id"))
	if err != nil {
		return resp.BadRequest(c, "无效的 ID")
	}

	actorID, actorName := httpx.UserID(c), httpx.Username(c)

	sub, err := h.svc.Toggle(c.Request().Context(), id, actorID, actorName)
	if err != nil {
		code, msg := mapWebhookError(err)
		if code == http.StatusNotFound {
			return c.JSON(http.StatusNotFound, resp.ErrorResponse{Code: http.StatusNotFound, Message: msg})
		}
		return resp.InternalError(c, msg)
	}

	return resp.OK(c, FormatSubscriptionForResponse(sub))
}

// TestSend handles POST /api/admin/webhooks/subscriptions/:id/test
func (h *SubscriptionHandler) TestSend(c echo.Context) error {
	id, err := ParseWebhookID(c.Param("id"))
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
