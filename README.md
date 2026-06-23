# SQLFlow — SQL 审批管理平台

<p align="center">
  <strong>在线查询 · AI 评审 · 变更审批 · 数据脱敏 · 全程可追溯</strong>
</p>

---

SQLFlow 是一个面向开发团队和 DBA 的 SQL 审批管理平台。它将低风险查询自助化、高风险操作走审批流程，所有操作全程留痕可追溯，帮助运维从「执行者」转变为「审批者和规则制定者」。

## ✨ 核心功能

### 多数据源在线查询
- 支持 **MySQL / PostgreSQL / MongoDB / Elasticsearch** 四种数据源（基于 Driver 抽象层，新数据源实现 `Driver` 接口 + `Register()` 即可接入）
- SQL 编辑器（CodeMirror 6）+ 语法校验 + 实时执行 + EXPLAIN 执行计划
- 结果表格展示，支持分页、CSV/Excel 导出、字段级过滤、异步大导出任务
- 慢查询检测，超时自动中断（默认 30s）
- MongoDB 支持基本 aggregation pipeline（白名单 stage 校验）

### AI 前置评审
- 提交 SQL 后自动进行风险分级 + 优化建议 + 变更影响分析
- 支持多种 AI Provider：
  - **OpenAI**（GPT-4 等）
  - **智谱 GLM**（glm-4 等）
  - **Azure OpenAI**
  - **自定义**（任意 OpenAI 兼容 API）
- 流式返回（SSE），3-10 秒出结果
- 超时自动降级为静态规则评审

### 数据脱敏
- 字段级脱敏规则，查询结果**默认脱敏**
- 8 种内置规则：手机号、身份证、姓名、邮箱、银行卡、地址、全掩码、自定义正则
- 按表级别标记敏感数据
- Casbin 权限控制脱敏豁免（`desensitize:bypass`），豁免操作记录审计日志

### 变更工单审批流
- DDL/DML/MongoDB update 操作必须走工单
- 工单状态机：`SUBMITTED → AI_REVIEWED → PENDING_APPROVAL → APPROVED → EXECUTING → DONE`
- **审批引擎**：多级审批链 + 条件策略匹配（按数据源/SQL 类型/风险等级）+ 策略排序，AI 评审结果决定简化/标准流程
- **SLA 管理**：工单超时自动驳回，定时调度器巡检（10 分钟间隔），超时通知 DBA
- PostgreSQL 工单执行支持事务（多语句整批回滚），MySQL 逐条 auto-commit
- 审批通过后需手动执行，支持定时调度执行，DBA 可选择合适时机

### RBAC 权限管理
- 三种内置角色：**admin**（系统管理）、**dba**（审批 + 配置）、**developer**（查询 + 提交工单）
- 基于 Casbin RBAC with domains，支持数据源级别隔离
- 权限粒度：数据源 → 表 → 操作类型（select / update / delete / ddl / export）
- 敏感表默认拒绝访问，需管理员显式授权
- **权限申请流**：开发者可发起临时权限申请，DBA/Admin 审批后生效（带有效期）

### 操作审计
- 全量记录：查询、变更、导出、权限策略变更
- 记录字段：操作人、时间、SQL、影响行数、执行耗时、评审结果
- 审计日志支持 **FTS 全文检索**，按用户 / 时间 / 数据源 / 操作类型筛选
- 审计日志 API 层面不可删除

### 通知集成（多通道）
- **钉钉机器人 Webhook**：工单提交 / 审批结果通知，中高风险实时告警
- **飞书 Webhook**：支持多 Webhook 管理、加签、死信队列重试
- **通用 Webhook 订阅**：可订阅任意事件类型，自定义回调地址
- 用户级通知偏好设置（按事件开关）

### 认证与 SSO
- 本地账号 + JWT（Access Token + Refresh Token）
- **API Token**：用于 CI/CD 自动化场景，独立 scope 控制
- **OIDC 单点登录**：支持多 IdP（任意 OIDC 兼容服务），按 provider 动态注册
- **钉钉 OAuth2 登录**（可选）

