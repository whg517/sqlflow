# User Stories：查询与结果

## US-QRY-001：执行受控只读查询

**故事**：作为 Developer，我希望在被授权的数据源上执行只读查询，以便无需共享生产凭据即可排障和取数。

**关联需求**：FR-QRY-001、FR-QRY-002、FR-SEC-001

**验收标准**：

- Given 用户对数据源和目标对象有 `select` 权限，When 提交合法只读查询，Then 在超时和行数限制内返回结果。
- Given SQL 是 DDL/DML 或被风险规则阻断，When 从查询入口执行，Then 不访问写执行路径并提示提交工单。
- Given 用户缺少表级权限或临时权限已过期，When 查询目标对象，Then 请求被拒绝并留下必要审计上下文。
- Given 目标数据源不可达或查询超过 30 秒，When 执行，Then 请求受控失败且不会泄露连接凭据。

**代表性验证**：`internal/query/query_test.go`。

## US-QRY-002：获得查询分析与 AI 建议

**故事**：作为查询用户，我希望在执行前查看语法、风险、EXPLAIN 和优化建议，以便降低慢查询和误操作概率。

**关联需求**：FR-QRY-004

**验收标准**：

- Given 查询可解析，When 请求分析，Then 返回操作类型、目标对象、风险、警告或执行计划信息。
- Given AI 已配置，When 发起评审，Then 通过流式响应逐步提供建议。
- Given AI 未配置、超时或失败，When 发起评审，Then 返回确定性规则结果且核心查询功能仍可用。
- Then AI 建议不得改变服务端权限判断或直接触发执行。

**代表性验证**：`internal/ticket/ai_review_test.go`、`internal/ticket/sql_analyzer_test.go`。

## US-QRY-003：查看和管理查询历史

**故事**：作为查询用户，我希望查看自己的查询历史和高频查询，以便复用语句并分析执行表现。

**关联需求**：FR-QRY-005、FR-QRY-009

**验收标准**：

- Given 用户已执行查询，When 查看历史，Then 返回其本人的 SQL 摘要、目标、耗时和结果行数。
- Given 用户 A，When 请求删除用户 B 的历史记录，Then 操作被拒绝或表现为资源不存在。
- Given 历史量超过配置上限，Then 系统按用户执行保留限制。
- When 查看慢查询/性能统计，Then 数据来源和时间范围清晰，不宣称为目标数据库全局指标。

**代表性验证**：`internal/query/query_history_test.go`、`internal/query/performance_test.go`。

## US-QRY-004：导出受保护结果

**故事**：作为获授权用户，我希望将查询、工单或审计数据导出为 CSV/XLSX，以便离线分析和留档。

**关联需求**：FR-QRY-006、FR-SEC-004

**验收标准**：

- Given 用户拥有导出和目标数据访问权限，When 发起导出，Then 导出内容遵循查询行限制、筛选和脱敏规则。
- Given 导出量进入异步路径，When 创建任务，Then 用户可查看状态并在完成后下载自己的文件。
- Given 普通用户请求 Admin 审计导出或他人任务文件，Then 操作被拒绝。
- Given 导出失败，Then 任务状态为 failed，并返回不含本地文件路径和凭据的错误。

**代表性验证**：`internal/query/query_export_test.go`、`internal/query/export_async_test.go`。

## US-QRY-005：安全分享结果快照

**故事**：作为查询用户，我希望把结果快照通过限时链接分享给协作者，以便在不授予数据库权限的情况下沟通数据。

**关联需求**：FR-QRY-007

**验收标准**：

- Given 已脱敏的查询结果，When 创建有效期和可选密码的分享，Then 获得不可预测的公开 Token。
- Given 分享未到期且未撤销，When 访问无密码链接或通过密码验证，Then 只返回保存的结果快照。
- Given 分享过期、撤销或密码错误，When 访问，Then 不返回结果内容。
- Given 创建者撤销分享，Then 后续公开访问立即失败。

**代表性验证**：`internal/query/share_test.go`。

## US-QRY-006：复用 SQL 模板

**故事**：作为查询用户，我希望保存带参数的 SQL 模板并控制是否公开，以便团队复用经过验证的查询模式。

**关联需求**：FR-QRY-008

**验收标准**：

- Given 用户创建私有模板，Then 其他普通用户不可修改或删除它。
- Given 用户创建私有模板，When 其他用户按 ID 读取或渲染，Then 返回不存在且不泄露模板内容。
- Given 模板公开，When 其他用户浏览列表，Then 可以发现并渲染模板，但所有权仍受保护。
- Given 参数不完整或不合法，When 渲染模板，Then 返回校验错误且不执行 SQL。
- Given MySQL 或 PostgreSQL 的只读模板已成功渲染，When 用户选择“在查询中使用”，Then 工作台新建标签并保留模板来源、数据库类型和有序参数。
- Given 模板查询被执行或导出（以及执行 MySQL EXPLAIN），Then 参数由数据库驱动绑定而不是拼接到 SQL；查询历史保存参数并可恢复。
- Given 用户编辑从模板带入的 SQL，Then 原参数绑定立即解除，避免参数位置与新语句错配。
- Given 模板是 MongoDB 或变更语句，Then 只提供渲染预览和复制，不绕过查询只读门禁或工单流程。

**代表性验证**：`internal/query/sql_template_test.go`、`internal/driver/mysql/mysql_test.go`、`internal/driver/postgresql/postgresql_test.go`、`web/src/store/__tests__/queryStore.test.ts`。
