package api

import (
	"github.com/whg517/sqlflow/internal/app"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/handler"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/pkg/metrics"

	echoSwagger "github.com/swaggo/echo-swagger"
	_ "github.com/whg517/sqlflow/internal/api/openapi"
)

// NewRouter creates and configures an Echo instance with middleware and routes.
// 所有依赖从 *app.Container 获取，替代了原先 28 个位置参数。
func NewRouter(c *app.Container) *echo.Echo {
	e := echo.New()

	// Global middleware
	e.Use(middleware.Recovery())
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	// Prometheus metrics middleware
	if c.Cfg.Metrics.Enabled {
		e.Use(metrics.Middleware())
	}

	// Health check endpoints (public)
	healthHandler := handler.NewHealthHandler(c.DB.DB)
	healthHandler.SetConnPoolManager(c.ConnMgr)
	healthHandler.SetPoolManager(c.PoolMgr)
	e.GET("/health", healthHandler.Health)
	e.GET("/healthz", healthHandler.Healthz) // Liveness probe (no dependency checks)
	e.GET("/readyz", healthHandler.Readyz)   // Readiness probe (checks all deps)
	e.GET("/api/health", healthHandler.Health)

	// Prometheus metrics endpoint
	if c.Cfg.Metrics.Enabled {
		e.GET("/metrics", healthHandler.Metrics)
	}

	// Swagger UI
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	// Auth handlers
	userHandler := handler.NewUserHandler(c.Auth, c.Permission)
	dsHandler := handler.NewDatasourceHandler(c.Datasource, c.Permission)
	permHandler := handler.NewPermissionHandler(c.Permission)
	queryHandler := handler.NewQueryHandler(c.Query, c.History)
	ticketHandler := handler.NewTicketHandler(c.Ticket)
	approvalHandler := handler.NewApprovalHandler(c.ApprovalEngine)
	approvalHandler.SetAuditService(c.Audit)
	approvalHandler.SetPermissionService(c.Permission)
	maskRuleHandler := handler.NewMaskRuleHandler(c.MaskRule)
	aiReviewHandler := handler.NewAIReviewHandler(c.AIReview, c.Datasource)
	auditHandler := handler.NewAuditHandler(c.Audit)
	exportHandler := handler.NewExportHandler(c.Export, c.ExportAsync)
	dashboardHandler := handler.NewDashboardHandler(c.Dashboard)
	backupHandler := handler.NewBackupHandler(c.Backup)
	performanceHandler := handler.NewPerformanceHandler(c.History)
	gitHandler := handler.NewGitHandler(c.Git)
	tokenHandler := handler.NewTokenHandler(c.Token)
	reportHandler := handler.NewAuditReportHandler(c.Report)
	permReqHandler := handler.NewPermReqHandler(c.PermRequest)
	sqlTemplateHandler := handler.NewSQLTemplateHandler(c.SQLTemplate)

	shareHandler := handler.NewShareHandler(c.Share)
	webVitalsHandler := handler.NewWebVitalsHandler(c.WebVitals)

	// Public routes
	e.POST("/api/auth/login", userHandler.Login)
	e.POST("/api/auth/refresh", userHandler.Refresh)

	// OIDC (public)
	oidcHandler := handler.NewOIDCHandler(c.OIDC)
	e.GET("/api/auth/oidc/:provider", oidcHandler.Login)
	e.GET("/api/auth/oidc/:provider/callback", oidcHandler.Callback)
	e.GET("/api/auth/providers", oidcHandler.Providers)

	// Shared result public access (no auth required)
	e.GET("/s/:token", shareHandler.GetShare)
	e.POST("/s/:token/verify", shareHandler.VerifySharePassword)

	// Core Web Vitals ingestion (public, rate-limited)
	e.POST("/api/metrics/web-vitals", webVitalsHandler.RecordVitals)

	// Authenticated routes (supports both JWT and API Token)
	authGroup := e.Group("", middleware.Auth(c.Auth, c.Token))
	authGroup.GET("/api/dashboard/stats", dashboardHandler.GetStats)
	authGroup.GET("/api/dashboard/overview", dashboardHandler.GetFullStats)
	authGroup.GET("/api/auth/me", userHandler.Me)
	authGroup.PUT("/api/auth/password", userHandler.ChangePassword)

	// Datasource discovery and metadata: authenticated users can access safe summaries.
	authGroup.GET("/api/datasources/available", dsHandler.ListAvailableDatasources, middleware.RequireScope("read:datasource"))
	authGroup.GET("/api/datasources/:id/tables", dsHandler.GetTables, middleware.RequireScope("read:datasource"))
	authGroup.GET("/api/datasources/:id/tables/:name/columns", dsHandler.GetTableColumns, middleware.RequireScope("read:datasource"))
	authGroup.GET("/api/datasources/:id/es/indices", dsHandler.GetESIndices, middleware.RequireScope("read:datasource"))
	authGroup.GET("/api/datasources/:id/es/indices/:index/fields", dsHandler.GetESIndexFields, middleware.RequireScope("read:datasource"))

	// Query execution & history (authenticated users)
	authGroup.POST("/api/query/execute", queryHandler.ExecuteQuery, middleware.RequireScope("execute:query"))
	authGroup.POST("/api/query/explain", queryHandler.ExplainQuery, middleware.RequireScope("execute:query"))
	authGroup.POST("/api/query/review", aiReviewHandler.ReviewStream, middleware.RequireScope("execute:query"))
	authGroup.POST("/api/query/export", queryHandler.ExportQuery, middleware.RequireScope("execute:query"))
	authGroup.GET("/api/query/history", queryHandler.ListHistory, middleware.RequireScope("read:query"))
	authGroup.GET("/api/query/history/frequent", queryHandler.GetFrequentQueries, middleware.RequireScope("read:query"))
	authGroup.DELETE("/api/query/history/:id", queryHandler.DeleteHistory, middleware.RequireScope("execute:query"))
	authGroup.DELETE("/api/query/history", queryHandler.ClearHistory, middleware.RequireScope("execute:query"))

	// Shared query results (authenticated users)
	authGroup.POST("/api/query/share", shareHandler.CreateShare)
	authGroup.GET("/api/query/share", shareHandler.ListMyShares)
	authGroup.DELETE("/api/query/share/:id", shareHandler.RevokeShare)

	// Performance analysis (authenticated users)
	authGroup.GET("/api/query/performance/slow", performanceHandler.ListSlowQueries)
	authGroup.GET("/api/query/performance/stats", performanceHandler.GetPerformanceStats)

	// Ticket routes (authenticated users can create/list/view; approve/reject/execute restricted by role)
	authGroup.POST("/api/tickets", ticketHandler.CreateTicket, middleware.RequireScope("write:ticket"))
	authGroup.GET("/api/tickets", ticketHandler.ListTickets, middleware.RequireScope("read:ticket"))
	authGroup.GET("/api/tickets/:id", ticketHandler.GetTicket, middleware.RequireScope("read:ticket"))
	authGroup.POST("/api/tickets/batch-approve", ticketHandler.BatchApprove, middleware.RequireScope("write:ticket"))
	authGroup.POST("/api/tickets/batch-reject", ticketHandler.BatchReject, middleware.RequireScope("write:ticket"))
	authGroup.POST("/api/tickets/:id/approve", ticketHandler.ApproveTicket, middleware.RequireScope("write:ticket"))
	authGroup.POST("/api/tickets/:id/reject", ticketHandler.RejectTicket, middleware.RequireScope("write:ticket"))
	authGroup.POST("/api/tickets/:id/cancel", ticketHandler.CancelTicket, middleware.RequireScope("write:ticket"))
	authGroup.POST("/api/tickets/:id/schedule", ticketHandler.ScheduleTicket, middleware.RequireScope("write:ticket"))
	authGroup.POST("/api/tickets/:id/cancel-schedule", ticketHandler.CancelSchedule, middleware.RequireScope("write:ticket"))
	authGroup.POST("/api/tickets/:id/execute", ticketHandler.ExecuteTicket, middleware.RequireScope("write:ticket"))
	authGroup.GET("/api/tickets/:id/execution-results", ticketHandler.GetExecutionResults, middleware.RequireScope("read:ticket"))
	authGroup.PUT("/api/tickets/:id/resubmit", ticketHandler.ResubmitTicket, middleware.RequireScope("write:ticket"))
	authGroup.GET("/api/tickets/:id/revisions", ticketHandler.ListRevisions, middleware.RequireScope("read:ticket"))

	// Comment routes (authenticated users)
	commentHandler := handler.NewCommentHandler(c.Comment)
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
	adminGroup := e.Group("", middleware.Auth(c.Auth, c.Token), middleware.RequireScope("admin"), middleware.Admin())
	userAdminGroup := e.Group("", middleware.Auth(c.Auth, c.Token), middleware.RequireScope("admin"), middleware.SystemPermission(c.Permission, "users", "manage"))
	userAdminGroup.POST("/api/users", userHandler.CreateUser)
	userAdminGroup.GET("/api/users", userHandler.ListUsers)
	userAdminGroup.GET("/api/users/:id", userHandler.GetUser)
	userAdminGroup.PUT("/api/users/:id", userHandler.UpdateUser)
	userAdminGroup.DELETE("/api/users/:id", userHandler.DeleteUser)
	userAdminGroup.PUT("/api/users/:id/reset-password", userHandler.ResetPassword)

	// Datasource management (delegable platform permission)
	datasourceAdminGroup := e.Group("", middleware.Auth(c.Auth, c.Token), middleware.RequireScope("admin"), middleware.SystemPermission(c.Permission, "datasources", "manage"))
	datasourceAdminGroup.POST("/api/datasources", dsHandler.CreateDatasource)
	datasourceAdminGroup.GET("/api/datasources", dsHandler.ListDatasources)
	datasourceAdminGroup.POST("/api/datasources/test-config", dsHandler.TestConnectionConfig)
	datasourceAdminGroup.GET("/api/datasources/:id", dsHandler.GetDatasource)
	datasourceAdminGroup.PUT("/api/datasources/:id", dsHandler.UpdateDatasource)
	datasourceAdminGroup.PUT("/api/datasources/:id/status", dsHandler.UpdateDatasourceStatus)
	datasourceAdminGroup.DELETE("/api/datasources/:id", dsHandler.DeleteDatasource)
	datasourceAdminGroup.POST("/api/datasources/:id/test", dsHandler.TestConnection)

	// Role & permission management (delegable platform permission)
	rbacAdminGroup := e.Group("", middleware.Auth(c.Auth, c.Token), middleware.RequireScope("admin"), middleware.SystemPermission(c.Permission, "rbac", "manage"))
	rbacAdminGroup.GET("/api/roles", permHandler.ListRoles)
	rbacAdminGroup.POST("/api/roles", permHandler.CreateRole)
	rbacAdminGroup.GET("/api/roles/:role", permHandler.GetRole)
	rbacAdminGroup.PUT("/api/roles/:role", permHandler.UpdateRole)
	rbacAdminGroup.DELETE("/api/roles/:role", permHandler.DeleteRole)
	rbacAdminGroup.POST("/api/policies", permHandler.AddPolicy)
	rbacAdminGroup.GET("/api/policies", permHandler.ListPolicies)
	rbacAdminGroup.DELETE("/api/policies/:id", permHandler.DeletePolicy)
	rbacAdminGroup.POST("/api/policies/sync", permHandler.SyncPolicies)

	// Mask rules management (delegable platform permission)
	securityAdminGroup := e.Group("", middleware.Auth(c.Auth, c.Token), middleware.RequireScope("admin"), middleware.SystemPermission(c.Permission, "security", "manage"))
	securityAdminGroup.POST("/api/mask-rules", maskRuleHandler.CreateMaskRule)
	securityAdminGroup.GET("/api/mask-rules", maskRuleHandler.ListMaskRules)
	securityAdminGroup.GET("/api/mask-rules/:id", maskRuleHandler.GetMaskRule)
	securityAdminGroup.PUT("/api/mask-rules/:id", maskRuleHandler.UpdateMaskRule)
	securityAdminGroup.DELETE("/api/mask-rules/:id", maskRuleHandler.DeleteMaskRule)

	// Sensitive tables management
	securityAdminGroup.POST("/api/sensitive-tables", maskRuleHandler.CreateSensitiveTable)
	securityAdminGroup.GET("/api/sensitive-tables", maskRuleHandler.ListSensitiveTables)
	securityAdminGroup.DELETE("/api/sensitive-tables/:id", maskRuleHandler.DeleteSensitiveTable)

	// Audit and reports (delegable; DBA receives this permission by default)
	auditAdminGroup := e.Group("", middleware.Auth(c.Auth, c.Token), middleware.RequireScope("admin"), middleware.SystemPermission(c.Permission, "audit", "view"))
	auditAdminGroup.GET("/api/audit-logs", auditHandler.ListAuditLogs)
	auditAdminGroup.GET("/api/audit-logs/search", auditHandler.SearchAuditLogs)

	// Audit reports (admin/dba can view)
	auditAdminGroup.GET("/api/reports/usage", reportHandler.GetUsageStats)
	auditAdminGroup.GET("/api/reports/errors", reportHandler.GetErrorStats)
	auditAdminGroup.GET("/api/reports/performance", reportHandler.GetPerformanceReport)
	auditAdminGroup.GET("/api/reports/tickets", reportHandler.GetTicketReport)

	// User behavior analytics (admin only)
	auditAdminGroup.GET("/api/audit/user-analytics", reportHandler.GetUserAnalytics)

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
	auditAdminGroup.GET("/api/export/audit", exportHandler.ExportAuditLogs)
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
	notifyHandler := handler.NewNotifyHandler(c.Notify, c.AIReview)
	settingsAdminGroup := e.Group("", middleware.Auth(c.Auth, c.Token), middleware.RequireScope("admin"), middleware.SystemPermission(c.Permission, "settings", "manage"))

	// Feishu webhook CRUD (admin)
	feishuWebhookHandler := handler.NewFeishuWebhookHandler(c.FeishuWebhook)

	settingsAdminGroup.GET("/api/settings", notifyHandler.GetSettings)
	settingsAdminGroup.PUT("/api/settings/notify/webhook", notifyHandler.UpdateNotifyConfig)
	settingsAdminGroup.POST("/api/settings/notify/webhook/test", notifyHandler.TestNotify)
	settingsAdminGroup.PUT("/api/settings/ai", notifyHandler.UpdateAIConfig)
	settingsAdminGroup.PUT("/api/settings/feishu", notifyHandler.UpdateFeishuConfig)
	settingsAdminGroup.POST("/api/settings/feishu/test", notifyHandler.TestFeishuNotify)

	// Notification preferences (auth)
	notifPrefHandler := handler.NewNotificationPreferenceHandler(c.NotificationPreference)
	authGroup.GET("/api/notifications/preferences", notifPrefHandler.GetPreferences)
	authGroup.PUT("/api/notifications/preferences", notifPrefHandler.UpdatePreferences)

	// Feishu webhook CRUD API
	settingsAdminGroup.POST("/api/settings/feishu/webhooks", feishuWebhookHandler.Create)
	settingsAdminGroup.GET("/api/settings/feishu/webhooks", feishuWebhookHandler.List)
	settingsAdminGroup.GET("/api/settings/feishu/webhooks/:id", feishuWebhookHandler.Get)
	settingsAdminGroup.PUT("/api/settings/feishu/webhooks/:id", feishuWebhookHandler.Update)
	settingsAdminGroup.DELETE("/api/settings/feishu/webhooks/:id", feishuWebhookHandler.Delete)
	settingsAdminGroup.GET("/api/settings/feishu/webhooks/dead-letters", feishuWebhookHandler.ListDeadLetters)

	// API Token admin management
	adminGroup.GET("/api/admin/tokens", tokenHandler.ListAllTokens)
	adminGroup.DELETE("/api/admin/tokens/:id", tokenHandler.RevokeAnyToken)

	// SLA configuration management (admin only)
	slaHandler := handler.NewSLAHandler(c.SLA)

	settingsAdminGroup.GET("/api/settings/sla", slaHandler.ListSLAConfigs)
	settingsAdminGroup.POST("/api/settings/sla", slaHandler.CreateSLAConfig)
	settingsAdminGroup.PUT("/api/settings/sla/:id", slaHandler.UpdateSLAConfig)
	settingsAdminGroup.DELETE("/api/settings/sla/:id", slaHandler.DeleteSLAConfig)

	// Approval policy management routes (admin only — SF-FEAT0056-BE)
	settingsAdminGroup.GET("/api/admin/approval-policies", approvalHandler.ListPolicies)
	settingsAdminGroup.POST("/api/admin/approval-policies", approvalHandler.CreatePolicy)
	settingsAdminGroup.PUT("/api/admin/approval-policies/reorder", approvalHandler.ReorderPolicies)
	settingsAdminGroup.GET("/api/admin/approval-policies/approvers", approvalHandler.GetApprovers)
	settingsAdminGroup.GET("/api/admin/approval-policies/:id", approvalHandler.GetPolicy)
	settingsAdminGroup.PUT("/api/admin/approval-policies/:id", approvalHandler.UpdatePolicy)
	settingsAdminGroup.DELETE("/api/admin/approval-policies/:id", approvalHandler.DeletePolicy)
	settingsAdminGroup.PUT("/api/admin/approval-policies/:id/toggle", approvalHandler.TogglePolicy)

	// Legacy approval policy routes (admin) — backward compatibility
	settingsAdminGroup.POST("/api/approval/policies", approvalHandler.CreatePolicy)
	settingsAdminGroup.GET("/api/approval/policies", approvalHandler.ListPolicies)
	settingsAdminGroup.GET("/api/approval/policies/:id", approvalHandler.GetPolicy)
	settingsAdminGroup.PUT("/api/approval/policies/:id", approvalHandler.UpdatePolicy)
	settingsAdminGroup.DELETE("/api/approval/policies/:id", approvalHandler.DeletePolicy)

	// Approval chain & action routes (auth)
	authGroup.GET("/api/tickets/:id/approval-chain", approvalHandler.GetApprovalChain)
	authGroup.POST("/api/tickets/:id/engine-approve", approvalHandler.ProcessApproval)
	authGroup.GET("/api/tickets/:id/approval-history", approvalHandler.GetApprovalHistory)
	settingsAdminGroup.GET("/api/sla-notifications", slaHandler.ListSLANotifications)

	// Webhook subscription management (admin)
	webhookSubHandler := handler.NewWebhookSubscriptionHandler(c.WebhookSubscription)
	settingsAdminGroup.GET("/api/admin/webhooks/subscriptions", webhookSubHandler.List)
	settingsAdminGroup.POST("/api/admin/webhooks/subscriptions", webhookSubHandler.Create)
	settingsAdminGroup.GET("/api/admin/webhooks/subscriptions/:id", webhookSubHandler.Get)
	settingsAdminGroup.PUT("/api/admin/webhooks/subscriptions/:id", webhookSubHandler.Update)
	settingsAdminGroup.DELETE("/api/admin/webhooks/subscriptions/:id", webhookSubHandler.Delete)
	settingsAdminGroup.POST("/api/admin/webhooks/subscriptions/:id/toggle", webhookSubHandler.Toggle)
	settingsAdminGroup.POST("/api/admin/webhooks/subscriptions/:id/test", webhookSubHandler.TestSend)

	// Ticket SLA status query (authenticated users)
	authGroup.GET("/api/tickets/sla-status", slaHandler.GetTicketSLAStatuses)

	// Coverage audit system (SF-QA0025) — MUST-1: nil pgDB guard inside RegisterCoverageRoutes
	// coverage 需独立 PG 库，平台为 SQLite，此处传 nil 表示禁用（设计性禁用，非 bug）
	handler.RegisterCoverageRoutes(e, middleware.Auth(c.Auth, c.Token), middleware.Admin(), nil)

	// Frontend SPA (must be after API routes)
	serveFrontend(e)

	return e
}
