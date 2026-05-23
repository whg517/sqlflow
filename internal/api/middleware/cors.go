package middleware

import (
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// CORS returns a CORS middleware.
// In production, set SQLFLOW_CORS_ORIGINS to a comma-separated list of allowed origins.
// If not set, defaults to permissive mode (suitable for development only).
func CORS() echo.MiddlewareFunc {
	origins := os.Getenv("SQLFLOW_CORS_ORIGINS")
	if origins == "" {
		// Development default: allow all origins
		return middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: []string{"*"},
			AllowMethods: []string{
				echo.GET,
				echo.POST,
				echo.PUT,
				echo.DELETE,
				echo.PATCH,
				echo.OPTIONS,
			},
			AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
			AllowCredentials: false, // Cannot use true with wildcard origins
		})
	}

	// Production: parse comma-separated origins
	allowedOrigins := splitAndTrim(origins)
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: allowedOrigins,
		AllowMethods: []string{
			echo.GET,
			echo.POST,
			echo.PUT,
			echo.DELETE,
			echo.PATCH,
			echo.OPTIONS,
		},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
	})
}

// splitAndTrim splits a comma-separated string and trims whitespace from each element.
func splitAndTrim(s string) []string {
	var result []string
	for _, v := range splitByComma(s) {
		v = trimSpace(v)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

func splitByComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
