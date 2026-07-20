# ADR-0002：使用 SQLite 保存平台元数据并保留显式 SQL Migration

- 状态：accepted
- 日期：2026-07-14（补录）
- 决策者：SQLFlow Team
- 关联需求：FR-OPS-004、NFR-REL-001、NFR-SCL-001
- 取代：无
- 被取代：无

## 背景

平台元数据需要事务、索引、全文检索、备份和低运维成本。SQLFlow 当前以单实例自托管为主，不希望要求用户先部署额外元数据库。项目正在引入 Ent，但 SQLite 的 ALTER TABLE 限制使自动迁移可能重建表并改变 SQL 默认表达式。

## 决策

使用 SQLite 保存平台元数据，启用 WAL、外键和 busy timeout，并限制单连接以减少写锁冲突。DDL 继续由嵌入二进制的 `golang-migrate` SQL 文件管理；Ent Client 和原始 SQL 在迁移阶段共享同一连接，但暂不运行 Ent 自动迁移。

## 备选方案

- PostgreSQL 作为平台库：并发和扩展性更好，但增加部署和升级成本。
- Ent 自动迁移：开发便利，但当前存在 SQLite 表重建和默认表达式漂移风险。
- 仅使用原始 SQL：减少双轨复杂度，但失去逐步获得类型安全查询的机会。

## 后果

- 优点：零外部元数据库依赖，文件级备份简单，适合单实例部署。
- 代价：单写者限制吞吐；多副本共享文件不安全；SQL/Ent 双轨期间有模型漂移风险。
- 约束：结构变更必须提供显式 up/down migration，并同步 Ent Schema、原始 SQL 和测试 fixture。

## 验证

- `internal/db/db.go` 配置 SQLite 并只通过 `MigrateDB` 执行 DDL。
- `internal/db/migrations/` 保存顺序化 migration。
- 备份服务操作的是平台 SQLite，而不是目标数据源。

