// Package httpx carries the authenticated caller across the HTTP boundary.
//
// The auth middleware writes the identity; every domain's handlers read it. It
// lives in platform rather than in either of them so a domain package can serve
// HTTP without importing internal/api, which would invert the dependency and
// pull the whole transport layer into the domain.
//
// This package must not import any domain package.
package httpx

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Keys under which the auth middleware stores the caller's identity.
const (
	ContextKeyUserID   = "user_id"
	ContextKeyUsername = "username"
	ContextKeyRole     = "role"
)

// UserID returns the authenticated user's ID, or 0 when the request carries no
// identity.
//
// The accessors return zero values instead of an error because a missing
// identity is not distinguishable here from a malformed one, and both mean the
// same thing to a handler: the caller is not authenticated. Handlers that need
// an identity must call RequireAuth, which turns that into a 401 — reading 0 and
// proceeding would silently attribute the action to no one.
func UserID(c echo.Context) int64 {
	id, _ := c.Get(ContextKeyUserID).(int64)
	return id
}

// Username returns the authenticated user's name, or "" when absent.
func Username(c echo.Context) string {
	s, _ := c.Get(ContextKeyUsername).(string)
	return s
}

// Role returns the authenticated user's role, or "" when absent.
func Role(c echo.Context) string {
	s, _ := c.Get(ContextKeyRole).(string)
	return s
}

// RequireAuth returns the caller's identity, or a 401 when the request has none.
func RequireAuth(c echo.Context) (userID int64, username, role string, err error) {
	userID, username, role = UserID(c), Username(c), Role(c)
	if userID == 0 || username == "" {
		return 0, "", "", echo.NewHTTPError(http.StatusUnauthorized, "登录已过期，请重新登录")
	}
	return userID, username, role, nil
}
