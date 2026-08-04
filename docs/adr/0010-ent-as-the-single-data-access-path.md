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

**退出条件已于 2026-08-04 达成**，守卫为 `TestDomainsDoNotQueryThroughDatabaseSQL`。双轨期结束，本节转为已生效的约束而非过渡计划。

**唯一例外**：`internal/db` 自身。migration 执行、连接建立与健康检查发生在 Ent Client 存在之前，必须使用 `database/sql`。

## 备选方案

- **反向收敛到原始 SQL**：删掉 Ent 可省下约 205 个生成文件与一条 `go generate` 链路。但换库时约 200 处查询的方言正确性将完全依靠人工审查，而 Ent 的 dialect 层正是为此存在。
- **维持双轨并补一层方言抽象**：需要自建占位符改写与分页 SQL 生成，等于重写 Ent 已有的能力，且这层抽象本身没有测试覆盖它的两个分支。
- **维持现状**：换库成本已经证明了这一项的代价，不再是假设。

## 后果

- **优点**：方言差异收敛到 Ent 一处，换库不再需要逐条审查 SQL；查询获得编译期类型检查；schema 与查询的漂移由生成器消除。
- **代价**：改写了 141 处查询。聚合与报表类在 Ent 中确实更冗长，其中 6 处走 `Modify` 逃生口。
- **约束**：使用 Ent 逃生口（`sql.Modifier`、原始谓词）时必须写明为何 Ent 的类型化 API 表达不了该查询——逃生口本身不违反本 ADR，无理由地使用才违反。

## 迁移中发现的缺陷

改写过程本身暴露了 8 个既有缺陷。它们都是合法的 Go 程序，都在运行期才失败，其中 5 个静默失败——这正是「双轨的代价是可测量的」的实证：

| 缺陷 | 症状 |
|---|---|
| `getTemplate` 复用 `$1` | 拿模板 id 比对 user_id，私有模板归属校验形同虚设 |
| `INSERT OR IGNORE` | SQLite 语法，通知去重记录从未写入，工单通知重复发送 |
| Excel 导出混用 `?` 与 `$1` | 任何带过滤条件的导出直接语法错误 |
| `Toggle` 写 0/1 进 boolean 列 | 禁用 webhook 订阅每次都失败 |
| 断路器把 boolean 扫进 int | 自动禁用从未触发，坏端点被无限重试 |
| `GetPolicies` 叠加 ptype 条件 | 按 `g` 筛选恒为空 |
| 脱敏规则重复检查吞掉错误 | 查询失败即落入插入，同字段产生第二条规则 |
| 未转义的 LIKE | 搜 `%` 返回全表 |

## 验证

- `internal/arch/TestDomainsDoNotQueryThroughDatabaseSQL`：领域包不得调用 `database/sql` 的查询方法。放回一处裸 SQL 即失败。
- `internal/platform/sqlutil` 的 `BuildWhereClause`、`PaginatedCountSQL`、`PaginatedQuerySQL`、`AppendLimitArgs`、`FilterClause` 已随最后一个调用方删除。剩下的 `ParsePagination`（传输层关注点）、`EscapeLike` 与 `NumberPlaceholders`（服务于 Ent 无构建器的全文检索与脱敏查询）仍有调用方。
