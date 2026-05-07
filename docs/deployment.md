# SQLFlow 部署文档

## 部署方式概览

| 方式 | 适用场景 | 说明 |
|------|---------|------|
| Docker Compose | 生产部署（推荐） | 单容器，自带前端构建产物 |
| Docker 手动构建 | 自定义部署 | 灵活控制构建参数 |
| 本地开发 | 开发调试 | 前后端分别启动 |

---

## 1. Docker Compose 部署（推荐）

### 前置条件

- Docker 20.10+
- Docker Compose V2

### 步骤

```bash
# 1. 克隆仓库
git clone https://github.com/whg517/sqlflow.git
cd sqlflow

# 2. 创建环境变量文件
cat > .env << 'EOF'
# 必填
JWT_SECRET=your-strong-secret-key-here
ADMIN_USERNAME=admin
ADMIN_PASSWORD=your-strong-password

# 可选：数据源密码加密密钥（16 字节 hex，如 a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6）
# 不设置则首次启动自动生成（每次重启会变化，导致已有数据源密码无法解密）
ENCRYPTION_KEY=

# AI 评审配置（可选，不配置则 AI 评审功能降级为静态规则）
AI_PROVIDER=openai
AI_MODEL=gpt-4
AI_API_KEY=sk-your-api-key
AI_BASE_URL=https://api.openai.com/v1

# 钉钉通知（可选）
DINGTALK_WEBHOOK_URL=
DINGTALK_SECRET=
EOF

# 3. 构建并启动
docker compose up -d

# 4. 查看日志
docker compose logs -f sqlflow

# 5. 检查健康状态
curl http://localhost:8080/api/health
```

### 访问

- 地址：`http://<host>:8080`
- 默认管理员：`.env` 中 `ADMIN_USERNAME` / `ADMIN_PASSWORD`

### 停止与清理

```bash
# 停止
docker compose down

# 停止并删除数据卷（清除所有数据）
docker compose down -v
```

---

## 2. Docker 手动构建

```bash
# 构建镜像
docker build -t sqlflow:latest .

# 运行
docker run -d \
  --name sqlflow \
  -p 8080:8080 \
  -v sqlflow-data:/app/data \
  -e JWT_SECRET=your-secret \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=admin123 \
  sqlflow:latest
```

### 构建参数

Dockerfile 使用多阶段构建：

1. **Stage 1 (frontend)** — Node.js 22 构建 React 前端 → `web/dist/`
2. **Stage 2 (builder)** — Go 1.25 编译二进制，CGO_ENABLED=0 静态链接
3. **Stage 3 (runtime)** — Alpine 3.21 最小运行时，仅包含 `sqlflow` 二进制 + `config.yaml` + `entrypoint.sh`

最终镜像约 30MB。

---

## 3. 本地开发

### 前置条件

- Go 1.25+
- Node.js 22+
- npm

### 后端

```bash
# 安装 Go 依赖
go mod download

# 运行（默认读取 ./config/config.yaml）
go run ./cmd/server/

# 或编译后运行
go build -o server ./cmd/server/
./server
```

### 前端

```bash
cd web

# 安装依赖
npm install

# 开发模式（HMR，代理 API 到 localhost:8080）
npm run dev

# 生产构建
npm run build
```

### 数据持久化

本地开发时，SQLite 数据库默认存储在 `./data/sqlflow.db`。该目录已在 `.gitignore` 中排除。

---

## 配置详解

### 配置加载优先级

1. 环境变量（最高优先级）
2. `config/config.yaml` 文件

环境变量名与配置项对应关系为全大写 + 下划线，如 `jwt.secret` → `JWT_SECRET`。

### 完整配置项

```yaml
server:
  port: 8080                # 服务监听端口

jwt:
  secret: "change-me"       # JWT 签名密钥（生产环境必须修改）
  expiry: "24h"             # Token 有效期

admin:
  username: "admin"         # 初始管理员用户名（首次启动创建，后续不覆盖）
  password: "admin123"      # 初始管理员密码

db:
  path: "./data/sqlflow.db" # SQLite 数据库文件路径

encryption_key: ""          # 数据源密码 AES 加密密钥（16 字节 hex）
                            # 不设置则自动生成（重启后变化，需注意）

query_history_max: 200      # 每用户最大查询历史条数

ai:
  provider: "openai"        # AI 提供商
  model: "gpt-4"            # 模型名称
  api_key: ""               # API Key（建议通过环境变量设置）
  base_url: "https://api.openai.com/v1"  # API Base URL
  timeout: "10s"            # 评审超时时间
```

