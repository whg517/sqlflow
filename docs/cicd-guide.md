# SQLFlow CI/CD 配置指南

## 流水线概览

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Push / PR → CI                               │
│                                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐  │
│  │ Backend Lint │  │ Frontend Lint│  │   Security Scan         │  │
│  │ (fmt/vet/    │  │ (ESLint/     │  │   (govulncheck/trivy)   │  │
│  │  golangci)   │  │  lockfile)   │  │                         │  │
│  └──────┬───────┘  └──────┬───────┘  └──────────────────────────┘  │
│         │                 │                                         │
│  ┌──────▼───────┐  ┌──────▼───────┐                                │
│  │ Backend Test │  │ Frontend Build│                               │
│  │ (coverage)   │  │ (tsc + vite)  │                               │
│  └──────┬───────┘  └──────┬───────┘                                │
│         │                 │          ┌──────────────────────┐      │
│         │          ┌──────▼───────┐  │ Frontend E2E         │      │
│         │          │ Frontend E2E │  │ (Playwright)         │      │
│         └──────────┤ (Playwright) │  └──────────────────────┘      │
│                    └──────────────┘                                │
│                           │                                        │
│                    ┌──────▼───────────┐                             │
│                    │ Docker Build      │                            │
│                    │ (smoke test)      │                            │
│                    └──────────────────┘                             │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                    main push → CD                                    │
│                                                                     │
│  ┌───────────────────────┐    ┌──────────────────────────────┐     │
│  │ Build & Push Docker   │───▶│ Deploy to Server             │     │
│  │ (multi-arch GHCR)     │    │ (SSH + health check)         │     │
│  └───────────────────────┘    └──────────────────────────────┘     │
│                                        │                            │
│                                 ┌──────▼──────┐                    │
│                                 │ Notification │                    │
│                                 │ (DingTalk)   │                    │
│                                 └─────────────┘                    │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                    v* tag → Release                                 │
│                                                                     │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────────┐    │
│  │ CI Check        │─▶│ Build Binaries  │  │ Build Docker     │    │
│  │ (reuse ci.yml)  │  │ (5 platforms)   │  │ (multi-arch)     │    │
│  └─────────────────┘  └────────┬────────┘  └────────┬─────────┘    │
│                               │                    │               │
│                        ┌──────▼────────────────────▼─────────┐     │
│                        │ Create GitHub Release               │     │
│                        │ (artifacts + checksums + notes)    │     │
│                        └────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────────┘
```

## 工作流文件

| 文件 | 触发条件 | 说明 |
|------|---------|------|
| `ci.yml` | push to main/develop, PR to main | 代码检查 → 测试 → 构建 → E2E → 安全扫描 |
| `cd.yml` | push to main, workflow_dispatch | Docker 构建 → 推送 GHCR → SSH 部署 |
| `release.yml` | push tag `v*` | CI 检查 → 多平台编译 → Docker 推送 → GitHub Release |
| `stale.yml` | cron 每天 08:00 CST | 自动标记/关闭不活跃的 Issue 和 PR |

## GitHub Secrets 配置

### 必需 Secrets（CD 部署需要）

| Secret 名称 | 说明 | 获取方式 |
|-------------|------|---------|
| `GITHUB_TOKEN` | GitHub 自动提供，无需手动配置 | GitHub Actions 内置 |
| `DEPLOY_HOST` | 部署服务器 IP 或域名 | 服务器管理员提供 |
| `DEPLOY_USER` | SSH 登录用户名 | 服务器管理员提供 |
| `DEPLOY_SSH_KEY` | SSH 私钥（完整内容） | `cat ~/.ssh/id_ed25519` |
| `DEPLOY_PORT` | SSH 端口（默认 22） | 服务器管理员提供 |

### 可选 Secrets

| Secret 名称 | 说明 | 默认值 |
|-------------|------|--------|
| `DINGTALK_WEBHOOK_URL` | 钉钉机器人 Webhook URL | 无（不发送通知） |
| `DINGTALK_SECRET` | 钉钉机器人加签密钥 | 无 |

### GitHub Variables 配置

| Variable 名称 | 说明 | 示例 |
|--------------|------|------|
| `DEPLOY_URL` | 部署后的访问地址 | `https://sqlflow.example.com` |

### Secrets 配置步骤

```bash
# 1. 进入 GitHub 仓库 → Settings → Secrets and variables → Actions

# 2. 手动添加 Secret，或使用 GitHub CLI：
gh secret set DEPLOY_HOST --body "your-server-ip"
gh secret set DEPLOY_USER --body "deploy"
gh secret set DEPLOY_SSH_KEY < ~/.ssh/id_ed25519
gh secret set DEPLOY_PORT --body "22"

# 3. 添加 Variables：
gh variable set DEPLOY_URL --body "https://sqlflow.example.com"
```

## GitHub 分支保护规则（推荐）

建议在 Settings → Branches → Branch protection rules 中配置：

### main 分支

| 规则 | 设置 |
|------|------|
| Require a pull request before merging | ✅ 启用 |
| Required approving reviews | 1 |
| Require status checks to pass | ✅ 启用 |
| Required status checks | `ci-summary` |
| Require branches to be up to date | ✅ 启用 |
| Require signed commits | 可选 |

### PR 自动检查

PR 创建时自动显示 CI 状态。合并前需通过：
- ✅ Backend Lint & Vet
- ✅ Backend Test
- ✅ Frontend Lint
- ✅ Frontend Build
- ✅ Frontend E2E
- ✅ Docker Build Verify
- ✅ Security Scan
- ✅ CI Summary

## 版本发布流程

```bash
# 1. 创建版本 tag（触发 Release 流水线）
git tag v1.0.0
git push origin v1.0.0

# 2. 预发布版本
git tag v1.1.0-rc.1
git push origin v1.1.0-rc.1

# 3. 查看发布页面
# https://github.com/whg517/sqlflow/releases
```

## Docker 镜像标签策略

| 标签 | 说明 | 示例 |
|------|------|------|
| `latest` | 最新稳定版（仅非预发布） | `ghcr.io/whg517/sqlflow:latest` |
| `<semver>` | 精确版本 | `ghcr.io/whg517/sqlflow:1.0.0` |
| `<major>.<minor>` | 次版本跟踪 | `ghcr.io/whg517/sqlflow:1.0` |
| `<major>` | 主版本跟踪（仅稳定版） | `ghcr.io/whg517/sqlflow:1` |
| `<sha>` | Git commit SHA（短） | `ghcr.io/whg517/sqlflow:abc1234` |

## Dependabot 自动更新

配置在 `.github/dependabot.yml`，每周一 09:00 CST 自动检查：

- **Go 模块**：每周更新，最多 5 个 PR
- **npm 依赖**：每周更新（仅补丁版本和安全更新），最多 5 个 PR
- **GitHub Actions**：每周更新，最多 3 个 PR

## 故障排查

### CI 失败

1. 检查 GitHub Actions 页面的具体错误日志
2. 本地复现：`go build ./... && go test ./... && cd web && npm ci && npm run build`
3. 安全扫描失败：检查 `govulncheck` 和 `trivy` 输出

### CD 部署失败

1. 检查 SSH 连接：`ssh -p $DEPLOY_PORT $DEPLOY_USER@$DEPLOY_HOST`
2. 检查 GHCR 登录：确认 `GITHUB_TOKEN` 有 `write:packages` 权限
3. 检查目标服务器 Docker 环境
4. 查看 GitHub Actions 日志中的部署脚本输出

### Release 失败

1. 确认 tag 格式正确（`v*`）
2. 确认 CI 已通过
3. 检查 Docker 构建是否成功
