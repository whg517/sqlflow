# SF-FEAT0031 PostgreSQL 数据源支持

> 状态：开发中
> 优先级：P1🟠重要
> 负责人：陈岩
> 创建日期：2026-05-27
> 需求编号：recvkMoBOs2GAM

## 需求概述

SQLFlow 新增 PostgreSQL 数据源支持，包括数据源注册、连接测试、SQL 查询执行、库表列查询、工单审批全流程。

## 评审结论

待评审。

## 技术方案

### 架构设计

- **Go 驱动**：pgx/v5（stdlib 模式，通过 `database/sql` 接口使用）
- **连接池**：复用现有 `connpool.Manager`，将 `mysqlPools` 重命名为 `sqlPools`（MySQL + PostgreSQL 共用 `*sql.DB` 缓存）
- **连接池 Key**：MySQL `mysql:{dsID}:{host}:{port}:{db}`，PostgreSQL `pg:{dsID}:{host}:{port}:{db}`，互不冲突

### 数据库变更

- `datasources` 表新增 `sslmode TEXT DEFAULT ''`（SSL 模式：disable/prefer/require/verify-ca/verify-full）
- `datasources` 表新增 `schema_name TEXT DEFAULT ''`（PostgreSQL schema，默认 public）

### 接口设计

| 接口 | 变更 |
|------|------|
| POST /api/datasources | 新增 sslmode、schema_name 字段 |
| PUT /api/datasources/:id | 新增 sslmode、schema_name 字段 |
| GET /api/datasources | 响应新增 sslmode、schema_name |
| POST /api/query/execute | 支持 postgresql 类型数据源查询 |
| GET /api/datasources/:id/tables | 支持 PostgreSQL pg_tables 查询 |
| GET /api/datasources/:id/tables/:name/columns | 支持 PostgreSQL information_schema 查询 |

### 安全约束

- **[MUST]** PostgreSQL 查询必须复用现有 AST 类型检查，仅允许 SELECT 直接执行，DDL/DML 走工单审批
- DSN 中的 password 需做 URL 编码防止特殊字符导致连接失败

### PG 数据类型映射

| PostgreSQL 类型 | 映射为 |
|----------------|--------|
| smallint/int/integer/int2/int4/int8/bigint | integer |
| decimal/numeric/real/double precision/float4/float8 | number |
| character varying/character/text/char/varchar/bpchar/name | string |
| boolean/bool | boolean |
| date | date |
| timestamp/timestamptz | timestamp |
| uuid | uuid |
| json/jsonb | json |
| bytea | binary |
| ARRAY | array |

## 设计规范

- 前端数据源创建/编辑表单：新增 PostgreSQL 类型选项、SSL 模式下拉、Schema 名称输入框
- 非 PostgreSQL 类型时隐藏 SSL/Schema 字段

## 验收标准

1. PostgreSQL 数据源 CRUD + 连接测试
2. SELECT 查询执行 + 结果分页展示
3. 库表列表 + 列信息查询（含 PG 类型映射）
4. 工单提交/审批/执行流程完整
5. 连接池正常复用，更新/禁用时正确清理
6. SQL 类型检查：非 SELECT 拦截走工单

## 实现记录

_待上线后补充_

## Code Review 记录

| 日期 | 审查人 | 结论 | 备注 |
|------|--------|------|------|
| 2026-05-27 | Marcus | ⛔ MUST 修复 | executePostgreSQL 缺少 SQL 类型检查，DDL/DML 可绕过工单 |

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
