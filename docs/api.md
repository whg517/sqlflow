# SQLFlow API 文档

> **基础 URL**: `http://<host>:8080`
> **事实源**：本文档以 [`internal/api/router.go`](../internal/api/router.go) 为唯一事实源。若与代码不一致，以代码为准。
> **Swagger UI**：运行时访问 `/swagger`（由 `make docs` 从注解生成 `docs/swagger.{json,yaml}`）。
> **认证**：除标注「公开」的端点外，所有 `/api/*` 端点需 `Authorization: Bearer <JWT>` 或 `Authorization: Bearer <API-Token>`（API Token 用于自动化场景，独立 scope）。
> **权限约定**：`公开` = 无需认证；`登录` = 任意已认证用户；`Admin` / `DBA` = 按角色鉴权。

## 通用响应格式

### 成功响应

```json
{ "code": 0, "message": "ok", "data": { ... } }
```

### 分页响应

```json
{ "code": 0, "message": "ok", "data": [ ... ], "page": 1, "page_size": 50, "total": 100 }
```

### 错误响应

```json
{ "code": 400, "message": "错误描述" }
```

常见 HTTP 状态码：`400`（参数错误）、`401`（未认证）、`403`（无权限）、`404`（不存在）、`500`（内部错误）。

---

## 系统端点（公开）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/health`、`/api/health` | 健康检查（含依赖状态）|
| `GET` | `/healthz` | Liveness 探针（不检查依赖）|
| `GET` | `/readyz` | Readiness 探针（检查所有依赖）|
| `GET` | `/metrics` | Prometheus 指标（需启用 `metrics.enabled`）|
| `GET` | `/swagger/*` | Swagger UI |

---

## 认证（Auth）

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/auth/login` | 用户名密码登录 | 公开 |
| `POST` | `/api/auth/refresh` | 刷新 Access Token | 公开 |
| `GET` | `/api/auth/oidc/:provider` | OIDC 登录跳转 | 公开 |
| `GET` | `/api/auth/oidc/:provider/callback` | OIDC 回调 | 公开 |
| `GET` | `/api/auth/providers` | 已启用的登录方式列表 | 公开 |
| `GET` | `/api/auth/me` | 当前用户信息 | 登录 |
| `PUT` | `/api/auth/password` | 修改密码 | 登录 |

### POST /api/auth/login

**请求体：**

```json
{ "username": "admin", "password": "admin123" }
```

**响应：**

```json
{
  "code": 0, "message": "ok",
  "data": {
    "token": "eyJhbGci...",
    "user": { "id": 1, "username": "admin", "role": "admin" }
  }
}
```

### PUT /api/auth/password

**请求体：** `{ "old_password": "admin123", "new_password": "newPass456" }`

### GET /api/health

**响应：** `{ "status": "ok" }`

---

## 用户管理（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/users` | 创建用户（`role`: admin/dba/developer）|
| `GET` | `/api/users` | 用户列表（`page`、`page_size`）|
| `GET` | `/api/users/:id` | 用户详情 |
| `PUT` | `/api/users/:id` | 编辑用户（角色、状态）|
| `DELETE` | `/api/users/:id` | 删除用户（不能删自己）|
| `PUT` | `/api/users/:id/reset-password` | 重置密码 |

### POST /api/users

**请求体：**

```json
{ "username": "developer1", "password": "pass1234", "role": "developer" }
```

`role` 可选值：`admin`、`dba`、`developer`。响应 HTTP 201。

---

## 数据源管理

### 管理员接口（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/datasources` | 创建数据源 |
| `GET` | `/api/datasources` | 数据源列表 |
| `GET` | `/api/datasources/:id` | 数据源详情 |
| `PUT` | `/api/datasources/:id` | 编辑数据源 |
| `DELETE` | `/api/datasources/:id` | 禁用数据源 |
| `POST` | `/api/datasources/:id/test` | 测试连接 |

### 查询接口（登录）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/datasources/:id/tables` | 库表列表（用于自动补全，`database` 可选）|
| `GET` | `/api/datasources/:id/tables/:name/columns` | 表字段元数据 |
| `GET` | `/api/datasources/:id/es/indices` | Elasticsearch 索引列表 |
| `GET` | `/api/datasources/:id/es/indices/:index/fields` | ES 索引字段映射 |

### POST /api/datasources

**请求体：**

```json
{
  "name": "mysql-prod",
  "type": "mysql",
  "host": "10.0.0.1",
  "port": 3306,
  "username": "readonly",
  "password": "secret",
  "database": "app_db",
  "max_open": 10, "max_idle": 5,
  "max_lifetime": 300, "max_idle_time": 180
}
```

