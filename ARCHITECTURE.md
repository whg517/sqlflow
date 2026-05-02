# SQL 审批管理平台 - 技术架构

> 创建日期：2026-04-21
> 最后更新：2026-05-02（评审修订 v4）
> 状态：设计阶段

## 技术选型

| 层级 | 技术 | 说明 |
|------|------|------|
| 后端 | Golang (Echo) | 高性能、并发友好、单二进制部署 |
| 认证 | JWT (golang-jwt) | 用户名密码登录，Bearer Token 认证 |
| 权限 | Casbin (RBAC) | 基于角色的访问控制，多数据源隔离 |
| 前端 | React + Vite + Bun | 现代前端工具链 |
| SQL 编辑器 | CodeMirror 6 | 轻量、SQL 支持好、性能优 |
| 平台数据存储 | SQLite (WAL 模式) | MVP 零运维、单文件、Docker 友好 |
| ORM | Ent (entgo.io) | 代码生成、Schema 迁移、类型安全查询 |
| 目标数据库 | MySQL + MongoDB | 仅生产环境 |
| MySQL SQL 解析 | pingcap/parser | 纯 Go 实现，支持 AST 解析 |
| MongoDB 解析 | go.mongodb.org/mongo-driver/bson | BSON 解析 + 自定义规则匹配 |
| AI 评审 | 外部 LLM API + SSE | 点击执行后流式评审，3-10 秒 |
| 通知 | 钉钉机器人 Webhook | 工单 + 告警通知 |
| 部署 | Docker | 单容器 MVP |

## 架构图

```
┌─────────────────────────────────────────────────┐
│                   Frontend                       │
│         (SQL编辑器 + 工单 + 管理后台)              │
└────────────────────┬────────────────────────────┘
                     │ HTTP (JSON API)
┌────────────────────▼────────────────────────────┐
│                 API Layer                        │
│          (路由 + 日志 + 错误处理中间件)             │
├─────────────────────────────────────────────────┤
│                                                  │
│  ┌──────────┐ ┌──────────┐ ┌───────────────┐    │
│  │ Query    │ │ Ticket   │ │ Audit         │    │
│  │ Service  │ │ Service  │ │ Service       │    │
│  └────┬─────┘ └────┬─────┘ └───────┬───────┘    │
│       │            │                │            │
│  ┌────▼─────┐ ┌────▼─────┐  ┌──────▼──────┐    │
│  │ Desensit │ │ Approve  │  │ Notify      │    │
│  │ Service  │ │ Service  │  │ Service     │    │
│  └────┬─────┘ └──────────┘  └──────┬──────┘    │
│       │                          │             │
│  ┌────▼──────────────────────────▼──────┐      │
│  │          AI Review Service           │      │
│  │    (风险分级 + 优化建议 + 影响分析)    │      │
│  └──────────────┬───────────────────────┘      │
│                                                  │
│  ┌──────────────────────────────────────┐       │
│  │       Auth & Permission (Casbin)      │       │
│  │    (JWT验证 + RBAC权限校验 + 脱敏控制) │       │
│  └──────────────────────────────────────┘       │
│                                                  │
│  ┌──────────────────────────────────────┐       │
│  │          Connection Pool Manager      │       │
│  │     (MySQL连接池 + MongoDB连接池)      │       │
│  └──────────────────────────────────────┘       │
└──────────────────────────────────────────────────┘
         │                    │              │
    ┌────▼────┐    ┌──────────▼───┐   ┌─────▼─────┐
    │  MySQL  │    │   MongoDB    │   │  SQLite   │
    │ (目标库) │    │  (目标库)    │   │(平台数据) │
    └─────────┘    └──────────────┘   └───────────┘
```

## 核心设计

### 1. 平台数据存储 — SQLite (WAL 模式)

存储内容：用户、角色、权限、工单、审计日志、脱敏规则、AI 评审记录、目标数据库实例配置