### 其他能力
- **SQL 模板**：参数化 SQL 复用，支持 `{{var}}` 渲染
- **查询结果分享**：生成带密码保护的公开链接（`/s/:token`）
- **慢查询性能分析**：自动采集慢查询，统计报表
- **数据库备份**：SQLite 定时备份（gzip 压缩 + 保留份数）
- **Prometheus 指标** + **Web Vitals 采集**（前端性能）
- **Swagger API 文档**（`/swagger`）

---

## 🛠 技术栈

| 层级 | 技术 |
|------|------|
| **后端** | Go 1.25 + Echo v4 + Ent ORM |
| **前端** | React 19 + Vite 8 + TypeScript 6 + TailwindCSS 4 + Shadcn/ui |
| **SQL 编辑器** | CodeMirror 6 |
| **状态管理** | Zustand + TanStack Query / Table |
| **平台数据库** | SQLite（WAL 模式，零运维）|
| **目标数据库** | MySQL + PostgreSQL + MongoDB + Elasticsearch |
| **SQL 解析** | pingcap/parser（MySQL）+ pg_query_go（PostgreSQL）|
| **权限** | Casbin RBAC with domains |
| **认证** | JWT（HS256）+ Refresh Token + API Token + OIDC + 钉钉 OAuth2 |
| **AI 评审** | OpenAI / 智谱 GLM / Azure / 自定义（SSE 流式）|
| **可观测性** | Prometheus metrics + Web Vitals + OpenTelemetry |
| **部署** | Docker（单容器，前端 embed 进 Go 二进制）|

---

## 🚀 快速开始

### Docker 部署（推荐）

```bash
# 1. 克隆仓库
git clone https://github.com/whg517/sqlflow.git
cd sqlflow

# 2. 创建环境变量文件
cp .env.example .env
# 编辑 .env，设置 JWT_SECRET、ADMIN_PASSWORD 等敏感配置

# 3. 启动服务
docker compose up -d

# 4. 访问 http://localhost:8080
#    默认管理员账号: admin / admin123
```

### 环境变量（常用）

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `SQLFLOW_SERVER_PORT` | 服务监听端口 | `8080` |
| `SQLFLOW_JWT_SECRET` | JWT 签名密钥（**生产环境必须修改，≥16 字符**）| `change-me-in-production` |
| `SQLFLOW_ADMIN_USERNAME` | 初始管理员用户名 | `admin` |
| `SQLFLOW_ADMIN_PASSWORD` | 初始管理员密码（**生产环境必须修改，≥8 字符含字母+数字**）| `admin123` |
| `SQLFLOW_DB_PATH` | SQLite 数据库文件路径 | `/app/data/sqlflow.db` |
| `SQLFLOW_ENCRYPTION_KEY` | 数据源密码加密密钥（16/24/32 字节 hex）| — |
| `SQLFLOW_AI_PROVIDER` | AI 服务商：`openai` / `zhipu` / `azure` / `custom` | `openai` |
| `SQLFLOW_AI_MODEL` | AI 模型名称 | `gpt-4` |
| `SQLFLOW_AI_API_KEY` | AI API Key | — |
| `SQLFLOW_AI_BASE_URL` | AI API 地址（留空使用 Provider 默认地址）| — |
| `SQLFLOW_NOTIFY_WEBHOOK_URL` | 钉钉机器人 Webhook 地址 | — |
| `SQLFLOW_NOTIFY_SECRET` | 钉钉机器人签名密钥 | — |
| `SQLFLOW_FEISHU_WEBHOOK_URL` | 飞书 Webhook 地址 | — |
| `SQLFLOW_BACKUP_ENABLED` | 启用定时备份 | `true` |
| `SQLFLOW_METRICS_ENABLED` | 启用 Prometheus `/metrics` 端点 | `false` |

