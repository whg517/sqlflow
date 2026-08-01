package notify

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/platform/httpx"
	"github.com/whg517/sqlflow/internal/resp"
)

// PreferenceHandler handles notification preference endpoints.
type PreferenceHandler struct {
	prefSvc *PreferenceService
}

// NewPreferenceHandler creates a new handler.
func NewPreferenceHandler(prefSvc *PreferenceService) *PreferenceHandler {
	return &PreferenceHandler{prefSvc: prefSvc}
}

// GetPreferences handles GET /api/notifications/preferences.
func (h *PreferenceHandler) GetPreferences(c echo.Context) error {
	userID := httpx.UserID(c)

	prefs, err := h.prefSvc.GetPreferences(c.Request().Context(), userID)
	if err != nil {
		log.Printf("GetPreferences failed: %v", err)
		return resp.BadRequest(c, err.Error())
	}
	if prefs == nil {
		prefs = []Preference{}
	}
	return resp.OK(c, prefs)
}

// UpdatePreferences handles PUT /api/notifications/preferences.
func (h *PreferenceHandler) UpdatePreferences(c echo.Context) error {
	userID := httpx.UserID(c)

	var req UpdatePreferencesRequest
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
