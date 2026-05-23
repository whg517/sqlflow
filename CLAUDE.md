# CLAUDE.md — Claude Code 项目指令

> Claude Code 在此项目目录下启动时自动加载此文件。严格遵守所有规则。

## 项目概览

- **项目名**: SQLFlow — SQL 审批管理平台
- **模块路径**: `github.com/whg517/sqlflow`
- **Go 版本**: 1.25（需要 `/usr/local/go/bin` 在 PATH 中）
- **技术栈**: Go (Echo) + SQLite WAL（手写 model + db.go） | React 19 + Vite + TypeScript + TailwindCSS + Shadcn/ui
- **规格管理**: OpenSpec（`openspec/specs/` 为真相来源）
- **设计文档**: `docs/spec/PRD.md`、`docs/spec/ARCHITECTURE.md`、`docs/spec/UI-DESIGN.md`
- **AI Provider**: 支持 OpenAI / 智谱 GLM / 自定义 OpenAI 兼容 API（配置 `config.yaml` 的 `ai` 段）

## 🔴 强制规则（最高优先级）

### 1. 完成后必须自验证

**每个任务完成后，必须自己运行以下命令并确认全部通过：**

```bash
# Go 环境
export PATH=$PATH:/usr/local/go/bin

# 编译检查
go build ./...

# 运行所有测试
go test ./...

# 前端编译（如果修改了前端代码）
cd web && npm run build && cd ..
```

**只有以上命令全部成功退出（exit code 0）后，才能声称任务完成。**

### 2. 不要破坏现有代码

- 修改函数签名前，先搜索所有调用点（`grep -rn`）确保兼容
- 不要删除被其他文件 import 的函数/类型，除非确认无调用
- 不要修改 `internal/model/*.go` 中的现有字段（除非任务明确要求）
- 不要修改 `internal/api/router.go` 的路由结构，除非任务明确要求

### 3. Go 代码规范

- 使用 `make()` 而非 `var` 初始化 slice/map（避免 JSON null）
- 错误处理：不要用 `panic`，用 `return fmt.Errorf` 或 `errors.New`
- 包级错误用 `var ErrXxx = errors.New("...")` 定义
- 公开函数必须有注释（`// FunctionName does ...`）
- 使用 `log.Printf` 而非 `fmt.Println` 记录日志

### 4. 测试规范

- 每个新功能必须有对应的 `_test.go` 文件
- 使用 `t.Run()` 组织子测试
- 边界情况必须覆盖：空输入、nil、非法参数
- 使用表驱动测试（table-driven tests）

### 5. 前端规范

- 组件使用 TypeScript，props 必须有类型定义
- 使用 Shadcn/ui 组件，不要自行实现已有的 UI 基础组件
- API 请求通过 `src/api/client.ts` 发起，不要直接用 fetch
- 使用 Design Token（Tailwind CSS 变量），不要硬编码颜色值
- 参照 `docs/spec/UI-DESIGN.md` 中的 wireframe 和交互设计

## 📋 任务执行流程

收到任务后，按以下顺序执行：

1. **读 Spec** — 先读 `openspec/specs/` 中相关的规格文档理解需求
2. **读设计文档** — 再读 `docs/spec/ARCHITECTURE.md` 和 `docs/spec/UI-DESIGN.md` 了解架构和 UI 设计
3. **读现有代码** — 理解现有实现，避免重复或冲突
4. **编写代码** — 按任务要求实现
5. **自验证** — `go build ./...` + `go test ./...`（强制）
6. **报告完成** — 简述做了什么、改了哪些文件、验证结果

## 🔍 常见错误预防

| 错误 | 预防 |
|------|------|
| `nil slice` 序列化为 JSON `null` | 使用 `make([]T, 0)` |
| `var` 声明 map 未初始化导致 panic | 使用 `make(map[K]V)` |
| pingcap/parser 接口方法缺失 | 完整实现 ast.Node 接口（Accept + Restore + Format） |
| Ent schema 字段名冲突 | 检查现有 schema，避免重复字段 |
| 前端 401 跳转死循环 | 检查 `src/api/client.ts` 拦截器逻辑 |

## 📁 项目结构

```
sql-platform/
├── openspec/                    # OpenSpec 规格管理
│   ├── config.yaml              # OpenSpec 配置
│   ├── specs/                   # 当前生效的规格（真相来源）
│   │   ├── datasource-management/
│   │   ├── online-query/
│   │   ├── ai-review/
│   │   ├── data-masking/
│   │   ├── ticket-workflow/
│   │   ├── user-auth/
│   │   ├── rbac-permissions/
│   │   ├── audit-logging/
│   │   └── dingtalk-notification/
│   └── changes/                 # 活跃的变更提案
├── cmd/server/main.go
├── config/config.go
├── internal/
│   ├── api/{handler,middleware,router}.go
│   ├── connpool/                # MySQL + MongoDB 连接池
│   ├── db/db.go                 # Ent 客户端
│   ├── ent/schema/              # 数据模型
│   ├── model/model.go
│   ├── pkg/{sqlparser,casbin,crypto,mask}/
│   ├── service/                 # 业务逻辑
│   └── resp/response.go
├── web/src/
│   ├── api/client.ts
│   ├── components/{ui,Layout}/
│   └── pages/
└── docs/spec/                   # 设计参考文档（PRD、架构、UI）
```
