# ADR-0010：Ent 是唯一的数据访问方式

- 状态：accepted
- 日期：2026-08-01
- 决策者：SQLFlow Team
- 关联需求：NFR-MNT-001、NFR-TST-001
- 取代：[ADR-0002](0002-sqlite-metadata-and-migrations.md) 中关于数据访问方式的部分
- 被取代：无

## 背景

ADR-0002 写道：「Ent Client 和原始 SQL 在迁移阶段共享同一连接」，并在后果中承认「双轨期间有模型漂移风险」。但它没有说明什么条件结束这个「迁移阶段」，也没有说明最终哪一边胜出。ROADMAP 把它挂为 DEBT-03，但开发者在动手前读的是 ADR，不是 ROADMAP。

结果是可测量的：拆分后的八个领域包中，使用原始 SQL 的文件数普遍多于使用 Ent 的文件数，共约 200 处带 `?` 占位符的查询。这些查询在 [ADR-0009](0009-postgresql-platform-metadata.md) 决定换用 PostgreSQL 时，成为迁移的主要成本——PostgreSQL 只接受 `$N` 占位符。

带过渡态而不写退出条件的决策，实际效果是推迟决策。本 ADR 补上这个条件。

## 决策

Ent 是访问平台元数据的唯一方式。领域包不得直接使用 `database/sql` 查询平台库。

**退出条件**：当 `internal/{audit,datasource,iam,notify,ops,query,security,ticket}` 中不再出现 `sql.DB` 的 `Query`/`QueryContext`/`Exec`/`ExecContext`/`QueryRow` 调用时，双轨期结束。届时新增一条架构测试守卫此约束，与 `internal/arch` 中既有的分层守卫同级。

在退出条件达成前，**新代码一律使用 Ent**；只有在修改既有原始 SQL 时才允许保留其形式，且应优先顺手迁移。

**唯一例外**：`internal/db` 自身。migration 执行、连接建立与健康检查发生在 Ent Client 存在之前，必须使用 `database/sql`。

## 备选方案

- **反向收敛到原始 SQL**：删掉 Ent 可省下约 205 个生成文件与一条 `go generate` 链路。但换库时约 200 处查询的方言正确性将完全依靠人工审查，而 Ent 的 dialect 层正是为此存在。
- **维持双轨并补一层方言抽象**：需要自建占位符改写与分页 SQL 生成，等于重写 Ent 已有的能力，且这层抽象本身没有测试覆盖它的两个分支。
- **维持现状**：换库成本已经证明了这一项的代价，不再是假设。

## 后果

- **优点**：方言差异收敛到 Ent 一处，换库不再需要逐条审查 SQL；查询获得编译期类型检查；schema 与查询的漂移由生成器消除。
- **代价**：约 200 处查询需要改写，其中聚合与报表类查询在 Ent 中比原始 SQL 冗长；少数复杂查询可能需要 `Modify` 逃生口。
- **约束**：使用 Ent 逃生口（`sql.Modifier`、原始谓词）时必须写明为何 Ent 的类型化 API 表达不了该查询——逃生口本身不违反本 ADR，无理由地使用才违反。

## 验证

- `internal/arch` 在退出条件达成后新增守卫：领域包不得直接持有 `*sql.DB` 并执行查询。
- `internal/platform/sqlutil` 的占位符与分页助手在迁移完成后应大部分失去调用者；届时按死代码处理。