**并发策略：**
- 开启 WAL（Write-Ahead Logging）模式，读写不互斥
- 审计日志写入通过内存队列批量入库（每秒刷一次或满 100 条刷一次）
- 其余写入操作（工单、权限、Casbin 策略）并发量低，直接写即可
- Ent 使用 \`sql.DB\` 连接池，设置 \`SetMaxOpenConns(1)\` 限制 SQLite 写并发为 1，避免 \`database is locked\`。读操作不受限（WAL 允许多读）
- Casbin 策略变更（add/remove policy）走同一个写连接，与查询的读连接隔离

**后续迁移路径：** Ent Schema 定义，driver 切换为 MySQL/PostgreSQL 即可，业务代码无需改动。

### 2. 目标数据库连接管理

**per-instance 独立连接池：**
- 每个注册的 MySQL/MongoDB 实例各自维护独立连接池，不共享
- 每个实例配置：名称、类型、地址、端口、账号、密码（AES 加密存储）
- 连接池参数（可按实例配置，有全局默认值）：
  - `max_open`：最大连接数，默认 10
  - `max_idle`：最大空闲连接数，默认 5
  - `max_lifetime`：连接最大生命周期，默认 5min
  - `max_idle_time`：连接最大空闲时间，默认 3min

**单查询超时控制：**
- 每个 SQL 执行带 context timeout，默认 30s
- 超时后 context 取消，driver 自动中断查询并归还连接
- 防止单条慢查询占住连接

**健康检查：**
- 每 30s ping 各实例（用池内空闲连接）
- 连续 3 次失败 → 标记实例为不可用，前端提示并通知 DBA
- 恢复后自动标记为可用

**查询前权限校验：**
- 通过 Casbin RBAC 校验当前用户对目标数据源/库/表的操作权限
- JWT 中间件在 API 层统一验证 token 有效性，提取用户信息注入上下文

### 2.1 认证 — JWT

**登录流程：**
1. 用户提交用户名 + 密码
2. 服务端验证密码（bcrypt 比对）
3. 签发 JWT Access Token（包含 user_id、username、roles）
4. 前端存储 token，后续请求通过 `Authorization: Bearer <token>` 携带

**JWT 配置：**
- 签名算法：HS256
- 有效期：默认 24h（可配置）
- 不使用 refresh token（MVP 简化，过期重新登录）

**初始管理员：**
- 启动时检测 SQLite 中是否存在 admin 用户
- 不存在则使用环境变量/启动参数创建（密码 bcrypt 哈希后存储）
- 已存在则跳过，不覆盖

### 2.2 权限 — Casbin RBAC

**模型选择：** RBAC with domains（多数据源隔离）

**核心逻辑：**
```
请求到达 → JWT 中间件提取用户信息
  → Casbin Enforcer 校验: enforce(user, datasource, table, action)
  → 允许：继续执行
  → 拒绝：返回 403
```

**Casbin Adapter：** 使用 Ent ORM Adapter，策略和角色绑定存储在 SQLite

**中间件集成：**
- `auth.go`：JWT 验证中间件（白名单：/api/login、静态资源）
- `permission.go`：Casbin 权限校验中间件（按路由配置所需 action）

### 3. SQL 解析

**MySQL — pingcap/parser：**
- 纯 Go 实现，解析 SQL 为 AST
- 用于：提取操作类型（SELECT/DDL/DML）、提取涉及表名、检测无 WHERE 的 UPDATE/DELETE、提取 WHERE 条件
- 不依赖 C 库，交叉编译友好

**MongoDB — bson 驱动 + 自定义规则：**
- 解析 `find`/`update` 的 filter 条件
- 检测是否有条件过滤（空 filter 的 update 判定为高风险）
- 提取涉及集合名称

**静态拦截规则（不依赖 AI）：**
- `DROP DATABASE` / `DROP TABLE` → 直接拦截
- 无 WHERE 的 `DELETE` / `UPDATE` → 直接标记高风险
- MongoDB 空 filter 的 `deleteMany` → 直接拦截
- 超过配置行数限制的查询未加 LIMIT → 警告

### 4. AI 评审流程

```
用户提交SQL（点击执行）
    │
    ▼
SQL Parser 解析（本地，毫秒级）
    │
    ├─ 静态规则拦截（高风险直接拦截或强制工单）
    │   └─ 通过 → 继续 AI 评审
    │
    ▼
调用外部 LLM API（SSE 流式返回，10s 超时）
    │
    ├─ 成功：返回风险等级 + 建议 + 影响分析
    │   └─ 评审结果有效期 30s，期间直接执行，超时需重新评审
    │
    ├─ 超时但可连通：降级为"中风险"处理（执行+记录+通知DBA）
    │
    ├─ 网络不可用：使用静态规则兜底（无WHERE UPDATE直接拦截等）
    │
    ├─ 完全离线：提示用户稍后重试，不执行
    │
    ▼
