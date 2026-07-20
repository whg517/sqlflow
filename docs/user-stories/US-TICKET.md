# User Stories：工单与审批

## US-TKT-001：提交数据库变更工单

**故事**：作为 Developer，我希望提交包含 SQL、数据源、变更原因和风险信息的工单，以便让数据库变更进入正式治理流程。

**关联需求**：FR-TKT-001、FR-TKT-002

**验收标准**：

- Given 用户已认证且输入完整，When 创建工单，Then 系统解析 SQL 类型、受影响对象、SQL Hash 和风险并保存工单。
- Given 语句为空、数据源不存在、类型不匹配或不受支持，When 创建工单，Then 不创建无效工单。
- Given 审批策略匹配，Then 按优先级应用审批链或满足条件的自动审批；无匹配时使用默认策略。
- Then 创建、策略应用和通知结果不允许绕过工单状态机。

**代表性验证**：`internal/service/ticket_test.go`、`internal/service/approval_engine_test.go`、`e2e/tests/login-query-ticket.spec.ts`。

## US-TKT-002：分级审批工单

**故事**：作为 DBA/Admin，我希望根据审批策略批准或拒绝当前阶段，以便让风险匹配正确的评审责任人。

**关联需求**：FR-TKT-003、FR-TKT-004

**验收标准**：

- Given 工单处于 `PENDING_APPROVAL` 且操作者符合当前阶段角色，When 批准，Then 记录审批动作并前进到下一阶段或 `APPROVED`。
- Given 操作者角色不符合、工单不在当前阶段或重复提交同一动作，When 审批，Then 状态不改变。
- Given 审批人拒绝，When 提交非空原因，Then 工单进入 `REJECTED` 并保留审批记录。
- Given 批量批准/拒绝，Then 每个工单独立返回成功或失败，单项失败不伪装成整体成功。

**代表性验证**：`e2e/tests/multi-stage-approval.spec.ts`、`e2e/tests/batch-approval-ui.spec.ts`、`e2e/tests/approval-security.spec.ts`。

## US-TKT-003：执行或定时执行已批准工单

**故事**：作为工单提交人或获授权的 DBA/Admin，我希望立即或定时执行已批准工单，以便在可控窗口完成变更并保存逐语句结果。

**关联需求**：FR-TKT-005、NFR-REL-001

**验收标准**：

- Given 工单为 `APPROVED`，When 安排未来时间，Then 进入 `SCHEDULED`；取消计划后回到 `APPROVED`。
- Given 工单为 `APPROVED` 或到期的 `SCHEDULED`，When 合法操作者执行，Then 以条件更新进入 `EXECUTING`，避免重复执行。
- Given 全部执行成功，Then 工单为 `DONE` 并保存每条语句的耗时和影响行数。
- Given 执行失败，Then 工单为 `FAILED` 并保存受控错误；事务/回滚语义按驱动明确呈现。

**代表性验证**：`internal/service/ticket_executor_tx_test.go`、`internal/service/scheduler_test.go`、`e2e/tests/ticket-approve-execute.spec.ts`。

## US-TKT-004：修订被拒绝工单

**故事**：作为原提交人，我希望根据拒绝意见修改工单并重新提交，以便保留讨论历史而无需创建不相关的新工单。

**关联需求**：FR-TKT-006

**验收标准**：

- Given 工单为 `REJECTED` 且当前用户是提交人，When 更新 SQL、摘要或原因，Then 旧版本保存为 Revision，新版本回到 `SUBMITTED`。
- Given 用户不是提交人或工单不在 `REJECTED`，When 尝试修订，Then 操作被拒绝。
- Given 多次拒绝和修订，Then Revision 单调递增且可按时间查看全部版本。

**代表性验证**：`internal/service/ticket_resubmit_test.go`、`e2e/tests/resubmit.spec.ts`。

## US-TKT-005：协作、SLA 与代码关联

**故事**：作为工单参与者，我希望评论、接收 SLA 提醒并关联 Commit/PR，以便审批上下文和代码变更保持可追踪。

**关联需求**：FR-TKT-007

**验收标准**：

- Given 用户可查看工单，When 添加评论或回复，Then 评论按工单保存并展示作者与时间。
- Given 用户不是评论作者且不是 DBA/Admin，When 删除评论，Then 操作被拒绝。
- Given 工单超过 SLA 阈值，Then 系统按配置提醒、升级或自动拒绝，并避免重复发送同一阶段动作。
- Given 合法 Commit/PR 信息，When 关联工单，Then 可查看和删除关联且记录创建者。

**代表性验证**：`internal/service/comment_test.go`、`internal/service/sla_auto_reject_test.go`、`internal/service/git_test.go`。
