package api

import (
	"github.com/whg517/sqlflow/internal/db"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/api/handler"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/pkg/metrics"
	"github.com/whg517/sqlflow/internal/service"

	echoSwagger "github.com/swaggo/echo-swagger"
	_ "github.com/whg517/sqlflow/docs"
)

// NewRouter creates and configures an Echo instance with middleware and routes.
func NewRouter(authSvc *service.AuthService, dsSvc *service.DatasourceService, permSvc *service.PermissionService, querySvc *service.QueryService, historySvc *service.QueryHistoryService, ticketSvc *service.TicketService, maskRuleSvc *service.MaskRuleService, aiReviewSvc *service.AIReviewService, auditSvc *service.AuditService, exportSvc *service.ExportService, exportAsyncSvc *service.ExportAsyncService, notifySvc *service.NotifyService, dashboardSvc *service.DashboardService, commentSvc *service.CommentService, oidcSvc *service.OIDCService, backupSvc *service.BackupService, gitSvc *service.GitService, tokenSvc *service.TokenService, reportSvc *service.AuditReportService, permReqSvc *service.PermissionRequestService, templateSvc *service.TemplateService, shareSvc *service.ShareService, vitalsSvc *service.WebVitalsService, approvalEngine *service.ApprovalEngine, database *db.DB, cfg *config.Config, connMgr *connpool.Manager, poolMgr *driver.PoolManager) *echo.Echo {
	e := echo.New()

	// Global middleware
	e.Use(middleware.Recovery())
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	// Prometheus metrics middleware
	if cfg.Metrics.Enabled {
		e.Use(metrics.Middleware())
	}

	// Health check endpoints (public)
	healthHandler := handler.NewHealthHandler(database.DB)
	healthHandler.SetConnPoolManager(connMgr)
	healthHandler.SetPoolManager(poolMgr)
	e.GET("/health", healthHandler.Health)
	e.GET("/healthz", healthHandler.Healthz)   // Liveness probe (no dependency checks)
	e.GET("/readyz", healthHandler.Readyz)     // Readiness probe (checks all deps)
	e.GET("/api/health", healthHandler.Health)

	// Prometheus metrics endpoint
	if cfg.Metrics.Enabled {
		e.GET("/metrics", healthHandler.Metrics)
	}

	// Swagger UI
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	// Auth handlers
	userHandler := handler.NewUserHandler(authSvc)
	dsHandler := handler.NewDatasourceHandler(dsSvc)
	permHandler := handler.NewPermissionHandler(permSvc)
	queryHandler := handler.NewQueryHandler(querySvc, historySvc)
	ticketHandler := handler.NewTicketHandler(ticketSvc)
	approvalHandler := handler.NewApprovalHandler(approvalEngine)
	maskRuleHandler := handler.NewMaskRuleHandler(maskRuleSvc)
	aiReviewHandler := handler.NewAIReviewHandler(aiReviewSvc, dsSvc)
	auditHandler := handler.NewAuditHandler(auditSvc)
	exportHandler := handler.NewExportHandler(exportSvc, exportAsyncSvc)
	dashboardHandler := handler.NewDashboardHandler(dashboardSvc)
	backupHandler := handler.NewBackupHandler(backupSvc)
	performanceHandler := handler.NewPerformanceHandler(historySvc)
	gitHandler := handler.NewGitHandler(gitSvc)
	tokenHandler := handler.NewTokenHandler(tokenSvc)
	reportHandler := handler.NewAuditReportHandler(reportSvc)
	permReqHandler := handler.NewPermReqHandler(permReqSvc)
	sqlTemplateHandler := handler.NewSQLTemplateHandler(templateSvc)

	shareHandler := handler.NewShareHandler(shareSvc)
	webVitalsHandler := handler.NewWebVitalsHandler(vitalsSvc)

	// Public routes
	e.POST("/api/auth/login", userHandler.Login)
	e.POST("/api/auth/refresh", userHandler.Refresh)

	// OIDC (public)
	oidcHandler := handler.NewOIDCHandler(oidcSvc)
	e.GET("/api/auth/oidc/:provider", oidcHandler.Login)
	e.GET("/api/auth/oidc/:provider/callback", oidcHandler.Callback)
	e.GET("/api/auth/providers", oidcHandler.Providers)

	// Shared result public access (no auth required)
	e.GET("/s/:token", shareHandler.GetShare)
	e.POST("/s/:token/verify", shareHandler.VerifySharePassword)

	// Core Web Vitals ingestion (public, rate-limited)
	e.POST("/api/metrics/web-vitals", webVitalsHandler.RecordVitals)

	// Authenticated routes (supports both JWT and API Token)
	authGroup := e.Group("", middleware.Auth(authSvc, tokenSvc))
	authGroup.GET("/api/dashboard/stats", dashboardHandler.GetStats)
	authGroup.GET("/api/dashboard/overview", dashboardHandler.GetFullStats)
	authGroup.GET("/api/auth/me", userHandler.Me)
	authGroup.PUT("/api/auth/password", userHandler.ChangePassword)

	// Tables endpoint: authenticated users can access
	authGroup.GET("/api/datasources/:id/tables", dsHandler.GetTables)
	authGroup.GET("/api/datasources/:id/tables/:name/columns", dsHandler.GetTableColumns)
	authGroup.GET("/api/datasources/:id/es/indices", dsHandler.GetESIndices)
	authGroup.GET("/api/datasources/:id/es/indices/:index/fields", dsHandler.GetESIndexFields)

	// Query execution & history (authenticated users)
	authGroup.POST("/api/query/execute", queryHandler.ExecuteQuery)
	authGroup.POST("/api/query/explain", queryHandler.ExplainQuery)
	authGroup.POST("/api/query/review", aiReviewHandler.ReviewStream)
	authGroup.POST("/api/query/export", queryHandler.ExportQuery)
	authGroup.GET("/api/query/history", queryHandler.ListHistory)
	authGroup.DELETE("/api/query/history/:id", queryHandler.DeleteHistory)
	authGroup.DELETE("/api/query/history", queryHandler.ClearHistory)

	// Shared query results (authenticated users)
	authGroup.POST("/api/query/share", shareHandler.CreateShare)
	authGroup.GET("/api/query/share", shareHandler.ListMyShares)
	authGroup.DELETE("/api/query/share/:id", shareHandler.RevokeShare)

	// Performance analysis (authenticated users)
	authGroup.GET("/api/query/performance/slow", performanceHandler.ListSlowQueries)
	authGroup.GET("/api/query/performance/stats", performanceHandler.GetPerformanceStats)

	// Ticket routes (authenticated users can create/list/view; approve/reject/execute restricted by role)
	authGroup.POST("/api/tickets", ticketHandler.CreateTicket)
	authGroup.GET("/api/tickets", ticketHandler.ListTickets)
	authGroup.GET("/api/tickets/:id", ticketHandler.GetTicket)
	authGroup.POST("/api/tickets/batch-approve", ticketHandler.BatchApprove)
	authGroup.POST("/api/tickets/batch-reject", ticketHandler.BatchReject)
	authGroup.POST("/api/tickets/:id/approve", ticketHandler.ApproveTicket)
	authGroup.POST("/api/tickets/:id/reject", ticketHandler.RejectTicket)
	authGroup.POST("/api/tickets/:id/cancel", ticketHandler.CancelTicket)
	authGroup.POST("/api/tickets/:id/schedule", ticketHandler.ScheduleTicket)
	authGroup.POST("/api/tickets/:id/cancel-schedule", ticketHandler.CancelSchedule)
	authGroup.POST("/api/tickets/:id/execute", ticketHandler.ExecuteTicket)
	authGroup.GET("/api/tickets/:id/execution-results", ticketHandler.GetExecutionResults)
	authGroup.PUT("/api/tickets/:id/resubmit", ticketHandler.ResubmitTicket)
	authGroup.GET("/api/tickets/:id/revisions", ticketHandler.ListRevisions)

	// Comment routes (authenticated users)
	commentHandler := handler.NewCommentHandler(commentSvc)
	authGroup.GET("/api/tickets/:id/comments", commentHandler.ListComments)
	authGroup.POST("/api/tickets/:id/comments", commentHandler.CreateComment)
	authGroup.DELETE("/api/comments/:id", commentHandler.DeleteComment)

	// Git link routes (authenticated users)
	authGroup.POST("/api/git-links", gitHandler.CreateGitLink)
	authGroup.GET("/api/git-links", gitHandler.ListGitLinks)
	authGroup.DELETE("/api/git-links/:id", gitHandler.DeleteGitLink)

	// SQL Template management (authenticated users)
	authGroup.POST("/api/sql-templates", sqlTemplateHandler.CreateTemplate)
	authGroup.GET("/api/sql-templates", sqlTemplateHandler.ListTemplates)
	authGroup.GET("/api/sql-templates/:id", sqlTemplateHandler.GetTemplate)
	authGroup.PUT("/api/sql-templates/:id", sqlTemplateHandler.UpdateTemplate)
	authGroup.DELETE("/api/sql-templates/:id", sqlTemplateHandler.DeleteTemplate)
	authGroup.POST("/api/sql-templates/:id/render", sqlTemplateHandler.RenderTemplate)

	// API Token management (authenticated users manage their own tokens)
	authGroup.POST("/api/tokens", tokenHandler.CreateToken)
	authGroup.GET("/api/tokens", tokenHandler.ListMyTokens)
	authGroup.GET("/api/tokens/stats", tokenHandler.GetTokenStats)
	authGroup.DELETE("/api/tokens/:id", tokenHandler.RevokeMyToken)

	// Admin-only routes (supports both JWT and API Token with admin scope)
	adminGroup := e.Group("", middleware.Auth(authSvc, tokenSvc), middleware.Admin())
	adminGroup.POST("/api/users", userHandler.CreateUser)
	adminGroup.GET("/api/users", userHandler.ListUsers)
	adminGroup.GET("/api/users/:id", userHandler.GetUser)
	adminGroup.PUT("/api/users/:id", userHandler.UpdateUser)
	adminGroup.DELETE("/api/users/:id", userHandler.DeleteUser)
	adminGroup.PUT("/api/users/:id/reset-password", userHandler.ResetPassword)

	// Datasource management (admin)
	adminGroup.POST("/api/datasources", dsHandler.CreateDatasource)
	adminGroup.GET("/api/datasources", dsHandler.ListDatasources)
	adminGroup.GET("/api/datasources/:id", dsHandler.GetDatasource)
	adminGroup.PUT("/api/datasources/:id", dsHandler.UpdateDatasource)
	adminGroup.DELETE("/api/datasources/:id", dsHandler.DisableDatasource)
	adminGroup.POST("/api/datasources/:id/test", dsHandler.TestConnection)

	// Role & permission management (admin)
	adminGroup.GET("/api/roles", permHandler.ListRoles)
	adminGroup.GET("/api/roles/:role", permHandler.GetRole)
	adminGroup.POST("/api/policies", permHandler.AddPolicy)
	adminGroup.GET("/api/policies", permHandler.ListPolicies)
	adminGroup.DELETE("/api/policies/:id", permHandler.DeletePolicy)
	adminGroup.POST("/api/policies/sync", permHandler.SyncPolicies)

	// Mask rules management (admin)
	adminGroup.POST("/api/mask-rules", maskRuleHandler.CreateMaskRule)
	adminGroup.GET("/api/mask-rules", maskRuleHandler.ListMaskRules)
	adminGroup.GET("/api/mask-rules/:id", maskRuleHandler.GetMaskRule)
	adminGroup.PUT("/api/mask-rules/:id", maskRuleHandler.UpdateMaskRule)
	adminGroup.DELETE("/api/mask-rules/:id", maskRuleHandler.DeleteMaskRule)

	// Sensitive tables management (admin)
	adminGroup.POST("/api/sensitive-tables", maskRuleHandler.CreateSensitiveTable)
	adminGroup.GET("/api/sensitive-tables", maskRuleHandler.ListSensitiveTables)
	adminGroup.DELETE("/api/sensitive-tables/:id", maskRuleHandler.DeleteSensitiveTable)

	// Audit logs (admin/dba can view)
	adminGroup.GET("/api/audit-logs", auditHandler.ListAuditLogs)
	adminGroup.GET("/api/audit-logs/search", auditHandler.SearchAuditLogs)

	// Audit reports (admin/dba can view)
	adminGroup.GET("/api/reports/usage", reportHandler.GetUsageStats)
	adminGroup.GET("/api/reports/errors", reportHandler.GetErrorStats)
	adminGroup.GET("/api/reports/performance", reportHandler.GetPerformanceReport)
	adminGroup.GET("/api/reports/tickets", reportHandler.GetTicketReport)

	// Permission request management
	authGroup.POST("/api/permission-requests", permReqHandler.CreateRequest)
	authGroup.GET("/api/permission-requests/mine", permReqHandler.MyRequests)
	authGroup.GET("/api/permission-requests/active", permReqHandler.MyActiveRequests)
	authGroup.GET("/api/permission-requests/:id", permReqHandler.GetRequest)

	adminGroup.GET("/api/permission-requests", permReqHandler.ListRequests)
	adminGroup.POST("/api/permission-requests/:id/approve", permReqHandler.ApproveRequest)
	adminGroup.POST("/api/permission-requests/:id/reject", permReqHandler.RejectRequest)
	adminGroup.POST("/api/permission-requests/:id/revoke", permReqHandler.RevokeRequest)
	adminGroup.POST("/api/permission-requests/expire", permReqHandler.ExpireOverdue)

	// Export routes — audit export requires admin/dba; ticket export requires auth
	adminGroup.GET("/api/export/audit", exportHandler.ExportAuditLogs)
	authGroup.GET("/api/export/tickets", exportHandler.ExportTickets)
	// Async export task management (authenticated users)
	authGroup.GET("/api/export/tasks", exportHandler.ListExportTasks)
	authGroup.GET("/api/export/tasks/:id", exportHandler.GetExportTask)
	authGroup.GET("/api/export/tasks/:id/download", exportHandler.DownloadExportFile)

	// Database backup management (admin)
	adminGroup.POST("/api/backups", backupHandler.TriggerBackup)
	adminGroup.GET("/api/backups", backupHandler.ListBackups)
	adminGroup.GET("/api/backups/:filename/download", backupHandler.DownloadBackup)
	adminGroup.DELETE("/api/backups/:filename", backupHandler.DeleteBackup)

	// Notification & Settings (admin)
	notifyHandler := handler.NewNotifyHandler(notifySvc, aiReviewSvc)

	// Feishu webhook CRUD (admin)
	feishuWebhookSvc := service.NewFeishuWebhookService(database.DB, cfg.EncryptionKey)
	feishuWebhookHandler := handler.NewFeishuWebhookHandler(feishuWebhookSvc)
	notifySvc.SetFeishuWebhookService(feishuWebhookSvc)

	adminGroup.GET("/api/settings", notifyHandler.GetSettings)
	adminGroup.PUT("/api/settings/notify/webhook", notifyHandler.UpdateNotifyConfig)
	adminGroup.POST("/api/settings/notify/webhook/test", notifyHandler.TestNotify)
	adminGroup.PUT("/api/settings/ai", notifyHandler.UpdateAIConfig)
	adminGroup.PUT("/api/settings/feishu", notifyHandler.UpdateFeishuConfig)
	adminGroup.POST("/api/settings/feishu/test", notifyHandler.TestFeishuNotify)

	// Feishu webhook CRUD API
	adminGroup.POST("/api/settings/feishu/webhooks", feishuWebhookHandler.Create)
	adminGroup.GET("/api/settings/feishu/webhooks", feishuWebhookHandler.List)
	adminGroup.GET("/api/settings/feishu/webhooks/:id", feishuWebhookHandler.Get)
	adminGroup.PUT("/api/settings/feishu/webhooks/:id", feishuWebhookHandler.Update)
	adminGroup.DELETE("/api/settings/feishu/webhooks/:id", feishuWebhookHandler.Delete)
	adminGroup.GET("/api/settings/feishu/webhooks/dead-letters", feishuWebhookHandler.ListDeadLetters)

	// API Token admin management
	adminGroup.GET("/api/admin/tokens", tokenHandler.ListAllTokens)
	adminGroup.DELETE("/api/admin/tokens/:id", tokenHandler.RevokeAnyToken)

	// SLA configuration management (admin only)
	slaSvc := service.NewSLAService(database, notifySvc)
	slaHandler := handler.NewSLAHandler(slaSvc)

	adminGroup.GET("/api/settings/sla", slaHandler.ListSLAConfigs)
	adminGroup.POST("/api/settings/sla", slaHandler.CreateSLAConfig)
	adminGroup.PUT("/api/settings/sla/:id", slaHandler.UpdateSLAConfig)
	adminGroup.DELETE("/api/settings/sla/:id", slaHandler.DeleteSLAConfig)

	// Approval policy routes (admin)
	adminGroup.POST("/api/approval/policies", approvalHandler.CreatePolicy)
	adminGroup.GET("/api/approval/policies", approvalHandler.ListPolicies)
	adminGroup.GET("/api/approval/policies/:id", approvalHandler.GetPolicy)
	adminGroup.PUT("/api/approval/policies/:id", approvalHandler.UpdatePolicy)
	adminGroup.DELETE("/api/approval/policies/:id", approvalHandler.DeletePolicy)

	// Approval action routes (auth)
	authGroup.POST("/api/tickets/:id/engine-approve", approvalHandler.ProcessApproval)
	authGroup.GET("/api/tickets/:id/approval-history", approvalHandler.GetApprovalHistory)
	adminGroup.GET("/api/sla-notifications", slaHandler.ListSLANotifications)

	// Ticket SLA status query (authenticated users)
	authGroup.GET("/api/tickets/sla-status", slaHandler.GetTicketSLAStatuses)

	// Coverage audit system (SF-QA0025) — MUST-1: nil pgDB guard inside RegisterCoverageRoutes
	handler.RegisterCoverageRoutes(e, middleware.Auth(authSvc, tokenSvc), middleware.Admin(), nil)

	// Frontend SPA (must be after API routes)
	serveFrontend(e)

	return e
}