`type` 可选值：`mysql`、`postgresql`、`mongodb`、`elasticsearch`（密码不返回）。`max_lifetime` / `max_idle_time` 单位秒。

### GET /api/datasources/:id/tables

**响应：**

```json
{
  "code": 0,
  "data": {
    "databases": ["app_db", "analytics"],
    "tables": { "app_db": ["users", "orders", "products"] }
  }
}
```

### POST /api/datasources/:id/test

**响应：** `{ "code": 0, "message": "连接成功" }`

---

## 权限管理（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/roles` | 角色列表 |
| `GET` | `/api/roles/:role` | 角色详情（含权限策略）|
| `POST` | `/api/policies` | 添加权限策略 |
| `GET` | `/api/policies` | 策略列表 |
| `DELETE` | `/api/policies/:id` | 删除策略 |
| `POST` | `/api/policies/sync` | 同步 Casbin 策略到内存 |

### POST /api/policies

**请求体：**

```json
{
  "role": "developer",
  "domain": "ds_1",
  "object": "orders",
  "action": "select"
}
```

`domain` 为数据源标识（`ds_<id>`），`object` 为表名（`*` 代表全部），`action` 可选值：`select`、`update`、`delete`、`ddl`、`export`、`desensitize:bypass`、`*`。

---

## 权限申请流

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/permission-requests` | 发起权限申请（带有效期）| 登录 |
| `GET` | `/api/permission-requests/mine` | 我的申请 | 登录 |
| `GET` | `/api/permission-requests/active` | 我的生效中申请 | 登录 |
| `GET` | `/api/permission-requests/:id` | 申请详情 | 登录 |
| `GET` | `/api/permission-requests` | 全部申请列表 | Admin |
| `POST` | `/api/permission-requests/:id/approve` | 审批通过 | Admin |
| `POST` | `/api/permission-requests/:id/reject` | 审批驳回 | Admin |
| `POST` | `/api/permission-requests/:id/revoke` | 撤销申请 | Admin |
| `POST` | `/api/permission-requests/expire` | 过期清理（定时调用）| Admin |

---

## SQL 查询

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/query/execute` | 执行查询 | 登录 |
| `POST` | `/api/query/explain` | 执行计划 | 登录 |
| `POST` | `/api/query/review` | AI 评审（SSE）| 登录 |
| `POST` | `/api/query/export` | 同步导出（CSV/JSON/Excel）| 登录 |

### POST /api/query/execute

**请求体：**

```json
{
  "datasource_id": 1,
  "database": "app_db",
  "sql": "SELECT * FROM users LIMIT 10"
}
```

**响应（低风险，直接执行）：**

```json
{
  "code": 0,
  "data": {
    "columns": ["id", "name", "email"],
    "rows": [ { "id": 1, "name": "张**", "email": "z***g@example.com" } ],
    "total": 1,
    "execution_time_ms": 12,
    "affected_rows": 0,
    "desensitized": true,
    "desensitized_fields": ["name", "email"],
    "warnings": []
  }
}
```

**错误场景**：高风险拦截 → `403`；语法错误 → `400`；超时 → `400`；权限不足 → `403`。

### POST /api/query/export

**请求体：**

```json
{
  "datasource_id": 1, "database": "app_db",
  "sql": "SELECT * FROM users LIMIT 100",
  "format": "csv", "columns": ["id", "name"]
}
```

`format` 可选值：`csv`、`json`、`xlsx`。单次同步导出上限 10000 行，更大请走异步导出任务。响应为文件下载（非 JSON）。

### POST /api/query/review

AI 流式评审，返回 Server-Sent Events。`Content-Type: text/event-stream`。

**请求体：**

```json
{
  "datasource_id": 1, "database": "app_db",
  "sql": "UPDATE users SET status = 'inactive' WHERE last_login < '2025-01-01'"
}
```

SSE 事件：`content`（流式片段）、`result`（最终结果 JSON）、`error`、`done`。

**`result` 事件数据：**

```json
{
  "risk_level": "high", "risk_score": 85, "decision": "ticket",
  "summary": "批量更新操作，影响范围较大",
  "suggestions": ["建议添加 LIMIT 限制"],
  "impact_analysis": "预计影响约 5000 行",
  "rollback_sql": "UPDATE users SET status = 'active' WHERE ...",
  "warnings": ["无 WHERE 条件的 UPDATE/DELETE 为高风险操作"],
  "review_source": "ai",
  "reviewed_at": "2026-05-01T10:00:00Z",
  "model_used": "gpt-4"
}
```

