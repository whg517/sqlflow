# SQLFlow v1.0 Release Notes

| 项目 | 信息 |
|------|------|
| **产品名称** | SQLFlow — SQL 审批管理平台 |
| **版本** | 1.0.0 |
| **发布日期** | 2026-05-23 |
| **状态** | 正式发布 🎉 |
| **团队** | 棱镜 Prism |

---

## 1. 产品概述

SQLFlow 是一个面向开发团队和 DBA 的 **SQL 审批管理平台**。v1.0 是首个正式发布版本，提供了从 SQL 在线查询、AI 前置评审、变更工单审批到数据脱敏、权限管理和操作审计的完整功能闭环。

**核心价值主张**：

> 将低风险查询自助化、高风险操作走审批流程，所有操作全程留痕可追溯，帮助运维从「执行者」转变为「审批者和规则制定者」。

**解决的核心问题**：

| 问题 | SQLFlow 方案 |
|------|-------------|
| 开发者直接连库查询，DBA 无法管控 | 统一查询入口，RBAC 权限隔离 |
| DDL/DML 变更无审批，线上事故频发 | 工单审批流 + AI 前置风险评审 |
| 查询结果含敏感数据，存在泄露风险 | 字段级数据脱敏，默认启用 |
| 操作无法追溯，出事后查不到记录 | 全量审计日志，不可删除 |
| DBA 被大量低风险查询请求淹没 | 低风险查询自助化，高风险走审批 |

---

## 2. 核心功能

### 2.1 在线 SQL 查询 🔍

- 支持 **MySQL** 和 **MongoDB** 双数据源
- CodeMirror 6 SQL 编辑器，语法高亮 + 自动补全
- 结果表格展示，支持分页浏览
- 导出为 CSV / JSON 格式（单次上限 10,000 行）
- 慢查询自动检测，30s 超时中断
- MongoDB 支持 aggregation pipeline（白名单 stage 校验）

### 2.2 AI 前置评审 🤖

- 提交 SQL 后自动进行 **风险分级** + **优化建议** + **变更影响分析**
- 多 Provider 支持：

  | Provider | 说明 |
  |----------|------|
  | OpenAI | GPT-4 等模型 |
  | 智谱 GLM | glm-4 等模型 |
  | Azure OpenAI | 企业级部署 |
  | 自定义 | 任意 OpenAI 兼容 API |

- SSE 流式返回，3-10 秒出结果，前端实时展示
- AI 超时自动降级为静态规则评审（基于 AST 的确定性规则）
- 评审结果缓存 30 秒，避免重复调用

### 2.3 数据脱敏 🔒

- 字段级脱敏规则，查询结果 **默认脱敏**
- 8 种内置规则：

  | 规则类型 | 示例（输入 → 输出） |
  |---------|---------------------|
  | 手机号 | 13812345678 → 138****5678 |
  | 身份证 | 110101199001011234 → 110101****1234 |
  | 姓名 | 张三丰 → 张** |
  | 邮箱 | zhangsan@example.com → z****n@example.com |
  | 银行卡 | 6222021234567890123 → 6222****0123 |
  | 地址 | 北京市朝阳区xx路 → 北京市** |
  | 全掩码 | any text → ****** |
  | 自定义正则 | 用户自定义 pattern + template |

- 敏感表标记（low/medium/high），默认拒绝访问
- Casbin 权限控制脱敏豁免（`desensitize:bypass`），豁免操作记录审计日志

### 2.4 变更工单审批流 📋

- DDL/DML/MongoDB update 操作必须走工单
- 完整状态机：

  ```
  SUBMITTED → AI_REVIEWED → PENDING_APPROVAL → APPROVED → EXECUTING → DONE
                                    ↓
                                 REJECTED
                                    ↓
                                CANCELLED
  ```

- AI 评审结果决定简化/标准工单流程
- 审批通过后 DBA 手动执行，可选定时执行（Scheduler 自动触发）
- 工单内评论讨论，支持嵌套回复

### 2.5 RBAC 权限管理 👥

- 三种内置角色：

  | 角色 | 权限范围 |
  |------|---------|
  | **admin** | 全部权限（系统管理 + 用户管理 + 数据源 + 策略 + 审计） |
  | **dba** | 审批 + 配置 + 查询 + 导出 + 脱敏豁免 |
  | **developer** | 查询 + 提交工单 + AI 评审 |

