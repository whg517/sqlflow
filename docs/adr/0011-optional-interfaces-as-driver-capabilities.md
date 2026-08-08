# ADR-0011：驱动能力由可选接口表达，不再显式声明

- 状态：accepted
- 日期：2026-08-08
- 决策者：SQLFlow Team
- 关联需求：FR-DS-001、FR-DS-003、NFR-MNT-001
- 取代：[ADR-0003](0003-capability-based-datasource-drivers.md)
- 被取代：无

## 背景

ADR-0003 要求驱动「显式声明查询、工单执行、元数据、表级权限、字段脱敏、解析和导出能力」，并把 `CapabilitySet` 作为验证依据。这套机制上线后被逐条检验，结论是**它不是没被强制，而是本身就是错的**：

- `CapQuery`、`CapMetadata`、`CapFieldMasking` 被全部五个驱动声明为真。一个所有实现都为真的位不承载信息，也不可能拒绝任何东西。
- `CapSQLParse` 与 `CapExport` 被 MongoDB / Elasticsearch 声明为假，而两者都做得到。**照 `CapExport` 执行会拒掉一个可用的 MongoDB 导出**——声明比实现更保守，且保守的方向是错的。
- `CapTableLevelPermission` 被 Elasticsearch 声明为假，而 ES 索引实际正被 Casbin 校验。**照它执行会删掉一处真实的访问检查**——这一条如果被"修复"成强制，会制造一个安全缺口。

七个位里只有 `CapMetadata` 和 `CapTicketExec` 守着真实存在的差异，而它们守的是 `Driver` 接口上的方法是否为空实现——这是结构性的，类型系统本来就能回答。

根因在于：能力位是与实现**并行**的第二份声明，两份声明之间没有任何东西保证一致。ADR-0003 记录的决策方向（显式声明差异）恰恰制造了这个缺口。

## 决策

删除全部能力位。驱动能力由**可选接口 + 编译期断言**表达：

- `MetadataBrowser`、`StatementExecutor`、`ParameterizedQueryExecutor`、`ParameterBinder`、`QueryExplainer`、`ConfigValidator`、`ConfigDecoder`、`StatementSplitter` —— 方法在不在，类型系统说了算。
- `driver.Describe` 用类型断言合成 `Descriptor`，前端读 `GET /api/datasources/:id/capabilities` 得到的就是类型系统的答案，不存在第二份声明。
- 每个驱动必须写编译期断言（`var _ driver.X = (*TDriver)(nil)`），因为可选接口是结构化满足的：方法被改名或漏掉不会中断构建，只会让能力静默上报 false。曾有一次重构整个漏掉两个 `ExplainQuery`，`go build` 照样通过，症状只是所有数据源静默上报 `explain=false`。
- 驱动专属配置存在 `extra_config`，由驱动自己解码（`ConfigDecoder`），与 `ConfigValidator` 对称。不再为某个驱动加列。

## 备选方案

- **保留能力位并补上强制**：会把上述两个错误声明变成真实故障——拒掉可用的导出，删掉可用的权限检查。方向本身是错的，强制只会让它更快出事。
- **让能力位由实现自动派生**：那就是可选接口，只是多绕一层运行时表示。
- **强制完整同构接口**：表面统一，但会逼出空实现——SQLite 和 Elasticsearch 曾被迫提供只会返回「不支持」的 `ExecuteStatement`，这是 LSP 违反，且只有手写的 `CapTicketExec` 检查挡在调用方和这些桩之间。

## 后果

- 优点：差异由类型系统裁决，不存在与实现不一致的声明；新增驱动的改动面收敛到实现接口 + 在 `internal/driver/all` 注册两处。
- 代价：能力是编译期概念，无法在运行时按数据源实例开关——这是刻意的，「平台对某个数据源的判断」不属于驱动。
- 约束：**不得再引入运行时声明的能力位。** 新能力若是「有没有这个方法」，用可选接口；若是「平台对这个数据源的判断」，它不属于驱动。

## 验证

- `internal/driver/driver.go`、`config.go` 定义可选接口；`descriptor.go` 的 `Describe` 用类型断言合成。
- `internal/driver/assertion_completeness_test.go` 的 `TestEveryDriverAssertsWhatItSatisfies` 强制断言块完整：类型系统说驱动满足什么，源码就必须写出什么。它是补上的，因为约定曾经只写在文档里且**已经漂移**——Elasticsearch 实现了整个 `MetadataBrowser` 却一个断言都没写。
- `internal/driver/capability_meaning_test.go` 记录被删掉的七个位各自为什么是错的。
- `internal/arch` 的 `TestPlatformDoesNotBranchOnDatasourceType` 禁止 `internal/platform` 出现按类型名的分支。
