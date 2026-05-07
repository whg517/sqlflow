# 003-test-coverage — 测试覆盖率提升至 80%+

> 当前总覆盖率 78.2%（450 tests, -count=1），目标 85%+。主要补齐 service 层（74%）和 connpool（48%）的缺口。
>
> ⚠️ 注意：此前用 cached 结果统计为 42.4%，实际 `-count=1` 重新跑后为 78.2%。

---

## 当前覆盖率基线（`go test -count=1`）

| 模块 | 当前 | 目标 | 提升 |
|------|------|------|------|
| `internal/pkg/mask` | 100% | 100% | — |
| `internal/api/middleware` | 93.1% | 95% | +2% |
| `config` | 91.7% | 95% | +3% |
| `internal/pkg/sqlparser` | 82.0% | 85% | +3% |
| `internal/api/handler` | 84.7% | 90% | +5% |
| `internal/service` | 74.1% | 85% | +11% |
| `internal/pkg/crypto` | 78.1% | 90% | +12% |
| `internal/connpool` | 48.2% | 75% | +27% |
| `internal/model` | 0% | 0% | —（纯结构体，无需测试） |
| `internal/resp` | 0% | 0% | —（纯 JSON 封装，无需测试） |
| `internal/db` | 0% | 60% | +60% |
| **总计** | **78.2%** | **~85%** | **+7%** |

---

## 策略

1. **禁用缓存**：所有测试使用 `go test -count=1`，避免 cached 覆盖率虚高
2. **Handler 层 84.7%**：已基本达标，补齐少量遗漏即可
3. **Service 层 74.1%**：主要缺口在 datasource.go（需真实 DB）、query.go（需 mock 连接池）
4. **Connpool 48.2%**：需要真实 MySQL/MongoDB 或 mock server
5. **验收必检**：每个 Task 完成后用 `-count=1 -coverprofile` 验证真实覆盖率

---

## Phase A: Service 层补充（预估 +5%，5 个 Task）

| Task | 名称 | 覆盖文件 | 预估提升 |
|------|------|---------|---------|
| A.1 | Datasource Service 测试 | datasource.go (9 func) | +1.5% |
| A.2 | Query Service 测试 | query.go (8 func) | +1% |
| A.3 | Auth Service 测试 | auth.go (10 func) | +1% |
| A.4 | Permission Service 补充 | permission.go (10 func) | +0.8% |
| A.5 | Pagination + Query Export 测试 | pagination.go + query_export.go | +0.7% |

## Phase B: Connpool + 基础设施（预估 +1.5%，2 个 Task）

| Task | 名称 | 覆盖文件 | 预估提升 |
|------|------|---------|---------|
| B.1 | Connpool 补充测试 | manager.go + mysql.go + mongodb.go | +1% |
| B.2 | DB 层 + Crypto 补充 | db.go + crypto.go | +0.5% |

## 依赖关系

```
A.1-A.5（可并行）
B.1-B.2（可并行）
Phase A + B 可同时推进
```

---

## 验收标准

1. 每个完成后 `go build ./...` + `go test ./... -count=1` 全部通过
2. 每个完成后 `go test ./... -count=1 -coverprofile=coverage.out` 检查覆盖率提升
3. 最终总覆盖率 ≥ 85%
4. 不引入新的 lint 问题
5. 不修改业务代码逻辑（仅新增测试文件）
6. 使用表驱动测试（table-driven tests）

## 总预估

- **7 个 Task**
- **总工作量约 10-12h**
- **预期最终覆盖率：~85%**
