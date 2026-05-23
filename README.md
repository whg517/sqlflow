# SQLFlow

> SQL 审批管理平台 — 安全可控的数据库查询与变更审批系统

## 简介

SQLFlow 是一个轻量级的 SQL 审批管理平台，帮助团队安全地管理数据库查询和数据变更。支持 MySQL 和 MongoDB 数据源，提供在线查询、AI 前置评审、数据脱敏、变更工单审批、RBAC 权限控制等核心功能。

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.25 + Echo v4 + SQLite (modernc.org/sqlite) |
| 前端 | React 19 + TypeScript + Tailwind CSS 4 + Radix UI |
| 权限 | Casbin RBAC |
| 加密 | AES-GCM (数据源密码) + bcrypt (用户密码) + JWT (认证) |
| 容器 | Docker + Docker Compose |

## 核心功能

- **数据源管理**：支持 MySQL、MongoDB，连接池管理，连接测试
- **在线查询**：SQL 编辑器（CodeMirror），查询历史，结果导出
- **AI 前置评审**：提交工单前自动进行 SQL 安全评审
- **数据脱敏**：自定义脱敏规则，查询结果自动脱敏
- **变更工单**：SQL 变更审批流程，状态跟踪
- **用户管理**：用户 CRUD，角色分配（admin/dba/developer）
- **权限控制**：Casbin RBAC，按数据源+表+操作维度精细控制
- **操作审计**：全量操作日志记录，可追溯
- **钉钉通知**：工单状态变更钉钉消息推送

## 快速开始

### 环境要求

- Go 1.25+
- Node.js 18+ / npm
- Docker & Docker Compose（容器部署时）

### Docker 部署（推荐）

```bash
# 1. 克隆仓库
git clone https://github.com/whg517/sqlflow.git
cd sqlflow

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env，设置 ADMIN_PASSWORD 和 JWT_SECRET

# 3. 准备配置文件
cp config/config.yaml.example config/config.yaml
# 编辑 config/config.yaml，设置 jwt.secret、encryption_key

# 4. 启动
docker compose up -d

# 5. 访问 http://localhost:8080
# 默认用户名: admin，密码: .env 中的 ADMIN_PASSWORD
```

### 本地开发

```bash
# 后端
cd sqlflow
cp config/config.yaml.example config/config.yaml
# 编辑 config/config.yaml
go run ./cmd/server/

# 前端（开发模式）
cd sqlflow/web
npm install
npm run dev

# 前端（生产构建，嵌入到 Go 二进制）
cd sqlflow/web
npm run build
# 构建产物在 web/dist/，通过 go:embed 嵌入
```

## 配置说明

详见 [config/config.yaml.example](config/config.yaml.example)

| 配置项 | 说明 | 必填 |
|--------|------|------|
| `server.port` | 服务端口，默认 8080 | 否 |
| `jwt.secret` | JWT 签名密钥，建议 `openssl rand -hex 32` | **是** |
| `jwt.expiry` | Token 有效期，默认 24h | 否 |
| `admin.username` | 初始管理员用户名 | 否 |
| `admin.password` | 初始管理员密码 | **是** |
| `db.path` | SQLite 数据库路径，默认 `./data/sqlflow.db` | 否 |
| `encryption_key` | 数据源密码加密密钥，建议 `openssl rand -hex 16` | **是** |

## 项目结构

```
sqlflow/
├── cmd/server/main.go          # 入口
├── config/                     # 配置
├── internal/
│   ├── api/                    # HTTP 层
│   │   ├── handler/            # 请求处理器
│   │   ├── middleware/         # 中间件（JWT/CORS/Logger/Recovery）
│   │   └── router.go           # 路由定义
│   ├── connpool/               # 数据库连接池
│   ├── db/                     # SQLite 数据库初始化
│   ├── ent/schema/             # 数据模型
│   ├── model/                  # 数据结构
│   ├── pkg/                    # 工具包
│   │   ├── casbin/             # RBAC 策略
│   │   ├── crypto/             # AES-GCM 加解密
│   │   ├── mask/               # 数据脱敏
│   │   └── sqlparser/          # SQL 解析
│   ├── resp/                   # 统一响应格式
│   └── service/                # 业务逻辑层
├── web/                        # 前端（React + TypeScript）
│   ├── src/
│   │   ├── api/                # API 客户端
│   │   ├── components/         # UI 组件
│   │   ├── pages/              # 页面
│   │   └── main.tsx            # 入口
│   └── vite.config.ts
├── docs/                       # 项目文档
├── Dockerfile
├── docker-compose.yaml
└── Makefile
```

## API

启动服务后访问 `http://localhost:8080/api/health` 检查健康状态。

主要 API 端点：

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | `/api/auth/login` | 登录 | 公开 |
| GET | `/api/auth/me` | 当前用户信息 | 登录 |
| PUT | `/api/auth/password` | 修改密码 | 登录 |
| GET/POST | `/api/datasources` | 数据源管理 | 管理员 |
| GET | `/api/datasources/:id/tables` | 获取表列表 | 登录 |
| GET/POST | `/api/users` | 用户管理 | 管理员 |
| GET/POST | `/api/roles` | 角色管理 | 管理员 |
| GET/POST/DELETE | `/api/policies` | 权限策略管理 | 管理员 |

## 安全说明

- 用户密码使用 bcrypt 哈希存储
- 数据源密码使用 AES-GCM 加密存储
- API 认证基于 JWT
- RBAC 权限控制（Casbin）
- 生产部署时务必：
  - 设置 `jwt.secret`（不要使用默认值）
  - 设置 `encryption_key`
  - 修改默认管理员密码
  - 配置 `CORS_ALLOWED_ORIGINS` 环境变量

## License

MIT
