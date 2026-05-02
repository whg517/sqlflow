package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
)

// NewRouter creates and configures an Echo instance with middleware and routes.
func NewRouter() *echo.Echo {
	e := echo.New()

	// Middleware
	e.Use(middleware.Recovery())
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	// Health check
	e.GET("/api/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	return e
}
