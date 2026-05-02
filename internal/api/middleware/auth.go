package middleware

import (
	"net/http"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/service"
)

const (
	ContextKeyUserID   = "user_id"
	ContextKeyUsername = "username"
	ContextKeyRole     = "role"
)

// JWT returns a middleware that validates JWT tokens from the Authorization header.
func JWT(authSvc *service.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "登录已过期，请重新登录",
				})
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "登录已过期，请重新登录",
				})
			}

			claims, err := authSvc.ParseToken(parts[1])
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "登录已过期，请重新登录",
				})
			}

			c.Set(ContextKeyUserID, claims.UserID)
			c.Set(ContextKeyUsername, claims.Username)
			c.Set(ContextKeyRole, claims.Role)

			return next(c)
		}
	}
}

// Admin returns a middleware that checks if the authenticated user has admin role.
func Admin() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, ok := c.Get(ContextKeyRole).(string)
			if !ok || role != "admin" {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "权限不足，需要管理员权限",
				})
			}
			return next(c)
		}
	}
}

// Permission returns a middleware that checks Casbin RBAC permission.
// action is the required action (e.g. "select", "update", "delete", "ddl", "export").
// The middleware reads dom from query param "datasource" or form field, and obj from "table".
func Permission(enforcer *casbin.Enforcer, action string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, _ := c.Get(ContextKeyRole).(string)
			dom := c.QueryParam("datasource")
			if dom == "" {
				dom = c.FormValue("datasource")
			}
			obj := c.QueryParam("table")
			if obj == "" {
				obj = c.FormValue("table")
			}

			ok, err := enforcer.Enforce(role, dom, obj, action)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{
					"error": "权限校验失败",
				})
			}
			if !ok {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "权限不足",
				})
			}
			return next(c)
		}
	}
}