> 完整环境变量映射见 `config/config.go` 的 `envBindings`。配置文件（`config/config.yaml`）与环境变量二选一，环境变量优先级更高。

---

## 💻 开发指南

### 环境要求

- Go 1.25+
- Node.js 22+
- npm

### 本地运行

```bash
# 后端（默认监听 :8080）
cp config/config.example.yaml config/config.yaml
# 编辑 config/config.yaml 设置 SQLFLOW_JWT_SECRET 等
go run ./cmd/server/

# 前端（另一个终端，dev server 代理 API 到后端）
cd web
npm install
npm run dev
```

### 测试

```bash
# 后端单元测试
go test ./...

# 前端单元测试（Vitest）
cd web && npm run test

# E2E 测试（Playwright，需先启动测试环境）
make e2e-all    # setup + test + teardown
```

### 构建

```bash
# 一键构建（前端 + 后端）
make build

# 或分步
cd web && npm run build      # 前端构建
go build -o sqlflow ./cmd/server/   # 后端（前端需先构建，embed 进二进制）

# Docker 构建（自动包含前端构建）
docker build -t sqlflow .
```

### 质量检查

```bash
make lint      # golangci-lint + ESLint
make verify    # 完整 CI 检查（lint + build + test）
make docs      # 生成 Swagger API 文档（docs/swagger.{json,yaml}）
```

---

## ⚙️ 配置说明

配置文件路径：`config/config.yaml`（参考 `config/config.example.yaml`）。所有配置项均可通过 `SQLFLOW_*` 环境变量覆盖。

### AI Provider 配置

```yaml
ai:
  provider: "openai"    # openai | zhipu | azure | custom
  model: "gpt-4"        # openai→gpt-4, zhipu→glm-4
  api_key: ""           # 也可通过 SQLFLOW_AI_API_KEY 注入
  base_url: ""          # 留空使用 Provider 默认地址
  timeout: "10s"        # 超时后降级为静态规则评审
```

### OIDC 单点登录（可选）

```yaml
oidc:
  providers:
    - name: "google"
      issuer: "https://accounts.google.com"
      client_id: "xxx"
      client_secret: "xxx"      # 推荐用环境变量注入
      redirect_url: "http://localhost:8080/api/auth/oidc/google/callback"
      scopes: "openid profile email"
      enabled: true
```

### 飞书 Webhook（可选）

> 飞书 Webhook 支持多实例管理（DB 持久化），通过管理后台 `/api/settings/feishu/webhooks` CRUD 配置，此处仅配置默认实例。

```yaml
feishu:
  webhook_url: "https://open.feishu.cn/open-apis/bot/v2/hook/xxx"
```

### 通知配置

```yaml
notify:
  webhook_url: "https://oapi.dingtalk.com/robot/send?access_token=xxx"  # 钉钉
  secret: "SEC..."       # 钉钉加签密钥（可选）
```

### 备份与监控

```yaml
backup:
  enabled: true
  dir: "./data/backups"
  interval: "6h"
  keep: 10
  compress: true

metrics:
  enabled: false
  port: 9090
```

---

## 📁 项目结构