决策引擎：根据操作类型 + 风险等级 → 免审 / 简化工单 / 标准工单
    │
    ├─ 低风险 → 评审完成后立即执行
    ├─ 中风险 → 评审完成后提示用户确认，用户点确认后执行
    └─ 高风险 → 评审完成后跳转工单提交
```

### 5. 数据脱敏

在结果返回前做脱敏，不修改原始数据：

```
查询结果集 → 匹配表/字段脱敏规则 → 替换敏感字段 → 返回前端
```

- 脱敏规则按 库.表.字段 三级匹配，字段级优先
- 脱敏行为由 Casbin 权限控制：拥有 `desensitize:bypass` 权限的用户查询时跳过脱敏
- 导出功能同样应用脱敏（拥有 bypass 权限时可选择导出原始数据，并记录审计日志）

### 6. 工单状态机

```
SUBMITTED → AI_REVIEWED → PENDING_APPROVAL → APPROVED → EXECUTING → DONE
                                            → REJECTED   → DONE
                                            → CANCELLED  → DONE
```

每个状态变更记录：时间戳、操作人、备注。

### 7. 查询历史

用户执行过的 SQL 查询记录，用于快速恢复历史查询。

**存储：** SQLite，按用户隔离，存储最近 200 条/用户（超出自动清理最旧记录）

**数据模型（Ent Schema）：**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int64 (PK) | 自增主键 |
| user_id | int64 (FK → User) | 所属用户 |
| datasource_id | int64 (FK → DataSource) | 数据源 |
| database | string | 库名 |
| sql_content | text | SQL 全文 |
| sql_summary | string(100) | SQL 前 100 字符摘要 |
| db_type | string | MySQL / MongoDB |
| execution_time | int64 | 执行耗时 (ms) |
| result_rows | int64 | 返回行数 |
| affected_rows | int64 | 影响行数 |
| created_at | time | 执行时间 |

**API：**

```
GET  /api/query/history              # 当前用户查询历史（分页，默认最近 50 条）
DELETE /api/query/history/:id         # 删除单条记录
DELETE /api/query/history             # 清空当前用户所有历史
```

**查询历史 vs 审计日志：**
- 查询历史：个人视角，可删除，用于快速恢复查询，存储 SQL 全文
- 审计日志：管理视角，不可删除，全量记录，用于合规审计

### 8. 错误处理策略

| 场景 | 处理方式 |
|------|---------|
| 目标数据库连接失败 | 返回明确错误提示 + 记录审计日志 + 钉钉告警通知 DBA |
| SQL 语法错误 | 返回语法错误详情（由 parser 或数据库返回），不记录审计 |
| SQL 执行超时 | 自动中断查询 + 返回超时提示 + 记录审计日志 |
| AI API 超时/失败 | 分层降级：超时→中风险，离线→静态规则兜底，不可达→提示重试 |
| 并发写入冲突（SQLite） | WAL 模式 + 内存队列缓冲，极端情况重试 3 次 |
| 导出数据量过大 | 限制单次导出行数上限（默认 10000），超出提示分批导出 |

**AI 评审结果存储：** 评审结果（风险等级、建议、影响分析）以 JSON 字段存储在工单表中（`ai_review_result`）。直接执行的查询，评审结果记录在审计日志的扩展字段中。

## 项目结构

```
sql-platform/
├── cmd/server/
│   └── main.go                  # 入口
├── internal/
│   ├── api/                     # HTTP 层
│   │   ├── handler/             # 请求处理
│   │   │   ├── query.go
│   │   │   ├── ticket.go
│   │   │   ├── audit.go
│   │   │   ├── datasource.go
│   │   │   ├── permission.go
│   │   │   ├── user.go
│   │   │   └── role.go          # 角色管理（admin 操作）
│   │   ├── middleware/          # 中间件
│   │   │   ├── auth.go        # JWT 认证中间件
│   │   │   ├── permission.go  # Casbin 权限校验中间件
│   │   │   ├── logger.go
│   │   │   ├── recovery.go
│   │   │   └── cors.go
│   │   ├── router.go            # 路由注册
│   │   └── response.go          # 统一响应格式
│   ├── service/                 # 业务逻辑
│   │   ├── query.go             # SQL 查询执行
│   │   ├── ticket.go            # 工单管理
│   │   ├── audit.go             # 审计日志（内存队列 + 批量写入）
│   │   ├── desensitize.go       # 数据脱敏
│   │   ├── ai_review.go         # AI 评审
│   │   ├── permission.go        # 权限管理（Casbin）
│   │   ├── auth.go              # 认证服务（JWT 签发/验证）
│   │   ├── notify.go            # 钉钉通知
│   │   ├── datasource.go        # 数据源管理
│   │   └── query_history.go      # 查询历史
│   ├── model/                   # 数据模型
│   ├── ent/                     # Ent ORM Schema & 生成的代码
│   │   ├── schema/             # Schema 定义（User, Role, Ticket, AuditLog 等）
│   │   └── client.go            # Ent Client
│   ├── connpool/                # 连接池管理
│   │   ├── pool.go              # 通用接口
│   │   ├── mysql.go             # MySQL 连接池
│   │   └── mongodb.go           # MongoDB 连接池
│   └── pkg/                     # 工具函数
│       ├── sqlparser/           # SQL 解析封装
│       ├── mask/                # 脱敏规则引擎
│       ├── crypto/              # 加密工具（AES）
│       └── casbin/              # Casbin 模型定义
│           ├── model.conf       # RBAC with domains 模型
│           └── policy.csv       # 初始策略（种子数据）
├── web/                         # React 前端
│   ├── src/
│   │   ├── api/                 # API 请求封装
│   │   ├── components/          # 通用组件
│   │   ├── pages/               # 页面
│   │   │   ├── Query/           # SQL 查询页
│   │   │   ├── Ticket/          # 工单管理页
│   │   │   ├── Audit/           # 审计日志页
│   │   │   ├── Permissions/     # 权限管理（角色/策略/用户）
│   │   │   ├── Settings/         # 设置（数据源/脱敏/AI 配置）
│   │   │   ├── Profile/          # 个人设置（修改密码）
│   │   │   └── Login/           # 简易登录页
│   │   ├── hooks/               # 自定义 Hooks
│   │   ├── store/               # 状态管理
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── package.json
│   └── vite.config.ts
├── config.yaml
├── Dockerfile
└── docker-compose.yaml
```

## API 概览

> 除 `/api/login` 外，所有 API 均需 JWT 认证。
> 权限校验由 Casbin 中间件根据路由自动执行。

```
# 认证（无需 JWT）
POST   /api/auth/login              # 用户名密码登录，返回 JWT

