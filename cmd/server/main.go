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
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/service"
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

	// Initialize services
	authSvc := service.NewAuthService(database.DB, cfg.JWT.Secret, cfg.JWT.Expiry)

	// Initialize connection pool manager
	connMgr := connpool.NewManager()
	defer connMgr.Close()

	dsSvc := service.NewDatasourceService(database.DB, cfg.EncryptionKey, connMgr)
	permSvc, err := service.NewPermissionService(database.DB)
	if err != nil {
		log.Fatalf("failed to initialize permission service: %v", err)
	}
	log.Println("permission service initialized")

	historySvc := service.NewQueryHistoryService(database.DB)

	auditSvc := service.NewAuditService(database.DB, 0, 0)
	defer auditSvc.Close()
	log.Println("audit service initialized")

	exportSvc := service.NewExportService(database.DB, auditSvc)
	log.Println("export service initialized")

	exportAsyncSvc := service.NewExportAsyncService(database.DB, exportSvc, auditSvc, cfg.DB.Path)
	defer exportAsyncSvc.Close()
	log.Println("async export service initialized")

	querySvc := service.NewQueryService(database.DB, dsSvc, historySvc, permSvc, auditSvc, cfg.EncryptionKey, connMgr)
	log.Println("query service initialized")

	ticketSvc := service.NewTicketService(database.DB, auditSvc, nil)
	log.Println("ticket service initialized")

	// Initialize and start the ticket scheduler
	scheduler := service.NewScheduler(ticketSvc, 1*time.Minute)
	scheduler.Start()
	defer scheduler.Stop()
	log.Println("ticket scheduler started")

	notifySvc := service.NewNotifyService(cfg.DingTalk.WebhookURL, cfg.DingTalk.Secret)
	log.Println("notify service initialized")
	ticketSvc.SetNotifyService(notifySvc)

	maskRuleSvc := service.NewMaskRuleService(database.DB, permSvc, auditSvc)
	log.Println("mask rule service initialized")

	aiReviewSvc := service.NewAIReviewService(database.DB, cfg.AI.Provider, cfg.AI.Model, cfg.AI.APIKey, cfg.AI.BaseURL, cfg.AI.Timeout)
	log.Println("AI review service initialized")

	dashboardSvc := service.NewDashboardService(database.DB)
	log.Println("dashboard service initialized")

	// Initialize backup service
	backupSvc := service.NewBackupService(database.DB, cfg.DB.Path, cfg.Backup)
	log.Println("backup service initialized")

	// Initialize audit report service
	reportSvc := service.NewAuditReportService(database.DB)
	log.Println("audit report service initialized")

	// Initialize permission request service
	permReqSvc := service.NewPermissionRequestService(database.DB, permSvc, auditSvc)
	log.Println("permission request service initialized")

	commentSvc := service.NewCommentService(database.DB)
	log.Println("comment service initialized")

	// Initialize DingTalk OAuth service
	dingOAuthSvc := service.NewDingTalkOAuthService(
		database.DB, authSvc,
		cfg.DingTalk.OAuth.AppKey,
		cfg.DingTalk.OAuth.AppSecret,
		cfg.DingTalk.OAuth.RedirectURL,
		cfg.DingTalk.OAuth.Enabled,
	)
	log.Println("dingtalk oauth service initialized")

	// Initialize git service
	gitSvc := service.NewGitService(database.DB)
	log.Println("git service initialized")
	ticketSvc.SetGitService(gitSvc)

	// Initialize SLA service and scheduler (single-instance, constructor injection)
	slaSvc := service.NewSLAService(database.DB, notifySvc)
	ticketSvc.SetSLAService(slaSvc)

	slaScheduler := service.NewSLAScheduler(slaSvc, 10*time.Minute)
	slaScheduler.Start()
	defer slaScheduler.Stop()
	log.Println("SLA scheduler started (interval=10m)")

	// Initialize API token service
	tokenSvc := service.NewTokenService(database.DB)
	log.Println("api token service initialized")

	// Initialize SQL template service
	templateSvc := service.NewSQLTemplateService(database.DB)
	log.Println("sql template service initialized")

	// Initialize share service (SF-FEAT0038)
	shareSvc := service.NewShareService(database.DB)
	log.Println("share service initialized")

	// Initialize Web Vitals service (SF-ENG0033)
	vitalsSvc := service.NewWebVitalsService(database.DB)
	log.Println("web vitals service initialized")

	// Seed initial admin if users table is empty
	count, err := authSvc.UserCount(context.Background())
	if err != nil {
		log.Fatalf("failed to count users: %v", err)
	}
	if count == 0 {
		admin, err := authSvc.CreateUser(context.Background(), cfg.Admin.Username, cfg.Admin.Password, "admin")
		if err != nil {
			log.Fatalf("failed to create admin user: %v", err)
		}
		log.Printf("initial admin user created: %s (id=%d)", admin.Username, admin.ID)
	} else {
		log.Println("admin user already exists, skipping seed")
	}

	// Start backup scheduler
	backupSvc.Start()
	defer backupSvc.Stop()

	// Start server
	e := api.NewRouter(authSvc, dsSvc, permSvc, querySvc, historySvc, ticketSvc, maskRuleSvc, aiReviewSvc, auditSvc, exportSvc, exportAsyncSvc, notifySvc, dashboardSvc, commentSvc, dingOAuthSvc, backupSvc, gitSvc, tokenSvc, reportSvc, permReqSvc, templateSvc, shareSvc, vitalsSvc, database.DB, cfg)

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