- 基于 **Casbin RBAC with domains**，数据源级别隔离
- 权限粒度：数据源 → 表 → 操作类型（select / update / delete / ddl / export / desensitize:bypass）
- 敏感表默认拒绝访问，需管理员显式授权

### 2.6 操作审计 📊

- 全量记录：查询、变更、导出、权限策略变更
- 记录字段：操作人、时间、SQL、影响行数、执行耗时、评审结果、IP 地址
- 支持多维度筛选：用户/时间/数据源/操作类型/关键词
- SQLite FTS5 全文搜索 + 关键词高亮
- 审计日志 API 层面不可删除

### 2.7 通知集成 🔔

- 钉钉 Webhook 机器人通知
- 工单提交 / 审批结果 / 执行完成实时推送
- 中高风险操作实时告警到 DBA 群

### 2.8 系统管理 🖥

- **Dashboard 概览**：统计卡片 + 待办事项
- **数据源管理**：注册/编辑/连接测试/库表列表
- **用户管理**：Admin CRUD + 重置密码
- **数据库备份**：定时自动备份，gzip 压缩，自动轮转（默认保留 10 个）
- **系统设置**：AI Provider 配置 + 脱敏规则 + 钉钉通知

### 2.9 钉钉 OAuth 登录（可选）

- 支持通过钉钉扫码/授权登录
- 自动关联或创建系统用户
- 可在配置中开启/关闭

---

## 3. 技术架构

### 3.1 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| **后端** | Go 1.25 + Echo v4 | 高性能 HTTP 框架 |
| **前端** | React 19 + Vite 8 + TypeScript 6 | 现代前端技术栈 |
| **UI 框架** | TailwindCSS 4 + Shadcn/ui | 原子化 CSS + 组件库 |
| **SQL 编辑器** | CodeMirror 6 | 专业级代码编辑器 |
| **状态管理** | Zustand + TanStack Query | 轻量状态 + 服务端状态 |
| **平台数据库** | SQLite（WAL 模式） | 零运维嵌入式数据库 |
| **目标数据库** | MySQL + MongoDB | 双数据源支持 |
| **SQL 解析** | pingcap/parser（AST） | 语法树级别的 SQL 分析 |
| **权限引擎** | Casbin RBAC with domains | 企业级权限控制 |
| **认证** | JWT（HS256）+ Refresh Token | 无状态认证 |
| **AI 评审** | OpenAI / GLM / Azure / 自定义 | SSE 流式调用 |
| **部署** | Docker（单容器） | 前端 embed 进 Go 二进制 |

### 3.2 项目结构

```
sql-platform/
├── cmd/server/main.go           # 启动入口
├── config/                      # 配置加载 + Viper
├── internal/
│   ├── api/
│   │   ├── handler/             # HTTP 请求处理器
│   │   ├── middleware/          # 中间件（JWT/CORS/Logger/Admin）
│   │   └── router.go            # 路由注册
│   ├── connpool/                # MySQL/MongoDB 连接池管理
│   ├── db/db.go                 # 数据库初始化 + 迁移
│   ├── model/model.go           # 数据模型定义
│   ├── pkg/
│   │   ├── casbin/              # Casbin RBAC 适配器
│   │   ├── crypto/              # AES 加密工具
│   │   ├── mask/                # 数据脱敏引擎
│   │   └── sqlparser/           # SQL 解析器（AST）
│   ├── resp/response.go         # 统一响应封装
│   └── service/                 # 业务逻辑层（核心）
├── web/src/                     # React 前端
├── docs/                        # 文档（API / 部署 / 安全审计）
├── .github/workflows/           # CI/CD 流水线
├── Dockerfile                   # 多阶段 Docker 构建
└── docker-compose.yaml          # Docker Compose 编排
```

### 3.3 Docker 架构

单容器部署，前端构建产物 embed 进 Go 二进制：

