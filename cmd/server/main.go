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
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/api"
	"github.com/whg517/sqlflow/internal/app"
	"github.com/whg517/sqlflow/internal/db"
)

const (
	// shutdownTimeout bounds how long in-flight requests have to finish.
	shutdownTimeout = 10 * time.Second

	// Server timeouts. Only the HTTP→HTTPS redirect listener used to have any,
	// so the server that does the actual work would hold a slow or idle
	// connection open indefinitely.
	//
	// The write timeout is the loose one on purpose: an export streams its rows
	// through the same server, and the query path is bounded at 30 seconds by
	// the service.
	serverReadTimeout  = 30 * time.Second
	serverWriteTimeout = 5 * time.Minute
	serverIdleTimeout  = 2 * time.Minute
)

// main does nothing but decide the exit code.
//
// Everything else is in run, because log.Fatalf calls os.Exit and os.Exit does
// not run deferred functions. Both server branches used to end in Fatalf, so a
// clean SIGTERM shutdown still skipped container.Close() and database.Close():
// the schedulers kept running and the pool was never drained. Worse, a
// successful graceful shutdown makes StartTLS return http.ErrServerClosed,
// which landed in that same Fatalf — exit code 1 for a shutdown that worked.
func main() {
	if err := run(); err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
	log.Println("shutdown complete")
}

func run() error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	database, err := db.Open(cfg.DB.DSN)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	if err := database.Migrate(); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	log.Println("database migrated successfully")

	container, err := app.NewContainer(database, cfg)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer container.Close()
	log.Println("application container initialized")

	return serve(api.NewRouter(container), cfg)
}

// serve runs the HTTP server until it fails or a signal arrives.
//
// TLS and plain HTTP share this path. They used to be two branches, and only
// the TLS one installed a signal handler — so in the deployment mode the
// project actually defaults to, SIGTERM killed the process outright with
// requests in flight and schedulers mid-tick.
func serve(e *echo.Echo, cfg *config.Config) error {
	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	e.Server.ReadTimeout = serverReadTimeout
	e.Server.WriteTimeout = serverWriteTimeout
	e.Server.IdleTimeout = serverIdleTimeout
	e.TLSServer.ReadTimeout = serverReadTimeout
	e.TLSServer.WriteTimeout = serverWriteTimeout
	e.TLSServer.IdleTimeout = serverIdleTimeout

	var redirect *http.Server
	if cfg.Server.TLS.Enable && cfg.Server.TLS.RedirectHTTP {
		redirect = newHTTPRedirect(cfg.Server.TLS.HTTPPort)
		go func() {
			log.Printf("starting HTTP→HTTPS redirect server on %s", redirect.Addr)
			if err := redirect.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("HTTP redirect server error: %v", err)
			}
		}()
	}

	served := make(chan error, 1)
	go func() {
		if cfg.Server.TLS.Enable {
			log.Printf("starting HTTPS server on %s", addr)
			served <- e.StartTLS(addr, cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
			return
		}
		log.Printf("starting HTTP server on %s", addr)
		served <- e.Start(addr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-served:
		// The server stopped on its own — a bind failure, say. A closed server
		// here means something else shut it down, which is not an error either.
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server: %w", err)

	case sig := <-quit:
		log.Printf("received %s, shutting down", sig)

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if redirect != nil {
			if err := redirect.Shutdown(ctx); err != nil {
				log.Printf("redirect server shutdown: %v", err)
			}
		}
		if err := e.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown: %w", err)
		}

		// Wait for the serve goroutine, so the deferred Close calls in run do
		// not race a handler that is still returning.
		if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	}
}

// newHTTPRedirect builds the listener that sends plain HTTP to HTTPS.
func newHTTPRedirect(httpPort int) *http.Server {
	return &http.Server{
		Addr: fmt.Sprintf(":%d", httpPort),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
		}),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	}
}
