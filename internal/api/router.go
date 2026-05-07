package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/api/handler"
	"github.com/whg517/sqlflow/internal/api/middleware"
	"github.com/whg517/sqlflow/internal/service"
)

// NewRouter creates and configures an Echo instance with middleware and routes.
func NewRouter(authSvc *service.AuthService, dsSvc *service.DatasourceService, permSvc *service.PermissionService, querySvc *service.QueryService, historySvc *service.QueryHistoryService, ticketSvc *service.TicketService, maskRuleSvc *service.MaskRuleService, aiReviewSvc *service.AIReviewService, auditSvc *service.AuditService, notifySvc *service.NotifyService) *echo.Echo {
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
	dsHandler := handler.NewDatasourceHandler(dsSvc)
	permHandler := handler.NewPermissionHandler(permSvc)
	queryHandler := handler.NewQueryHandler(querySvc, historySvc)
	ticketHandler := handler.NewTicketHandler(ticketSvc)
	maskRuleHandler := handler.NewMaskRuleHandler(maskRuleSvc)
	aiReviewHandler := handler.NewAIReviewHandler(aiReviewSvc, dsSvc)
	auditHandler := handler.NewAuditHandler(auditSvc)

	// Public routes
	e.POST("/api/auth/login", userHandler.Login)

	// Authenticated routes
	authGroup := e.Group("", middleware.JWT(authSvc))
	authGroup.GET("/api/auth/me", userHandler.Me)
	authGroup.PUT("/api/auth/password", userHandler.ChangePassword)

	// Tables endpoint: authenticated users can access
	authGroup.GET("/api/datasources/:id/tables", dsHandler.GetTables)

	// Query execution & history (authenticated users)
	authGroup.POST("/api/query/execute", queryHandler.ExecuteQuery)
	authGroup.POST("/api/query/review", aiReviewHandler.ReviewStream)
	authGroup.POST("/api/query/export", queryHandler.ExportQuery)
	authGroup.GET("/api/query/history", queryHandler.ListHistory)
	authGroup.DELETE("/api/query/history/:id", queryHandler.DeleteHistory)
	authGroup.DELETE("/api/query/history", queryHandler.ClearHistory)

	// Ticket routes (authenticated users can create/list/view; approve/reject/execute restricted by role)
	authGroup.POST("/api/tickets", ticketHandler.CreateTicket)
	authGroup.GET("/api/tickets", ticketHandler.ListTickets)
	authGroup.GET("/api/tickets/:id", ticketHandler.GetTicket)
	authGroup.POST("/api/tickets/:id/approve", ticketHandler.ApproveTicket)
	authGroup.POST("/api/tickets/:id/reject", ticketHandler.RejectTicket)
	authGroup.POST("/api/tickets/:id/cancel", ticketHandler.CancelTicket)
	authGroup.POST("/api/tickets/:id/execute", ticketHandler.ExecuteTicket)

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

	// Notification & Settings (admin)
	notifyHandler := handler.NewNotifyHandler(notifySvc, aiReviewSvc)
	adminGroup.GET("/api/settings", notifyHandler.GetSettings)
	adminGroup.PUT("/api/settings/dingtalk", notifyHandler.UpdateNotifyConfig)
	adminGroup.POST("/api/settings/dingtalk/test", notifyHandler.TestNotify)
	adminGroup.PUT("/api/settings/ai", notifyHandler.UpdateAIConfig)

	return e
}
