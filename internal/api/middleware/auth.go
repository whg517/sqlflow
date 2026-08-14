package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/authz"
	"github.com/whg517/sqlflow/internal/iam"
	"github.com/whg517/sqlflow/internal/platform/auditlog"
	"github.com/whg517/sqlflow/internal/platform/httpx"
	"github.com/whg517/sqlflow/internal/security"
)

// ContextKeyTokenID is the context key for the authenticated API token ID.
const ContextKeyTokenID = "token_id"

// ContextKeyTokenScopes is the context key for the API token scopes.
const ContextKeyTokenScopes = "token_scopes"

// Auth returns a unified middleware that supports both JWT session authentication
// and API Token authentication (tokens prefixed with "sqlflow_").
// The Authorization header is inspected: if the bearer token starts with
// "sqlflow_", it is validated via TokenService; otherwise it is treated as a JWT.
func Auth(authSvc *iam.Service, tokenSvc *iam.TokenService) echo.MiddlewareFunc {
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

			token := parts[1]

			// API Token path: handle tokens prefixed with "sqlflow_"
			if strings.HasPrefix(token, "sqlflow_") {
				userID, username, role, scopes, err := tokenSvc.ValidateTokenWithRole(c.Request().Context(), token)
				if err != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{
						"error": "API Token 无效或已过期",
					})
				}
				c.Set(httpx.ContextKeyUserID, userID)
				c.Set(httpx.ContextKeyUsername, username)
				c.Set(httpx.ContextKeyRole, role)
				c.Set(ContextKeyTokenID, true)
				c.Set(ContextKeyTokenScopes, scopes)
				setAuditActor(c, userID)
				return next(c)
			}

			// JWT path: validate session token
			claims, err := authSvc.ParseToken(token)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "登录已过期，请重新登录",
				})
			}
			user, err := authSvc.GetUserByID(c.Request().Context(), claims.UserID)
			if err != nil || authSvc.ValidateRole(c.Request().Context(), user.Role) != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "用户角色已变更或停用，请重新登录",
				})
			}
			// Reject tokens issued before the most recent password reset.
			// JWT is stateless, so without this check an admin-initiated password
			// reset leaves the user's old access tokens alive until they expire —
			// and a reset is almost always done precisely because the old
			// credential may have leaked.
			//
			// Truncate password_changed_at to second precision before comparing:
			// JWT iat is serialized at second granularity (jwt.NewNumericDate),
			// but the DB timestamp carries sub-second precision. Without
			// truncation, a user created and logged-in within the same second is
			// falsely rejected because password_changed_at (with microseconds)
			// tests as strictly after iat (truncated to the second).
			if claims.IssuedAt != nil && user.PasswordChangedAt.Truncate(time.Second).After(claims.IssuedAt.Time) {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "密码已被重置，请重新登录",
				})
			}
			c.Set(httpx.ContextKeyUserID, user.ID)
			c.Set(httpx.ContextKeyUsername, user.Username)
			c.Set(httpx.ContextKeyRole, user.Role)
			setAuditActor(c, user.ID)

			return next(c)
		}
	}
}

// JWT returns a middleware that validates JWT tokens from the Authorization header.
// NOTE: Prefer Auth() for new code — it handles both JWT and API Token in one middleware.
// This middleware is kept for backward compatibility.
func JWT(authSvc *iam.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip if already authenticated by a preceding middleware (e.g. APITokenAuth)
			if _, ok := c.Get(httpx.ContextKeyUserID).(int64); ok {
				return next(c)
			}

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
			c.Set(httpx.ContextKeyUserID, claims.UserID)
			c.Set(httpx.ContextKeyUsername, claims.Username)
			c.Set(httpx.ContextKeyRole, claims.Role)

			return next(c)
		}
	}
}

// RequireScope returns a middleware that checks if the authenticated
// API token has the required scope.
func RequireScope(requiredScope string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// If authenticated via JWT (normal session), bypass scope check
			_, isToken := c.Get(ContextKeyTokenID).(bool)
			if !isToken {
				return next(c)
			}

			scopes, _ := c.Get(ContextKeyTokenScopes).([]string)
			if !iam.HasScope(scopes, requiredScope) {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "API Token 权限不足",
				})
			}

			return next(c)
		}
	}
}

// Admin returns a middleware that checks if the authenticated user has admin role.
func Admin() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, ok := c.Get(httpx.ContextKeyRole).(string)
			if !ok || role != "admin" {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "权限不足，需要管理员权限",
				})
			}
			return next(c)
		}
	}
}

// SystemPermission authorizes a platform-management operation through Casbin.
// The built-in admin wildcard remains effective, while custom roles can receive
// narrowly delegated control-plane permissions.
func SystemPermission(permissionSvc *security.Service, object, action string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, _ := c.Get(httpx.ContextKeyRole).(string)
			allowed, err := permissionSvc.Enforce(role, "system", object, action)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{
					"error": "权限校验失败",
				})
			}
			if !allowed {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "权限不足",
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
			role, _ := c.Get(httpx.ContextKeyRole).(string)
			sub, dom, obj, act, err := authz.NormalizeTuple(
				role,
				extractDatasource(c),
				extractTable(c),
				action,
			)
			if err != nil {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "权限请求参数无效",
				})
			}

			ok, err := enforcer.Enforce(sub, dom, obj, act)
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
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return ""
	}
}

// setAuditActor puts the authenticated user's id on the request context.
//
// Identity already reaches handlers through echo's own context, but services
// receive a plain context.Context. Attaching it here means a domain can name
// who acted without every one of its methods taking a userID parameter it
// otherwise has no use for — and, more importantly, without each handler having
// to remember to pass one.
func setAuditActor(c echo.Context, userID int64) {
	req := c.Request()
	c.SetRequest(req.WithContext(auditlog.WithActor(req.Context(), userID)))
}