```
┌─────────────────────────────────────┐
│           sqlflow container          │
│                                     │
│  ┌──────────┐    ┌────────────────┐ │
│  │ Go Binary │────│  web/dist/    │ │
│  │ (embed)   │    │  (static FS)  │ │
│  └──────────┘    └────────────────┘ │
│       │                             │
│  ┌──────────┐    ┌────────────────┐ │
│  │ SQLite   │    │  Connection    │ │
│  │ (WAL)    │    │  Pool Manager  │ │
│  └──────────┘    └───────┬────────┘ │
└──────────────────────────┼──────────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
         ┌────┴───┐  ┌────┴───┐  ┌────┴───┐
         │ MySQL  │  │MongoDB │  │ DingTalk│
         └────────┘  └────────┘  └────────┘
```

---

## 4. 质量指标

### 4.1 测试

| 指标 | 值 |
|------|------|
| Go 测试覆盖率 | **85.2%**（300+ 测试函数，11937 行测试代码） |
| E2E 测试 | **7 个套件**（auth / datasources / tickets / global-interactions / permission-isolation / login-query-ticket） |
| CI 流水线 | **8 个 Job**（Lint → Test → Build → E2E → Docker Verify → Security Scan → Summary） |

### 4.2 安全审计

| 严重等级 | 发现数 | 修复数 | 状态 |
|---------|--------|--------|------|
| 🔴 严重 (Critical) | 3 | 3 | ✅ 全部修复 |
| 🟠 高危 (High) | 2 | 2 | ✅ 全部修复 |
| 🟡 中等 (Medium) | 4 | 2 | ⚠️ 2 项延迟至 v1.1 |
| 🟢 低危 (Low) | 3 | 1 | ℹ️ 可接受风险 |
| ℹ️ 信息 (Info) | 3 | 0 | ℹ️ 良好实践确认 |

**关键安全措施**：
- 密钥全部支持环境变量注入，配置文件不存储明文
- 启动时强制校验密钥长度和密码复杂度
- CORS 生产环境域名白名单
- SQL 注入防护（AST 解析 + 参数化查询）
- API Key 脱敏返回
- CI 集成 govulncheck + Trivy 扫描

### 4.3 性能基准（20 并发）

| 接口 | 吞吐量 (req/s) | P50 延迟 | P95 延迟 | 基准线 |
|------|---------------|----------|----------|--------|
| Login | 3,494 | 0.98ms | 5.49ms | 500ms ✅ |
| Auth Me | 7,009 | 1.71ms | 5.79ms | — |
| Dashboard Stats | 3,046 | 5.48ms | 11.01ms | 200ms ✅ |
| Query History | 2,800 | 4.12ms | 27.38ms | — |
| Ticket List | 2,788 | 5.75ms | 15.85ms | 300ms ✅ |
| Ticket Create | 3,079 | 1.26ms | 48.02ms | 500ms ✅ |

| 数据库操作 | P50 延迟 | P99 延迟 |
|-----------|----------|----------|
| Insert | 43.9µs | 187.8µs |
| Select (COUNT) | 3.4µs | 17.8µs |
| Select (indexed) | 5.3µs | 20.6µs |
| Select (paginated) | 48.7µs | 283.5µs |
| Concurrent Write | 350.3µs | 1.63ms（528K rows/s） |

**所有性能指标均通过基准线检查** ✅

### 4.4 构建

| 检查项 | 状态 |
|--------|------|
| Go 编译 | ✅ 通过 |
| React 生产构建 | ✅ 通过（310KB gzipped） |
| Docker 镜像构建 | ✅ 通过 |
| Docker Smoke Test | ✅ 通过（Binary + Entrypoint + Frontend） |

---

## 5. 快速开始

### 5.1 Docker 部署（推荐）

```bash
# 1. 克隆仓库
git clone https://github.com/whg517/sqlflow.git
cd sqlflow

# 2. 准备配置
cp release/v1.0/config.yaml.example config/config.yaml

# 3. 设置环境变量（必填）
export SQLFLOW_JWT_SECRET="$(openssl rand -hex 32)"
export SQLFLOW_ENCRYPTION_KEY="$(openssl rand -hex 16)"
export SQLFLOW_ADMIN_PASSWORD="<your-strong-password>"

# 4. 启动
docker compose up -d

# 5. 访问
# http://localhost:8080
```

> ⚠️ **安全提醒**：生产环境务必通过环境变量设置 JWT_SECRET、ENCRYPTION_KEY 和 ADMIN_PASSWORD，不要使用默认值。

