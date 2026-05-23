# SQLFlow - 技术架构文档

> 项目代号 SQLFlow，代码仓库 `sql-platform`
> 创建日期：2026-04-21
> 最后更新：2026-05-23（v1.0 正式版：AI Provider 多接入、去 MVP 限制）
> 状态：已确认
> 产品需求见 [PRD.md](./PRD.md)

## 变更记录

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v1.0 | 2026-05-23 | 正式版：AI Provider 多接入（OpenAI/智谱/自定义）、去 MVP 限制、安全加固 |
| v0.2 | 2026-05-17 | 评审反馈修复：补充 SQLite 并发说明、AI 评审去有效期、工单执行权限校验、密钥管理、健康检查、CI/CD、前端 embed、性能预期等 |
| v0.1 | 2026-05-08 | 004-frontend-completion 合并 |

---

## 技术选型

| 层级 | 技术 | 说明 |
|------|------|------|
| 后端 | Golang (Echo) | 高性能、并发友好、单二进制部署 |
| 认证 | JWT (golang-jwt, HS256) | 用户名密码登录，Bearer Token 认证（需求来源：PRD §6） |
| 权限 | Casbin (RBAC with domains) | 基于角色的访问控制，多数据源隔离（需求来源：PRD §7） |
| Casbin Adapter | Ent ORM Adapter | 策略和角色绑定存储在 SQLite（需求来源：PRD §7） |
| 前端 | React + Vite + Node.js 22+ | 现代前端工具链 |
| SQL 编辑器 | CodeMirror 6 | 轻量、SQL 支持好、性能优 |
| 平台数据存储 | SQLite (WAL 模式) | MVP 零运维、单文件、Docker 友好 |
| ORM | Ent (entgo.io) | 代码生成、Schema 迁移、类型安全查询 |
| 目标数据库 | MySQL + MongoDB | 仅生产环境 |
| MySQL SQL 解析 | pingcap/parser | 纯 Go 实现，支持 AST 解析 |
| MongoDB 解析 | go.mongodb.org/mongo-driver/bson | BSON 解析 + 自定义规则匹配 |
| AI 评审 | 外部 LLM API + SSE（多 Provider） | 支持 OpenAI / 智谱 GLM / 自定义 OpenAI 兼容 API，流式评审，3-10 秒 |
| 通知 | 钉钉机器人 Webhook | 工单 + 告警通知（需求来源：PRD §9） |
| 部署 | Docker | 单容器 MVP，前端 embed 进 Go 二进制 |

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

> 需求来源：PRD §1 数据源管理、§5 变更工单、§8 操作审计

存储内容：用户、角色、权限、工单、审计日志、脱敏规则、AI 评审记录、目标数据库实例配置

**并发策略：**
- 开启 WAL（Write-Ahead Logging）模式，读写不互斥
- 审计日志写入通过内存队列批量入库（每秒刷一次或满 100 条刷一次）
- 其余写入操作（工单、权限、Casbin 策略）并发量低，直接写即可
- Ent 使用 `sql.DB` 连接池，设置 `SetMaxOpenConns(1)` 限制 SQLite 写并发为 1，避免 `database is locked`。读操作不受限（WAL 允许多读）
- Casbin 策略变更（add/remove policy）走同一个写连接，与查询的读连接隔离

**SQLite 并发能力说明：**

| 指标 | 预期值 | 说明 |
|------|--------|------|
| 写入 QPS | <10 | 平台管理操作（工单提交、审批、权限变更等）频率极低 |
| 读取 QPS | <100 | Dashboard 统计、查询历史、工单列表等 |
| 写入瓶颈场景 | DBA 同时审批多个工单 | 由于 SetMaxOpenConns(1)，多个并发写请求排队等待 |
| 写入等待上限 | <1s | 单次写操作耗时 <10ms，极端排队 10 个请求也 <100ms |
| 用户感知 | 无感知 | 审批、提交工单等操作间隔远大于排队时间 |