`decision` 可选值：`execute`（低风险免审）、`confirm`（中风险确认）、`ticket`（高风险走工单）、`blocked`（拦截）、`fallback`（AI 不可用静态兜底）。

---

## 查询历史

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/query/history` | 当前用户历史（`page`、`page_size`）|
| `GET` | `/api/query/history/frequent` | 频繁查询推荐 |
| `DELETE` | `/api/query/history/:id` | 删除单条（仅自己）|
| `DELETE` | `/api/query/history` | 清空自己的历史 |

---

## 查询结果分享

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/query/share` | 创建分享链接（可设密码）| 登录 |
| `GET` | `/api/query/share` | 我的分享列表 | 登录 |
| `DELETE` | `/api/query/share/:id` | 撤销分享 | 登录 |
| `GET` | `/s/:token` | 访问分享（公开）| 公开 |
| `POST` | `/s/:token/verify` | 校验分享密码 | 公开 |

---

## 性能分析

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/query/performance/slow` | 慢查询列表 |
| `GET` | `/api/query/performance/stats` | 性能统计 |

---

## 工单管理

### 工单基础（登录）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/tickets` | 创建工单 |
| `GET` | `/api/tickets` | 工单列表 |
| `GET` | `/api/tickets/:id` | 工单详情 |
| `GET` | `/api/tickets/:id/execution-results` | 执行结果 |
| `PUT` | `/api/tickets/:id/resubmit` | 重新提交（修订 SQL）|
| `GET` | `/api/tickets/:id/revisions` | 修订历史 |
| `GET` | `/api/tickets/:id/comments` | 工单评论列表 |
| `POST` | `/api/tickets/:id/comments` | 添加评论 |
| `DELETE` | `/api/comments/:id` | 删除评论 |

### 工单流转（按角色）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/tickets/batch-approve` | 批量审批 |
| `POST` | `/api/tickets/batch-reject` | 批量驳回 |
| `POST` | `/api/tickets/:id/approve` | 审批通过（DBA/Admin）|
| `POST` | `/api/tickets/:id/reject` | 审批驳回（DBA/Admin）|
| `POST` | `/api/tickets/:id/cancel` | 取消工单（提交人/DBA/Admin）|
| `POST` | `/api/tickets/:id/execute` | 执行 SQL（提交人/DBA/Admin）|
| `POST` | `/api/tickets/:id/schedule` | 定时执行 |
| `POST` | `/api/tickets/:id/cancel-schedule` | 取消定时 |

### 工单状态机

`SUBMITTED` → `AI_REVIEWED` → `PENDING_APPROVAL` → `APPROVED` → `EXECUTING` → `DONE` / `REJECTED` / `CANCELLED`

### POST /api/tickets

**请求体：**

```json
{
  "datasource_id": 1, "database": "app_db",
  "sql": "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
  "db_type": "mysql",
  "change_reason": "业务需要记录用户手机号",
  "risk_level": "medium",
  "ai_review_result": ""
}
```

### GET /api/tickets（查询参数）

| 参数 | 说明 |
|------|------|
| `page` / `page_size` | 分页 |
| `status` | 按状态筛选 |
| `datasource_id` | 按数据源筛选 |
| `submitter_id` | 按提交人筛选 |
| `risk_level` | 按风险等级筛选 |
| `keyword` | 关键词搜索 |
| `scope` | `mine`（我的）/ `pending`（待审批）|

---

## 审批引擎（Approval Engine）

### 审批链与动作（登录）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/tickets/:id/approval-chain` | 审批链查询 |
| `POST` | `/api/tickets/:id/engine-approve` | 引擎驱动审批 |
| `GET` | `/api/tickets/:id/approval-history` | 审批历史 |

### 审批策略管理（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/admin/approval-policies` | 策略列表 |
| `POST` | `/api/admin/approval-policies` | 创建策略（条件匹配 + 审批节点）|
| `PUT` | `/api/admin/approval-policies/reorder` | 策略排序 |
| `GET` | `/api/admin/approval-policies/approvers` | 可用审批人列表 |
| `GET` | `/api/admin/approval-policies/:id` | 策略详情 |
| `PUT` | `/api/admin/approval-policies/:id` | 更新策略 |
| `DELETE` | `/api/admin/approval-policies/:id` | 删除策略 |
| `PUT` | `/api/admin/approval-policies/:id/toggle` | 启停策略 |

> 旧路径 `/api/approval/policies/*` 保留向后兼容，行为与 `/api/admin/approval-policies/*` 一致。

---

