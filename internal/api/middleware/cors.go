package middleware

import (
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// CORS returns a CORS middleware.
// In production, set CORS_ALLOWED_ORIGINS env var (comma-separated).
// Defaults to allowing all origins when CORS_ALLOWED_ORIGINS is not set.
func CORS() echo.MiddlewareFunc {
	allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		// Development mode: allow all origins
		return middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: []string{"*"},
			AllowMethods: []string{
				echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.PATCH, echo.OPTIONS,
			},
			AllowHeaders: []string{"*"},
		})
	}

	origins := strings.FieldsFunc(allowedOrigins, func(r rune) bool { return r == ',' })
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: origins,
		AllowMethods: []string{
			echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.PATCH, echo.OPTIONS,
		},
		AllowHeaders:     []string{"*"},
		AllowCredentials: true,
	})
}