# 当前用户
GET    /api/auth/me                 # 获取当前用户信息
PUT    /api/auth/password           # 修改当前用户密码

# 用户管理（admin）
POST   /api/users                   # 创建用户
GET    /api/users                   # 用户列表
GET    /api/users/:id               # 用户详情
PUT    /api/users/:id               # 编辑用户（角色、状态）
DELETE /api/users/:id               # 删除用户（不能删除自己）
PUT    /api/users/:id/reset-password # 重置用户密码

# 数据源（admin / dba）
POST   /api/datasources             # 添加数据源
GET    /api/datasources             # 数据源列表
PUT    /api/datasources/:id         # 编辑数据源
DELETE /api/datasources/:id         # 禁用数据源
GET    /api/datasources/:id/tables  # 获取库表列表（用于补全）
POST   /api/datasources/:id/test    # 测试数据源连接

# 角色管理（admin）
GET    /api/roles                   # 角色列表
GET    /api/roles/:id               # 角色详情（含权限策略）

# 权限策略（admin / dba）
POST   /api/policies                # 添加权限策略
GET    /api/policies                # 权限策略列表
DELETE /api/policies/:id            # 删除权限策略
POST   /api/policies/sync           # 同步 Casbin 策略到内存

# 脱敏规则（admin / dba）
POST   /api/mask-rules              # 添加脱敏规则
GET    /api/mask-rules              # 规则列表
PUT    /api/mask-rules/:id          # 编辑规则
DELETE /api/mask-rules/:id          # 删除规则

