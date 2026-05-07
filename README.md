# SQLFlow — SQL 审批管理平台

SQLFlow 是一个面向开发团队和 DBA 的 SQL 审批管理平台，将低风险查询自助化，高风险操作走审批流程，全程留痕可追溯。

## 功能概览

- **数据源管理** — 注册 MySQL / MongoDB 实例，连接池管理，健康检查
- **在线查询** — SQL 编辑器 + 语法校验 + 执行 + 结果表格展示（支持分页）
- **AI 前置评审** — 提交 SQL 后自动风险分级、优化建议、变更影响分析
- **数据脱敏** — 字段级脱敏规则，Casbin 权限控制脱敏豁免
- **变更工单** — DDL/DML 走工单流程：提交 → AI 评审 → DBA 审批 → 手动执行
- **操作审计** — 全量记录查询、变更、导出操作
- **钉钉通知** — 工单审批结果 + 高风险操作实时告警
- **权限管理** — Casbin RBAC，admin / dba / developer 三种角色

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.25 + Echo v4 + Ent ORM |
| 前端 | React 19 + Vite + TypeScript + TailwindCSS + Shadcn/ui |
| SQL 编辑器 | CodeMirror 6 |
| 平台数据库 | SQLite (WAL 模式) |
| 目标数据库 | MySQL + MongoDB |
| SQL 解析 | pingcap/parser (AST) |
| 权限 | Casbin RBAC with domains |
| 认证 | JWT (HS256) |
| 部署 | Docker (单容器) |

## 快速开始

### 环境要求

- Go 1.25+
- Node.js 22+
- Docker & Docker Compose（可选）

### Docker 部署（推荐）

```bash
# 1. 克隆仓库
git clone https://github.com/whg517/sqlflow.git
cd sqlflow

# 2. 创建 .env 文件（或直接使用默认值测试）
cp .env.example .env
# 编辑 .env 设置 JWT_SECRET、ADMIN_PASSWORD 等敏感配置

# 3. 启动
docker compose up -d

# 4. 访问
# http://localhost:8080
# 默认管理员: admin / admin123
```

### 本地开发

```bash
# 后端
export PATH=$PATH:/usr/local/go/bin
go run ./cmd/server/

# 前端（另一个终端）
cd web
npm install
npm run dev
```

后端默认监听 `:8080`，前端 dev server 代理 API 请求到后端。

## 配置

配置通过 `config/config.yaml` 加载，敏感配置通过环境变量覆盖：

| 环境变量 | 说明 | 默认值 |
|----------|------|--------|
| `SERVER_PORT` | 服务端口 | `8080` |
| `JWT_SECRET` | JWT 签名密钥 | `change-me` |
| `ADMIN_USERNAME` | 初始管理员用户名 | `admin` |
| `ADMIN_PASSWORD` | 初始管理员密码 | `admin123` |
| `DB_PATH` | SQLite 数据库路径 | `./data/sqlflow.db` |
| `ENCRYPTION_KEY` | 数据源密码加密密钥（16 字节 hex） | 自动生成 |
| `AI_PROVIDER` | AI 提供商 | `openai` |
| `AI_MODEL` | AI 模型 | `gpt-4` |
| `AI_API_KEY` | AI API Key | — |
| `AI_BASE_URL` | AI API Base URL | `https://api.openai.com/v1` |
| `DINGTALK_WEBHOOK_URL` | 钉钉机器人 Webhook | — |
| `DINGTALK_SECRET` | 钉钉签名密钥 | — |

## 项目结构

```
sql-platform/
├── cmd/server/main.go          # 启动入口
├── config/                     # 配置加载 + config.yaml
├── internal/
│   ├── api/
│   │   ├── handler/            # HTTP handlers
│   │   ├── middleware/         # 中间件 (auth, cors, logger, recovery, admin)
│   │   └── router.go           # 路由注册
│   ├── db/db.go                # Ent 客户端初始化
│   ├── model/model.go          # 请求/响应模型
│   ├── pkg/
│   │   ├── sqlparser/          # SQL 解析器
│   │   ├── mask/               # 数据脱敏
│   │   └── crypto/             # 加密工具
│   └── service/                # 业务逻辑层
├── web/src/                    # React 前端
│   ├── api/                    # API 请求封装
│   ├── components/             # 通用组件 + Shadcn/ui
│   ├── pages/                  # 页面组件
│   └── store/                  # Zustand 状态管理
├── docs/
│   ├── spec/                   # PRD、架构、UI 设计文档
│   ├── api.md                  # API 文档
│   └── deployment.md           # 部署文档
├── Dockerfile                  # 多阶段构建
├── docker-compose.yaml         # 编排配置
└── entrypoint.sh               # 容器入口脚本
```

## 文档

- [API 文档](docs/api.md) — 全部 HTTP API 端点说明
- [部署文档](docs/deployment.md) — Docker 部署、本地开发、配置详解
- [产品需求文档](docs/spec/PRD.md) — 功能需求规格
- [技术架构](docs/spec/ARCHITECTURE.md) — 架构设计
- [UI 设计](docs/spec/UI-DESIGN.md) — 交互与视觉规范

## 许可证

私有项目，未授权禁止使用。
