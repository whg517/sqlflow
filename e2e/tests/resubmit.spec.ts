/**
 * resubmit.spec.ts
 *
 * E2E 驳回重提流程测试 (SF-QA0034)
 *
 * 覆盖 API 端点：
 *   POST /api/tickets/:id/resubmit      — 重提工单
 *   GET  /api/tickets/:id/revisions      — 查询历史版本
 *   POST /api/tickets                    — 创建工单
 *   POST /api/tickets/:id/approve        — 审批
 *   POST /api/tickets/:id/reject         — 驳回
 *   GET  /api/tickets/:id                — 查询工单
 *   DELETE /api/approval/policies/:id    — 清理策略
 *
 * 测试范围：
 *   - REJECTED → 修改 SQL → RESUBMIT → 重新审批
 *   - revision 递增验证
 *   - 历史版本查询（revisions API）
 *   - 非法状态拦截（非 REJECTED 状态不能重提）
 *   - 重提后 risk_level / ai_review_result 被清空
 *   - 超长评论边界条件
 *
 * 隔离：工单 change_reason 使用 e2e_test_ 前缀
 * 前置：docker-compose.test.yml 环境，e2eadmin 账号可用
 */
import { test, expect, BASE_URL, ADMIN_USER, ADMIN_PASS, loginViaApi, getFirstDatasourceId, apiHelper } from '../support/real-test-helpers'
import type { Page } from '@playwright/test'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

const E2E_PREFIX = `e2e_test_${Date.now()}_`

/** Create a ticket via API. */
async function createTicket(page: Page, datasourceId: number, sql: string, reason: string): Promise<number> {
  const { status, data } = await apiHelper(page, 'POST', '/tickets', {
    datasource_id: datasourceId,
    database: 'testdb',
    sql,
    db_type: 'mysql',
    change_reason: reason,
  })
  expect(status).toBe(200)
  const body = data as { code: number; data: { id: number } }
  expect(body.code).toBe(0)
  return body.data.id
}

/** Get ticket by ID. */
async function getTicket(page: Page, ticketId: number) {
  const { status, data } = await apiHelper(page, 'GET', `/tickets/${ticketId}`)
  expect(status).toBe(200)
  return data as { code: number; data: Record<string, unknown> }
}

/** Reject a ticket. */
async function rejectTicket(page: Page, ticketId: number, reason: string) {
  const { status, data } = await apiHelper(page, 'POST', `/tickets/${ticketId}/reject`, { comment: reason })
  expect(status).toBe(200)
  return data as { code: number }
}

/** Approve a ticket. */
async function approveTicket(page: Page, ticketId: number, comment = 'approved') {
  const { status, data } = await apiHelper(page, 'POST', `/tickets/${ticketId}/approve`, { comment })
  expect(status).toBe(200)
  return data as { code: number }
}

/** Resubmit a ticket. */
async function resubmitTicket(page: Page, ticketId: number, sql: string, reason: string) {
  const { status, data } = await apiHelper(page, 'PUT', `/tickets/${ticketId}/resubmit`, {
    sql,
    change_reason: reason,
  })
  return { status, data: data as Record<string, unknown> }
}

/** Get revisions for a ticket. */
async function getRevisions(page: Page, ticketId: number): Promise<Array<Record<string, unknown>>> {
  const { status, data } = await apiHelper(page, 'GET', `/tickets/${ticketId}/revisions`)
  expect(status).toBe(200)
  return data as unknown as Array<Record<string, unknown>>
}

// --- Tests ---

