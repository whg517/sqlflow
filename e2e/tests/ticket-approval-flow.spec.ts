/**
 * ticket-approval-flow.spec.ts
 *
 * E2E 工单审批流程 UI 交互测试 — 聚焦详情抽屉、审批按钮、状态 badge、toast 等细节。
 *
 * 与 ticket-flow.spec.ts 的区别：
 *   - ticket-flow 做基础全流程（API 创建→审批→执行→验证数据）
 *   - 本文件聚焦 UI 层：详情抽屉打开/关闭、按钮显隐状态、审批对话框交互、toast 提示
 *
 * 前置：docker-compose.test.yml 环境，e2eadmin 账号可用
 */
import { test, expect, BASE_URL, ADMIN_USER, loginViaUI, apiHelper, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

const UID = Date.now()
const E2E_TABLE = `e2e_appr_${UID}`

/** Create a DDL ticket via API and return the created ticket ID. */
async function createTicket(
  page: import('@playwright/test').Page,
  datasourceId: number,
  sql: string,
  reason: string,
): Promise<number> {
  const { status, data } = await apiHelper(page, 'POST', '/tickets', {
    datasource_id: datasourceId,
    database: 'testdb',
    sql,
    db_type: 'mysql',
    change_reason: reason,
  })
  expect(status).toBeLessThan(300)
  const body = data as { code: number; data: { id: number } }
  expect(body.code).toBe(0)
  return body.data.id
}

/** Approve a ticket via API. */
async function approveTicket(page: import('@playwright/test').Page, ticketId: number, comment = 'E2E auto approve') {
  const { status, data } = await apiHelper(page, 'POST', `/tickets/${ticketId}/approve`, { comment })
  expect(status).toBeLessThan(300)
  const body = data as { code: number }
  expect(body.code).toBe(0)
}

/** Reject a ticket via API. */
async function rejectTicket(page: import('@playwright/test').Page, ticketId: number, reason: string) {
  const { status, data } = await apiHelper(page, 'POST', `/tickets/${ticketId}/reject`, { comment: reason })
  expect(status).toBeLessThan(300)
  const body = data as { code: number }
  expect(body.code).toBe(0)
}

/** Execute a ticket via API and poll until DONE. */
async function executeTicketAndWait(page: import('@playwright/test').Page, ticketId: number) {
  await apiHelper(page, 'POST', `/tickets/${ticketId}/execute`, {})
  const start = Date.now()
  while (Date.now() - start < 25_000) {
    const { data } = await apiHelper(page, 'GET', `/tickets/${ticketId}`)
    const body = data as { code: number; data: { status: string } }
    if (body.data?.status === 'DONE') return
    if (['REJECTED', 'CANCELLED'].includes(body.data?.status)) {
      throw new Error(`Ticket ${ticketId} ended with ${body.data.status}`)
    }
    await page.waitForTimeout(1_000)
  }
  throw new Error(`Ticket ${ticketId} did not reach DONE within 25s`)
}

/** Cancel a ticket via API. */
async function cancelTicket(page: import('@playwright/test').Page, ticketId: number, reason: string) {
  const { status, data } = await apiHelper(page, 'POST', `/tickets/${ticketId}/cancel`, { comment: reason })
  expect(status).toBeLessThan(300)
  const body = data as { code: number }
  expect(body.code).toBe(0)
}

/** Cleanup: DROP TABLE IF EXISTS (best-effort). */
async function cleanupTable(page: import('@playwright/test').Page, datasourceId: number) {
  try {
    await apiHelper(page, 'POST', '/query/execute', {
      datasource_id: datasourceId,
      database: 'testdb',
      sql: `DROP TABLE IF EXISTS ${E2E_TABLE}`,
    })
  } catch { /* ignore */ }
}

// --- Tests ---

test.describe('工单审批流程 UI 交互', () => {
  let datasourceId: number
  let ticketId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    const page = await ctx.newPage()
    await loginViaUI(page)
    datasourceId = (await getFirstDatasourceId(page)).id
    await cleanupTable(page, datasourceId)
    await ctx.close()
  })

  test.afterAll(async ({ browser }) => {
    try {
      const ctx = await browser.newContext()
      const page = await ctx.newPage()
      await loginViaUI(page)
      await cleanupTable(page, datasourceId)
      await ctx.close()
    } catch { /* ignore */ }
  })

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  // ── 审批通过完整 UI 流程 ─────────────────────────────────────────────────

  test('审批通过 — 审批对话框交互、toast、状态 badge 变化', async ({ page }) => {
    // 1. 通过 API 创建待审批工单
    ticketId = await createTicket(
      page, datasourceId,
      `CREATE TABLE ${E2E_TABLE} (id INT PRIMARY KEY, name VARCHAR(100))`,
      'E2E: approval UI flow test',
    )

    // 2. 导航到工单列表
    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 等待工单出现在列表
    const ticketRow = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await ticketRow.waitFor({ state: 'visible', timeout: 10_000 })

    // 验证列表中状态 badge 为「待审批」
    await expect(ticketRow.getByText(/待审批|SUBMITTED/)).toBeVisible()

    // 3. 打开工单详情抽屉
    await ticketRow.click()

    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })
    await expect(sheet.getByText(`工单 #${ticketId}`)).toBeVisible()

    // 验证抽屉内状态 badge
    await expect(sheet.getByText(/待审批|SUBMITTED/)).toBeVisible()

    // 4. 点击「通过」按钮 — 打开审批对话框
    await sheet.getByRole('button', { name: '通过' }).click()

    const dialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await dialog.waitFor({ state: 'visible', timeout: 5_000 })
    await expect(dialog.getByText(/确认通过|审批通过/)).toBeVisible()

    // 5. 填写审批备注
    await page.getByPlaceholder(/审批备注|填写/).first().fill('LGTM, approved via E2E')

    // 6. 确认审批
    await dialog.getByRole('button', { name: '确认通过' }).click()

    // 7. 验证成功 toast
    await expect(page.getByText(/审批通过|已通过/)).toBeVisible({ timeout: 10_000 })

    // 8. 验证抽屉内状态 badge 变为「已通过」
    await expect(sheet.getByText(/已通过|APPROVED/)).toBeVisible({ timeout: 5_000 })

    // 清理：执行工单以便后续测试
    try { await executeTicketAndWait(page, ticketId) } catch { /* ok */ }
  })

  // ── 审批对话框可以取消 ────────────────────────────────────────────────────

  test('审批通过对话框可以取消', async ({ page }) => {
    // 创建待审批工单
    const tid = await createTicket(
      page, datasourceId,
      `SELECT 1 AS e2e_appr_cancel_test`,
      'E2E: approval dialog cancel test',
    )

    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 打开详情
    await page.getByRole('row', { name: new RegExp(`#${tid}`) }).click()
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // 打开审批对话框
    await sheet.getByRole('button', { name: '通过' }).click()
    const dialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await dialog.waitFor({ state: 'visible', timeout: 5_000 })

    // 点击取消
    await page.getByRole('button', { name: '取消' }).first().click()

    // 对话框消失
    await expect(page.getByRole('dialog').or(page.getByRole('alertdialog'))).not.toBeVisible({ timeout: 5_000 })

    // 工单状态不变
    await expect(sheet.getByText(/待审批|SUBMITTED/)).toBeVisible()
  })

  // ── 拒绝流程 UI 交互 ────────────────────────────────────────────────────

  test('拒绝流程 — 对话框交互、必填校验、toast、状态 badge', async ({ page }) => {
    const REJECT_REASON = 'SQL 变更风险过高，需要优化后重新提交'

    const tid = await createTicket(
      page, datasourceId,
      `DELETE FROM orders WHERE created_at < "2025-01-01"`,
      'E2E: reject UI flow test',
    )

    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 打开详情
    await page.getByRole('row', { name: new RegExp(`#${tid}`) }).click()
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // 点击拒绝按钮
    await sheet.getByRole('button', { name: '拒绝' }).click()

    const dialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await dialog.waitFor({ state: 'visible', timeout: 5_000 })
    await expect(dialog.getByText(/驳回/)).toBeVisible()

    // 不填原因直接提交 → 应有校验提示或按钮禁用
    const confirmBtn = dialog.getByRole('button', { name: '确认驳回' })
    // 检查按钮状态：禁用或点击后提示
    if (await confirmBtn.isEnabled()) {
      await confirmBtn.click()
      await expect(page.getByText(/请填写|必填|原因/)).toBeVisible({ timeout: 3_000 })
    } else {
      await expect(confirmBtn).toBeDisabled()
    }

    // 填写拒绝原因
    await page.getByPlaceholder(/原因/).fill(REJECT_REASON)

    // 确认拒绝
    await dialog.getByRole('button', { name: '确认驳回' }).click()

    // 验证成功 toast
    await expect(page.getByText(/已驳回|拒绝成功/)).toBeVisible({ timeout: 10_000 })

    // 验证状态 badge 变为「已拒绝」
    await expect(sheet.getByText(/已拒绝|REJECTED/)).toBeVisible({ timeout: 5_000 })

    // 验证拒绝原因显示在详情中
    await expect(sheet.getByText(REJECT_REASON)).toBeVisible()

    // 验证执行按钮不可见
    await expect(sheet.getByRole('button', { name: '执行' })).not.toBeVisible()
  })

  // ── 取消工单流程 ────────────────────────────────────────────────────────

  test('取消工单 — 对话框交互、toast、状态 badge', async ({ page }) => {
    const tid = await createTicket(
      page, datasourceId,
      `SELECT 1 AS e2e_cancel_test`,
      'E2E: cancel ticket UI flow',
    )

    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 打开详情
    await page.getByRole('row', { name: new RegExp(`#${tid}`) }).click()
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // 点击取消工单
    await sheet.getByRole('button', { name: '取消工单' }).click()

    const dialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await dialog.waitFor({ state: 'visible', timeout: 5_000 })
    await expect(dialog.getByText(/取消工单|确认取消/)).toBeVisible()

    // 填写取消原因
    await page.getByPlaceholder(/原因/).fill('需求变更，不再需要')

    // 确认取消
    await dialog.getByRole('button', { name: '确认取消' }).click()

    // 验证成功 toast
    await expect(page.getByText(/已取消|取消成功/)).toBeVisible({ timeout: 10_000 })

    // 验证状态 badge
    await expect(sheet.getByText(/已取消|CANCELLED/)).toBeVisible({ timeout: 5_000 })
  })

  // ── 详情抽屉 — SQL 内容展示 ─────────────────────────────────────────────

  test('详情抽屉展示工单 SQL 内容和变更原因', async ({ page }) => {
    const sql = `CREATE TABLE ${E2E_TABLE}_detail (id INT, note VARCHAR(200))`
    const reason = 'E2E: verify detail drawer content'

    const tid = await createTicket(page, datasourceId, sql, reason)

    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 打开详情
    await page.getByRole('row', { name: new RegExp(`#${tid}`) }).click()
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // 验证 SQL 内容可见
    await expect(sheet.getByText(/CREATE TABLE/)).toBeVisible()

    // 验证变更原因可见
    await expect(sheet.getByText(reason)).toBeVisible()
  })
})
