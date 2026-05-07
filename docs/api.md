# SQLFlow API 文档

> 基础 URL: `http://<host>:8080`
> 除 `/api/auth/login` 和 `/api/health` 外，所有 API 均需 JWT 认证。
> 认证方式：`Authorization: Bearer <token>`

## 通用响应格式

### 成功响应

```json
{
  "code": 0,
  "message": "ok",
  "data": { ... }
}
```

### 分页响应

```json
{
  "code": 0,
  "message": "ok",
  "data": [ ... ],
  "page": 1,
  "page_size": 50,
  "total": 100
}
```

### 错误响应

```json
{
  "code": 400,
  "message": "错误描述"
}
```

常见 HTTP 状态码：`400`（参数错误）、`401`（未认证）、`403`（无权限）、`404`（不存在）、`500`（内部错误）。

---

## 认证

### POST /api/auth/login

用户名密码登录，返回 JWT。

**请求体：**

```json
{
  "username": "admin",
  "password": "admin123"
}
```

**响应：**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "eyJhbGci...",
    "user": {
      "id": 1,
      "username": "admin",
      "role": "admin"
    }
  }
}
```

---

### GET /api/auth/me

获取当前用户信息。

**响应：**

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "username": "admin",
    "role": "admin",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z"
  }
}
```

---

### PUT /api/auth/password

修改当前用户密码。

**请求体：**

```json
{
  "old_password": "admin123",
  "new_password": "newPass456"
}
```

**响应：**

```json
{
  "code": 0,
  "message": "密码修改成功"
}
```

---

## 健康检查

### GET /api/health

无需认证。

**响应：**

```json
{
  "status": "ok"
}
```

---

## 用户管理（Admin）

### POST /api/users

创建用户。需 admin 角色。

**请求体：**

```json
{
  "username": "developer1",
  "password": "pass1234",
  "role": "developer"
}
```

`role` 可选值：`admin`、`dba`、`developer`。

**响应：** HTTP 201，返回用户对象。

---

### GET /api/users

获取用户列表。需 admin 角色。

**查询参数：** `page`（默认 1）、`page_size`（默认 50）。

**响应：** 分页格式的用户列表。

---

### GET /api/users/:id

获取用户详情。需 admin 角色。

---

### PUT /api/users/:id

编辑用户（角色、状态）。需 admin 角色。

**请求体：**

```json
{
  "role": "dba",
  "status": "active"
}
```

---

### DELETE /api/users/:id

删除用户（不能删除自己）。需 admin 角色。

---

### PUT /api/users/:id/reset-password

重置用户密码。需 admin 角色。

**请求体：**

```json
{
  "new_password": "newPass456"
}
```

---

## 数据源管理（Admin）

### POST /api/datasources

添加数据源。需 admin 角色。

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
  "max_open": 10,
  "max_idle": 5,
  "max_lifetime": 300,
  "max_idle_time": 180
}
```

`type` 可选值：`mysql`、`mongodb`。`max_lifetime` 和 `max_idle_time` 单位为秒。

**响应：** HTTP 201，返回数据源对象（密码不返回）。

---

### GET /api/datasources

获取数据源列表。需 admin 角色。

**响应：** 分页格式的数据源列表。

---

### GET /api/datasources/:id

获取数据源详情。需 admin 角色。

---

### PUT /api/datasources/:id

编辑数据源。需 admin 角色。

---

### DELETE /api/datasources/:id

禁用数据源。需 admin 角色。

---

### GET /api/datasources/:id/tables

获取数据源的库表列表（用于自动补全）。需认证。

**查询参数：** `database`（可选，指定库名）。

**响应：**

```json
{
  "code": 0,
  "data": {
    "databases": ["app_db", "analytics"],
    "tables": {
      "app_db": ["users", "orders", "products"]
    }
  }
}
```

---

### POST /api/datasources/:id/test

测试数据源连接。需 admin 角色。

**响应：**

```json
{
  "code": 0,
  "message": "连接成功"
}
```

---

## 权限管理（Admin）

### GET /api/roles

获取角色列表。需 admin 角色。

---

### GET /api/roles/:role

获取角色详情（含权限策略）。需 admin 角色。

**路径参数：** `role` — 角色名（admin / dba / developer）。

---

### POST /api/policies

添加权限策略。需 admin 角色。

**请求体：**

```json
{
  "role": "developer",
  "domain": "mysql-prod",
  "object": "orders",
  "action": "select"
}
```

`domain` 为数据源标识，`object` 为表名（`*` 代表全部），`action` 可选值：`select`、`update`、`delete`、`ddl`、`export`、`desensitize:bypass`、`*`。

---

### GET /api/policies

获取权限策略列表。需 admin 角色。

---

### DELETE /api/policies/:id

删除权限策略。需 admin 角色。

---

### POST /api/policies/sync

同步 Casbin 策略到内存。需 admin 角色。

---

## SQL 查询

### POST /api/query/execute

执行 SQL 查询。需认证。

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
    "rows": [
      {"id": 1, "name": "张**", "email": "z***g@example.com"}
    ],
    "total": 1,
    "execution_time_ms": 12,
    "affected_rows": 0,
    "desensitized": true,
    "desensitized_fields": ["name", "email"],
    "warnings": []
  }
}
```

