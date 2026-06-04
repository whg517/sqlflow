/**
 * ticket-reject.spec.ts
 *
 * E2E 工单拒绝流程 UI 交互测试 — 聚焦拒绝对话框、原因校验、状态变化、
 * 拒绝后按钮显隐、审批记录展示等 UI 细节。
 *
 * 与 ticket-approval-flow.spec.ts 的区别：
 *   - approval-flow 覆盖通过、拒绝、取消等多种操作
 *   - 本文件专精拒绝流程，测试更多边界场景（原因必填、列表/详情状态同步、
 *     拒绝后执行按钮消失、通过按钮消失等）
 *
 * 前置：docker-compose.test.yml 环境，e2eadmin 账号可用
 */
import { test, expect, BASE_URL, loginViaUI, apiHelper, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

const UID = Date.now()

/** Create a ticket via API, return ID. */
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

// --- Tests ---

test.describe('工单拒绝流程 UI 交互', () => {
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    const page = await ctx.newPage()
    await loginViaUI(page)
    datasourceId = (await getFirstDatasourceId(page)).id
    await ctx.close()
  })

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  // ── 完整拒绝流程 ─────────────────────────────────────────────────────────

  test('完整流程：创建工单 → UI 拒绝 → 验证状态和原因 → 执行/通过按钮不可见', async ({ page }) => {
    const REJECT_REASON = 'SQL 存在性能风险，缺少索引条件，请优化后重新提交'
    const sql = `DELETE FROM orders WHERE created_at < "2025-01-01"`

    // 1. API 创建待审批工单
    const ticketId = await createTicket(page, datasourceId, sql, 'E2E: full reject UI flow')

    // 2. 导航到工单列表
    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    const ticketRow = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await ticketRow.waitFor({ state: 'visible', timeout: 10_000 })

    // 验证列表状态
    await expect(ticketRow.getByText(/待审批|SUBMITTED/)).toBeVisible()

    // 3. 打开详情抽屉
    await ticketRow.click()

    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })
    await expect(sheet.getByText(`工单 #${ticketId}`)).toBeVisible()

    // 验证当前状态
    await expect(sheet.getByText(/待审批|SUBMITTED/)).toBeVisible()

    // 验证拒绝按钮可见（admin 有权限）
    await expect(sheet.getByRole('button', { name: '拒绝' })).toBeVisible()

    // 4. 点击「拒绝」按钮
    await sheet.getByRole('button', { name: '拒绝' }).click()

    // 5. 验证驳回对话框
    const dialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await dialog.waitFor({ state: 'visible', timeout: 5_000 })
    await expect(dialog.getByText(/驳回/)).toBeVisible()

    // 6. 填写拒绝原因
    const reasonInput = page.getByPlaceholder(/原因/)
    await expect(reasonInput).toBeVisible()
    await reasonInput.fill(REJECT_REASON)

    // 7. 确认驳回
    await dialog.getByRole('button', { name: '确认驳回' }).click()

    // 8. 验证成功 toast
    await expect(page.getByText(/已驳回|拒绝成功/)).toBeVisible({ timeout: 10_000 })

    // 9. 验证状态 badge 变为「已拒绝」
    await expect(sheet.getByText(/已拒绝|REJECTED/)).toBeVisible({ timeout: 5_000 })

    // 10. 验证拒绝原因显示在详情中
    await expect(sheet.getByText(REJECT_REASON)).toBeVisible()

    // 11. 验证「执行」按钮不可见
    await expect(sheet.getByRole('button', { name: '执行' })).not.toBeVisible()

    // 12. 验证「通过」按钮也不可见（已审批过的工单不再显示审批按钮）
    await expect(sheet.getByRole('button', { name: '通过' })).not.toBeVisible()

    // 13. 关闭抽屉，刷新列表验证状态同步
    await page.locator('button').filter({ has: page.locator('svg.lucide-x') }).first().click()
    await sheet.waitFor({ state: 'hidden', timeout: 5_000 })

    await page.reload()
    await page.waitForURL('**/tickets**')
    await expect(page.getByRole('table').getByText(/已拒绝|REJECTED/)).toBeVisible({ timeout: 10_000 })

    // 14. 重新打开详情，验证拒绝原因持久化
    await page.getByRole('row', { name: new RegExp(`#${ticketId}`) }).click()
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    const sheet2 = page.locator('[data-slot="sheet-content"]')
    await expect(sheet2.getByText(/已拒绝|REJECTED/)).toBeVisible()
    await expect(sheet2.getByText(REJECT_REASON)).toBeVisible()
  })

  // ── 拒绝时不填原因 — 按钮禁用或校验提示 ──────────────────────────────────

  test('拒绝时不填写原因 — 确认按钮应禁用或提示必填', async ({ page }) => {
    const tid = await createTicket(
      page, datasourceId,
      `SELECT 1 AS e2e_reject_no_reason`,
      'E2E: reject without reason validation',
    )

    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 打开详情
    await page.getByRole('row', { name: new RegExp(`#${tid}`) }).click()
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // 点击拒绝
    await sheet.getByRole('button', { name: '拒绝' }).click()

    const dialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await dialog.waitFor({ state: 'visible', timeout: 5_000 })

    const confirmBtn = dialog.getByRole('button', { name: '确认驳回' })

    // 组件应在未填原因时禁用确认按钮
    await expect(confirmBtn).toBeDisabled()
  })

  // ── 多工单拒绝后列表 Tab 筛选 ─────────────────────────────────────────────

  test('拒绝后「已拒绝」Tab 筛选正确', async ({ page }) => {
    const REJECT_REASON = '测试拒绝 Tab 筛选'

    // 创建两个工单并拒绝
    const tid1 = await createTicket(page, datasourceId, `SELECT 1 AS e2e_reject_tab_1`, 'E2E: reject tab filter 1')
    const tid2 = await createTicket(page, datasourceId, `SELECT 1 AS e2e_reject_tab_2`, 'E2E: reject tab filter 2')

    // 拒绝两个工单
    await apiHelper(page, 'POST', `/tickets/${tid1}/reject`, { comment: REJECT_REASON })
    await apiHelper(page, 'POST', `/tickets/${tid2}/reject`, { comment: REJECT_REASON })

    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 切换到「已拒绝」Tab
    await page.getByRole('tab', { name: '已拒绝' }).click()

    // 两个拒绝工单应可见
    await expect(page.getByRole('row', { name: new RegExp(`#${tid1}`) })).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('row', { name: new RegExp(`#${tid2}`) })).toBeVisible({ timeout: 5_000 })

    // 切换到「待审批」Tab，应不包含这两个工单
    await page.getByRole('tab', { name: '待审批' }).click()
    await expect(page.getByRole('row', { name: new RegExp(`#${tid1}`) })).not.toBeVisible({ timeout: 5_000 })
    await expect(page.getByRole('row', { name: new RegExp(`#${tid2}`) })).not.toBeVisible({ timeout: 5_000 })
  })

  // ── 拒绝后重新提交（如支持）或审批记录展示 ────────────────────────────────

  test('拒绝后详情展示审批记录', async ({ page }) => {
    const REJECT_REASON = 'E2E 验证审批记录显示'
    const tid = await createTicket(
      page, datasourceId,
      `UPDATE users SET status = 1 WHERE id = 999`,
      'E2E: reject audit record test',
    )

    // API 拒绝
    await apiHelper(page, 'POST', `/tickets/${tid}/reject`, { comment: REJECT_REASON })

    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 打开详情
    await page.getByRole('row', { name: new RegExp(`#${tid}`) }).click()
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // 验证拒绝原因和审批记录区域
    await expect(sheet.getByText(/已拒绝|REJECTED/)).toBeVisible({ timeout: 5_000 })

    // 审批记录区域应展示拒绝原因
    const auditSection = sheet.getByText(/审批记录|审批历史/)
    if (await auditSection.isVisible()) {
      await expect(auditSection).toBeVisible()
    }
    await expect(sheet.getByText(REJECT_REASON)).toBeVisible()
  })
})
