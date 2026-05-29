# SF-FEAT0034 API Token 外部集成管理

> 状态：待评审
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-27
> 需求编号：recvkMoz158WTe

## 需求概述

为外部系统（监控、BI 工具、自动化脚本）提供 API Token 认证能力，支持创建/吊销/权限控制。

## 评审结论

待评审。

## 技术方案

### Token 设计

- 格式：`sfp_` 前缀 + 32 字节随机 hex（类似 GitHub PAT）
- 存储：SHA-256 hash 存储，不存明文
- 创建时一次性显示完整 Token（之后只显示前4后4位）

### 数据库变更

新增 `api_tokens` 表：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| name | TEXT NOT NULL | Token 名称 |
| token_hash | TEXT NOT NULL UNIQUE | SHA-256 hash |
| token_prefix | TEXT NOT NULL | 前8位（用于展示识别） |
| user_id | INTEGER NOT NULL | 创建者 |
| role | TEXT NOT NULL | 继承的角色（admin/dba/developer） |
| datasource_ids | TEXT | 限制的数据源 ID 列表（JSON 数组，空=全部） |
| expires_at | DATETIME | 过期时间 |
| last_used_at | DATETIME | 最后使用时间 |
| status | TEXT NOT NULL | active/revoked |
| created_at | DATETIME | 创建时间 |

### 认证流程

1. 请求 Header 携带 `Authorization: Bearer sfp_xxxxx`
2. Auth 中间件识别 `sfp_` 前缀 → 走 Token 认证路径
3. 计算 SHA-256 hash → 查 api_tokens 表匹配
4. 检查过期时间 + status
5. 构造上下文用户信息（标记来源为 API Token）

### 接口设计

| 接口 | 说明 |
|------|------|
| POST /api/tokens | 创建 Token（返回明文，仅一次） |
| GET /api/tokens | 列表（当前用户创建的） |
| DELETE /api/tokens/:id | 吊销 Token |
| GET /api/tokens/:id | 获取详情 |

### 审计

- Token 认证成功的请求写入审计日志，标记 token_id
- Token 创建/吊销操作写入审计日志

## 验收标准

1. Token CRUD + 吊销
2. Bearer Token 认证可用，查询/导出接口正常
3. Token 权限隔离正常（继承角色 + 可限制数据源）
4. 过期 Token 自动失效
5. Token 使用记录入审计
6. 审计日志可区分 JWT 和 Token 来源

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