```
sqlflow/
├── cmd/server/main.go              # 启动入口 + 依赖装配
├── config/                         # 配置加载 + config.example.yaml
├── internal/
│   ├── api/
│   │   ├── handler/                # HTTP 请求处理器（按模块分文件）
│   │   ├── middleware/             # 中间件（Auth/CORS/Logger/Recovery/Admin）
│   │   └── router.go               # 路由注册（API 唯一事实源）
│   ├── connpool/                   # ⚠️ 旧连接池（MySQL/PG/Mongo/ES），迁移到 driver 层中
│   ├── driver/                     # ✅ 新数据源抽象层（Driver 接口 + 注册表 + PoolManager）
│   │   ├── driver.go               #   Driver 接口 + Capability 声明
│   │   ├── registry.go             #   驱动注册表（init 自动注册）
│   │   ├── pool.go                 #   PoolManager（按数据源 ID 缓存连接）
│   │   ├── mysql/ postgresql/ mongodb/ elasticsearch/  # 四种驱动实现
│   ├── db/                         # ent ORM 生成代码 + 手写迁移 (migrations/)
│   ├── coverage/                   # 代码覆盖率审计（可选，需独立 PG 库）
│   ├── model/                      # 请求/响应模型定义
│   ├── pkg/
│   │   ├── casbin/                 # Casbin RBAC 适配器
│   │   ├── crypto/                 # AES 加密工具
│   │   ├── mask/                   # 数据脱敏引擎
│   │   ├── metrics/                # Prometheus 指标
│   │   ├── performance/            # 性能采集
│   │   └── sqlparser/              # SQL/Mongo 解析器
│   ├── resp/                       # 统一响应封装
│   └── service/                    # 业务逻辑层（44 个服务，含审批引擎/SLA/导出/通知等）
├── web/src/                        # React 前端
│   ├── api/                        # API 请求封装
│   ├── components/                 # 通用组件 + Shadcn/ui + AIReview/ExportDialog
│   ├── pages/                      # 页面（52 个 .tsx：Dashboard/Query/Ticket/Settings 等）
│   ├── hooks/ store/               # 自定义 Hook + Zustand 状态管理
│   └── lib/                        # 工具函数
├── e2e/                            # Playwright E2E 测试（20+ spec）
├── docs/
│   ├── spec/                       # PRD-v2 / ARCHITECTURE-v2 / UI-DESIGN-v2 / DESIGN-TOKENS
│   ├── requirements/               # 需求规格文档（SF-FEAT*/SF-ENG*/SF-QA*）
│   ├── retrospectives/             # 复盘记录
│   ├── api.md                      # API 端点文档（详细版）
│   └── swagger.{json,yaml}         # Swagger 自动生成（make docs）
├── Dockerfile                      # 多阶段构建（前端构建 → Go 编译 → Alpine 运行）
├── docker-compose.yml              # 生产编排
├── docker-compose.{dev,test}.yml   # 开发/测试编排
└── Makefile                        # build/dev/test/lint/e2e/docs 等命令
```

---

## 📡 API 端点概览

> 完整端点（~140 条）见 [docs/api.md](docs/api.md)。以下按模块精简分组。所有 `/api/*` 端点（除标注「公开」外）需 Bearer JWT 或 API Token。

### 认证与用户（Auth）
| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/auth/login` | 登录 | 公开 |
| `POST` | `/api/auth/refresh` | 刷新 Token | 公开 |
| `GET` | `/api/auth/oidc/:provider` | OIDC 登录跳转 | 公开 |
| `GET` | `/api/auth/oidc/:provider/callback` | OIDC 回调 | 公开 |
| `GET` | `/api/auth/me` | 当前用户信息 | 登录 |
| `PUT` | `/api/auth/password` | 修改密码 | 登录 |
| CRUD | `/api/users` | 用户管理 | Admin |

### 查询与导出（Query / Export）
| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/query/execute` | 执行 SQL | 登录 |
| `POST` | `/api/query/explain` | 执行计划 | 登录 |
| `POST` | `/api/query/review` | AI 评审（SSE）| 登录 |
| `POST` | `/api/query/export` | 同步导出 | 登录 |
| `GET/DELETE` | `/api/query/history` | 查询历史（含频繁查询）| 登录 |
| `POST/DELETE` | `/api/query/share` | 结果分享（带密码）| 登录 |
| `GET` | `/s/:token` | 访问分享结果 | 公开 |
| CRUD | `/api/export/tasks` | 异步导出任务 | 登录 |
| `GET` | `/api/export/audit` | 审计日志导出 | Admin/DBA |

