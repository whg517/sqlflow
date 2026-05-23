# SQLFlow — SQL 审批管理平台

<p align="center">
  <strong>在线查询 · AI 评审 · 变更审批 · 数据脱敏 · 全程可追溯</strong>
</p>

---

SQLFlow 是一个面向开发团队和 DBA 的 SQL 审批管理平台。它将低风险查询自助化、高风险操作走审批流程，所有操作全程留痕可追溯，帮助运维从「执行者」转变为「审批者和规则制定者」。

## ✨ 核心功能

### 在线 SQL 查询
- 支持 **MySQL** 和 **MongoDB** 双数据源
- SQL 编辑器（CodeMirror 6）+ 语法校验 + 实时执行
- 结果表格展示，支持分页、导出（CSV）
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
- AI 评审结果决定简化/标准工单流程
- 审批通过后需手动执行，DBA 可选择合适时机

### RBAC 权限管理
- 三种内置角色：**admin**（系统管理）、**dba**（审批 + 配置）、**developer**（查询 + 提交工单）
- 基于 Casbin RBAC with domains，支持数据源级别隔离
- 权限粒度：数据源 → 表 → 操作类型（select / update / delete / ddl / export）
- 敏感表默认拒绝访问，需管理员显式授权

### 操作审计
- 全量记录：查询、变更、导出、权限策略变更
- 记录字段：操作人、时间、SQL、影响行数、执行耗时、评审结果
- 支持按用户 / 时间 / 数据源 / 操作类型筛选
- 审计日志 API 层面不可删除

### 钉钉通知集成
- 工单提交 / 审批结果通过钉钉机器人 Webhook 通知
- 中高风险操作实时告警到 DBA 群

---

## 🛠 技术栈

| 层级 | 技术 |
|------|------|
| **后端** | Go 1.25 + Echo v4 |
| **前端** | React 19 + Vite 8 + TypeScript 6 + TailwindCSS 4 + Shadcn/ui |
| **SQL 编辑器** | CodeMirror 6 |
| **状态管理** | Zustand + TanStack Query |
| **平台数据库** | SQLite（WAL 模式，零运维） |
| **目标数据库** | MySQL + MongoDB |
| **SQL 解析** | pingcap/parser（AST） |
| **权限** | Casbin RBAC with domains |
| **认证** | JWT（HS256） |
| **AI 评审** | OpenAI / 智谱 GLM / Azure / 自定义（SSE 流式） |
| **部署** | Docker（单容器，前端 embed 进 Go 二进制） |

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

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `SERVER_PORT` | 服务监听端口 | `8080` |
| `JWT_SECRET` | JWT 签名密钥（**生产环境必须修改**） | `change-me-in-production` |
| `ADMIN_USERNAME` | 初始管理员用户名 | `admin` |
| `ADMIN_PASSWORD` | 初始管理员密码（**生产环境必须修改**） | `admin123` |
| `DB_PATH` | SQLite 数据库文件路径 | `/app/data/sqlflow.db` |
| `ENCRYPTION_KEY` | 数据源密码加密密钥（16/24/32 字节 hex） | — |
| `AI_PROVIDER` | AI 服务商：`openai` / `zhipu` / `azure` / `custom` | `openai` |
| `AI_MODEL` | AI 模型名称 | `gpt-4` |
| `AI_API_KEY` | AI API Key | — |
| `AI_BASE_URL` | AI API 地址（留空使用 Provider 默认地址） | — |
| `DINGTALK_WEBHOOK_URL` | 钉钉机器人 Webhook 地址 | — |
| `DINGTALK_SECRET` | 钉钉机器人签名密钥 | — |

> 💡 `AI_API_KEY` 和 `AI_BASE_URL` 也可在 config.yaml 中配置，环境变量优先级更高。

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
# 编辑 config/config.yaml 设置 JWT_SECRET 等
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

# 前端 E2E 测试
cd web
npx playwright test
```

### 构建

```bash
# 前端构建
cd web && npm run build

# 后端构建（前端需先构建，embed 到 Go 二进制）
go build -o sqlflow ./cmd/server/

# Docker 构建（自动包含前端构建）
docker build -t sqlflow .
```

---

## ⚙️ 配置说明

配置文件路径：`config/config.yaml`（参考 `config/config.example.yaml`）。

### AI Provider 配置

```yaml
ai:
  # Provider 选择
  provider: "openai"    # openai | zhipu | azure | custom

  # 模型名称
  model: "gpt-4"        # openai→gpt-4, zhipu→glm-4

  # API Key（也可通过环境变量 AI_API_KEY 注入）
  api_key: ""

  # API 地址（留空使用 Provider 默认地址）
  # openai: https://api.openai.com/v1
  # zhipu:  https://open.bigmodel.cn/api/paas/v4
  # azure:  需要填写
  # custom: 需要填写
  base_url: ""

  # 评审超时（超时后降级为静态规则）
  timeout: "10s"
```

### 钉钉 Webhook 配置

```yaml
dingtalk:
  webhook_url: "https://oapi.dingtalk.com/robot/send?access_token=xxx"
  secret: "SEC..."       # 加签密钥（可选）
```

### 其他配置项

```yaml
server:
  port: 8080             # 服务端口

jwt:
  secret: "..."          # JWT 签名密钥（至少 16 字符）
  expiry: "24h"          # Token 过期时间

