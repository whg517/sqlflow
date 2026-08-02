# 用户故事 QA：2026-08-03

对照 `docs/user-stories/` 的验收标准审查实现。这是**静态审查**——读代码验证功能完整性与不变量，不依赖测试结果：PostgreSQL 迁移（P3）正在进行中，6 个域的测试当前有失败，跑测试会用迁移噪音淹没真实问题。

## 结论

两个实质缺陷，都在 CLAUDE.md 列为「不可违反」的不变量上。其余抽查项实现正确。

| 编号 | 严重度 | 违反的不变量 | 位置 |
|---|---|---|---|
| QA-01 | 高 | 脱敏不能被绕过 | `internal/query/query_export.go` |
| QA-02 | 高 | 状态迁移只能用 CAS | `internal/ticket/approval_engine.go` |

---

## QA-01：导出路径可绕过脱敏（聚合结果）

**违反**：不变量 4「脱敏不能被绕过。查询、导出、分享共用同一套规则。无法施加脱敏的结果形态（如聚合载荷）必须拒绝返回，而不是放行。」

**违反**：US-QRY-004 验收点「导出内容遵循查询行限制、筛选和脱敏规则」。

**事实**：

查询路径在 `internal/query/query.go:256` 有明确守卫，注释也写清了理由：

```go
// Aggregation payloads are driver-native and arbitrarily nested, so the
// row-oriented masker cannot reach inside them. Returning one unmasked
// would turn an aggregation into a way to read protected fields, so refuse
// it instead of shipping a result the masking rules never inspected.
if result.Shape == driver.ShapeAggregation &&
    s.maskingApplies(...) {
    return nil, ErrAggregationMaskingUnsupported
}
```

导出路径没有这道守卫。`ExportQuery` 把 `drvResult.Aggregations` 原样拷进结果，随后调用同一个 `applyDesensitizationForActor`——而该函数只遍历 `result.Rows`，从不触碰 `Aggregations`。

**后果**：一个无权查看被脱敏字段的用户，通过查询入口会被拒绝，但改走导出入口、对同一 Elasticsearch 索引发起聚合查询即可取得该字段的值（作为桶键或聚合值）。这是一条完整的绕过路径，不是理论风险。

**修复方向**：把聚合守卫提取为查询与导出共用的一处判断，而不是在导出里复制一份——复制正是当初两条路径分叉的原因。

**回归测试要求**：先写一个会失败的用例——对带脱敏规则的索引发起聚合导出，断言被拒绝；撤销修复后该用例须复现。

---

## QA-02：自动审批路径未使用 CAS

**违反**：不变量 2「状态迁移只能用 CAS。工单状态的每次变更都是 `Update().Where(id, 期望状态)` 并检查影响行数。读-改-写曾让 4 个并发审批同时成功。」

**事实**：

`applyApprovalPolicy` 有两处状态迁移使用裸 `UpdateOneID`，没有前置状态条件：

- `internal/ticket/approval_engine.go:279` — 自动审批写入 `APPROVED`
- `internal/ticket/approval_engine.go:299` — 进入多阶段审批写入 `PENDING_APPROVAL`

对照之下，同文件的多阶段审批（:381、:391）**做对了**——用 stage 作为守卫条件，注释还解释了为什么：「Two approvers acting on the same stage would otherwise both write a record and both advance the chain, turning one decision into two.」

`internal/ticket/ticket.go` 的 `casTicketStatus` helper 已经存在且被 4 处正确使用。自动审批路径没有走它。

**后果**：两次并发的策略应用（例如提交后重试、或调度器与人工操作重叠）可以让同一工单被写入两次终态，或让一次取消被自动审批覆盖。这与不变量注释里记载的「4 个并发审批同时成功」是同一类缺陷。

**修复方向**：两处都改用 `casTicketStatus`，前置状态取各自的合法来源态；影响行数为 0 时按「状态已变更」处理而非静默继续。

**回归测试要求**：并发调用 `applyApprovalPolicy`，断言恰好一次成功。撤销修复后须复现。

---

## 抽查通过的项

以下验收点逐条核对实现，未发现问题：

| 故事 | 验收点 | 实现 |
|---|---|---|
| US-QRY-003 | 用户 A 不能删除用户 B 的历史 | `DeleteHistory` 谓词含 `user_id`，删 0 行报错，不泄露记录是否存在 |
| US-QRY-005 | 分享 Token 不可预测 | `crypto/rand` 16 字节（128 位）hex |
| US-QRY-005 | 过期/撤销/密码错误不返回内容 | 三重校验，未授权时返回空 `Rows` 而非报错泄露存在性 |
| US-QRY-006 | 私有模板对他人表现为不存在 | handler 全部走 `*ForUser` 变体，SQL 带 `user_id = ? OR is_public`，未命中返回 `ErrTemplateNotFound` |
| 不变量 3 | 失败路径写审计 | query 与 export 各有 2 处 `*_failed` 审计写入 |
| 不变量 5 | 客户端不得影响风险等级 | `createTicketRequest` 与 `CreateTicket` 签名均不含风险等级或 AI 结论 |
| 不变量 2 | 多阶段审批的并发保护 | :381/:391 以 stage 为 CAS 守卫，注释说明理由 |

## 顺带记录（非缺陷）

`internal/query/sql_template.go:115` 的 `is_public = 1` 在 PostgreSQL 下会因布尔列类型不匹配而失败，`var pub int` 的扫描同理。这属于 P3 query 域迁移的既有工作范围，不单列任务。
