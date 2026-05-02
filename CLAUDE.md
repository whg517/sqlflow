# CLAUDE.md — Claude Code 项目指令

> Claude Code 在此项目目录下启动时自动加载此文件。严格遵守所有规则。

## 项目概览

- **项目名**: SQLFlow — SQL 审批管理平台
- **模块路径**: `github.com/whg517/sqlflow`
- **Go 版本**: 1.25（需要 `/usr/local/go/bin` 在 PATH 中）
- **技术栈**: Go (Echo) + Ent ORM + SQLite WAL | React 19 + Vite + TypeScript + TailwindCSS + Shadcn/ui
- **Spec 文档**: `docs/spec/PRD.md`、`docs/spec/ARCHITECTURE.md`、`docs/spec/UI-DESIGN.md`
- **开发计划**: `docs/proposals/001-mvp-initial/plan.md`

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

如果编译或测试失败：
1. 分析错误原因
2. 修复代码
3. 重新运行验证
4. 重复直到全部通过

**不要在编译/测试未通过的情况下报告完成。**

### 2. 不要破坏现有代码

- 修改函数签名前，先搜索所有调用点（`grep -rn`）确保兼容
- 不要删除被其他文件 import 的函数/类型，除非确认无调用
- 不要修改 `internal/ent/schema/*.go` 中的现有字段（除非任务明确要求）
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
- 不要用 `t.Skip()` 除非确实无法实现（需注释说明原因）

### 5. 前端规范

- 组件使用 TypeScript，props 必须有类型定义
- 使用 Shadcn/ui 组件，不要自行实现已有的 UI 基础组件
- API 请求通过 `src/api/client.ts` 发起，不要直接用 fetch
- 使用 Design Token（Tailwind CSS 变量），不要硬编码颜色值
- 参照 `docs/spec/UI-DESIGN.md` 中的 wireframe 和交互设计

## 📋 任务执行流程

收到任务后，按以下顺序执行：

1. **读 Spec** — 先读相关 spec 文档理解需求上下文
2. **读现有代码** — 理解现有实现，避免重复或冲突
3. **编写代码** — 按任务要求实现
4. **自验证** — `go build ./...` + `go test ./...`（强制）
5. **报告完成** — 简述做了什么、改了哪些文件、验证结果

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
├── cmd/server/main.go          # 启动入口
├── config/config.go            # 配置管理
├── internal/
│   ├── api/
│   │   ├── handler/            # HTTP handlers
│   │   ├── middleware/         # 中间件 (auth, cors, logger, recovery)
│   │   └── router.go           # 路由注册
│   ├── connpool/               # 数据库连接池 (MySQL + MongoDB)
│   ├── db/db.go                # Ent 客户端初始化
│   ├── ent/schema/             # Ent 数据模型定义
│   ├── model/model.go          # 请求/响应模型
│   ├── pkg/
│   │   ├── sqlparser/          # SQL 解析器
│   │   ├── casbin/             # Casbin 权限
│   │   ├── crypto/             # 加密工具
│   │   └── mask/               # 数据脱敏
│   ├── service/                # 业务逻辑层
│   └── resp/response.go        # 统一响应格式
├── web/src/
│   ├── api/client.ts           # API 请求层
│   ├── components/             # 通用组件
│   ├── components/ui/          # Shadcn/ui 组件
│   ├── pages/                  # 页面组件
│   └── lib/utils.ts            # 工具函数
├── docs/spec/                  # 需求规格文档
└── docs/proposals/             # 开发提案
```
