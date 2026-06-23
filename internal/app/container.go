// Package app 聚合应用依赖，封装 service 的构造、循环依赖注入与生命周期管理。
//
// 它替代了 cmd/server/main.go 中 ~100 行手工 wiring，以及 api.NewRouter 的 28 个位置参数。
// NewContainer 负责按正确顺序构造所有 service，处理 TicketService 的循环依赖 setter，
// 执行启动副作用（scheduler / backup / admin seed / OIDC providers / default policy），
// 并通过 Close() 提供优雅关闭。
package app

import (
	"context"
	"log"
	"time"

	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/service"
)

// Container 聚合应用启动所需的所有依赖。
// 字段按 router/handler 的消费顺序排列，service 之间的循环依赖通过 Set* 方法在
// NewContainer 内部完成注入，调用方无需关心顺序。
type Container struct {
	// 基础设施
	DB      *db.DB
	Cfg     *config.Config
	ConnMgr *connpool.Manager
	PoolMgr *driver.PoolManager

	// 认证 & 用户
	Auth *service.AuthService

	// 数据源 & 权限
	Datasource *service.DatasourceService
	Permission *service.PermissionService

	// 查询
	Query  *service.QueryService
	History *service.QueryHistoryService

	// 工单 & 审批
	Ticket         *service.TicketService
	ApprovalEngine *service.ApprovalEngine

	// 脱敏 & 审计
	MaskRule *service.MaskRuleService
	Audit    *service.AuditService

	// 导出
	Export      *service.ExportService
	ExportAsync *service.ExportAsyncService

	// 通知
	Notify                  *service.NotifyService
	FeishuWebhook           *service.FeishuWebhookService
	NotificationPreference  *service.NotificationPreferenceService
	WebhookSubscription     *service.WebhookSubscriptionService

	// SLA
	SLA *service.SLAService

	// 其他 service
	Dashboard     *service.DashboardService
	Comment       *service.CommentService
	OIDC          *service.OIDCService
	Backup        *service.BackupService
	Git           *service.GitService
	Token         *service.TokenService
	Report        *service.AuditReportService
	PermRequest   *service.PermissionRequestService
	SQLTemplate   *service.TemplateService
	Share         *service.ShareService
	WebVitals     *service.WebVitalsService
	AIReview      *service.AIReviewService

	// 调度器（需在 Close 时停止）
	ticketScheduler *service.Scheduler
	slaScheduler    *service.SLAScheduler
}

