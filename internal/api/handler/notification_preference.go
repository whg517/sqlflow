package handler

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// NotificationPreferenceHandler handles notification preference endpoints.
type NotificationPreferenceHandler struct {
	prefSvc *service.NotificationPreferenceService
}

// NewNotificationPreferenceHandler creates a new handler.
func NewNotificationPreferenceHandler(prefSvc *service.NotificationPreferenceService) *NotificationPreferenceHandler {
	return &NotificationPreferenceHandler{prefSvc: prefSvc}
}

// GetPreferences handles GET /api/notifications/preferences.
func (h *NotificationPreferenceHandler) GetPreferences(c echo.Context) error {
	userID := getContextUserID(c)

	prefs, err := h.prefSvc.GetPreferences(c.Request().Context(), userID)
	if err != nil {
		log.Printf("GetPreferences failed: %v", err)
		return resp.BadRequest(c, err.Error())
	}
	if prefs == nil {
		prefs = []service.NotificationPreference{}
	}
	return resp.OK(c, prefs)
}

// UpdatePreferences handles PUT /api/notifications/preferences.
func (h *NotificationPreferenceHandler) UpdatePreferences(c echo.Context) error {
	userID := getContextUserID(c)

	var req service.UpdatePreferencesRequest
	if err := c.Bind(&req); err != nil {
		return resp.BadRequest(c, "请求格式错误")
	}
	if len(req.Preferences) == 0 {
		return resp.BadRequest(c, "偏好列表不能为空")
	}

	if err := h.prefSvc.UpdatePreferences(c.Request().Context(), userID, req); err != nil {
		return resp.BadRequest(c, err.Error())
	}

	// Return updated preferences
	prefs, _ := h.prefSvc.GetPreferences(c.Request().Context(), userID)
	return resp.OK(c, prefs)
}
