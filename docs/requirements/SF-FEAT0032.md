# SF-FEAT0032 敏感表临时权限申请

> 状态：待评审
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-27
> 需求编号：recvkMoz1ifMVM

## 需求概述

开发者可申请临时访问敏感表的查询权限，走审批流程，到期自动回收。

## 评审结论

待评审。

## 技术方案

### 数据库变更

新增 `access_requests` 表：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| user_id | INTEGER NOT NULL | 申请人 |
| datasource_id | INTEGER NOT NULL | 数据源 |
| table_name | TEXT NOT NULL | 申请访问的表 |
| reason | TEXT NOT NULL | 申请理由 |
| requested_hours | INTEGER NOT NULL | 申请时长（小时） |
| status | TEXT NOT NULL | pending/approved/rejected/expired |
| approver_id | INTEGER | 审批人 |
| approved_hours | INTEGER | 批准时长（可修改） |
| started_at | DATETIME | 权限生效时间 |
| expires_at | DATETIME | 权限过期时间 |
| created_at | DATETIME | 申请时间 |
| updated_at | DATETIME | 更新时间 |

### Casbin 策略

- 审批通过后，自动添加临时 Casbin policy：`sub, datasourceID:tableName, select, allow`
- 策略携带过期时间元数据
- Scheduler 定时清理过期策略（每 10 分钟检查）
- 清理时同步更新 access_requests 状态为 expired

### 接口设计

| 接口 | 说明 |
|------|------|
| POST /api/access-requests | 提交权限申请 |
| GET /api/access-requests | 列表（Admin 查看全部，用户查看自己的） |
| PUT /api/access-requests/:id/approve | 审批通过 |
| PUT /api/access-requests/:id/reject | 审批拒绝 |
| GET /api/access-requests/my | 我的申请列表 |

### 前端

- 查询页面：敏感表查询被拒时弹出"申请权限"按钮
- 新增权限申请页（列表 + 申请弹窗）
- Admin 审批页

## 验收标准

1. 敏感表查询被拒时提示可申请权限
2. 申请-审批-生效-过期-自动回收全流程
3. 过期策略自动清理（Scheduler）
4. 申请记录可查，权限状态实时显示

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