**错误场景：**
- 高风险操作被拦截 → `403` + `"高风险操作需提交工单"`
- SQL 语法错误 → `400`
- 查询超时 → `400`
- 权限不足 → `403`

---

### POST /api/query/export

导出查询结果（CSV / JSON 流式下载）。需认证。

**请求体：**

```json
{
  "datasource_id": 1,
  "database": "app_db",
  "sql": "SELECT * FROM users LIMIT 100",
  "format": "csv"
}
```

`format` 可选值：`csv`、`json`。单次导出上限 10000 行。

**响应：** 文件下载（非 JSON），`Content-Type` 为 `text/csv` 或 `application/json`。

---

## AI 评审

### POST /api/query/review

AI 流式评审 SQL。需认证。返回 Server-Sent Events (SSE) 流。

**请求体：**

```json
{
  "datasource_id": 1,
  "database": "app_db",
  "sql": "UPDATE users SET status = 'inactive' WHERE last_login < '2025-01-01'"
}
```

**响应：** `Content-Type: text/event-stream`

SSE 事件类型：

| 事件 | 说明 |
|------|------|
| `content` | 流式文本片段（AI 思考过程） |
| `result` | 最终评审结果（JSON） |
| `error` | 错误信息 |
| `done` | 流结束标记 |

**`result` 事件数据结构：**

```json
{
  "risk_level": "high",
  "risk_score": 85,
  "decision": "ticket",
  "summary": "批量更新操作，影响范围较大",
  "suggestions": ["建议添加 LIMIT 限制", "建议先 SELECT 确认影响行数"],
  "impact_analysis": "预计影响约 5000 行",
  "rollback_sql": "UPDATE users SET status = 'active' WHERE ...",
  "warnings": ["无 WHERE 条件的 UPDATE/DELETE 为高风险操作"],
  "review_source": "ai",
  "reviewed_at": "2026-05-01T10:00:00Z",
  "expires_at": "2026-05-01T10:00:30Z",
  "model_used": "gpt-4"
}
```

`decision` 可选值：`execute`（低风险免审）、`confirm`（中风险需确认）、`ticket`（高风险走工单）、`blocked`（直接拦截）、`fallback`（AI 不可用，静态规则兜底）。

---

## 查询历史

### GET /api/query/history

获取当前用户的查询历史。需认证。

**查询参数：** `page`（默认 1）、`page_size`（默认 50）。

**响应：** 分页格式，每条记录包含 `id`、`datasource_id`、`database`、`sql_content`、`sql_summary`、`db_type`、`execution_time`、`result_rows`、`affected_rows`、`created_at`。

---

### DELETE /api/query/history/:id

删除单条查询历史。需认证（只能删除自己的）。

---

### DELETE /api/query/history

清空当前用户所有查询历史。需认证。

---

## 工单管理

### POST /api/tickets

提交变更工单。需认证。

**请求体：**

```json
{
  "datasource_id": 1,
  "database": "app_db",
  "sql": "ALTER TABLE users ADD COLUMN phone VARCHAR(20)",
  "db_type": "mysql",
  "change_reason": "业务需要记录用户手机号",
  "risk_level": "medium",
  "ai_review_result": ""
}
```

**响应：** HTTP 201，返回工单对象。

---

### GET /api/tickets

获取工单列表。需认证。

**查询参数：**

| 参数 | 说明 |
|------|------|
| `page` | 页码，默认 1 |
| `page_size` | 每页条数，默认 50 |
| `status` | 按状态筛选 |
| `datasource_id` | 按数据源筛选 |
| `submitter_id` | 按提交人筛选 |
| `risk_level` | 按风险等级筛选 |
| `keyword` | 关键词搜索 |
| `scope` | `mine`（我的工单）/ `pending`（待审批） |

**工单状态：** `SUBMITTED` → `AI_REVIEWED` → `PENDING_APPROVAL` → `APPROVED` → `EXECUTING` → `DONE` / `REJECTED` / `CANCELLED`