### 工单与审批（Ticket / Approval）
| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| CRUD | `/api/tickets` | 工单管理（含批量审批/调度/重交/修订）| 登录 |
| `POST` | `/api/tickets/:id/{approve,reject,cancel,execute,schedule}` | 工单流转 | 按角色 |
| `GET` | `/api/tickets/:id/approval-chain` | 审批链查询 | 登录 |
| `POST` | `/api/tickets/:id/engine-approve` | 引擎驱动审批 | 登录 |
| CRUD | `/api/admin/approval-policies` | 审批策略管理（含排序/启停）| Admin |
| `GET` | `/api/tickets/sla-status` | 工单 SLA 状态 | 登录 |
| CRUD | `/api/settings/sla` | SLA 配置 | Admin |

### 数据源与权限（Datasource / Permission）
| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `GET` | `/api/datasources/:id/tables` | 表列表 | 登录 |
| `GET` | `/api/datasources/:id/tables/:name/columns` | 表字段 | 登录 |
| `GET` | `/api/datasources/:id/es/indices` | ES 索引列表 | 登录 |
| CRUD | `/api/datasources` | 数据源管理（含连接测试）| Admin |
| CRUD | `/api/policies` | Casbin 权限策略 | Admin |
| CRUD | `/api/permission-requests` | 权限申请流 | 登录/Admin |

### 脱敏与敏感表（Mask）
| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| CRUD | `/api/mask-rules` | 脱敏规则管理 | Admin |
| CRUD | `/api/sensitive-tables` | 敏感表标记 | Admin |

### 审计与报表（Audit / Reports）
| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `GET` | `/api/audit-logs` | 审计日志（含 FTS 检索）| Admin/DBA |
| `GET` | `/api/reports/{usage,errors,performance,tickets}` | 运营报表 | Admin/DBA |
| `GET` | `/api/audit/user-analytics` | 用户行为分析 | Admin |

### 通知与设置（Notify / Settings）
| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `GET/PUT` | `/api/settings` | 系统设置（钉钉/AI/飞书）| Admin |
| CRUD | `/api/settings/feishu/webhooks` | 飞书 Webhook 管理（含死信）| Admin |
| CRUD | `/api/admin/webhooks/subscriptions` | 通用 Webhook 订阅 | Admin |
| `GET/PUT` | `/api/notifications/preferences` | 用户通知偏好 | 登录 |

### 其他（Token / Template / Git / Backup / Coverage）
| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| CRUD | `/api/tokens` | API Token 管理（自己/Admin）| 登录/Admin |
| CRUD | `/api/sql-templates` | SQL 模板（含渲染）| 登录 |
| CRUD | `/api/git-links` | Git 关联链接 | 登录 |
| CRUD | `/api/backups` | 数据库备份（触发/下载/删除）| Admin |
| CRUD | `/api/v1/coverage/*` | 代码覆盖率审计（**可选**，需独立 PG 库）| 登录/Admin |

### 系统端点（公开）
| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/health` `/api/health` | 健康检查 |
| `GET` | `/healthz` | Liveness 探针 |
| `GET` | `/readyz` | Readiness 探针（检查所有依赖）|
| `GET` | `/metrics` | Prometheus 指标（需启用 `metrics.enabled`）|
| `GET` | `/swagger/*` | Swagger UI |

> 完整 API 文档见 [docs/api.md](docs/api.md)，Swagger UI 运行时访问 `/swagger`。

---

## 🧪 测试与质量

- **后端单元测试**：service 层几乎 1:1 配套测试（45 个 `_test.go`，含审批引擎/SLA 自动驳回/事务回滚等关键路径覆盖）
- **前端单元测试**：Vitest + Testing Library（`web/src/test/`）
- **E2E 测试**：Playwright（`e2e/tests/`，20+ spec，覆盖工单流/查询/导出/权限/数据源管理）
- **CI**：GitHub Actions（lint / test / e2e / release 四条流水线）
- **pre-commit**：githooks/pre-commit（自动检查）

---

## 📄 License

私有项目，未授权禁止使用。