## SLA 管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `GET` | `/api/tickets/sla-status` | 工单 SLA 状态 | 登录 |
| `GET` | `/api/settings/sla` | SLA 配置列表 | Admin |
| `POST` | `/api/settings/sla` | 创建 SLA 配置 | Admin |
| `PUT` | `/api/settings/sla/:id` | 更新配置 | Admin |
| `DELETE` | `/api/settings/sla/:id` | 删除配置 | Admin |
| `GET` | `/api/sla-notifications` | SLA 通知记录 | Admin |

SLA 调度器每 10 分钟巡检，工单超时自动驳回并通知 DBA。

---

## 脱敏规则（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/mask-rules` | 创建脱敏规则 |
| `GET` | `/api/mask-rules` | 规则列表（`datasource_id`、`database`、`table_name`）|
| `GET` | `/api/mask-rules/:id` | 规则详情 |
| `PUT` | `/api/mask-rules/:id` | 编辑规则 |
| `DELETE` | `/api/mask-rules/:id` | 删除规则 |

### POST /api/mask-rules

**请求体：**

```json
{
  "datasource_id": 1, "database": "app_db",
  "table_name": "users", "field": "phone",
  "mask_type": "phone", "custom_regex": "", "custom_template": ""
}
```

`mask_type` 可选值：`phone`、`id_card`、`name`、`email`、`bank_card`、`address`、`full_mask`、`custom`。`custom` 时需提供 `custom_regex` + `custom_template`。

---

## 敏感表管理（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/sensitive-tables` | 标记敏感表 |
| `GET` | `/api/sensitive-tables` | 敏感表列表 |
| `DELETE` | `/api/sensitive-tables/:id` | 取消标记 |

### POST /api/sensitive-tables

**请求体：**

```json
{
  "datasource_id": 1, "database": "app_db",
  "table_name": "users", "sensitivity_level": "high"
}
```

`sensitivity_level` 可选值：`low`、`medium`（默认）、`high`。

---

## 审计日志（Admin/DBA）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/audit-logs` | 审计日志（`page`、`page_size`、`user_id`、`action`、`datasource_id`、`start`、`end`、`keyword`）|
| `GET` | `/api/audit-logs/search` | FTS 全文检索 |

**响应条目字段**：`id`、`user_id`、`username`、`action`、`datasource_id`、`database`、`sql_content`、`sql_summary`、`result_rows`、`affected_rows`、`execution_time_ms`、`error_message`、`desensitized_fields`、`ip_address`、`created_at`。

审计日志应用层面**不支持删除**。

---

## 审计报表（Admin/DBA）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/reports/usage` | 使用量统计 |
| `GET` | `/api/reports/errors` | 错误统计 |
| `GET` | `/api/reports/performance` | 性能报表 |
| `GET` | `/api/reports/tickets` | 工单报表 |
| `GET` | `/api/audit/user-analytics` | 用户行为分析（Admin）|

---

## 导出

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `GET` | `/api/export/audit` | 审计日志导出 | Admin/DBA |
| `GET` | `/api/export/tickets` | 工单导出 | 登录 |
| `GET` | `/api/export/tasks` | 异步导出任务列表 | 登录 |
| `GET` | `/api/export/tasks/:id` | 任务详情 | 登录 |
| `GET` | `/api/export/tasks/:id/download` | 下载导出文件 | 登录 |

异步导出用于大数据量场景，任务异步执行后通过 `download` 端点拉取。

---

## 数据库备份（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/backups` | 触发手动备份 |
| `GET` | `/api/backups` | 备份列表 |
| `GET` | `/api/backups/:filename/download` | 下载备份（gzip）|
| `DELETE` | `/api/backups/:filename` | 删除备份 |

---

## 通知与设置（Admin）

### 系统设置

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/settings` | 获取系统配置（钉钉 + AI + 飞书）|
| `PUT` | `/api/settings/notify/webhook` | 更新钉钉配置（旧路径 `/api/settings/dingtalk` 已废弃）|
| `POST` | `/api/settings/notify/webhook/test` | 测试钉钉消息（旧 `/test`）|
| `PUT` | `/api/settings/ai` | 更新 AI 配置 |
| `PUT` | `/api/settings/feishu` | 更新飞书默认配置 |
| `POST` | `/api/settings/feishu/test` | 测试飞书消息 |

### GET /api/settings

**响应：**

```json
{
  "code": 0,
  "data": {
    "dingtalk": { "webhook_url": "https://...", "secret": "SEC***", "enabled": true },
    "ai": { "provider": "openai", "model": "gpt-4", "api_key": "sk-1***abcd", "base_url": "https://api.openai.com/v1", "timeout": "10s", "enabled": true }
  }
}
```

### 飞书 Webhook 管理（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/settings/feishu/webhooks` | 创建 Webhook（支持多实例 + 加签）|
| `GET` | `/api/settings/feishu/webhooks` | Webhook 列表 |
| `GET` | `/api/settings/feishu/webhooks/:id` | Webhook 详情 |
| `PUT` | `/api/settings/feishu/webhooks/:id` | 更新 |
| `DELETE` | `/api/settings/feishu/webhooks/:id` | 删除 |
| `GET` | `/api/settings/feishu/webhooks/dead-letters` | 死信队列 |

