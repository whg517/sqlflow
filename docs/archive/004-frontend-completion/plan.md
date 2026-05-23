# Plan: 004-frontend-completion — 前端功能补全

> 状态: 已完成
> 更新日期: 2026-05-08

## Task 拆分

### Phase 1: Admin 用户管理页面（0.5 工作日）

#### Task 1.1 用户列表页

- [x] 创建 `web/src/api/user.ts`（API 封装）
- [x] 创建 `web/src/pages/Users/index.tsx`（用户列表+搜索+新建/编辑/重置密码/禁用弹窗）
- [x] Layout 侧边栏添加「用户管理」入口（仅 admin 角色可见）
- [x] App.tsx 注册路由 `/users`

**验收**：tsc ✅ npm build ✅

### Phase 2: Dashboard 概览页（0.5 工作日）

#### Task 2.1 统计卡片 + 待办

- [x] 新建 `internal/service/dashboard.go` + `internal/api/handler/dashboard.go`
- [x] 新建测试 `dashboard_test.go`（service + handler）
- [x] router.go 注册 `GET /api/dashboard/stats`
- [x] 创建 `web/src/api/dashboard.ts` + `web/src/pages/Dashboard/index.tsx`
- [x] App.tsx 注册路由 `/` 为默认首页
- [x] Layout 侧边栏添加「概览」入口

**验收**：go build ✅ go test ✅ tsc ✅ npm build ✅

### Phase 3: 查询→工单联动（0.5 工作日）

#### Task 3.1 AI 评审高风险工单引导

- [x] 已有实现：AIReviewCard `decision=ticket` 时显示「提交工单」按钮
- [x] 已有实现：TicketSubmitSheet 侧边栏，自动预填充 SQL/数据源/AI评审结果
- 无需额外开发

**验收**：代码审查 ✅

## 实际结果

| Task | 计划工时 | 状态 |
|------|---------|------|
| 1.1  | 0.5d    | ✅ 已完成 |
| 2.1  | 0.5d    | ✅ 已完成 |
| 3.1  | 0.5d    | ✅ 已实现（之前阶段） |
