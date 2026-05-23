# AGENTS.md — SQLFlow 项目协作规范

> 此文件面向所有参与本项目的 AI Agent（小梦旋、Claude Code 等），定义统一的协作方式。

## 角色分工

| 角色 | 职责 | 工具 |
|------|------|------|
| 化刚 | PM + QA：需求决策、优先级、最终验收 | 飞书/代码审查 |
| 小梦旋 | PM 助理：拆任务、派发、review、汇报 | OpenClaw |
| Claude Code | 开发执行：编码、测试、提交 | `claude -p` |

## 规格管理（OpenSpec）

本项目使用 [OpenSpec](https://github.com/Fission-AI/OpenSpec) 进行规格驱动开发（SDD）。

### 目录结构

```
openspec/
├── config.yaml              # OpenSpec 配置（技术栈、规则）
├── specs/                   # 当前生效的规格（真相来源）
│   ├── datasource-management/
│   ├── online-query/
│   ├── ai-review/
│   ├── data-masking/
│   ├── ticket-workflow/
│   ├── user-auth/
│   ├── rbac-permissions/
│   ├── audit-logging/
│   └── dingtalk-notification/
└── changes/                 # 活跃的变更提案
```

### 工作流

```
/opsx:propose "description"  → 创建变更提案（自动生成 proposal + design + specs + tasks）
/opsx:apply                  → 开始实现（Claude Code 按 tasks 执行）
/opsx:archive                → 归档已完成的变更，更新 specs
```

### Spec 是真相来源

- 所有开发必须参照 `openspec/specs/` 中的规格
- 变更提案在 `openspec/changes/` 中，批准后合并到 specs
- Claude Code 每次任务必须引用对应的 spec 文件

## 任务派发与执行

### 小梦旋派发任务时

1. 从 OpenSpec change 的 `tasks.md` 中提取下一个待执行的 Task
2. 更新 `task-progress.json` 状态为 `running`
3. 通过 `claude -p` 后台执行，prompt 包含：
   - 任务编号和名称
   - 对应的 `openspec/specs/` 文档引用
   - 验收标准
   - `CLAUDE.md` 中的强制规则提醒
4. 等 exec 完成通知

### Claude Code 收到任务时

1. 先读 `CLAUDE.md` 了解项目规范
2. 再读相关 `openspec/specs/` 了解需求
3. 按规范执行开发
4. **必须自验证**（go build + go test）
5. 报告完成

### 小梦旋验收时

1. `go build ./...` 编译检查
2. `go test ./...` 测试检查
3. `cd web && npm run build` 前端检查
4. `git diff --stat` 查看变更范围
5. 对照 spec 验证功能完整性
6. 验收通过 → 更新状态 + 汇报化刚
7. 验收失败 → 记录问题 + 重新派发

## 验收标准模板

每个 Task 完成后，小梦旋 检查以下清单：

- [ ] `export PATH=$PATH:/usr/local/go/bin && go build ./...` — 无错误
- [ ] `export PATH=$PATH:/usr/local/go/bin && go test ./...` — 全部 PASS
- [ ] `cd web && npm run build` — 无错误（如有前端改动）
- [ ] 符合 `openspec/specs/` 中对应模块的规格要求
- [ ] 新代码有对应的测试文件

## 沟通规则

- **Task 完成后立即汇报化刚**：做了什么、成本、是否有问题
- **失败时说明原因**：不要只说"失败了"，要说清错误信息和已尝试的修复
- **简洁格式**：用模板，不写长文

## 文件变更约定

- 每个 Change 完成后提交一个 commit，格式：`feat(change-name): 简要描述`
- 不要在 commit 中混入不相关修改
- 二进制文件 `server` 不要提交到 git（已在 .gitignore 中）