### 环境变量完整列表

| 环境变量 | 对应配置项 | 默认值 | 说明 |
|----------|-----------|--------|------|
| `SERVER_PORT` | server.port | 8080 | |
| `JWT_SECRET` | jwt.secret | change-me | 生产环境必须修改 |
| `ADMIN_USERNAME` | admin.username | admin | |
| `ADMIN_PASSWORD` | admin.password | admin123 | 生产环境必须修改 |
| `DB_PATH` | db.path | ./data/sqlflow.db | |
| `ENCRYPTION_KEY` | encryption_key | 自动生成 | 16 字节 hex 字符串 |
| `QUERY_HISTORY_MAX` | query_history_max | 200 | |
| `AI_PROVIDER` | ai.provider | openai | |
| `AI_MODEL` | ai.model | gpt-4 | |
| `AI_API_KEY` | ai.api_key | — | |
| `AI_BASE_URL` | ai.base_url | https://api.openai.com/v1 | |
| `AI_TIMEOUT` | ai.timeout | 10s | |
| `DINGTALK_WEBHOOK_URL` | dingtalk.webhook_url | — | |
| `DINGTALK_SECRET` | dingtalk.secret | — | |

---

## 数据备份与恢复

### SQLite 数据备份

```bash
# 方法 1：直接复制文件（需停止容器）
docker compose down
cp -r data/ data_backup_$(date +%Y%m%d)/
docker compose up -d

# 方法 2：在线备份（无需停机）
docker compose exec sqlflow sqlite3 /app/data/sqlflow.db ".backup /app/data/backup.db"
docker compose cp sqlflow:/app/data/backup.db ./backup_$(date +%Y%m%d).db
```

### 数据卷位置

Docker Compose 使用命名卷 `sqlflow-data`，实际路径可通过 `docker volume inspect sqlflow_sqlflow-data` 查看。

---

## 健康检查

容器内置健康检查（`docker-compose.yaml` 已配置）：

- **端点：** `GET /api/health`
- **间隔：** 15 秒
- **超时：** 5 秒
- **重试：** 3 次
- **启动等待：** 10 秒

手动检查：

```bash
curl -f http://localhost:8080/api/health
# 成功: {"status":"ok"}
```

---

## 生产环境注意事项

### 安全

1. **JWT_SECRET** — 必须设置为强随机字符串（至少 32 字符）
2. **ADMIN_PASSWORD** — 必须修改默认密码，满足长度 8-128 字符，包含字母和数字
3. **ENCRYPTION_KEY** — 建议固定设置，避免重启后无法解密已有数据源密码
4. **AI_API_KEY** — 通过环境变量传递，不要写入配置文件
5. **HTTPS** — MVP 未内置 TLS，建议通过反向代理（Nginx/Caddy）终止 TLS

### 反向代理示例（Nginx）

```nginx
server {
    listen 443 ssl;
    server_name sqlflow.example.com;

    ssl_certificate     /etc/ssl/certs/sqlflow.pem;
    ssl_certificate_key /etc/ssl/private/sqlflow.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE 支持（AI 评审流式响应）
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 300s;
    }
}
```

### 资源需求

| 资源 | 最低 | 推荐 |
|------|------|------|
| CPU | 1 核 | 2 核 |
| 内存 | 256MB | 512MB |
| 磁盘 | 100MB + 数据 | 1GB + 数据 |

### 日志

```bash
# 查看容器日志
docker compose logs -f sqlflow

# 导出日志
docker compose logs --no-color sqlflow > sqlflow.log
```

---

## 故障排查

### 常见问题

| 问题 | 可能原因 | 解决方案 |
|------|---------|---------|
| 启动失败 "failed to open database" | 数据目录权限问题 | 检查 `data/` 目录权限 |
| 登录后 401 | JWT_SECRET 变化 | 确保 JWT_SECRET 不变 |
| 数据源连接失败 | 密码解密失败 | ENCRYPTION_KEY 变化导致，需重新配置数据源 |
| AI 评审不工作 | API Key 未配置 | 设置 `AI_API_KEY` 环境变量 |
| 钉钉通知不发送 | Webhook 未配置 | 设置 `DINGTALK_WEBHOOK_URL` |
| 前端白屏 | 后端未启动 | 检查后端日志，确认 API 可达 |

### 数据库锁定

SQLite 在高并发写入时可能出现 `database is locked`。平台已配置 WAL 模式缓解此问题。如果仍然频繁出现，可考虑迁移至 MySQL/PostgreSQL（Ent ORM 支持 driver 切换，业务代码无需改动）。