---

### GET /api/tickets/:id

获取工单详情。需认证。

---

### POST /api/tickets/:id/approve

审批通过。需 dba 或 admin 角色。

**请求体：**

```json
{
  "comment": "确认执行"
}
```

---

### POST /api/tickets/:id/reject

审批驳回。需 dba 或 admin 角色。

**请求体：**

```json
{
  "reason": "该表为核心表，需 DBA 手动执行"
}
```

---

### POST /api/tickets/:id/cancel

取消工单。提交人或 dba/admin 可操作（审批前）。

**请求体：**

```json
{
  "reason": "需求变更，不再需要"
}
```

---

### POST /api/tickets/:id/execute

手动执行工单 SQL。审批通过后，仅提交人或 dba/admin 可执行。

**无需请求体。**

---

## 脱敏规则（Admin）

### POST /api/mask-rules

创建脱敏规则。需 admin 角色。

**请求体：**

```json
{
  "datasource_id": 1,
  "database": "app_db",
  "table_name": "users",
  "field": "phone",
  "mask_type": "phone",
  "custom_regex": "",
  "custom_template": ""
}
```

`mask_type` 可选值：`phone`（手机号）、`id_card`（身份证）、`name`（姓名）、`email`（邮箱）、`bank_card`（银行卡）、`address`（地址）、`full_mask`（全掩码）、`custom`（自定义正则）。

当 `mask_type` 为 `custom` 时，需同时提供 `custom_regex` 和 `custom_template`。

---

### GET /api/mask-rules

获取脱敏规则列表。需 admin 角色。

**查询参数：** `page`、`page_size`、`datasource_id`、`database`、`table_name`。

---

### GET /api/mask-rules/:id

获取脱敏规则详情。需 admin 角色。

---

### PUT /api/mask-rules/:id

编辑脱敏规则。需 admin 角色。

---

### DELETE /api/mask-rules/:id

删除脱敏规则。需 admin 角色。

---

## 敏感表管理（Admin）

### POST /api/sensitive-tables

标记敏感表。需 admin 角色。

**请求体：**

```json
{
  "datasource_id": 1,
  "database": "app_db",
  "table_name": "users",
  "sensitivity_level": "high"
}
```

`sensitivity_level` 可选值：`low`、`medium`（默认）、`high`。

---

### GET /api/sensitive-tables

获取敏感表列表。需 admin 角色。

**查询参数：** `page`、`page_size`、`datasource_id`、`database`、`table_name`。

---

### DELETE /api/sensitive-tables/:id

取消敏感表标记。需 admin 角色。

---

## 审计日志（Admin）

### GET /api/audit-logs

获取审计日志。需 admin 角色。

**查询参数：**

| 参数 | 说明 |
|------|------|
| `page` | 页码，默认 1 |
| `page_size` | 每页条数，默认 50 |
| `user_id` | 按用户筛选 |
| `action` | 按操作类型筛选 |
| `datasource_id` | 按数据源筛选 |
| `start` | 开始时间 |
| `end` | 结束时间 |
| `keyword` | 关键词搜索 |

**响应条目字段：** `id`、`user_id`、`username`、`action`、`datasource_id`、`database`、`sql_content`、`sql_summary`、`result_rows`、`affected_rows`、`execution_time_ms`、`error_message`、`desensitized_fields`、`ip_address`、`created_at`。

审计日志应用层面不支持删除。

---

## 系统设置（Admin）

### GET /api/settings

获取系统配置（钉钉 + AI）。需 admin 角色。

**响应：**

```json
{
  "code": 0,
  "data": {
    "dingtalk": {
      "webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=...",
      "secret": "SEC***...",
      "enabled": true
    },
    "ai": {
      "provider": "openai",
      "model": "gpt-4",
      "api_key": "sk-1***...abcd",
      "base_url": "https://api.openai.com/v1",
      "timeout": "10s",
      "enabled": true
    }
  }
}
```

---

### PUT /api/settings/dingtalk

更新钉钉通知配置。需 admin 角色。

**请求体：**

```json
{
  "webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=xxx",
  "secret": "SECxxx"
}
```

---

### POST /api/settings/dingtalk/test

发送测试钉钉消息。需 admin 角色。

---

### PUT /api/settings/ai

更新 AI 评审配置。需 admin 角色。

**请求体：**

```json
{
  "provider": "openai",
  "model": "gpt-4",
  "api_key": "sk-xxx",
  "base_url": "https://api.openai.com/v1",
  "timeout": "10s"
}
```

`timeout` 为 Go duration 格式，如 `10s`、`30s`、`1m`。