### 通用 Webhook 订阅（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/admin/webhooks/subscriptions` | 订阅列表 |
| `POST` | `/api/admin/webhooks/subscriptions` | 创建订阅（事件类型 + 回调地址）|
| `GET` | `/api/admin/webhooks/subscriptions/:id` | 详情 |
| `PUT` | `/api/admin/webhooks/subscriptions/:id` | 更新 |
| `DELETE` | `/api/admin/webhooks/subscriptions/:id` | 删除 |
| `POST` | `/api/admin/webhooks/subscriptions/:id/toggle` | 启停 |
| `POST` | `/api/admin/webhooks/subscriptions/:id/test` | 测试发送 |

### 用户通知偏好（登录）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/notifications/preferences` | 我的偏好 |
| `PUT` | `/api/notifications/preferences` | 更新偏好（按事件开关）|

---

## API Token

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/tokens` | 创建 Token（自己）| 登录 |
| `GET` | `/api/tokens` | 我的 Token | 登录 |
| `GET` | `/api/tokens/stats` | Token 使用统计 | 登录 |
| `DELETE` | `/api/tokens/:id` | 撤销自己的 Token | 登录 |
| `GET` | `/api/admin/tokens` | 全部 Token | Admin |
| `DELETE` | `/api/admin/tokens/:id` | 撤销任意 Token | Admin |

API Token 用于 CI/CD 自动化，独立于 JWT，支持 scope 控制。

---

## SQL 模板

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/sql-templates` | 创建模板 | 登录 |
| `GET` | `/api/sql-templates` | 模板列表 | 登录 |
| `GET` | `/api/sql-templates/:id` | 详情 | 登录 |
| `PUT` | `/api/sql-templates/:id` | 更新 | 登录 |
| `DELETE` | `/api/sql-templates/:id` | 删除 | 登录 |
| `POST` | `/api/sql-templates/:id/render` | 渲染（`{{var}}` 参数化）| 登录 |

---

## Git 关联链接

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/git-links` | 关联 Git 提交/分支 | 登录 |
| `GET` | `/api/git-links` | 链接列表 | 登录 |
| `DELETE` | `/api/git-links/:id` | 删除 | 登录 |

---

## 代码覆盖率审计（Coverage，可选）

> **可选模块**：需独立配置 PostgreSQL 数据库（schema 使用 PG 方言 `BIGSERIAL`/`JSONB`/`INTEGER[]`，无法运行在平台 SQLite 上）。未配置时路由不注册。迁移 SQL 见 [`internal/coverage/migration/`](../internal/coverage/migration)。

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/v1/coverage/reports` | 上传覆盖率报告 | 登录 |
| `POST` | `/api/v1/coverage/reports/merge` | 合并报告 | 登录 |
| `GET` | `/api/v1/coverage/reports` | 报告列表 | 登录 |
| `GET` | `/api/v1/coverage/projects/:project` | 项目覆盖率 | 登录 |
| `GET` | `/api/v1/coverage/projects/:project/history` | 覆盖率历史 | 登录 |
| `GET` | `/api/v1/coverage/reports/:id/modules/:moduleID/files` | 模块文件 | 登录 |
| `GET` | `/api/v1/coverage/gate-configs` | 质量门配置 | Admin |
| `POST` | `/api/v1/coverage/gate-configs` | 创建门配置 | Admin |
| `PUT` | `/api/v1/coverage/gate-configs/:id` | 更新门配置 | Admin |
| `DELETE` | `/api/v1/coverage/gate-configs/:id` | 删除门配置 | Admin |

---

## 前端 Web Vitals 采集（公开）

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/metrics/web-vitals` | 上报 Core Web Vitals | 公开（限流）|

---

## 变更说明

本文档由 `internal/api/router.go` 自动核对。新增端点时请同步更新本文档，并保持与 `make docs` 生成的 Swagger 一致。如发现不一致，**以 `router.go` 为准**并提 PR 修正本文档。
