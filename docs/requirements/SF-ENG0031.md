# SF-ENG0031 数据库版本化迁移机制（golang-migrate）

> 状态：已通过
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-28
> 需求编号：recvkUDPqyWQq5

## 需求概述

引入 golang-migrate 替代当前手工迁移机制，实现数据库 schema 版本化管理、有序迁移、回滚能力。

## 评审结论

- **技术方案评审（方远）**：✅ 通过 — golang-migrate 适配性好，方案可行，6h 工时合理

## 技术方案

### 现状分析（代码审计）

| 问题 | 现状 | 影响 |
|------|------|------|
| 迁移代码集中 | 643 行 `db.go`，18 张表 + 14 个 ALTER TABLE 在一个 `Migrate()` 方法 | 维护困难 |
| 无版本追踪 | `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE` 忽略错误 | 无法确认当前 schema 版本 |
| 错误静默 | `_, _ = db.Exec(...)` 忽略 ALTER TABLE 错误 | 迁移失败无感知 |
| 无回滚 | 无版本记录，无回滚机制 | 错误迁移不可逆 |
| 并发风险 | 多实例部署时同时执行迁移 | 数据竞争 |

### 方案选型

**golang-migrate**（已确认）

| 维度 | 说明 |
|------|------|
| 数据库支持 | SQLite + PostgreSQL（满足 SF-ENG0032 切换需求） |
| Go 生态 | 成熟度高，GitHub 14k+ stars |
| CLI 支持 | `migrate` 命令行工具，便于手动操作 |
| 版本管理 | `schema_migrations` 表记录当前版本 |
| 迁移文件 | Up/Down 成对 SQL 文件，支持回滚 |

### 实施方案

#### 1. 迁移文件结构

```
internal/database/migrations/
├── 000001_init_schema.up.sql      # 初始 schema（18 张表）
├── 000001_init_schema.down.sql    # 回滚：DROP TABLE
├── 000002_add_audit_columns.up.sql # 增量迁移示例
└── 000002_add_audit_columns.down.sql
```

#### 2. 代码改动范围

| 文件 | 改动 |
|------|------|
| `internal/database/db.go` | 移除 `Migrate()` 方法中的所有 DDL，替换为 golang-migrate 调用 |
| `go.mod` | 添加 `github.com/golang-migrate/migrate/v4` 依赖 |
| `main.go` | 启动时调用新版迁移逻辑（可选：增加 CLI flag 控制迁移行为） |
| 新增迁移文件 | 将现有 18 张表的 CREATE TABLE + 14 个 ALTER TABLE 拆分为有序迁移文件 |

#### 3. 迁移执行策略

```go
// 启动时自动迁移
m, err := migrate.New("file://internal/database/migrations", dsn)
if err != nil { log.Fatal(err) }
if err := m.Up(); err != nil && err != migrate.ErrNoChange { log.Fatal(err) }

// CLI 支持（可选）
// -migrate up/down/version/force
```

#### 4. 与 SF-ENG0030（ent ORM）的关系

- SF-ENG0030（ent ORM）自带 Atlas 迁移引擎，功能与 golang-migrate 重叠
- **建议执行顺序**：先做 SF-ENG0031（引入版本化迁移），再做 SF-ENG0030（ent 重构）
  - 原因：golang-migrate 提供基础版本管理，ent 的 Atlas 迁移可以在此基础上演进
  - 如果先做 ent，Atlas 迁移会直接替代 golang-migrate，本需求变为无效工作
- **备选**：如果确定 SF-ENG0030 优先执行，则 SF-ENG0031 可以合并到 ent 重构中，由 Atlas 统一管理
- ⚠️ **需化刚确认执行顺序**

## 验收标准

1. `internal/database/migrations/` 下包含初始 schema 迁移文件（up/down 成对）
2. 应用启动时自动执行 pending migrations
3. `schema_migrations` 表记录当前版本号
4. `db.go` 中 `Migrate()` 方法的 DDL 代码已全部移除，替换为 golang-migrate 调用
5. 支持 SQLite 和 PostgreSQL 两种驱动
6. 手动执行 `migrate down` 可回滚到指定版本
7. 现有功能不受影响（regression 测试通过）

## 工时估算

| 任务 | 工时 |
|------|------|
| 依赖集成 + 迁移文件拆分 | 2h |
| 代码重构（移除旧迁移逻辑） | 2h |
| 多数据库驱动适配 | 1h |
| 测试验证 | 1h |
| **合计** | **6h** |

## Code Review 记录

| 日期 | 审查人 | 结论 | 备注 |
|------|--------|------|------|
| 2026-05-29 | Marcus | ✅ 通过 | commit 4f850b9，直接在 main 上完成（历史遗留，未走 PR 流程） |

## 实现记录

- **合并 commit**：`4f850b9`（2026-05-29 09:00）
- **实际工时**：约 4h
- **变更**：39 文件，+429/-593

### 实现方式
- db.go 从 609 行缩减到 ~46 行（移除全部内联 DDL，替换为 golang-migrate 调用）
- 18 个 migration 文件（up/down 成对），使用 `embed.FS` 嵌入二进制
- 初始 migration 使用 `CREATE TABLE IF NOT EXISTS` 兼容已有数据库
- 增量 migration（ALTER TABLE）保持幂等语义
- 新增 `MigrateDB()` 导出函数供测试代码复用
- `migrate.ErrNoChange` 正确忽略

### ⚠️ 流程偏离
- 此需求由 Marcus 直接在 main 分支开发，**未走 worktree + PR 流程**
- 这是历史遗留问题，后续需求必须严格遵守 Git Worktree 流程

### 与 SF-ENG0030 的关系
- ⚠️ 尚未化刚确认执行顺序
- 如果 SF-ENG0030（ent ORM）先执行，golang-migrate 可能被 Atlas 迁移替代
- 建议：如果确定做 ent ORM，golang-migrate 的 migration 文件仍可保留作为迁移历史

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-06-13 | 初版创建，评审结论写入 |