// NewContainer 构造并装配整个应用依赖图。
//
// 它复刻了原 main.go 的构造顺序，保留所有启动副作用：
//   - TicketService 的 6 个循环依赖 setter
//   - ticket scheduler + SLA scheduler 的启动
//   - backup scheduler 的启动
//   - admin 用户 seed
//   - OIDC providers 加载
//   - 审批默认策略初始化
//
// 注意：database 的 Close 由调用方持有（main 的 defer），本容器通过 Close 停止调度器与异步 service。
func NewContainer(database *db.DB, cfg *config.Config) (*Container, error) {
	connMgr := connpool.NewManager()
	poolMgr := driver.NewPoolManager()

	// --- 基础 service（无循环依赖）---
	authSvc := service.NewAuthService(database, cfg.JWT.Secret, cfg.JWT.Expiry)

	permSvc, err := service.NewPermissionService(database)
	if err != nil {
		connMgr.Close()
		poolMgr.Close()
		return nil, err
	}

	historySvc := service.NewQueryHistoryService(database)
	auditSvc := service.NewAuditService(database, 0, 0)
	exportSvc := service.NewExportService(database, auditSvc)
	exportAsyncSvc := service.NewExportAsyncService(database, exportSvc, auditSvc, cfg.DB.Path)

	dsSvc := service.NewDatasourceService(database, cfg.EncryptionKey, connMgr, poolMgr)

	// NotifyService 先构造（TicketService 依赖它，但它又依赖后续的 FeishuWebhook）
	notifySvc := service.NewNotifyService(cfg.Notify.WebhookURL, cfg.Notify.Secret)
	notifySvc.SetFeishuWebhook(cfg.Feishu.WebhookURL)
	notifySvc.SetDB(database.DB)

	// TicketService 构造（依赖 audit + notify）
	ticketSvc := service.NewTicketService(database, auditSvc, nil)

	// --- 循环依赖 setter（严格遵循原 main.go 顺序）---
	ticketSvc.SetNotifyService(notifySvc)
	ticketSvc.SetDatasourceService(dsSvc, connMgr, poolMgr, cfg.EncryptionKey)
	ticketSvc.SetPermissionService(permSvc)

	// QueryService 依赖 ds/perm/audit + 连接池
	querySvc := service.NewQueryService(database, dsSvc, historySvc, permSvc, auditSvc, cfg.EncryptionKey, connMgr, poolMgr)

	// MaskRule / PermissionRequest
	maskRuleSvc := service.NewMaskRuleService(database, permSvc, auditSvc)
	permReqSvc := service.NewPermissionRequestService(database, permSvc, auditSvc)

	// AI / Dashboard
	aiReviewSvc := service.NewAIReviewService(database.DB, cfg.AI.Provider, cfg.AI.Model, cfg.AI.APIKey, cfg.AI.BaseURL, cfg.AI.Timeout)
	dashboardSvc := service.NewDashboardService(database)

	// Backup（需 Start）
	backupSvc := service.NewBackupService(database, cfg.DB.Path, cfg.Backup)

	// AuditReport
	reportSvc := service.NewAuditReportService(database)

	// Comment / Git
	commentSvc := service.NewCommentService(database)
	gitSvc := service.NewGitService(database)

	// SLA（ticket 依赖它，它依赖 notify）
	slaSvc := service.NewSLAService(database, notifySvc)
	ticketSvc.SetSLAService(slaSvc)

	// API Token / SQL Template / Share / WebVitals
	tokenSvc := service.NewTokenService(database)
	templateSvc := service.NewSQLTemplateService(database)
	shareSvc := service.NewShareService(database)
	vitalsSvc := service.NewWebVitalsService(database)

	// OIDC（依赖 auth）
	oidcSvc := service.NewOIDCService(database, authSvc)
	if len(cfg.OIDC.Providers) > 0 {
		configProviders := make([]service.ConfigOIDCProvider, 0, len(cfg.OIDC.Providers))
		for _, p := range cfg.OIDC.Providers {
			configProviders = append(configProviders, service.ConfigOIDCProvider{
				Name: p.Name, Issuer: p.Issuer, ClientID: p.ClientID,
				ClientSecret: p.ClientSecret, RedirectURL: p.RedirectURL,
				Scopes: p.Scopes, Enabled: p.Enabled,
			})
		}
		if err := oidcSvc.LoadConfigProviders(context.Background(), configProviders); err != nil {
			log.Printf("warn: failed to load OIDC providers from config: %v", err)
		}
	}

	// ApprovalEngine（ticket + notify 依赖它）
	approvalEngine := service.NewApprovalEngine(database)
	approvalEngine.SetNotifyService(notifySvc)
	if err := approvalEngine.EnsureDefaultPolicy(context.Background()); err != nil {
		log.Printf("warn: failed to ensure default approval policy: %v", err)
	}
	ticketSvc.SetApprovalEngine(approvalEngine)
	ticketSvc.SetGitService(gitSvc)

	// router 内部曾各自 new 的 service（提到 Container，消除重复实例）
	feishuWebhookSvc := service.NewFeishuWebhookService(database.DB, cfg.EncryptionKey)
	notifySvc.SetFeishuWebhookService(feishuWebhookSvc)
	notifPrefSvc := service.NewNotificationPreferenceService(database)
	webhookSubSvc := service.NewWebhookSubscriptionService(database.DB, cfg.EncryptionKey)
	// slaSvc 已在上面构造，复用同一个实例（修复原 router.go 重复 new 的隐患）

	// --- admin seed ---
	count, err := authSvc.UserCount(context.Background())
	if err != nil {
		connMgr.Close()
		poolMgr.Close()
		return nil, err
	}
	if count == 0 {
		admin, err := authSvc.CreateUser(context.Background(), cfg.Admin.Username, cfg.Admin.Password, "admin")
		if err != nil {
			connMgr.Close()
			poolMgr.Close()
			return nil, err
		}
		log.Printf("initial admin user created: %s (id=%d)", admin.Username, admin.ID)
	}

	// --- 启动调度器与后台 service ---
	ticketScheduler := service.NewScheduler(ticketSvc, 1*time.Minute)
	ticketScheduler.Start()

	backupSvc.Start()

	slaScheduler := service.NewSLAScheduler(slaSvc, 10*time.Minute)
	slaScheduler.Start()

	return &Container{
		DB: database, Cfg: cfg, ConnMgr: connMgr, PoolMgr: poolMgr,
		Auth: authSvc, Datasource: dsSvc, Permission: permSvc,
		Query: querySvc, History: historySvc,
		Ticket: ticketSvc, ApprovalEngine: approvalEngine,
		MaskRule: maskRuleSvc, Audit: auditSvc,
		Export: exportSvc, ExportAsync: exportAsyncSvc,
		Notify: notifySvc, FeishuWebhook: feishuWebhookSvc,
		NotificationPreference: notifPrefSvc, WebhookSubscription: webhookSubSvc,
		SLA: slaSvc,
		Dashboard: dashboardSvc, Comment: commentSvc, OIDC: oidcSvc,
		Backup: backupSvc, Git: gitSvc, Token: tokenSvc,
		Report: reportSvc, PermRequest: permReqSvc,
		SQLTemplate: templateSvc, Share: shareSvc, WebVitals: vitalsSvc,
		AIReview: aiReviewSvc,
		ticketScheduler: ticketScheduler, slaScheduler: slaScheduler,
	}, nil
}

// Close 优雅关闭所有后台 service 与调度器。
// database 的 Close 由 main 的 defer 负责，此处不重复关闭。
func (c *Container) Close() {
	if c.ticketScheduler != nil {
		c.ticketScheduler.Stop()
	}
	if c.slaScheduler != nil {
		c.slaScheduler.Stop()
	}
	if c.Backup != nil {
		c.Backup.Stop()
	}
	if c.ExportAsync != nil {
		c.ExportAsync.Close()
	}
	if c.Audit != nil {
		c.Audit.Close()
	}
	if c.PoolMgr != nil {
		c.PoolMgr.Close()
	}
	if c.ConnMgr != nil {
		c.ConnMgr.Close()
	}
}
