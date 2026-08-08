// Package app 是组合根：它构造所有领域 service 并管理其生命周期。
//
// 它替代了 cmd/server/main.go 中 ~100 行手工 wiring，以及 api.NewRouter 的 28 个位置参数。
// NewContainer 按依赖顺序构造 service——依赖图是 DAG，不存在循环，早期版本的
// setter 注入只是构造顺序的产物而非真实的循环依赖——并执行启动副作用
// （scheduler / backup / admin seed / OIDC providers / default policy），
// 通过 Close() 提供优雅关闭。
//
// 这是唯一知道各领域具体实现的地方。领域包之间只通过消费方定义的窄接口
// （如 datasource.ObjectViewChecker）相互引用。
package app

import (
	"context"
	"log"
	"time"

	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/audit"
	"github.com/whg517/sqlflow/internal/connpool"
	"github.com/whg517/sqlflow/internal/datasource"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
	"github.com/whg517/sqlflow/internal/iam"
	"github.com/whg517/sqlflow/internal/notify"
	"github.com/whg517/sqlflow/internal/ops"
	"github.com/whg517/sqlflow/internal/query"
	"github.com/whg517/sqlflow/internal/security"
	"github.com/whg517/sqlflow/internal/ticket"
)

// Container 聚合应用启动所需的所有依赖。
// 字段按 router/handler 的消费顺序排列；每个 service 在 NewContainer 内构造完成后
// 即不可变，调用方无需关心构造顺序。
type Container struct {
	// 基础设施
	DB      *db.DB
	Cfg     *config.Config
	ConnMgr *connpool.Manager
	PoolMgr *driver.PoolManager

	// 认证 & 用户
	Auth *iam.Service

	// 数据源 & 权限
	Datasource *datasource.Service
	Permission *security.Service

	// 查询
	Query   *query.Service
	History *query.HistoryService

	// 工单 & 审批
	Ticket         *ticket.Service
	ApprovalEngine *ticket.ApprovalEngine

	// 脱敏 & 审计
	MaskRule *security.MaskService
	Audit    *audit.Service

	// 导出
	Export      *query.ExportService
	ExportAsync *query.AsyncExportService

	// 通知
	Notify                 *notify.Service
	FeishuWebhook          *notify.FeishuService
	NotificationPreference *notify.PreferenceService
	WebhookSubscription    *notify.WebhookSubscriptionService

	// SLA
	SLA *ticket.SLAService

	// 其他 service
	Dashboard   *ops.DashboardService
	Comment     *ticket.CommentService
	OIDC        *iam.OIDCService
	Backup      *ops.BackupService
	Git         *ops.GitService
	Token       *iam.TokenService
	Report      *audit.ReportService
	PermRequest *security.RequestService
	SQLTemplate *query.TemplateService
	Share       *query.ShareService
	WebVitals   *ops.WebVitalsService
	AIReview    *ticket.AIReviewService

	// 调度器（需在 Close 时停止）
	ticketScheduler *ticket.Scheduler
	slaScheduler    *ticket.SLAScheduler
}

