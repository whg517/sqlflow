# 003-test-coverage — 测试覆盖率提升至 85%+

> 当前总覆盖率 78.2%（450+ tests, -count=1），目标 85%+。聚焦 service 层和 connpool 缺口。

## 当前覆盖率基线

| 模块 | 当前 | 目标 | 主要缺口 |
|------|------|------|---------|
| `internal/api/middleware` | 93.1% | — | CORS/Logger/Recovery 0%（非关键） |
| `internal/pkg/mask` | 100% | — | ✅ |
| `config` | 91.7% | — | ✅ |
| `internal/api/handler` | 84.7% | — | audit handler 0%（小文件） |
| `internal/pkg/sqlparser` | 82.0% | — | driver.go 0%（pingcap 适配层，非业务） |
| `internal/connpool` | **48.2%** | 75% | MySQLPing/GetTables/MongoDB 需要真实连接或 mock |
| `internal/service` | **74.1%** | 85% | auth/datasource/query/permission/export 多个函数 0% |
| `internal/pkg/crypto` | 78.1% | 85% | 边界用例 |
| **总计** | **78.2%** | **~85%** | |

## Task 列表

### Phase A: Service 层核心缺口（3 个 Task）

| Task | 名称 | 目标函数 | 预估 |
|------|------|---------|------|
| A.1 | Auth Service 补充 | auth.go: UserCount/UpdateUserRole/DeleteUser/ResetPassword (4 func) | 1.5h |
| A.2 | Datasource + Query 执行 | datasource.go: GetTables; query.go: executeMySQL/executeMongoDB; query_export.go: ExportQuery | 2h |
| A.3 | Permission + Audit 补充 | permission.go: RemoveFilteredPolicy/SavePolicy/seedIfEmpty; audit.go: Close; audit handler: ListAuditLogs | 2h |

### Phase B: 基础设施补充（2 个 Task）

| Task | 名称 | 目标函数 | 预估 |
|------|------|---------|------|
| B.1 | Connpool mock 测试 | mysql.go: MySQLPing/MySQLGetTables; mongodb.go: MongoPing; manager.go: GetMongoDB/GetMongoDatabaseNames/RemoveMongo | 2h |
| B.2 | Crypto + SQL Parser 边界 | crypto: 边界用例; sqlparser: CheckSensitiveTables/extractOperationRegex | 1.5h |

## 并行策略

- A.1/A.2/A.3 修改不同文件，可并行（≤3）
- B.1/B.2 独立，可与 A 并行

## 验收标准

所有测试全部通过（go build + go test -count=1 + npm build），最终覆盖率 ≥ 85%。
