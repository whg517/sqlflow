package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/api/handler"
	"github.com/whg517/sqlflow/internal/service"
)

// NewRouter creates and configures an Echo instance with middleware and routes.
func NewRouter(authSvc *service.AuthService) *echo.Echo {
	e := echo.New()

	// Global middleware
	e.Use(middleware.Recovery())
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	// Health check (public)
	e.GET("/api/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Auth handlers
	userHandler := handler.NewUserHandler(authSvc)

	// Public routes
	e.POST("/api/auth/login", userHandler.Login)

	// Authenticated routes
	authGroup := e.Group("", middleware.JWT(authSvc))
	authGroup.GET("/api/auth/me", userHandler.Me)
	authGroup.PUT("/api/auth/password", userHandler.ChangePassword)

	// Admin-only routes
	adminGroup := e.Group("", middleware.JWT(authSvc), middleware.Admin())
	adminGroup.POST("/api/users", userHandler.CreateUser)
	adminGroup.GET("/api/users", userHandler.ListUsers)
	adminGroup.GET("/api/users/:id", userHandler.GetUser)
	adminGroup.PUT("/api/users/:id", userHandler.UpdateUser)
	adminGroup.DELETE("/api/users/:id", userHandler.DeleteUser)
	adminGroup.PUT("/api/users/:id/reset-password", userHandler.ResetPassword)

	return e
}
