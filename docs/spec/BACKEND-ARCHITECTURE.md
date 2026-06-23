# SQLFlow 后端架构现状与技术债记录

> 创建日期：2026-06-23
> 状态：进行中（技术债治理）
> 关联：本文记录后端真实架构，与 [ARCHITECTURE-v2.md](./ARCHITECTURE-v2.md)（前端体验专项）互为补充
> 目的：作为依赖注入重构（PR-2）与连接层统一（PR-3）的设计依据

本文档记录后端代码的真实架构状态，重点说明三个已识别的架构问题、它们的现状、风险与治理规划。所有结论基于代码事实，关键处附 `file:line` 引用。

---

## 一、整体架构

### 分层

```
cmd/server/main.go              启动入口 + 手工依赖装配
        │
        ▼
internal/api/                   HTTP 层
  ├── router.go                 路由注册（API 唯一事实源，~140 端点）
  ├── handler/                  请求处理器（按模块分文件，~30 个）
  └── middleware/              Auth（JWT+API Token）/CORS/Logger/Recovery/Admin
        │
        ▼
internal/service/              业务逻辑层（44 个服务，几乎 1:1 配测试）
        │
        ▼
internal/db/                   数据访问层
  ├── ent/                     Ent ORM 生成代码（schema 定义在 ent/schema/）
  ├── migrations/              手写 SQL 迁移（golang-migrate，35 套 up/down）
  └── db.go                    *db.DB 包装（SQLite + ent.Client 双轨）

internal/driver/               ✅ 数据源抽象层（新）
internal/connpool/             ⚠️ 旧连接池（迁移中，见技术债 #3）
internal/pkg/                  基础工具（casbin/crypto/mask/sqlparser/metrics/performance）
internal/coverage/             代码覆盖率审计（可选模块，见下文）
```

### 数据源支持（4 种，Driver 注册表模式）

| 类型 | 驱动实现 | Capabilities |
|------|---------|-------------|
| MySQL | `internal/driver/mysql/` | Query / TicketExec / Metadata / Permission / Masking / SQLParse / Export |
| PostgreSQL | `internal/driver/postgresql/` | 同上 |
| MongoDB | `internal/driver/mongodb/` | Query / TicketExec / Metadata / SQLParse |
| Elasticsearch | `internal/driver/elasticsearch/` | Query / Metadata / SQLParse（只读）|

新数据源接入流程：实现 `Driver` 接口 + 在包 `init()` 调用 `Register()` + 在 `main.go` 用 `_` 空导入。

### 平台数据库

- **SQLite（WAL 模式）**，零运维，单文件，`SetMaxOpenConns(1)` 避免锁竞争（`db.go:49`）
- 双轨数据访问：`*db.DB` 既持有原生 `*sql.DB`（迁移用）也持有 `ent.Client`（业务用）
- 迁移策略：`golang-migrate`（手写 SQL）与 ent 自动迁移并存（`db.go:15-19` 注释标注 Phase 1-2 双轨，Phase 3 移除手写迁移）

---

## 二、技术债 #1：依赖注入失控（待 PR-2 治理）

### 现状

`api.NewRouter` 接收 **28 个位置参数**（`router.go:20`），包括 24 个 service 指针 + `database` + `cfg` + `connMgr` + `poolMgr`。

`cmd/server/main.go` 用 **~100 行手工 wiring** 装配所有依赖，并通过 **6 个 `Set*` 延迟注入方法** 处理循环依赖（`ticket.go:87-119`：`SetDatasourceService` / `SetSLAService` / `SetPermissionService` / `SetApprovalEngine` / `SetGitService` / `SetNotifyService`）。

### 问题

1. **签名脆弱**：新增/删除任意 service 都要改 28 处，参数顺序极易传错（Go 无具名参数检查）
2. **循环依赖靠 setter 掩盖**：`TicketService` 依赖 6 个其它 service，启动顺序敏感，新增交叉依赖时容易遗漏 `Set*` 调用导致 nil panic
3. **无聚合根**：没有 `Container` / `Services` 结构体，每个 service 各自持有零散的依赖指针字段

### 治理方案（PR-2）

引入 **google/wire**（编译期 DI，无运行时开销）：
1. 新建 `internal/app/container.go`：定义 `Container` 聚合结构体
2. 新建 `internal/app/wire.go`（`//go:build wireinject`）：声明 provider，`wire.Build(...)` 组装
3. 生成 `wire_gen.go` 入仓
4. `NewRouter(c *app.Container)`：28 参数 → 1 参数
5. `main.go` 改为 `container, cleanup := app.InitializeApp(cfg)`

**风险**：低。`NewRouter` 无测试依赖（全仓 grep 确认），重构不影响现有测试。`Set*` 方法本轮保留（循环依赖后续用接口抽象解决）。

---

## 三、技术债 #2：双轨连接层（待 PR-3 治理）

### 现状

代码库存在**两套连接管理层**，是**半完成的迁移**（非并行设计）：