**Ent ORM 在 SQLite WAL 模式下的读写行为：**
- 写连接（SetMaxOpenConns=1 限制）：单写串行化，通过 SQLite 内部锁保证原子性
- 读连接：WAL 模式下读写不互斥，多个读连接可并发执行
- 读写一致性：读操作会读取最新的 WAL 快照，保证不会读到写操作的中间状态
- Ent 的 `sql.DB` 自动管理连接获取/归还，业务层无需感知底层锁

**后续迁移路径：** Ent Schema 定义，driver 切换为 MySQL/PostgreSQL 即可，业务代码无需改动。

### 2. 目标数据库连接管理

> 需求来源：PRD §1 数据源管理、§2 在线 SQL 查询

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
- **MySQL Ping 超时：5s**（context timeout）
- **MongoDB Ping 超时：5s**（context timeout）
- 连续 3 次失败 → 标记实例为不可用，前端提示并通知 DBA
- 恢复后自动标记为可用

**查询前权限校验：**
- 通过 Casbin RBAC 校验当前用户对目标数据源/库/表的操作权限
- JWT 中间件在 API 层统一验证 token 有效性，提取用户信息注入上下文

### 2.1 认证 — JWT

> 需求来源：PRD §6 用户管理

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

> 需求来源：PRD §7 权限管理

**模型选择：** RBAC with domains（多数据源隔离）

**Casbin 模型配置（model.conf）：**

```ini
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) && r.dom == p.dom && (r.obj == p.obj || p.obj == "*") && (r.act == p.act || p.act == "*")
```

**模型说明：**
- `sub`：用户或角色
- `dom`：数据源/库名（domain，多数据源隔离）
- `obj`：资源对象（表名，`*` 代表全部）
- `act`：操作类型（`select`、`update`、`delete`、`ddl`、`export`、`desensitize:bypass`）

**Casbin Adapter：** 使用 Ent ORM Adapter，策略和角色绑定存储在 SQLite。

**核心逻辑：**
```
请求到达 → JWT 中间件提取用户信息
  → Casbin Enforcer 校验: enforce(user, datasource, table, action)
  → 允许：继续执行
  → 拒绝：返回 403
```

**中间件集成：**
- `auth.go`：JWT 验证中间件（白名单：/api/login、/healthz、静态资源）
- `permission.go`：Casbin 权限校验中间件（按路由配置所需 action）

**策略变更审计：**
- 所有 Casbin 策略变更（add/remove policy）操作均记录到审计日志
- 审计内容包括：操作人、操作时间、变更类型（添加/删除）、策略内容

### 3. SQL 解析

> 需求来源：PRD §2 在线 SQL 查询、§3 AI 前置评审

**MySQL — pingcap/parser：**
- 纯 Go 实现，解析 SQL 为 AST
- 用于：提取操作类型（SELECT/DDL/DML）、提取涉及表名、检测无 WHERE 的 UPDATE/DELETE、提取 WHERE 条件
- 不依赖 C 库，交叉编译友好

**MongoDB — go.mongodb.org/mongo-driver/bson + 自定义规则：**
- 解析 `find`/`update` 的 filter 条件
- 检测是否有条件过滤（空 filter 的 update 判定为高风险）
- 提取涉及集合名称

**静态拦截规则（不依赖 AI）：**
- `DROP DATABASE` / `DROP TABLE` → 直接拦截
- 无 WHERE 的 `DELETE` / `UPDATE` → 直接标记高风险
- MongoDB 空 filter 的 `deleteMany` → 直接拦截
- 超过配置行数限制的查询未加 LIMIT → 警告

### 4. AI 评审流程

> 需求来源：PRD §3 AI 前置评审

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
    │   └─ 评审结果作为参考记录，不设过期时间
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

**评审结果有效期设计：**

