# User Stories：审计、集成与运维

## US-OPS-001：检索审计证据

**故事**：作为 Admin，我希望按用户、动作、数据源、时间和关键字检索审计记录，以便调查问题并提供合规证据。

**关联需求**：FR-OPS-001、FR-OPS-002

**验收标准**：

- Given 操作者具有审计查看权限，When 组合筛选条件，Then 返回分页且稳定排序的记录。
- Given 使用全文关键字，When 搜索 SQL 内容或摘要，Then 返回匹配项及安全的高亮信息。
- Given 普通用户，When 直接访问管理审计 API，Then 请求被拒绝。
- Then 审计响应不包含数据源凭据、Token 哈希或服务器本地文件路径。

**代表性验证**：`internal/audit/audit_test.go`、`internal/audit/audit_fts_test.go`。

## US-OPS-002：查看治理报表

**故事**：作为 Admin，我希望查看使用、错误、性能、工单和用户行为统计，以便发现风险与流程瓶颈。

**关联需求**：FR-OPS-002

**验收标准**：

- Given 时间范围和筛选条件合法，When 打开报表，Then 指标来自相同范围内的平台审计/工单记录。
- Given 没有数据，When 查看报表，Then 显示空状态而不是虚构零以外的数据。
- Given 查询报表失败，Then UI 呈现可恢复错误且其他页面仍可使用。

**代表性验证**：`internal/audit/audit_report_test.go`。

## US-OPS-003：配置通知与 Webhook

**故事**：作为 Admin，我希望配置钉钉、飞书和通用 Webhook，并允许用户设置通知偏好，以便把工单事件发送到团队协作渠道。

**关联需求**：FR-OPS-003

**验收标准**：

- Given Webhook 配置合法，When Admin 发送测试消息，Then 返回可解释的发送结果。
- Given 工单产生订阅事件，When 通知发送失败，Then 核心工单事务保持正确，并记录失败/死信信息供重试或查看。
- Given 用户修改个人偏好，Then 后续通知遵循该偏好且用户不能修改他人偏好。
- Then Webhook Secret 和签名密钥不出现在普通设置响应中。

**代表性验证**：`internal/notify/notify_test.go`、`internal/notify/feishu_webhook_test.go`、`internal/notify/webhook_subscription_test.go`。

## US-OPS-004：备份平台元数据

**故事**：作为 Admin/Operator，我希望手动或定时备份 SQLFlow 元数据并执行保留策略，以便在平台故障时恢复治理记录。

**关联需求**：FR-OPS-004、NFR-REL-001

**验收标准**：

- Given 备份目录可写，When 触发备份，Then 生成一致的 SQLite 备份并按配置可选压缩。
- Given 备份数超过保留上限，When 清理任务执行，Then 仅删除超出策略的旧备份。
- Given 普通用户，When 请求备份下载或删除，Then 操作被拒绝。
- Then 文档和 UI 明确该备份不包含目标 MySQL/PostgreSQL/MongoDB/Elasticsearch 数据。

**代表性验证**：`internal/ops/backup_test.go`。

## US-OPS-005：监控服务健康

**故事**：作为 Operator，我希望通过存活、就绪、指标和前端体验信号监控 SQLFlow，以便自动发现故障和性能退化。

**关联需求**：FR-OPS-005、FR-OPS-006、NFR-OBS-001

**验收标准**：

- When 请求 `/healthz`，Then 只反映进程存活，不因可选外部集成失败而误判进程死亡。
- When 请求 `/readyz`，Then 检查应用处理请求所需的核心依赖并返回合适状态码。
- Given Metrics 启用，When Prometheus 抓取 `/metrics`，Then 返回标准格式指标；未启用时不暴露该端点。
- Given 浏览器提交 Web Vitals，Then 入口受限流并只记录允许的性能字段。

**代表性验证**：健康 Handler 测试、`internal/ops/web_vitals_test.go`。