### 5.2 二进制部署

```bash
# 下载对应平台的二进制
# Linux amd64
wget https://github.com/whg517/sqlflow/releases/download/v1.0.0/sqlflow-linux-amd64.tar.gz
tar -xzf sqlflow-linux-amd64.tar.gz
chmod +x sqlflow-linux-amd64

# 准备配置
cp config/config.example.yaml config/config.yaml

# 设置环境变量
export SQLFLOW_JWT_SECRET="$(openssl rand -hex 32)"
export SQLFLOW_ENCRYPTION_KEY="$(openssl rand -hex 16)"
export SQLFLOW_ADMIN_PASSWORD="<your-strong-password>"

# 启动
./sqlflow-linux-amd64
```

### 5.3 从源码构建

```bash
# 环境要求：Go 1.25+, Node.js 24+

# 前端构建
cd web && npm ci && npm run build && cd ..

# 后端编译
go build -ldflags="-s -w -X main.version=v1.0.0" -o sqlflow ./cmd/server/

# 运行
./sqlflow
```

### 5.4 Docker 镜像

```bash
# 拉取官方镜像
docker pull ghcr.io/whg517/sqlflow:v1.0.0

# 或使用 latest tag
docker pull ghcr.io/whg517/sqlflow:latest
```

---

## 6. 配置说明

### 6.1 必填环境变量

| 变量名 | 说明 | 生成方法 |
|--------|------|---------|
| `SQLFLOW_JWT_SECRET` | JWT 签名密钥（≥16 字符） | `openssl rand -hex 32` |
| `SQLFLOW_ENCRYPTION_KEY` | AES 加密密钥（16/24/32 字节 hex） | `openssl rand -hex 16` |
| `SQLFLOW_ADMIN_PASSWORD` | 管理员初始密码（≥8 字符，含字母+数字） | 手动设置 |

### 6.2 推荐环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SQLFLOW_AI_API_KEY` | AI 评审 API Key | — |
| `SQLFLOW_CORS_ORIGINS` | CORS 允许的域名（逗号分隔） | `*`（开发模式） |
| `SQLFLOW_DINGTALK_SECRET` | 钉钉机器人签名密钥 | — |

### 6.3 配置文件

完整配置参考 `config/config.example.yaml`，主要配置段：

```yaml
server:
  port: 8080
  # TLS（可选）
  # tls:
  #   enable: true
  #   cert_file: "/etc/ssl/certs/sqlflow.crt"
  #   key_file: "/etc/ssl/private/sqlflow.key"

jwt:
  expiry: "24h"

ai:
  provider: "openai"    # openai | zhipu | azure | custom
  model: "gpt-4"
  timeout: "10s"

dingtalk:
  webhook_url: ""
  secret: ""
  oauth:
    enabled: false
    app_key: ""
    app_secret: ""
    redirect_url: "http://localhost:8080/api/v1/auth/dingtalk/callback"

backup:
  enabled: true
  dir: "./data/backups"
  interval: "6h"
  keep: 10
  compress: true
```

### 6.4 AI Provider 配置

| Provider | base_url | model |
|----------|----------|-------|
| OpenAI | `https://api.openai.com/v1`（默认） | `gpt-4` |
| 智谱 GLM | `https://open.bigmodel.cn/api/paas/v4` | `glm-4` |
| Azure OpenAI | 需填写 Azure Endpoint | 需填写部署名 |
| 自定义 | 需填写 | 需填写 |

---

## 7. API 端点概览

> 基础 URL: `http://<host>:8080`
> 认证方式：`Authorization: Bearer <token>`
> 完整 API 文档见 [docs/api.md](../docs/api.md)

### 认证

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/auth/login` | 用户登录 | 公开 |
| `GET` | `/api/auth/me` | 获取当前用户信息 | 登录 |
| `PUT` | `/api/auth/password` | 修改密码 | 登录 |

### 查询

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/query/execute` | 执行 SQL 查询 | 登录 |
| `POST` | `/api/query/review` | AI 评审（SSE 流式） | 登录 |
| `POST` | `/api/query/export` | 导出查询结果 | 登录 |
| `GET` | `/api/query/history` | 查询历史 | 登录 |