test.describe('驳回重提流程', () => {
  let page: Page
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id
  })

  test.afterAll(async () => {
    try { await page.context().close() } catch { /* ignore */ }
  })

  test.beforeEach(async () => {
    // page 已在 beforeAll 中 login
  })

  // ── 完整驳回重提流程 ──────────────────────────────────────────────────

  test('完整流程：创建 → 驳回 → 修改 SQL → 重提 → 重新审批通过', async () => {
    const originalSql = `DELETE FROM orders WHERE created_at < '2025-01-01'`
    const revisedSql = `DELETE FROM orders WHERE created_at < '2025-01-01' AND status = 'expired'`

    // 1. 创建工单
    const ticketId = await createTicket(page, datasourceId, originalSql, `${E2E_PREFIX}resubmit full flow`)

    // 2. 驳回
    await rejectTicket(page, ticketId, '缺少 WHERE 条件，数据安全风险')

    // 3. 验证状态为 REJECTED
    const rejected = await getTicket(page, ticketId)
    expect(rejected.data.status).toBe('REJECTED')
    const originalRevision = rejected.data.revision as number

    // 4. 重提（修改 SQL）
    const { status: resubStatus, data: resubData } = await resubmitTicket(
      page, ticketId, revisedSql, `${E2E_PREFIX}修复 WHERE 条件后重提`,
    )
    // resubmit 端点使用 PUT
    expect([200, 201]).toContain(resubStatus)

    // 5. 验证 revision 递增
    const resubmitted = await getTicket(page, ticketId)
    expect(resubmitted.data.revision as number).toBe(originalRevision + 1)
    expect(resubmitted.data.status).toBe('SUBMITTED')
    // risk_level 和 ai_review_result 应被清空
    expect(resubmitted.data.risk_level).toBeFalsy()
    expect(resubmitted.data.ai_review_result).toBeFalsy()

    // 6. 重新审批通过
    await approveTicket(page, ticketId, '修复后审批通过')

    const approved = await getTicket(page, ticketId)
    expect(approved.data.status).toBe('APPROVED')
  })

  // ── revision 递增验证 ──────────────────────────────────────────────────

  test('revision 递增 — 多次驳回重提 revision 持续递增', async () => {
    const ticketId = await createTicket(page, datasourceId, 'SELECT 1', `${E2E_PREFIX}revision counter`)

    // 第一次驳回 + 重提
    await rejectTicket(page, ticketId, '第一次驳回')
    await resubmitTicket(page, ticketId, 'SELECT 2', '第一次重提')
    const t1 = await getTicket(page, ticketId)
    expect(t1.data.revision).toBe(1)

    // 第二次驳回 + 重提
    await rejectTicket(page, ticketId, '第二次驳回')
    await resubmitTicket(page, ticketId, 'SELECT 3', '第二次重提')
    const t2 = await getTicket(page, ticketId)
    expect(t2.data.revision).toBe(2)

    // 第三次驳回 + 重提
    await rejectTicket(page, ticketId, '第三次驳回')
    await resubmitTicket(page, ticketId, 'SELECT 4', '第三次重提')
    const t3 = await getTicket(page, ticketId)
    expect(t3.data.revision).toBe(3)
  })

  // ── 历史版本查询 ─────────────────────────────────────────────────────

  test('历史版本 — 驳回后重提产生 revision 快照', async () => {
    const sql1 = 'CREATE TABLE e2e_rev_test (id INT)'
    const sql2 = 'CREATE TABLE e2e_rev_test (id INT, name VARCHAR(100))'

    const ticketId = await createTicket(page, datasourceId, sql1, `${E2E_PREFIX}revision snapshot`)

    // 驳回
    await rejectTicket(page, ticketId, '需要加 name 字段')

    // 重提
    await resubmitTicket(page, ticketId, sql2, '添加 name 字段')

    // 查询 revisions
    const revisions = await getRevisions(page, ticketId)
    // 应有 1 个历史版本（被驳回的原始版本）
    expect(revisions.length).toBeGreaterThanOrEqual(1)

    // 验证历史版本内容
    const rev0 = revisions[0]
    expect(rev0.sql_content).toBe(sql1)
    expect(rev0.status).toBe('REJECTED')
  })

  test('历史版本 — 多次驳回重提保留完整历史', async () => {
    const sqls = [
      'ALTER TABLE e2e_rev ADD COLUMN col1 INT',
      'ALTER TABLE e2e_rev ADD COLUMN col2 VARCHAR(50)',
      'ALTER TABLE e2e_rev ADD COLUMN col3 TEXT',
    ]

    const ticketId = await createTicket(page, datasourceId, sqls[0], `${E2E_PREFIX}multi revision`)

    // 两次驳回重提
    await rejectTicket(page, ticketId, 'review 1')
    await resubmitTicket(page, ticketId, sqls[1], 'revision 2')
    await rejectTicket(page, ticketId, 'review 2')
    await resubmitTicket(page, ticketId, sqls[2], 'revision 3')

    const revisions = await getRevisions(page, ticketId)
    // 应有 2 个历史版本（两次被驳回的版本）
    expect(revisions.length).toBeGreaterThanOrEqual(2)

    // 按时间排序，最早的版本在前
    expect(revisions[0].sql_content).toBe(sqls[0])
    expect(revisions[1].sql_content).toBe(sqls[1])
  })

  // ── 非法状态拦截 ─────────────────────────────────────────────────────

  test('非法状态 — SUBMITTED 状态不能重提', async () => {
    const ticketId = await createTicket(page, datasourceId, 'SELECT 1', `${E2E_PREFIX}resubmit blocked`)

    // SUBMITTED 状态重提应失败
    const { status } = await resubmitTicket(page, ticketId, 'SELECT 2', 'should fail')
    expect(status).toBe(400)
  })

  test('非法状态 — APPROVED 状态不能重提', async () => {
    const ticketId = await createTicket(page, datasourceId, 'SELECT 1', `${E2E_PREFIX}resubmit approved`)

    // 先审批通过
    await approveTicket(page, ticketId)

    // APPROVED 状态重提应失败
    const { status } = await resubmitTicket(page, ticketId, 'SELECT 2', 'should fail')
    expect(status).toBe(400)
  })

  // ── 边界条件 ──────────────────────────────────────────────────────────

  test('重提 — 空内容重提应失败', async () => {
    const ticketId = await createTicket(page, datasourceId, 'SELECT 1', `${E2E_PREFIX}empty resubmit`)

    await rejectTicket(page, ticketId, '驳回')
    const ticket = await getTicket(page, ticketId)
    expect(ticket.data.status).toBe('REJECTED')

    // 空内容重提应失败
    const { status } = await resubmitTicket(page, ticketId, '', 'empty sql')
    expect(status).toBe(400)
  })

  test('重提 — 超长评论不拦截（验证后端能处理长字符串）', async () => {
    const ticketId = await createTicket(page, datasourceId, 'SELECT 1', `${E2E_PREFIX}long comment`)

    await rejectTicket(page, ticketId, '驳回')

    const longComment = '这是一条很长的重提原因'.repeat(100) // ~3000 chars
    const { status } = await resubmitTicket(page, ticketId, 'SELECT 1', longComment)
    // 后端不应因评论长度失败
    expect([200, 201]).toContain(status)

    const ticket = await getTicket(page, ticketId)
    expect(ticket.data.change_reason).toBe(longComment)
  })
})
