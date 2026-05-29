# SF-DOC0009 API 文档自动生成（swag + Swagger UI）

> 状态：已通过
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-28
> 需求编号：recvkUEjwiowSt

## 需求概述

使用 swag 工具基于 Go 注释自动生成 Swagger/OpenAPI 文档，实现文档与代码同步更新。

## 评审结论

- **架构评审（方远）**：✅ 通过（实际工时 3-4h，DOC 需求无需 UI/UX 评审）

## 技术方案

### 现有功能

| 功能 | 说明 |
|------|------|
| Swagger UI 路由 | `GET /swagger/*`（echoSwagger.WrapHandler） |
| 依赖 | `github.com/swaggo/echo-swagger` 已引入 |
| 现状 | 手动维护文档，与代码不同步 |

### 实施方案

#### swag 注释规范

```go
// CreateUser godoc
// @Summary      创建用户
// @Description  创建新用户账号
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Param        request body CreateUserRequest true "用户信息"
// @Success      201 {object} UserResponse
// @Failure      400 {object} ErrorResponse
// @Failure      409 {object} ErrorResponse "用户名已存在"
// @Security     BearerAuth
// @Router       /api/users [post]
func (h *UserHandler) CreateUser(c echo.Context) error {
```

#### 代码改动

| 改动 | 说明 |
|------|------|
| API handler 注释 | 按规范添加 swag 注释到所有 handler |
| main.go | 添加 `@host`、`@BasePath`、`@SecurityDefinitions` 注释 |
| Makefile | 新增 `make docs` 目标（`swag init -g cmd/server/main.go`） |
| CI 集成 | build 阶段执行 `swag init` 生成文档 |

#### API 端点覆盖优先级

| 优先级 | 模块 | 端点数 | 说明 |
|--------|------|--------|------|
| P0 | 认证 | 3 | login, refresh, me |
| P0 | 数据源 | 5 | CRUD + test |
| P0 | 查询 | 3 | execute, explain, cancel |
| P1 | 工单 | 4 | CRUD + approve |
| P1 | 用户 | 3 | CRUD |
| P2 | 其他 | ~10 | settings, audit, export, dashboard 等 |

### 工时估算

| 任务 | 工时 |
|------|------|
| swag 注释覆盖 P0 端点（11 个） | 2h |
| swag 注释覆盖 P1 端点（7 个） | 1h |
| Makefile docs + CI 集成 | 0.5h |
| 验证 Swagger UI 渲染 | 0.5h |
| **合计** | **4h**（方远评估 3-4h） |

## 验收标准

1. `make docs` 成功生成 `docs/swagger.json` 和 `docs/swagger.yaml`
2. 访问 `/swagger/index.html` 可查看文档 UI
3. P0 + P1 端点（18 个）全部包含 swag 注释
4. 文档描述与实际 API 行为一致
5. CI 中自动生成文档

## Code Review 记录

| 日期 | 审查人 | 结论 | 备注 |
|------|--------|------|------|
| — | — | — | 待开发完成后填写 |

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-06-13 | 初版创建 |
