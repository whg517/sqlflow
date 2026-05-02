package middleware

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Recovery returns an Echo recovery middleware that recovers from panics.
func Recovery() echo.MiddlewareFunc {
	return middleware.Recover()
}