# SQL 查询
POST   /api/query/execute           # 执行 SQL
                                  # - 静态规则直接拦截：返回 JSON 错误
                                  # - 需要评审：返回 SSE 流（Content-Type: text/event-stream）
                                  #   流式推送评审过程 → 最终推送执行结果或工单跳转指令
                                  # - 低风险免审：直接返回执行结果
POST   /api/query/export            # 导出查询结果

# 工单
POST   /api/tickets                 # 提交工单
GET    /api/tickets                 # 工单列表（分页 + 筛选）
                                  # ?page=1&page_size=50&status=&datasource_id=&submitter_id=&risk_level=&keyword=&scope=mine|pending
GET    /api/tickets/:id             # 工单详情
POST   /api/tickets/:id/approve     # 审批通过
POST   /api/tickets/:id/reject      # 审批驳回
POST   /api/tickets/:id/cancel      # 取消工单
POST   /api/tickets/:id/execute     # 手动执行工单 SQL（审批通过后，仅提交人或 dba/admin 可执行）

# 审计日志
GET    /api/audit-logs              # 审计日志（分页 + 筛选）
                                  # ?page=1&page_size=50&user_id=&action=&datasource_id=&start=&end=&keyword=
```

**Casbin 权限矩阵示例：**

| 用户/角色 | 数据源 domain | 表 obj | 操作 act |
|-----------|---------------|--------|----------|
| alice (developer) | mysql-prod | orders | select |
| bob (dba) | * | * | select, update, delete, ddl, export |
| bob (dba) | * | * | desensitize:bypass |
| admin | * | * | * |

## 配置管理

```yaml
# config.yaml
server:
  port: 8080

database:
  path: ./data/platform.db    # SQLite 文件路径
  wal_mode: true              # WAL 模式
  audit_batch_size: 100       # 审计日志批量写入大小
  audit_flush_interval: 1s    # 审计日志刷盘间隔
  query_history_max: 200   # 每用户最大查询历史条数

query:
  default_row_limit: 1000     # 默认查询行数限制
  max_row_limit: 50000        # 最大查询行数限制
  timeout: 30s                # 查询超时
  export_max_rows: 10000      # 单次导出行数上限

datasource:
  default_max_open: 10       # 默认最大连接数
  default_max_idle: 5        # 默认最大空闲连接数
  default_max_lifetime: 5m   # 默认连接最大生命周期
  default_max_idle_time: 3m  # 默认连接最大空闲时间
  health_check_interval: 30s # 健康检查间隔
  health_check_failures: 3   # 连续失败次数后标记不可用

ai:
  provider: openai            # LLM 提供商
  model: gpt-4                # 模型名称
  api_key: ""                 # API Key（环境变量覆盖）
  timeout: 10s                # 超时时间

auth:
  jwt_secret: ""              # JWT 签名密钥（环境变量覆盖，必填）
  jwt_expiry: 24h             # Token 有效期
  admin_username: ""          # 初始管理员用户名（环境变量覆盖）
  admin_password: ""          # 初始管理员密码（环境变量覆盖）
  password_min_length: 8       # 密码最小长度
  password_max_length: 128     # 密码最大长度

dingtalk:
  webhook_url: ""             # 钉钉机器人 Webhook
  secret: ""                  # 签名密钥
```

敏感配置（API Key、数据库密码）通过环境变量覆盖，不写进配置文件。

## 非功能性需求

### 性能

- AI 评审响应：3-5 秒内
- SQL 查询：平台层面不额外增加明显延迟
- 前端首屏加载：< 2 秒
- 审计日志写入：异步，不影响查询响应时间

### 安全（MVP）

- 初始管理员通过启动参数/环境变量配置（`ADMIN_USERNAME`、`ADMIN_PASSWORD`）
- 非管理员用户由管理员添加（用户名 + 密码 + 角色）
- 密码 bcrypt 哈希存储
- JWT 认证，HS256 签名，token 有效期 24h
- Casbin RBAC 权限控制（admin / dba / developer 三种角色）
- 数据库连接密码 AES 加密存储

### 可用性

- MVP 不要求 SLA，不可用时回退原有口头沟通
- 后续版本考虑高可用

### 后续规划

- 钉钉 OAuth 登录
- JWT Refresh Token
- 传输加密（HTTPS）
- 审计日志防篡改
- Casbin ABAC 扩展（更细粒度的属性化控制）
