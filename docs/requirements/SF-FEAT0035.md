# SF-FEAT0035 SQL 变更 Git 集成

> 状态：待评审
> 优先级：P3🔵低优
> 负责人：待分配
> 创建日期：2026-05-27
> 需求编号：recvkMoz0L63c3

## 需求概述

SQL 工单与 Git 变更关联，实现数据库变更可追溯，支持手动关联和 Webhook 自动关联。

## 评审结论

待评审。

## 技术方案

### 数据库变更

新增 `git_integrations` 表：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| ticket_id | INTEGER NOT NULL | 关联工单 |
| commit_hash | TEXT | Git commit hash |
| pr_url | TEXT | PR/MR 链接 |
| repo_url | TEXT | 仓库地址 |
| commit_message | TEXT | commit message |
| author | TEXT | 提交者 |
| created_at | DATETIME | 关联时间 |

### 接口设计

| 接口 | 说明 |
|------|------|
| POST /api/tickets/:id/git-links | 手动关联 commit/PR |
| GET /api/tickets/:id/git-links | 获取关联的 Git 信息 |
| DELETE /api/tickets/:id/git-links/:linkId | 删除关联 |
| POST /api/webhooks/github | GitHub Webhook 接收（push 事件） |

### Webhook 自动关联（可选）

- GitHub/GitLab push event Webhook
- 解析 commit message 中的工单编号（如 `refs SF-FEAT0031`）
- 自动创建 git_integrations 记录

### 前端

- 工单详情页新增 "Git 关联" 区域
- 手动关联弹窗：输入 commit hash / PR 链接
- 变更时间线视图

## 验收标准

1. 工单可手动关联 commit hash / PR 链接
2. 工单详情展示关联的 Git 信息
3. 关联记录可删除
4. （可选）Webhook 自动关联 push 事件

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
