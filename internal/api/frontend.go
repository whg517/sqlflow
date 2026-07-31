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
	serveFrontendFromDir(e, "web/dist")
}

func serveFrontendFromDir(e *echo.Echo, distDir string) {
	stat, err := os.Stat(distDir)
	if os.IsNotExist(err) || !stat.IsDir() {
		log.Printf("[WARN] frontend dist not found at %s, SPA serving disabled", distDir)
		return
	}

	indexPath := distDir + "/index.html"
	if _, err := os.Stat(indexPath); err != nil {
		log.Printf("[WARN] failed to read index.html: %v", err)
		return
	}

	// Create a file server for static assets
	fs := http.FileServer(http.Dir(distDir))

	serveIndex := func(c echo.Context) error {
		// Read on every request so a production rebuild cannot leave the running
		// process pointing at deleted, content-hashed assets.
		indexHTML, err := os.ReadFile(indexPath)
		if err != nil {
			log.Printf("[WARN] failed to read index.html: %v", err)
			return c.NoContent(http.StatusInternalServerError)
		}
		c.Response().Header().Set(echo.HeaderCacheControl, "no-cache, no-store, must-revalidate")
		c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
		return c.HTMLBlob(http.StatusOK, indexHTML)
	}

	// Register a PRE middleware that intercepts frontend requests
	// before any route matching or auth middleware runs
	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path

			// Mutating requests always belong to backend routes. SPA navigation
			// only uses GET/HEAD.
			if c.Request().Method != http.MethodGet && c.Request().Method != http.MethodHead {
				return next(c)
			}

			// Skip non-SPA routes - let Echo handle them normally
			if strings.HasPrefix(path, "/api/") ||
				strings.HasPrefix(path, "/swagger/") ||
				path == "/health" ||
				path == "/healthz" ||
				path == "/readyz" ||
				path == "/metrics" {
				return next(c)
			}

			// /s/:token is both a browser page and a public result API.
			// Browser navigation asks for HTML; fetch() asks for the JSON route.
			if strings.HasPrefix(path, "/s/") &&
				!strings.Contains(c.Request().Header.Get(echo.HeaderAccept), echo.MIMETextHTML) {
				return next(c)
			}

			// Try to serve static file first
			if path != "/" {
				filePath := distDir + path
				if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
					if strings.HasPrefix(path, "/assets/") {
						c.Response().Header().Set(
							echo.HeaderCacheControl,
							"public, max-age=31536000, immutable",
						)
					}
					fs.ServeHTTP(c.Response().Writer, c.Request())
					return nil
				}
			}

			// Missing hashed bundles are genuine 404s. Returning index.html here
			// makes the browser reject the module because its MIME type is HTML.
			if strings.HasPrefix(path, "/assets/") {
				return c.NoContent(http.StatusNotFound)
			}

			// SPA fallback: serve the current index.html for frontend routes.
			return serveIndex(c)
		}
	})

	log.Printf("frontend SPA enabled, serving from %s", distDir)
}
