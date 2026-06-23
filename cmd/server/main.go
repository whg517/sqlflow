// @title SQLFlow API
// @version 1.0
// @description SQL 审核、查询和工单管理平台 API
// @host localhost:8080
// @BasePath /api
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/api"
	"github.com/whg517/sqlflow/internal/app"
	"github.com/whg517/sqlflow/internal/db"
	_ "github.com/whg517/sqlflow/internal/driver/elasticsearch"
	_ "github.com/whg517/sqlflow/internal/driver/mongodb"
	_ "github.com/whg517/sqlflow/internal/driver/mysql"
	_ "github.com/whg517/sqlflow/internal/driver/postgresql"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize database
	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("failed to close database: %v", err)
		}
	}()

	if err := database.Migrate(); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}
	log.Println("database migrated successfully")

	// Initialize application container (services, schedulers, circular deps, admin seed)
	container, err := app.NewContainer(database, cfg)
	if err != nil {
		log.Fatalf("failed to initialize application: %v", err)
	}
	defer container.Close()
	log.Println("application container initialized")

	e := api.NewRouter(container)

	if cfg.Server.TLS.Enable {
		// TLS mode: start HTTPS server
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		log.Printf("starting HTTPS server on %s", addr)

		// Optionally start HTTP→HTTPS redirect listener
		if cfg.Server.TLS.RedirectHTTP {
			go startHTTPRedirect(cfg.Server.TLS.HTTPPort, cfg.Server.Port)
		}

		// Graceful shutdown
		go func() {
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			<-quit
			log.Println("shutting down server...")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := e.Shutdown(ctx); err != nil {
				log.Printf("server shutdown error: %v", err)
			}
		}()

		if err := e.StartTLS(addr, cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil {
			log.Fatalf("failed to start TLS server: %v", err)
		}
	} else {
		// Plain HTTP mode (backward compatible)
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		log.Printf("starting HTTP server on %s", addr)
		if err := e.Start(addr); err != nil {
			log.Fatalf("failed to start server: %v", err)
		}
	}
}

// startHTTPRedirect starts a lightweight HTTP server that redirects all traffic to HTTPS.
func startHTTPRedirect(httpPort, httpsPort int) {
	redirectAddr := fmt.Sprintf(":%d", httpPort)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := fmt.Sprintf("https://%s%s", r.Host, r.URL.RequestURI())
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})

	srv := &http.Server{
		Addr:         redirectAddr,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	}

	log.Printf("starting HTTP→HTTPS redirect server on %s (→ :%d)", redirectAddr, httpsPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("HTTP redirect server error: %v", err)
	}
}
