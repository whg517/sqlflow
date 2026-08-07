# SQLFlow

数据访问治理平台：低风险查询开发者自助，高风险变更走「工单 → 审批 → 执行 → 审计」闭环。
Go 1.25 + Echo 后端，React 19 + TypeScript 前端，平台元数据存 PostgreSQL（见 [ADR-0009](docs/adr/0009-postgresql-platform-metadata.md)）。

**当前发布等级 L0（仅隔离开发验证）**，存在未关闭的发布阻断项。改动前先读
[docs/ROADMAP.md](docs/ROADMAP.md) 的阶段 0 与
[最新复核](docs/reviews/2026-07-31-implementation-verification.md)。

## 验证

测试需要一个真实的 PostgreSQL——每个用例独占一个 schema，跑完即删。
没有它任何 DB 相关测试都会直接 fail 而不是跳过。

```
make dev-db        # 起测试用 PostgreSQL；未设 SQLFLOW_TEST_DSN 时用它的地址
make verify        # lint + build + test，提交前跑这个
make arch          # 只查分包依赖方向，秒级
go test ./internal/ticket/      # 单包验证，比全量快一个数量级
golangci-lint run ./internal/... ./cmd/...
cd web && npx tsc -b && npm run test
```

`go test ./internal/...` 全量各包耗时合计约 13 分钟（并行执行墙钟更短），
`internal/query` 单包就占 3.5 分钟，`internal/ticket` 占 2 分钟。
改动集中时请只跑相关包，别用全量当默认反馈回路。

备份走 `pg_dump`，`internal/ops` 的测试需要它在 PATH 上。

## 不可违反的不变量

这些是被真实缺陷验证过的规则，不是风格偏好。破坏它们的改动会被拒绝。

1. **服务端是授权的唯一裁决者。** 前端隐藏菜单、禁用按钮只是体验优化。每个入口
   都要独立鉴权，不能依赖上游已经查过。
2. **状态迁移只能用 CAS。** 工单状态的每次变更都是
   `Update().Where(id, 期望状态)` 并检查影响行数。读-改-写曾让 4 个并发审批
   同时成功。
3. **失败路径也要写审计。** 连接失败、权限拒绝、执行出错都必须留下记录——
   查不到的那些查询恰恰是运维最需要的证据。
4. **脱敏不能被绕过。** 查询、导出、分享共用同一套规则。无法施加脱敏的结果形态
   （如聚合载荷）必须拒绝返回，而不是放行。
5. **客户端不得影响风险等级或审批路径。** 风险由服务端从 SQL 派生，它决定审批
   策略；AI 结论不接受客户端提供。

## 数据源抽象

差异由三组正交声明表达，Service 与 UI 只读声明，**不按类型名分支**：

| 轴 | 载体 | 回答 |
|---|---|---|
| 能做什么 | 可选接口（类型断言） | 能不能做 |
| 查询形态 | `QueryForm() QueryForm` | 查询怎么写（`sql`/`document`/`dsl`） |
| 结果形态 | `QueryResult.Shape` | 结果怎么读（`table`/`documents`/`aggregation`） |

**能力全部由可选接口表达，没有能力位。** `MetadataBrowser`、`StatementExecutor`、
`ParameterizedQueryExecutor`、`ParameterBinder`、`QueryExplainer`、`ConfigValidator`、
`ConfigDecoder`——方法在不在，类型系统说了算，`driver.Describe` 用类型断言合成 `Descriptor`。

**驱动专属配置存在 `extra_config` JSON，由驱动自己解码**（`ConfigDecoder`，对称于
`ConfigValidator`）。ES 曾有五个专属列，从 DDL 一路穿透到 ent schema、model、adapter
的键名 switch、三个请求结构体和前端表单——六层，每层都不知道那些值是什么意思；而同样
需要专属配置的 MongoDB 一列都没有。**不要再为某个驱动加列。** 凭据是例外，与
`password_encrypted` 同轴（密文存储、`Secrets` 传递），不进 `extra_config`。

曾经有七个手写的能力位，全部删除。**它们不是没被强制，而是本身就是错的**：
`CapQuery`/`CapFieldMasking` 五个驱动全声明为真；`CapSQLParse`/`CapExport` 被
Mongo/ES 声明为假但两者都做得到——照 `CapExport` 执行会拒掉可用的 MongoDB 导出；
`CapTableLevelPermission` 被 ES 声明为假，而 ES 索引实际正被 Casbin 校验——照它执行会
**删掉**一处访问检查。剩下两个（`CapMetadata`/`CapTicketExec`）守的是 `Driver` 上的方法，
是结构性的，已收进上述接口。

