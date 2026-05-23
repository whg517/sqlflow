package api

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

// serveFrontend configures SPA serving for the frontend.
// Uses a pre-routing middleware to serve static files and SPA fallback
// before Echo's route matching kicks in, avoiding auth middleware conflicts.
func serveFrontend(e *echo.Echo) {
	distDir := "web/dist"

	stat, err := os.Stat(distDir)
	if os.IsNotExist(err) || !stat.IsDir() {
		log.Printf("[WARN] frontend dist not found at %s, SPA serving disabled", distDir)
		return
	}

	// Read index.html for SPA fallback
	indexHTML, err := os.ReadFile(distDir + "/index.html")
	if err != nil {
		log.Printf("[WARN] failed to read index.html: %v", err)
		return
	}

	// Create a file server for static assets
	fs := http.FileServer(http.Dir(distDir))

	// Register a PRE middleware that intercepts frontend requests
	// before any route matching or auth middleware runs
	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path

			// Skip API routes - let Echo handle them normally
			if strings.HasPrefix(path, "/api/") {
				return next(c)
			}

			// Try to serve static file first
			if path != "/" {
				filePath := distDir + path
				if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
					fs.ServeHTTP(c.Response().Writer, c.Request())
					return nil
				}
			}

			// SPA fallback: serve index.html for all other non-API routes
			c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
			c.Response().WriteHeader(http.StatusOK)
			c.Response().Write(indexHTML)
			return nil
		}
	})

	log.Printf("frontend SPA enabled, serving from %s", distDir)
}
