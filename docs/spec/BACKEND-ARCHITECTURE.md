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

## 二、技术债 #1：依赖注入失控（✅ 已治理 PR-2）

### 原始问题

`api.NewRouter` 原接收 **28 个位置参数**，`main.go` 用 ~100 行手工 wiring + 6 个 `Set*` 延迟注入方法处理循环依赖。

### 治理结果（PR-2 已完成）

采用**手工 Container**（放弃 google/wire：循环依赖 + 启动副作用更适合手工编排）：

- 新建 `internal/app/container.go`：`Container` 聚合 28 个 service + db/cfg/connMgr/poolMgr
- `NewContainer(database, cfg)` 封装全部构造顺序、循环依赖 setter、启动副作用（scheduler/backup/admin seed/OIDC）、`Close()` 优雅关闭
- `api.NewRouter(c *app.Container)`：**28 参数 → 1 参数**
- `main.go`：删除 ~150 行手工 wiring

**修复的隐藏 bug**：SLA service 原在 main.go 和 router.go 各 new 一次（两个独立实例状态不一致），现 Container 单一实例共享。

### 遗留

6 个 `Set*` 延迟注入方法保留（循环依赖的本质未消除，仅封装到 Container 内部），后续可用接口抽象彻底解决。

---

## 三、技术债 #2：双轨连接层（治理中）

### 现状

代码库存在**两套连接管理层**，是**半完成的迁移**（非并行设计）：

| 层 | 包 | 定位 | 状态 |
|----|----|------|------|
| 旧 | `internal/connpool/` | MySQL/PG/Mongo/ES 各自缓存连接 | ⚠️ 迁移中（仍有依赖）|
| 新 | `internal/driver/` | 统一 Driver 接口 + PoolManager | ✅ 目标态 |

`driver/pool.go:14` 明确注释：「replaces the old connpool.Manager with a unified driver-based approach」。

### 已完成的迁移（PR-3 阶段一、二）

| Service | 迁移前 | 迁移后 | 状态 |
|---------|--------|--------|------|
| `TicketService` | 纯 connMgr（无 poolMgr）| poolMgr 优先 + connMgr fallback | ✅ DDL/DML 执行已迁移 |
| `QueryService` 查询 | MySQL/PG 走 driver，Mongo/ES 走 connMgr | MySQL/PG/**MongoDB** 走 driver，ES 走 connMgr | ✅ MongoDB 已迁移 |
| `DatasourceService` 失效 | 按类型分支 connMgr.Remove* | 统一 poolMgr.Remove(dsID) | ✅ 连接池失效已迁移 |
| `Driver` 接口 | 仅 ExecuteStatement（单条无事务）| 新增 **ExecuteStatements**（批量+事务）| ✅ PG 事务语义已对齐 |

Driver.ExecuteStatements 事务语义：
- **PostgreSQL**：单事务 `BeginTx` + 循环，首错 break + rollback，已成功语句标记 `rolled_back`（严格对齐 `executeSQLTransactional`）
- **MySQL**：逐条 auto-commit，首错继续收集所有结果
- **MongoDB**：降级为循环调用 ExecuteStatement
- **Elasticsearch**：返回 not supported（只读）

### 剩余工作（后续独立 PR）

connpool 包**暂不可删除**，以下路径仍依赖 connMgr：

1. **datasource.go 元数据查询**（7 处）：`GetTables`/`GetColumns`/`ListDatabases` 使用手写 SQL（`INFORMATION_SCHEMA.COLUMNS`、`SHOW TABLES`），与 driver 的 `ListTables`/`GetColumns` 返回结构有差异，需逐个验证数据一致性
2. **query.go ES 查询路径**：Elasticsearch 复杂聚合查询（search/aggregate/count），driver.ExecuteQuery 覆盖度需验证
3. **query_export.go 测试**：异步导出路径的 connMgr 注入

迁移策略：每类元数据查询单独迁移 + 测试，最后统一删除 connpool 包。

### 关键约束（已解决）

~~`ticket_executor.go` 执行真实 DDL/DML，对 PostgreSQL 走多语句事务，但 driver 层的 ExecuteStatement 是单条无事务~~ → **已通过 ExecuteStatements 解决**（见 `driver/postgresql/postgresql.go`）

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

## 六、治理进度

| 阶段 | 主题 | 状态 | 风险 |
|------|------|------|------|
| PR-1 | 文档对齐 + migration 冲突修复 | ✅ 已完成 | 零 |
| PR-2 | DI 重构（Container 聚合根，28→1 参数）| ✅ 已完成 | 低 |
| PR-3 阶段一 | Driver ExecuteStatements + ticket_executor 迁移 | ✅ 已完成 | 高（PG 事务语义）|
| PR-3 阶段二 | query.go MongoDB 覆盖 + datasource 连接池失效统一 | ✅ 已完成 | 中 |
| PR-3 阶段三 | 删除 connpool 包 | ⏳ 后续 | 高（元数据查询依赖）|

### 后续待办

1. datasource.go 元数据查询迁移（7 处手写 SQL → driver.ListTables/GetColumns）
2. query.go ES 查询路径迁移
3. 删除 `internal/connpool/` 整个包
4. 解决 Container 内 6 个 Set* 循环依赖（接口抽象）