- **AI 评审结果不设过期时间**，作为参考记录永久保存在工单表（`ai_review_result` JSON 字段）中
- **每次执行前都重新走静态规则检查**（毫秒级，无性能影响）：
  1. 校验工单状态是否为 APPROVED
  2. 校验当前用户身份（提交人或 dba/admin）
  3. 重新解析 SQL 静态规则（拦截 DROP、无 WHERE 等）
  4. 校验用户对目标数据源/表的执行权限（Casbin）
- **脱敏检查**和**权限检查**每次执行都走，与评审结果无关
- 对于直接执行的查询（非工单），AI 评审结果记录在审计日志的扩展字段中

**评审结果存储：** 评审结果（风险等级、建议、影响分析）以 JSON 字段存储在工单表中（`ai_review_result`）。直接执行的查询，评审结果记录在审计日志的扩展字段中。

### 5. 数据脱敏

> 需求来源：PRD §4 数据脱敏

在结果返回前做脱敏，不修改原始数据：

```
查询结果集 → 匹配表/字段脱敏规则 → 替换敏感字段 → 返回前端
```

- 脱敏规则按 库.表.字段 三级匹配，字段级优先
- 脱敏行为由 Casbin 权限控制：拥有 `desensitize:bypass` 权限的用户查询时跳过脱敏
- 导出功能同样应用脱敏（拥有 bypass 权限时可选择导出原始数据，并记录审计日志）

### 6. 工单状态机

> 需求来源：PRD §5 变更工单

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

**审计日志搜索能力：**
- **MVP 阶段**：基于 SQLite 的分页 + 筛选查询（按用户/时间/数据源/操作类型），支持关键词模糊匹配
- **P1 规划**：引入 SQLite FTS5 全文搜索引擎，支持 SQL 全文检索和更复杂的组合查询

### 8. 错误处理策略

| 场景 | 处理方式 |
|------|---------|
| 目标数据库连接失败 | 返回明确错误提示 + 记录审计日志 + 钉钉告警通知 DBA |
| SQL 语法错误 | 返回语法错误详情（由 parser 或数据库返回），不记录审计 |
| SQL 执行超时 | 自动中断查询 + 返回超时提示 + 记录审计日志 |
| AI API 超时/失败 | 分层降级：超时→中风险，离线→静态规则兜底，不可达→提示重试 |
| 并发写入冲突（SQLite） | WAL 模式 + 内存队列缓冲，极端情况重试 3 次 |
| 导出数据量过大 | 限制单次导出行数上限（默认 10000），超出提示分批导出 |

## 项目结构

```
sql-platform/
├── docs/
│   ├── spec/                    # 系统规格（真相来源）
│   │   ├── PRD.md              # 产品规格
│   │   ├── ARCHITECTURE.md     # 技术架构
│   │   └── UI-DESIGN.md        # UI 规格设计
│   └── proposals/               # 开发提案
│       └── 001-mvp-initial/    # MVP 初始开发
│           ├── proposal.md     # 提案描述
│           └── plan.md         # 实现计划
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
│   │   │   ├── role.go          # 角色管理（admin 操作）
│   │   │   └── health.go        # 健康检查
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
│   │   ├── query_history.go      # 查询历史
│   │   └── health.go            # 健康检查服务
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
├── docker-compose.dev.yaml       # 开发用 Docker Compose（MySQL + MongoDB 测试实例）
├── config.yaml
├── Dockerfile
└── docker-compose.yaml
```

## API 概览

> 除 `/api/login`、`/healthz` 外，所有 API 均需 JWT 认证。
> 权限校验由 Casbin 中间件根据路由自动执行。
> 需求来源：PRD §5 变更工单、§6 用户管理、§7 权限管理