**不要再引入运行时声明的能力位。** 如果新能力是「有没有这个方法」，用可选接口 +
编译期断言；如果它是「平台对这个数据源的判断」，那它不属于驱动。

**每个驱动必须写编译期断言**（`internal/driver/*/`）：

```go
var (
	_ driver.Driver          = (*MySQLDriver)(nil)
	_ driver.QueryExplainer  = (*MySQLDriver)(nil)
)
```

可选接口是结构化满足的。曾经有一次重构把两个 `ExplainQuery` 方法整个漏掉，
`go build` 照样通过，症状只是所有数据源静默上报 `explain=false`。断言把这类
问题变成编译失败。

**新增数据源类型的改动面应当只有两处**：实现 `driver.Driver`、在
`internal/driver/all` 注册。解析也归驱动——直接调 `sqlparser.Parse<方言>Dialect`，
`internal/platform` 里不得再出现按类型名的分支（`internal/arch` 强制）。查询、导出、AI 评审、元数据、连接测试、前端工作台
都不该改。如果你发现必须改，说明抽象漏了一个轴——先讨论再动手。

前端读 `GET /api/datasources/:id/capabilities` 决定编辑器、可用操作与渲染器，
见 `web/src/features/query/pages/Query/queryModes.ts`。

## 目录

按 feature 垂直切分：每个领域包自带 service、HTTP handler 和测试。

```
cmd/server/          进程入口
internal/app/        组合根：唯一知道全部具体实现的地方
internal/api/        路由、中间件、跨域聚合端点（health、settings）

内部领域（各自含 service + handler + test）
internal/audit/      审计写入、检索、报表、用户行为分析
internal/datasource/ 数据源 CRUD、连接测试、元数据浏览
internal/iam/        认证、JWT、API token、OIDC
internal/security/   Casbin 权限、权限申请、脱敏规则
internal/query/      查询执行、历史、导出、分享、SQL 模板
internal/ticket/     工单状态机、审批引擎、SLA、AI 评审、调度
internal/notify/     Webhook、飞书、通知偏好与订阅
internal/ops/        备份、仪表盘、Git 关联、前端性能指标

共享
internal/driver/     数据源端口 + 5 个驱动 + 注册表 + 连接池
internal/db/         PostgreSQL 连接、ent、SQL migration
internal/model/      跨领域的持久化模型
internal/authz/      Casbin 元组的唯一构造点
internal/platform/   领域无关能力：auditlog、httpx、crypto、mask、metrics、sqlparser、sqlutil、perf
internal/testutil/   跨包共享的测试夹具与驱动注册
internal/arch/       分包依赖方向的可执行约束（只有测试，无代码）

web/src/features/    前端按 feature 垂直切分，与后端领域同名
web/src/shared/      HTTP client、设计系统、布局外壳、通用工具
docs/                需求、架构、ADR、评审、路线图
```

**依赖方向由 `internal/arch` 的测试强制**，不是口头约定：

- `internal/platform/*` 不得依赖任何领域包
- 领域包不得依赖 `internal/api`（领域自带 handler，但不反向依赖传输层）
- 跨领域依赖必须在 `allowedDomainEdges` 中显式登记并说明理由；条目失效也会报错
- 只有 `internal/app` 可以认识全部领域

只用到对方一两个方法时，**在消费侧声明接口**而不是加一条边——
见 `datasource.ObjectViewChecker`、`iam.PlatformPermissionLister`、`auditlog.Writer`。

前端同构：`web/src/features/<name>` 与后端领域同名，跨 feature 导入由
`eslint.config.js` 的 `allowedFeatureEdges` 登记，未登记的会 lint 报错。
共用部分提到 `@/shared`，不要在 feature 之间互相 import。

## 已知边界

- **`internal/db/ent/` 是生成代码，不要手改。** 改 `internal/db/ent/schema/`
  后跑 `go generate ./internal/db/ent/` 重新生成。SQL migration 是 DDL 的唯一事实来源，ent 自动迁移未启用——
  改表结构要同时评估 migration、ent schema 和测试夹具。
