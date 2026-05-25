package api

import (
	"database/sql"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/api/handler"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/pkg/metrics"
	"github.com/whg517/sqlflow/internal/service"

	echoSwagger "github.com/swaggo/echo-swagger"
	_ "github.com/whg517/sqlflow/docs"
)

// NewRouter creates and configures an Echo instance with middleware and routes.
func NewRouter(authSvc *service.AuthService, dsSvc *service.DatasourceService, permSvc *service.PermissionService, querySvc *service.QueryService, historySvc *service.QueryHistoryService, ticketSvc *service.TicketService, maskRuleSvc *service.MaskRuleService, aiReviewSvc *service.AIReviewService, auditSvc *service.AuditService, exportSvc *service.ExportService, notifySvc *service.NotifyService, dashboardSvc *service.DashboardService, commentSvc *service.CommentService, dingOAuthSvc *service.DingTalkOAuthService, backupSvc *service.BackupService, db *sql.DB, cfg *config.Config) *echo.Echo {
	e := echo.New()

	// Global middleware
	e.Use(middleware.Recovery())
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	// Prometheus metrics middleware
	if cfg.Metrics.Enabled {
		e.Use(metrics.Middleware())
	}

	// Health check (public, enhanced with DB check)
	healthHandler := handler.NewHealthHandler(db)
	e.GET("/health", healthHandler.Health)
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
	maskRuleHandler := handler.NewMaskRuleHandler(maskRuleSvc)
	aiReviewHandler := handler.NewAIReviewHandler(aiReviewSvc, dsSvc)
	auditHandler := handler.NewAuditHandler(auditSvc)
	exportHandler := handler.NewExportHandler(exportSvc)
	dashboardHandler := handler.NewDashboardHandler(dashboardSvc)
	backupHandler := handler.NewBackupHandler(backupSvc)
	performanceHandler := handler.NewPerformanceHandler(historySvc)

	// Public routes
	e.POST("/api/auth/login", userHandler.Login)
	e.POST("/api/auth/refresh", userHandler.Refresh)

	// DingTalk OAuth (public)
	dingTalkHandler := handler.NewDingTalkHandler(dingOAuthSvc)
	e.GET("/api/v1/auth/dingtalk/login", dingTalkHandler.Login)
	e.GET("/api/v1/auth/dingtalk/callback", dingTalkHandler.Callback)
	e.GET("/api/v1/auth/dingtalk/enabled", dingTalkHandler.Enabled)

	// Authenticated routes
	authGroup := e.Group("", middleware.JWT(authSvc))
	authGroup.GET("/api/dashboard/stats", dashboardHandler.GetStats)
	authGroup.GET("/api/auth/me", userHandler.Me)
	authGroup.PUT("/api/auth/password", userHandler.ChangePassword)

	// Tables endpoint: authenticated users can access
	authGroup.GET("/api/datasources/:id/tables", dsHandler.GetTables)
	authGroup.GET("/api/datasources/:id/tables/:name/columns", dsHandler.GetTableColumns)

	// Query execution & history (authenticated users)
	authGroup.POST("/api/query/execute", queryHandler.ExecuteQuery)
	authGroup.POST("/api/query/explain", queryHandler.ExplainQuery)
	authGroup.POST("/api/query/review", aiReviewHandler.ReviewStream)
	authGroup.POST("/api/query/export", queryHandler.ExportQuery)
	authGroup.GET("/api/query/history", queryHandler.ListHistory)
	authGroup.DELETE("/api/query/history/:id", queryHandler.DeleteHistory)
	authGroup.DELETE("/api/query/history", queryHandler.ClearHistory)

	// Performance analysis (authenticated users)
	authGroup.GET("/api/query/performance/slow", performanceHandler.ListSlowQueries)
	authGroup.GET("/api/query/performance/stats", performanceHandler.GetPerformanceStats)

	// Ticket routes (authenticated users can create/list/view; approve/reject/execute restricted by role)
	authGroup.POST("/api/tickets", ticketHandler.CreateTicket)
	authGroup.GET("/api/tickets", ticketHandler.ListTickets)
	authGroup.GET("/api/tickets/:id", ticketHandler.GetTicket)
	authGroup.POST("/api/tickets/:id/approve", ticketHandler.ApproveTicket)
	authGroup.POST("/api/tickets/:id/reject", ticketHandler.RejectTicket)
	authGroup.POST("/api/tickets/:id/cancel", ticketHandler.CancelTicket)
	authGroup.POST("/api/tickets/:id/schedule", ticketHandler.ScheduleTicket)
	authGroup.POST("/api/tickets/:id/cancel-schedule", ticketHandler.CancelSchedule)
	authGroup.POST("/api/tickets/:id/execute", ticketHandler.ExecuteTicket)

	// Comment routes (authenticated users)
	commentHandler := handler.NewCommentHandler(commentSvc)
	authGroup.GET("/api/tickets/:id/comments", commentHandler.ListComments)
	authGroup.POST("/api/tickets/:id/comments", commentHandler.CreateComment)
	authGroup.DELETE("/api/comments/:id", commentHandler.DeleteComment)

	// Admin-only routes
	adminGroup := e.Group("", middleware.JWT(authSvc), middleware.Admin())
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

	// Export routes — audit export requires admin/dba; ticket export requires auth
	adminGroup.GET("/api/export/audit", exportHandler.ExportAuditLogs)
	authGroup.GET("/api/export/tickets", exportHandler.ExportTickets)

	// Database backup management (admin)
	adminGroup.POST("/api/backups", backupHandler.TriggerBackup)
	adminGroup.GET("/api/backups", backupHandler.ListBackups)
	adminGroup.GET("/api/backups/:filename/download", backupHandler.DownloadBackup)
	adminGroup.DELETE("/api/backups/:filename", backupHandler.DeleteBackup)

	// Notification & Settings (admin)
	notifyHandler := handler.NewNotifyHandler(notifySvc, aiReviewSvc)
	adminGroup.GET("/api/settings", notifyHandler.GetSettings)
	adminGroup.PUT("/api/settings/dingtalk", notifyHandler.UpdateNotifyConfig)
	adminGroup.POST("/api/settings/dingtalk/test", notifyHandler.TestNotify)
	adminGroup.PUT("/api/settings/ai", notifyHandler.UpdateAIConfig)

	// Frontend SPA (must be after API routes)
	serveFrontend(e)

	return e
}
