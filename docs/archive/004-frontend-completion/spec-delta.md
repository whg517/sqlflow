# Spec 变更预览

> 提案: 004-frontend-completion
> 状态: 待合并

## spec/PRD.md

### 新增

#### 用户管理前端
- Admin 用户可在 Web UI 上管理用户（创建、编辑、禁用、重置密码），无需命令行操作

#### Dashboard 概览页
- 新增 Dashboard 首页，展示系统全局统计（待审批工单、查询次数、活跃数据源、用户数）和待办事项快捷入口
- 用户登录后默认进入 Dashboard 而非直接进入查询页

#### 查询→工单联动
- AI 评审判定高风险（DecisionTicket）时，查询页展示「创建工单」按钮
- 点击后自动跳转工单创建页并预填充 SQL、数据源、数据库信息

### 修改

#### §6 用户管理（MVP）
- 补充：Web UI 支持用户 CRUD（原仅描述后端 API）

#### §3 AI 前置评审
- 补充：高风险评审结果展示工单创建引导入口

## spec/ARCHITECTURE.md

### 新增

#### Dashboard API
- `GET /api/dashboard/stats` — 返回聚合统计数据
  ```json
  {
    "pending_tickets": 3,
    "recent_queries_7d": 128,
    "active_datasources": 5,
    "total_users": 12
  }
  ```

## spec/UI-DESIGN.md

### 新增

#### 用户管理页 Wireframe
- 列表布局：搜索框 + 新建按钮 + 用户表格（用户名、角色、创建时间、操作）
- 新建/编辑弹窗：用户名（必填）、密码（新建必填）、角色下拉选择

#### Dashboard 页 Wireframe
- 顶部：4 个统计卡片（待审批工单、近 7 天查询、活跃数据源、用户数）
- 中部：待办事项列表（最近 5 条待审批工单）+ 最近查询历史
- 卡片支持点击跳转到对应页面

#### 查询页工单引导
- AIReviewCard 底部新增「创建工单」按钮（仅高风险时显示）
- 按钮样式：Primary，与现有按钮一致