- **`internal/connpool` 只剩一处用途**：ES 索引与字段浏览需要原生客户端。
  不要扩大它；其余路径一律走 `internal/driver.PoolManager`。
- **领域包访问平台库只能通过 ent**（[ADR-0010](docs/adr/0010-ent-as-the-single-data-access-path.md)），
  由 `internal/arch` 的 `TestDomainsDoNotQueryThroughDatabaseSQL` 强制。双轨期
  暴露了 8 个缺陷，5 个是静默的：占位符复用让私有模板的归属校验失效、
  `INSERT OR IGNORE` 让通知去重从未生效、布尔写成 0/1 让 webhook 无法禁用。
  这些都是合法的 Go 程序，类型化查询让它们写不出来。
- **需要 ent 表达不了的 SQL 时用 `Modify` 逃生口，并写明为什么。** 全文检索的
  `@@`、聚合上的 `ORDER BY`、`GROUP BY` 后取 top-N 都属于这一类。逃生口本身不
  违反 ADR-0010，无理由地使用才违反。
- 浏览器级端到端验证目前缺失（Playwright 套件已移除）。
- **前端单测套件对负载敏感**，`findByText` / `waitFor` 在并行压力下会超时，同一份
  代码连跑三次可能 0、2、48 个失败。这是既有问题（在重组前的树上同样复现），
  不是某次改动的回归。判断一次失败是否真实，先单独重跑该文件。
- 工单执行缺租约与崩溃恢复，进程在 `EXECUTING` 期间崩溃会使工单卡住。
- **语句边界归驱动**（`StatementSplitter`），分析与执行消费同一个 `ticketPlan`。
  曾经执行端是 `strings.Split(sql, ";")`、分析端只读首关键字，于是
  `SELECT 1; DROP TABLE users` 评 low 却执行 DROP——提交者改 SQL 就能改自己的
  审批路径。MySQL/SQLite 用手写词法扫描器（**被迫**：vendored 的 pingcap/parser
  拒绝 `ALTER TABLE ... RENAME COLUMN`、CTE、窗口函数，拿它当分词器会让普通变更
  工单提不了）；PostgreSQL 用 `pgquery.SplitWithParser`（scanner 会切碎
  `BEGIN ATOMIC` 函数体）。`/*!nnnnn ... */` 按代码扫描，不按注释——服务器会执行
  里面的内容。读不懂的语句体一律拒绝：无法定级不等于无害。

## 约定

- **注释写「为什么」，不写「做了什么」。** 尤其是非显然的取舍——为什么拒绝而不是
  降级、为什么这里不能加锁。
- **测试要有真实断言。** 「调用不 panic」不算覆盖。曾经有个只在空库上跑的
  调度器测试，让三个致命缺陷长期不被发现。修 bug 时先写会失败的用例。
- **死代码是门禁级问题。** `unused` 已在 `.golangci.yml` 中启用（含测试文件），
  但它不报导出符号——对 `internal/` 这个假设不成立，于是整簇代码烂在原地：
  connpool 留着一套没人碰过的 PostgreSQL 池，一个被取代的 `APITokenAuth` 中间件
  就摆在活的那个旁边、名字读起来像真的。`make deadcode` 用调用图补上这个缺口，
  失败时要么删、要么接线、要么写进 `scripts/deadcode.allow` **并说明理由**——
  没有理由的条目和疏忽无法区分，过期的条目会被同一个门禁拒绝。
  `make deadcode-report` 列出「生产不可达但测试可达」的那批，不做门禁：
  测试辅助函数本就该只有测试用，其余的是调用者走了、被调用者留下。
- **只写不读的字段同样是死代码，而 `unused` 看不见。** 构造函数赋了值就算「用过」，
  于是 `lastUse`（还带着数据竞争）、`poolEntry.config`、`permSvc` 三处躲过了全量 lint。
  `internal/arch` 的 `TestNoWriteOnlyFields` 补这个缺口，**只查未导出且无 struct tag
  的字段**：未导出保证包级扫描是完备的，无 tag 排除反射读取。带 tag 的（如 `StoreBytes`）
  是这条规则接受的漏报，换来的是零误报。
- **共享常量而非重复字面量。** 工单列清单曾在四处手工维护，drift 导致过两个缺陷；
  现在收敛为 `ticketColumns`。
- 错误信息面向用户用中文，代码注释与标识符用英文。
- 领域错误在 Service 层定义，Handler 负责映射为 HTTP 状态码。
