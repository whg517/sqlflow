# ADR-0009：平台元数据改用 PostgreSQL

- 状态：accepted
- 日期：2026-08-01
- 决策者：SQLFlow Team
- 关联需求：NFR-SCL-001、NFR-REL-001、FR-OPS-004
- 取代：[ADR-0002](0002-sqlite-metadata-and-migrations.md) 中关于数据库选型的部分
- 被取代：无

## 背景

ADR-0002 选择 SQLite 保存平台元数据，理由是零外部依赖、文件级备份简单、适合单实例自托管。它同时把「用 SQLite」和「用显式 SQL migration 而非 Ent 自动迁移」写成了同一个决策，导致后来讨论换库时，看起来必须连迁移管理一起重新论证。这两件事是正交的：PostgreSQL 同样可以配显式 migration。本 ADR 只取代前者，迁移管理与数据访问方式由 [ADR-0010](0010-ent-as-the-single-data-access-path.md) 承接。

SQLite 的单写者模型是结构性的，不是配置问题。`internal/db` 因此把连接池限制为 `MaxOpenConns(1)`，其直接后果是：持有 `rows` 游标期间发起写操作会阻塞到 ctx 超时——这个陷阱曾真实导致调度器故障。同时多副本共享数据库文件不安全，使横向扩展在存储层就被堵死。

## 决策

平台元数据改用 PostgreSQL。`internal/db` 只支持 PostgreSQL，不保留 SQLite 兼容路径。

不保留双方言的理由是可验证性：代码里同时存在两种方言时，只有一种会被测试实际覆盖，另一种的正确性靠推断。与其维护一条没人跑的路径，不如让它不存在。

## 备选方案

- **保留 SQLite，仅在生产可选 PostgreSQL**：本地开发零依赖，但代码中需要方言抽象，且 CI 只会覆盖其中一种。前述可验证性问题即由此而来。
- **继续用 SQLite，通过 Litestream/LiteFS 做复制**：解决持久性，不解决单写者，也不解决多副本写入。
- **换用 MySQL**：并发模型足够，但全文检索与 JSON 支持弱于 PostgreSQL，而审计检索是本平台的核心能力之一。

## 后果

- **优点**：写并发不再受单连接限制；`MaxOpenConns(1)` 及其游标陷阱一并消失；为多副本部署解除存储层障碍。
- **代价**：本地开发与 CI 都需要一个 PostgreSQL 实例；测试不再能用临时文件做隔离，改为按 schema 隔离。
- **约束**：PostgreSQL 只解除了横向扩展的**存储层**障碍。工单调度器与 SLA 调度器仍在进程内运行且缺少租约，多副本会重复执行。在补上分布式租约之前，本平台**仍然只能单副本部署**——NFR-SCL-001 的修订必须如实反映这一点，不得因为换了数据库就声称支持水平扩展。

## 连带影响

换库废掉了两处写死 SQLite 的功能，两者都不是本 ADR 的目标，但必须一并处理，否则会静默损坏：

- **平台备份**（FR-OPS-004）：`internal/ops/backup.go` 的实现是 `PRAGMA wal_checkpoint(TRUNCATE)` 加文件 `io.Copy`，对 PostgreSQL 无意义。改为 `pg_dump`，需求文本中的「平台 SQLite 备份」同步修订。
- **内置数据源**：`EnsureInternalDataSource` 把平台库自身注册为 `type = "sqlite"` 的可查询数据源，改为注册 `postgresql` 连接。注意这属于多数据源模块的配置数据，与平台库选型是两件事，不要混为一谈。
- **审计全文检索**：SQLite FTS5 虚拟表与三个同步触发器在 PostgreSQL 上不存在对应物，改用 `tsvector` 并以 `pg_trgm` 兜底中文（PostgreSQL 内置分词器不切分 CJK）。

## 验证

- `internal/db/db.go` 只打开 PostgreSQL 连接，Ent 使用 `dialect.Postgres`。
- `internal/db/migrations/` 中不再出现 `AUTOINCREMENT`、`datetime('now')` 等 SQLite 方言。
- `go.mod` 中不再有 `modernc.org/sqlite`（`internal/driver/sqlite` 除外——那是**目标数据源**驱动，与平台库无关）。