| 层 | 包 | 定位 | 状态 |
|----|----|------|------|
| 旧 | `internal/connpool/` | MySQL/PG/Mongo/ES 各自缓存连接 | ⚠️ 待移除 |
| 新 | `internal/driver/` | 统一 Driver 接口 + PoolManager | ✅ 目标态 |

`driver/pool.go:14` 明确注释：「replaces the old connpool.Manager with a unified driver-based approach」。

### 三种服务的迁移进度

| Service | connMgr 字段 | poolMgr 字段 | 当前模式 |
|---------|------------|------------|---------|
| `QueryService` | ✅ | ✅ | `if s.poolMgr != nil` 双轨 fallback（`query.go:179`）|
| `DatasourceService` | ✅ | ✅ | 同上（`datasource.go:240` 等）|
| `TicketService` | ✅ | ❌ | **纯旧路径，无 poolMgr 字段**（`ticket.go:75`）|

### 🔴 阻断性发现

`ticket_executor.go` 执行真实 DDL/DML，对 **PostgreSQL 走多语句事务**（`executeSQLTransactional`，整批回滚，`ticket_executor.go:69-71`），但 driver 层的 `ExecuteStatement` 是**单条无事务**实现（`postgresql.go:265` 直接 `ExecContext`）。

**彻底迁移前必须先给 Driver 接口补 `ExecuteStatements(ctx, db, []string) ([]StatementResult, error)`（批量 + 事务）**，否则会丢失 PG 事务原子性语义，导致工单执行半成功（部分语句提交、部分失败无法回滚）。

### 治理方案（PR-3，分步）

1. **扩展 Driver 接口**：新增 `ExecuteStatements`。PG driver 内部 `BeginTx` + 循环 + commit/rollback，严格对齐 `executeSQLTransactional` 语义；MySQL 保持逐条 auto-commit
2. **TicketService 加 poolMgr 字段 + setter**，双轨分支（与 query.go 模式对齐）
3. **迁移 ticket_executor.go / query.go / datasource.go** 三处执行路径
4. **删除 `internal/connpool/` 整个包** + 所有 service 的 connMgr 字段
5. **强化测试**：`ticket_executor_tx_test.go` 扩展为验证 driver 路径下 PG 事务回滚语义不变；E2E 全量回归

**风险**：高。PG 事务语义若失真，工单执行可能半成功。对策：driver 新方法先单测（模拟中途失败验证 rollback）→ sqlmock 集成测试 → E2E 真实容器。

---

## 四、技术债 #3：Coverage 模块的死代码风险（文档化，暂不处理）

### 现状

`internal/coverage/` 是代码覆盖率审计模块，**强依赖 PostgreSQL**（schema 用 `BIGSERIAL` / `JSONB` / `INTEGER[]` 等 PG 方言，见 `migration/001_create_coverage_tables.up.sql`，无法运行在平台 SQLite 上）。

`router.go:320` 调用 `handler.RegisterCoverageRoutes(e, authMW, adminMW, nil)` —— **故意传 `nil`**，触发 `coverage.go:232` 的 nil guard，整个 coverage 路由组不注册。

### 判断

**这不是 bug，是设计性禁用**：平台库是 SQLite，强行传 `database.DB` 会让 coverage store 的所有 SQL（`$1` 占位符、PG 类型）报错。正确的 nil 传入避免了运行时崩溃。

### 待办（独立于三大债，未来可选）

- 新增 `coverage` config 段 + 环境变量 `SQLFLOW_COVERAGE_PG_DSN`，让用户可选启用
- main.go 中按配置开启独立 PG 连接 + 跑 coverage migration，再传入 `RegisterCoverageRoutes`
- 本文 + README + api.md 已如实记录其为「可选模块，默认禁用」

---

## 五、其他观察

### 配置项命名不一致（已在 PR-1 修正）

- `Config.NotifyConfig` 的 mapstructure key 是 `notify`（`config.go:67`），但 `config.example.yaml` 历史遗留写作 `dingtalk:`（不会被 viper 解析进 `cfg.Notify`）
- `.env.example` 同时存在 `SQLFLOW_DINGTALK_*`（死变量）和 `SQLFLOW_NOTIFY_*`（生效变量）两套
- PR-1 已修正 example 为 `notify:`，并在 env 中标注兼容保留

### ent 双轨数据访问

`db.go:22-27` 的 `*db.DB` 同时持有 `*sql.DB`（golang-migrate 用）和 `ent.Client`（业务用）。注释标注「Phase 3 will remove the raw SQL path」。这是另一个待清理的双轨，与 PR-3 同期处理更合理。

---

## 六、治理优先级

| PR | 主题 | 风险 | 依赖 |
|----|------|------|------|
| PR-1 | 文档对齐（本文 + README + api.md + config）| 零 | 无 |
| PR-2 | DI 重构（google/wire）| 低 | 无 |
| PR-3 | 连接层统一（消灭 connpool）| 高 | PR-2 完成 |

每个 PR 独立可发布、独立回归，风险递增。