admin:
  username: "admin"      # 初始管理员用户名（仅首次启动创建）
  password: "admin123"   # 初始管理员密码

db:
  path: "./data/sqlflow.db"  # SQLite 数据库路径

encryption_key: "..."    # AES 加密密钥（用于加密数据源密码）
query_history_max: 200   # 每用户最大查询历史数
```

---

## 📁 项目结构

```
sql-platform/
├── cmd/server/main.go           # 启动入口
├── config/                      # 配置加载 + config.yaml
├── internal/
│   ├── api/
│   │   ├── handler/             # HTTP 请求处理器
│   │   ├── middleware/          # 中间件（JWT/CORS/Logger/Recovery/Admin）
│   │   └── router.go            # 路由注册
│   ├── connpool/                # MySQL/MongoDB 连接池管理
│   ├── db/db.go                 # 数据库初始化 + 迁移
│   ├── model/model.go           # 请求/响应模型定义
│   ├── pkg/
│   │   ├── casbin/              # Casbin RBAC 适配器
│   │   ├── crypto/              # AES 加密工具
│   │   ├── mask/                # 数据脱敏引擎
│   │   └── sqlparser/           # SQL 解析器
│   ├── resp/response.go         # 统一响应封装
│   └── service/                 # 业务逻辑层
│       ├── ai_review.go         #   AI 评审
│       ├── audit.go             #   操作审计
│       ├── auth.go              #   认证（登录/JWT/用户管理）
│       ├── dashboard.go         #   仪表盘统计
│       ├── datasource.go        #   数据源管理
│       ├── mask_rule.go         #   脱敏规则管理
│       ├── notify.go            #   钉钉通知
│       ├── permission.go        #   RBAC 权限
│       ├── query.go             #   查询执行
│       ├── query_export.go      #   查询导出
│       ├── query_history.go     #   查询历史
│       └── ticket.go            #   变更工单
├── web/src/                     # React 前端
│   ├── api/                     #   API 请求封装
│   ├── components/              #   通用组件 + Shadcn/ui
│   ├── pages/                   #   页面组件
│   └── store/                   #   Zustand 状态管理
├── docs/
│   ├── spec/                    #   PRD / 架构 / UI 设计文档
│   ├── api.md                   #   API 端点文档
│   └── deployment.md            #   部署文档
├── Dockerfile                   # 多阶段构建（前端构建 → Go 编译 → Alpine 运行）
├── docker-compose.yaml          # Docker Compose 编排
└── entrypoint.sh                # 容器入口脚本
```

---

## 📡 API 端点概览

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/auth/login` | 用户登录 | 公开 |
| `GET` | `/api/auth/me` | 获取当前用户信息 | 登录 |
| `PUT` | `/api/auth/password` | 修改密码 | 登录 |
| `GET` | `/api/dashboard/stats` | 仪表盘统计 | 登录 |
| `POST` | `/api/query/execute` | 执行 SQL 查询 | 登录 |
| `POST` | `/api/query/review` | AI 评审（SSE 流式） | 登录 |
| `POST` | `/api/query/export` | 导出查询结果 | 登录 |
| `GET` | `/api/query/history` | 查询历史列表 | 登录 |
| `POST` | `/api/tickets` | 创建工单 | 登录 |
| `GET` | `/api/tickets` | 工单列表 | 登录 |
| `GET` | `/api/tickets/:id` | 工单详情 | 登录 |
| `POST` | `/api/tickets/:id/approve` | 审批通过 | DBA/Admin |
| `POST` | `/api/tickets/:id/reject` | 审批驳回 | DBA/Admin |
| `POST` | `/api/tickets/:id/execute` | 执行工单 | 提交人/DBA/Admin |
| `POST` | `/api/tickets/:id/cancel` | 取消工单 | 提交人/DBA/Admin |
| `GET` | `/api/datasources/:id/tables` | 获取数据源表列表 | 登录 |
| `POST` | `/api/datasources` | 创建数据源 | Admin |
| `GET` | `/api/datasources` | 数据源列表 | Admin |
| `POST` | `/api/datasources/:id/test` | 测试连接 | Admin |
| `POST` | `/api/users` | 创建用户 | Admin |
| `GET` | `/api/users` | 用户列表 | Admin |
| `PUT` | `/api/users/:id` | 编辑用户 | Admin |
| `GET` | `/api/roles` | 角色列表 | Admin |
| `POST` | `/api/policies` | 添加权限策略 | Admin |
| `GET` | `/api/policies` | 权限策略列表 | Admin |
| `POST` | `/api/mask-rules` | 创建脱敏规则 | Admin |
| `GET` | `/api/mask-rules` | 脱敏规则列表 | Admin |
| `POST` | `/api/sensitive-tables` | 标记敏感表 | Admin |
| `GET` | `/api/audit-logs` | 审计日志查询 | Admin/DBA |
| `GET` | `/api/settings` | 获取系统设置 | Admin |
| `PUT` | `/api/settings/dingtalk` | 更新钉钉配置 | Admin |
| `PUT` | `/api/settings/ai` | 更新 AI 配置 | Admin |
| `GET` | `/api/health` | 健康检查 | 公开 |

> 完整 API 文档见 [docs/api.md](docs/api.md)。

---

## 📄 License

私有项目，未授权禁止使用。