```
# 健康检查（无需 JWT）
GET    /healthz                   # 健康检查（SQLite 连通性 + 各数据源连接池状态）

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

# Dashboard
GET    /api/dashboard/stats        # 聚合统计（待审批工单、近7天查询、活跃数据源、用户数）

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
POST   /api/tickets/:id/execute     # 手动执行工单 SQL
                                  # 权限校验：
                                  #   1. 工单状态必须为 APPROVED
                                  #   2. 当前用户 = 工单提交人 OR 当前用户角色为 dba/admin
                                  #   3. 当前用户必须有目标数据源/表的执行权限（Casbin 校验）
                                  #   上述任一条件不满足则返回 403
                                  # 执行前额外检查：
                                  #   - 重新解析 SQL 静态规则（拦截 DROP 等）
                                  #   - 脱敏检查（按需应用）
                                  #   - 权限检查（Casbin enforce）

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

## 健康检查

`GET /healthz` — 无需 JWT 认证

**响应格式：**

```json
{
  "status": "ok",
  "checks": {
    "sqlite": { "status": "ok", "latency_ms": 2 },
    "datasources": [
      { "name": "mysql-prod", "type": "mysql", "status": "ok", "pool_active": 1, "pool_idle": 4 },
      { "name": "mongo-prod", "type": "mongodb", "status": "ok", "pool_active": 0, "pool_idle": 3 }
    ]
  }
}
```

**检查内容：**
1. **SQLite 连通性**：执行 `SELECT 1`，返回延迟
2. **各数据源连接池状态**：遍历所有已注册数据源，报告连接池活跃/空闲连接数
3. 不可用的数据源标记为 `"status": "unavailable"`
4. 任一检查失败则整体 `"status": "degraded"`

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
  query_history_max: 200      # 每用户最大查询历史条数

security:
  # 数据源密码加密密钥（16 字节 hex = 32 字符）
  # 未配置时首次启动自动生成并打印到日志
  # ⚠️ 密钥丢失需重新录入所有数据源密码
  encryption_key: ""

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
  ping_timeout: 5s           # Ping 超时（MySQL / MongoDB 统一）

ai:
  # AI 评审 Provider 配置
  # provider: openai | zhipu | custom
  #   openai  → OpenAI 官方 API 或兼容接口（默认 https://api.openai.com/v1）
  #   zhipu   → 智谱 GLM 系列（默认 https://open.bigmodel.cn/api/paas/v4）
  #   custom  → 任意 OpenAI 兼容 API（需配 base_url）
  provider: openai
  model: gpt-4
  api_key: ""                 # API Key（环境变量 AI_API_KEY 覆盖）
  base_url: ""                # 自定义 endpoint（环境变量 AI_BASE_URL 覆盖）
  timeout: 10s                # 评审超时

  # 智谱 GLM 示例：
  # provider: zhipu
  # model: glm-4
  # api_key: "your-zhipu-api-key"

  # 自定义 OpenAI 兼容 API 示例：
  # provider: custom
  # model: your-model-name
  # api_key: "your-key"
  # base_url: "https://your-api-endpoint/v1"

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

### 加密密钥管理

> 需求来源：PRD §1 数据源管理（密码加密存储）

**数据源密码使用 AES 加密存储**，密钥管理规则：

1. **密钥格式**：16 字节，以 hex 编码存储（32 个字符）
2. **自动生成**：首次启动时，若未配置 `ENCRYPTION_KEY` 环境变量或 `security.encryption_key` 配置项，系统自动生成随机密钥并**打印到启动日志**（WARNING 级别）
3. **密钥持久化**：建议通过环境变量 `ENCRYPTION_KEY` 持久化配置，避免每次重启生成新密钥导致已加密密码不可解密
4. **密钥丢失后果**：⚠️ 密钥丢失后所有已加密的数据源密码无法解密，**需重新录入所有数据源密码**
5. **长期建议**：生产环境建议使用密钥管理服务（如 HashiCorp Vault）替代静态密钥配置

## 部署方案

### 前端 Embed 到 Go 二进制

MVP 采用**单二进制部署**，前端构建产物 embed 到 Go 二进制中：

```go
//go:embed web/dist/*
var frontendFS embed.FS
```

**实现方案：**
1. `make build` 时先 `npm run build` 生成 `web/dist/`
2. Go 使用 `embed.FS` 将 `web/dist/` 嵌入二进制
3. Echo 静态文件中间件从 `embed.FS` 提供前端资源
4. SPA 路由 fallback：所有非 API、非静态资源路径返回 `index.html`
5. 开发模式下（`make dev`）前端走 Vite dev server 代理，不走 embed

### Docker 部署

单容器部署，包含 Go 后端 + embed 的前端。

### 开发 Docker Compose

新增 `docker-compose.dev.yaml`，包含 MySQL + MongoDB 测试实例：

```yaml
# docker-compose.dev.yaml（仅用于开发）
services:
  mysql-test:
    image: mysql:8.0
    ports: ["3306:3306"]
    environment:
      MYSQL_ROOT_PASSWORD: test123
    volumes:
      - mysql_data:/var/lib/mysql

  mongo-test:
    image: mongo:7
    ports: ["27017:27017"]
    volumes:
      - mongo_data:/data/db