### 工单

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `POST` | `/api/tickets` | 创建工单 | 登录 |
| `GET` | `/api/tickets` | 工单列表 | 登录 |
| `GET` | `/api/tickets/:id` | 工单详情 | 登录 |
| `POST` | `/api/tickets/:id/approve` | 审批通过 | DBA/Admin |
| `POST` | `/api/tickets/:id/reject` | 审批驳回 | DBA/Admin |
| `POST` | `/api/tickets/:id/execute` | 执行工单 | 提交人/DBA/Admin |
| `POST` | `/api/tickets/:id/cancel` | 取消工单 | 提交人/DBA/Admin |

### 管理（Admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST/GET/PUT/DELETE` | `/api/users[/:id]` | 用户管理 |
| `POST/GET` | `/api/datasources[/:id]` | 数据源管理 |
| `GET/POST/DELETE` | `/api/policies[/:id]` | 权限策略 |
| `POST/GET/PUT/DELETE` | `/api/mask-rules[/:id]` | 脱敏规则 |
| `GET/POST/DELETE` | `/api/sensitive-tables[/:id]` | 敏感表 |
| `GET` | `/api/audit-logs` | 审计日志 |
| `GET/PUT` | `/api/settings/*` | 系统设置 |

### 健康检查

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| `GET` | `/api/health` | 健康检查 | 公开 |

---

## 8. 已知限制

| # | 限制 | 建议 |
|---|------|------|
| 1 | 前端 JS bundle 超过 500KB | 后续版本做 code-splitting |
| 2 | MongoDB 仅支持基本 aggregation pipeline | v1.1 扩展 stage 白名单 |
| 3 | 导出上限 10,000 行 | 设计限制，防止内存溢出 |
| 4 | 查询结果默认上限 1,000 行 | 可在配置中调整 |
| 5 | 仅支持 HTTP | 需反向代理层终止 TLS；v1.1 计划添加应用层 TLS |
| 6 | 查询执行端点无速率限制 | v1.1 计划添加令牌桶限流中间件 |
| 7 | 登录接口无暴力破解防护 | v1.1 计划添加失败计数 + IP 限流 + CAPTCHA |
| 8 | 审计日志数据库层缺少 DELETE 保护 | API 层已安全；v1.1 考虑 append-only 存储 |

---

## 9. 升级路径

此为 v1.0 首个正式版本，无升级路径。后续版本将遵循语义化版本规范。

**v1.1 规划方向**（基于安全审计建议）：
- 应用层 TLS 支持（`server.tls` 配置项）
- 令牌桶速率限制中间件
- 登录暴力破解防护（失败计数 + IP 限流）
- 审计日志 append-only 存储
- JWT 算法迁移（HS256 → RS256）
- 前端 code-splitting 优化

---

## 10. 贡献者

- **棱镜 Prism 团队** — 架构设计、全栈开发、测试、文档、DevOps
- **Marcus** ⚡ — 技术负责人，核心架构、代码审查、安全审计、发布管理
- **钱进** — 项目经理，需求管理、流程协调、发布协调

---

## 11. 许可证

内部项目，未经授权不得对外分发。

---

## 12. 相关文档

| 文档 | 说明 |
|------|------|
| [CHANGELOG.md](./CHANGELOG.md) | 完整变更记录 |
| [SECURITY_AUDIT.md](./SECURITY_AUDIT.md) | 安全审查报告 |
| [config.yaml.example](./config.yaml.example) | 配置示例 |
| [docker-compose.yaml](./docker-compose.yaml) | Docker Compose 编排 |
| [Dockerfile](./Dockerfile) | Docker 构建文件 |
| [API 文档](../docs/api.md) | 完整 API 端点文档 |
| [部署指南](../docs/deployment.md) | 部署文档 |
| [CI/CD 指南](../docs/cicd-guide.md) | CI/CD 流水线说明 |
| [PRD](../docs/spec/PRD.md) | 产品需求文档 |
| [架构设计](../docs/spec/ARCHITECTURE.md) | 技术架构设计 |
| [UI 设计](../docs/spec/UI-DESIGN.md) | UI 交互设计 |

---

*SQLFlow v1.0 — 让 SQL 审批管理更安全、更高效、更可追溯。*