// NewContainer 构造并装配整个应用依赖图。
//
// 它复刻了原 main.go 的构造顺序，保留所有启动副作用：
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
	authSvc := iam.NewService(database, cfg.JWT.Secret, cfg.JWT.Expiry)

	permSvc, err := security.NewService(database)
	if err != nil {
		connMgr.Close()
		poolMgr.Close()
		return nil, err
	}

	historySvc := query.NewHistoryService(database)
	auditSvc := audit.NewService(database, 0, 0)
	exportSvc := query.NewExportService(database, permSvc, auditSvc)
	exportAsyncSvc := query.NewAsyncExportService(database, exportSvc, auditSvc, cfg.DB.DataDir)

	dsSvc := datasource.NewService(database, cfg.EncryptionKey, connMgr, poolMgr, auditSvc)
	if _, err := dsSvc.EnsureInternalDataSource(context.Background(), cfg.DB.DSN); err != nil {
		connMgr.Close()
		poolMgr.Close()
		return nil, err
	}

	// Feishu 多 webhook service 不依赖 NotifyService，先构造它就能让 NotifyService
	// 一次性拿全依赖，不再需要事后 setter。
	feishuWebhookSvc := notify.NewFeishuService(database, cfg.EncryptionKey)
	notifySvc := notify.NewService(notify.Deps{
		WebhookURL: cfg.Notify.WebhookURL,
		Secret:     cfg.Notify.Secret,
		FeishuURL:  cfg.Feishu.WebhookURL,
		Feishu:     feishuWebhookSvc,
		DB:         database,
	})

	// QueryService 依赖 ds/perm/audit + 连接池
	querySvc := query.NewService(database, dsSvc, historySvc, permSvc, auditSvc, cfg.EncryptionKey, poolMgr)

	// MaskRule / PermissionRequest
	maskRuleSvc := security.NewMaskService(database, permSvc, auditSvc)
	permReqSvc := security.NewRequestService(database, permSvc, auditSvc)

	// AI / Dashboard
	aiReviewSvc := ticket.NewAIReviewService(database, cfg.AI.Provider, cfg.AI.Model, cfg.AI.APIKey, cfg.AI.BaseURL, cfg.AI.Timeout)
	dashboardSvc := ops.NewDashboardService(database)

	// Backup（需 Start）
	backupSvc := ops.NewBackupService(cfg.DB.DSN, cfg.Backup)

	// AuditReport
	reportSvc := audit.NewReportService(database)

	// Comment / Git
	commentSvc := ticket.NewCommentService(database)
	gitSvc := ops.NewGitService(database)

	// SLA（ticket 依赖它，它依赖 notify）
	slaSvc := ticket.NewSLAService(database, notifySvc)

	// API Token / SQL Template / Share / WebVitals
	tokenSvc := iam.NewTokenService(database)
	templateSvc := query.NewTemplateService(database)
	shareSvc := query.NewShareService(database, cfg.JWT.Secret)
	vitalsSvc := ops.NewWebVitalsService(database)

	// OIDC（依赖 auth）
	oidcSvc := iam.NewOIDCService(database, authSvc)
	if len(cfg.OIDC.Providers) > 0 {
		configProviders := make([]iam.ConfigOIDCProvider, 0, len(cfg.OIDC.Providers))
		for _, p := range cfg.OIDC.Providers {
			configProviders = append(configProviders, iam.ConfigOIDCProvider{
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
	approvalEngine := ticket.NewApprovalEngine(database, notifySvc)
	if err := approvalEngine.EnsureDefaultPolicy(context.Background()); err != nil {
		log.Printf("warn: failed to ensure default approval policy: %v", err)
	}

	// TicketService 最后构造：它是依赖图的汇点，所有协作者到此都已就绪。
	// 之前这里是「先 new 再六个 setter」，服务在整个生命周期里都可变；现在依赖
	// 在构造时全部固定，忘记接线会是编译期就能看见的空字段而非运行期的 nil。
	ticketSvc := ticket.New(ticket.Deps{
		DB:             database,
		Audit:          auditSvc,
		Notify:         notifySvc,
		Git:            gitSvc,
		SLA:            slaSvc,
		Datasource:     dsSvc,
		PoolManager:    poolMgr,
		EncryptionKey:  cfg.EncryptionKey,
		ApprovalEngine: approvalEngine,
	})

	// router 内部曾各自 new 的 service（提到 Container，消除重复实例）
	notifPrefSvc := notify.NewPreferenceService(database)
	webhookSubSvc := notify.NewWebhookSubscriptionService(database, cfg.EncryptionKey)
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
	ticketScheduler := ticket.NewScheduler(ticketSvc, 1*time.Minute)
	ticketScheduler.Start()

	backupSvc.Start()

	slaScheduler := ticket.NewSLAScheduler(slaSvc, 10*time.Minute)
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
		SLA:       slaSvc,
		Dashboard: dashboardSvc, Comment: commentSvc, OIDC: oidcSvc,
		Backup: backupSvc, Git: gitSvc, Token: tokenSvc,
		Report: reportSvc, PermRequest: permReqSvc,
		SQLTemplate: templateSvc, Share: shareSvc, WebVitals: vitalsSvc,
		AIReview:        aiReviewSvc,
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
