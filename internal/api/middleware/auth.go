package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
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
// The middleware reads dom (datasource) and obj (table) from, in priority order:
//  1. Path params: "datasource_id", "id" (for datasource); "table" (for table)
//  2. JSON body: "datasource_id" and "database"/"table_name" fields
//  3. Query params: "datasource" and "table" (fallback)
func Permission(enforcer *casbin.Enforcer, action string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, _ := c.Get(ContextKeyRole).(string)
			dom := extractDatasource(c)
			obj := extractTable(c)

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

// extractDatasource reads the datasource identifier from path params, JSON body, or query params.
func extractDatasource(c echo.Context) string {
	// 1. Path params
	if v := c.Param("datasource_id"); v != "" {
		return v
	}
	if v := c.Param("id"); v != "" {
		return v
	}

	// 2. JSON body
	if body := peekBody(c); body != nil {
		if id := bodyField(body, "datasource_id"); id != "" {
			return id
		}
	}

	// 3. Query param fallback
	return c.QueryParam("datasource")
}

// extractTable reads the table name from path params, JSON body, or query params.
func extractTable(c echo.Context) string {
	// 1. Path params
	if v := c.Param("table"); v != "" {
		return v
	}

	// 2. JSON body
	if body := peekBody(c); body != nil {
		if v, ok := body["table_name"].(string); ok && v != "" {
			return v
		}
		if v, ok := body["database"].(string); ok && v != "" {
			return v
		}
	}

	// 3. Query param fallback
	return c.QueryParam("table")
}

// peekBody reads and caches the JSON request body as a map.
// The body bytes are restored so downstream handlers can still read them.
func peekBody(c echo.Context) map[string]interface{} {
	if cached, ok := c.Get("__perm_body").(map[string]interface{}); ok {
		return cached
	}

	ct := c.Request().Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		return nil
	}

	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil || len(bodyBytes) == 0 {
		return nil
	}
	// Restore body so downstream handlers can re-read it
	c.Request().Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

	var body map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return nil
	}

	c.Set("__perm_body", body)
	return body
}

// bodyField extracts a string value from the parsed body map, handling both
// string and numeric types (e.g. datasource_id may be a JSON number).
func bodyField(body map[string]interface{}, key string) string {
	v, ok := body[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strings.TrimRight(strings.TrimRight(
			json.Number(strconv.FormatFloat(val, 'f', -1, 64)).String(), "0"), ".")
	default:
		return ""
	}
}