volumes:
  mysql_data:
  mongo_data:
```

## CI/CD

> 建议使用 GitHub Actions 自动化，基于 Makefile 统一命令。

### Makefile 核心命令

```makefile
# 后端
make lint          # golangci-lint 检查
make test          # go test ./...
make build         # 构建二进制（包含前端 embed）

# 前端
make frontend-lint # eslint 检查
make frontend-test # 前端单元测试
make frontend-build # npm run build

# 开发
make dev           # 后端热重载 + 前端 Vite dev server
```

### GitHub Actions 建议

```yaml
# .github/workflows/ci.yaml
on: [push, pull_request]
jobs:
  lint-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - uses: actions/setup-node@v4
        with: { node-version: '22' }
      - run: make lint
      - run: make test
      - run: make frontend-lint
      - run: make frontend-test
      - run: make build
```

## 非功能性需求

> 需求来源：PRD 非功能性需求

### 性能预期

| 指标 | MVP 目标 | 说明 |
|------|----------|------|
| API 响应时间（非 AI） | < 200ms (P95) | 查询历史、工单列表、数据源管理等 |
| API 响应时间（含 AI） | < 15s (P95) | 含 SSE 流式评审 + 执行 |
| 前端首屏加载 | < 2s | embed 模式，单次加载 |
| 并发查询能力 | ≥ 50 并发用户 | 受限于目标数据库连接池配置 |
| 内存占用 | < 256MB（后端） | 不含目标数据库连接 |
| SQLite 写入 QPS | < 10 | 平台管理操作频率极低 |
| 审计日志写入延迟 | < 1s | 异步批量写入，不阻塞请求 |

### 安全

- 用户认证：用户名 + 密码（bcrypt 加密存储），后续支持钉钉 OAuth
- 权限控制：RBAC 多数据源隔离，敏感表显式授权
- 数据库连接密码 AES-256 加密存储，密钥通过环境变量注入
- 审计日志不可删除（API 层限制）
- JWT Token 有效期可配置，支持密钥轮换
- AI Provider API Key 不落盘（环境变量注入）
- 传输加密：生产环境必须 HTTPS

### 可用性

- 单实例部署，Docker 内网运行
- 数据定期备份（SQLite 文件备份）
- 不可用时回退原有口头沟通流程

### 后续规划（v1.1+）

- 钉钉 OAuth 登录
- JWT Refresh Token
- 审计日志防篡改（哈希链）
- Casbin ABAC 扩展（更细粒度的属性化控制）
- SQLite FTS5 全文搜索
- 密钥管理服务（HashiCorp Vault）集成
- PostgreSQL 存储后端（替代 SQLite）
- 多实例高可用部署
