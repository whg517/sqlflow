# AGENTS.md — SQLFlow 项目协作规范

> 本文件面向所有参与 SQLFlow 开发的 Agent，定义统一的协作方式和工程纪律。
> 所属团队：**棱镜 Prism**

## 团队与角色

| 角色 | Agent | 职责 |
|------|-------|------|
| 老板 | 化刚 | 需求决策、优先级、最终验收 |
| Tech Lead | Marcus | 需求评估录入、Code Review、合并、工程体系 |
| PM | 钱进 | 需求流转、任务派发、进度追踪、巡检 |
| 前端 | 林夏 | React / TypeScript / Ant Design 开发 |
| 后端 | 陈岩 | Go / PostgreSQL / Redis 开发 |
| 全栈 | 周然 | Go + React 全栈、快速原型、工具脚本 |
| 运维 | 杨帆 | Docker / CI-CD / 基础设施 |
| UI/UX | 苏晴 | 设计规范、视觉走查 |
| QA | 叶青 | 测试策略、验收 |

## 需求管理

需求池通过 **reqmgr**（飞书多维表格）管理，编号规则 `SF-{类型}{序号}`。

- 化刚提需求 → Marcus 录入（待评审）→ 钱进流转 → 评审通过 → 开发 → 测试 → 上线
- 需求与 Git 分支一一对应：`SF-FEAT0019` ↔ `feat/SF-FEAT0019-page-fix`

## Git Worktree 流程（强制）

**所有开发必须在 worktree 中进行，禁止直接修改 main 分支。**

### 规则

- **Worktree 目录**：`.worktree/<branch-name>/`
- **分支命名**：`feat/<需求ID>-<描述>` / `fix/<需求ID>-<描述>`
- 开发 Agent 自主创建 worktree，完成后 commit + push 到远程分支
- Marcus 负责 Code Review → rebase main + squash merge → 清理 worktree

### 禁止行为

- ❌ 直接在 main 分支修改代码
- ❌ push 到 main 分支（必须走 PR + Review）
- ❌ 未创建 worktree 就开始开发

## Code Review 标准

Marcus 审查所有 PR，意见分级：

- `[MUST]` — 必须修改，阻塞合并
- `[NICE]` — 建议修改，不阻塞
- `[VISUAL]` — 视觉还原问题，阻塞合并（前端 PR）

审查维度：代码质量、架构合规、测试覆盖、安全、视觉对标（前端）

## 合并策略

- rebase main → squash merge
- 冲突由 Marcus 解决
- 合并后必须验证：`tsc --noEmit` + `npm run build` + `npm run test`（前端）
- 合并后执行 `./scripts/merge-cleanup.sh <branch>` 清理 worktree + 分支

## 验证命令

```bash
# 前端
cd web && npx tsc --noEmit      # 类型检查
cd web && npm run build          # 构建
cd web && npm run test           # 单元测试（508 tests）

# 后端
go build ./...                    # 编译
go test ./...                     # 测试

# 全量验证（前端+后端）
cd web && npx tsc --noEmit && npm run build && npm run test && cd .. && go build ./...
```

## 项目结构

```
sql-platform/
├── cmd/              # Go 入口
├── internal/         # Go 业务代码
├── config/           # 配置模块
├── web/              # React 前端
├── e2e/              # E2E 测试（Playwright，独立隔离环境）
├── docs/             # 项目文档
│   ├── api.md        # API 文档
│   ├── cicd-guide.md # CI/CD 配置
│   ├── deployment.md # 部署文档
│   └── spec/         # 设计规格
│       ├── PRD-v2.md
│       ├── ARCHITECTURE-v2.md
│       ├── UI-DESIGN-v2.md
│       └── DESIGN-TOKENS.md
├── scripts/          # 工具脚本
├── tests/            # Go 测试
├── Dockerfile        # 多阶段构建
├── docker-compose.yaml
└── Makefile          # 开发命令
```

## Commit 规范

格式：`{type}(scope): 简要描述`

- `feat(web): SF-FEAT0019 page-level style fixes`
- `fix(api): SF-FIX0001 JWT expiry handling`
- `refactor(web): SF-QA0023 consolidate e2e tests`

每个需求对应独立 commit，不要混入不相关修改。
