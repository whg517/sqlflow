package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
)

// ============================================================
// Safe context value extraction — prevents panic on nil/missing values
// ============================================================

// getContextUserID safely extracts user_id from echo context.
// Returns 0 if the value is missing or wrong type (e.g., expired context).
func getContextUserID(c echo.Context) int64 {
	v := c.Get(middleware.ContextKeyUserID)
	if v == nil {
		return 0
	}
	id, ok := v.(int64)
	if !ok {
		return 0
	}
	return id
}

// getContextUsername safely extracts username from echo context.
func getContextUsername(c echo.Context) string {
	v := c.Get(middleware.ContextKeyUsername)
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// getContextRole safely extracts role from echo context.
func getContextRole(c echo.Context) string {
	v := c.Get(middleware.ContextKeyRole)
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// requireAuth checks that user context values are present.
// Returns error response if any are missing (e.g., auth middleware not applied,
// expired context, or internal error).
func requireAuth(c echo.Context) (userID int64, username, role string, err error) {
	userID = getContextUserID(c)
	username = getContextUsername(c)
	role = getContextRole(c)

	if userID == 0 || username == "" {
		return 0, "", "", echo.NewHTTPError(http.StatusUnauthorized, "登录已过期，请重新登录")
	}
	return userID, username, role, nil
}
