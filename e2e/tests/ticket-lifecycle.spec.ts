/**
 * ticket-lifecycle.spec.ts
 *
 * E2E 工单生命周期状态转换 UI 测试 (SF-QA0036)
 *
 * P0 核心业务流程：验证工单从提交到执行完成的 UI 状态转换链。
 *
 * 测试范围：
 *   - 创建工单 → 列表出现 → 状态「待审批」
 *   - 打开详情抽屉 → 审批通过 → 状态「已通过」
 *   - 执行 → 状态「已完成」
 *   - 完整链路：提交→审批→执行
 *   - 驳回链路：提交→驳回→列表状态同步
 *   - 取消链路：提交→取消
 *
 * 前置：docker-compose.test.yml 环境，e2e-admin 账号可用
 */
import { test, expect, loginViaApi, getFirstDatasourceId } from '../support/real-test-helpers'
import {
  E2E_PREFIX,
  createTicketViaAPI,
  getTicketViaAPI,
  executeTicketAndWait,
  cleanupTable,
  navigateToTickets,
  openTicketDrawer,
  expectDrawerStatus,
  closeDrawer,
  type Page,
} from '../support/approval-helpers'

test.describe.configure({ timeout: 45_000 })

const E2E_TABLE = `e2e_lifecycle_${Date.now()}`

test.describe('工单生命周期状态转换 UI', () => {
  let page: Page
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id
    await cleanupTable(page, datasourceId, E2E_TABLE)
    await ctx.close()
  })

  test.afterAll(async ({ browser }) => {
    try {
      const ctx = await browser.newContext()
      const page = await ctx.newPage()
      await loginViaApi(page)
      await cleanupTable(page, datasourceId, E2E_TABLE)
      await ctx.close()
    } catch { /* best-effort */ }
  })

  test.beforeEach(async ({ page }) => {
    await loginViaApi(page)
  })

  // ── 创建工单 → 列表状态 ──────────────────────────────────────────────

  test('创建工单后列表显示「待审批」状态 badge', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      `SELECT 1 AS lifecycle_test`,
      `${E2E_PREFIX} lifecycle create`,
    )

    await navigateToTickets(page)

    // 工单行出现
    const row = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await row.waitFor({ state: 'visible', timeout: 10_000 })

    // 验证状态 badge
    await expect(row.getByText(/待审批|PENDING_APPROVAL/)).toBeVisible()
  })

  // ── 审批通过 → 状态变化 ─────────────────────────────────────────────

  test('审批通过 → 抽屉内状态 badge 变为「已通过」', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      `SELECT 1 AS approve_lifecycle`,
      `${E2E_PREFIX} lifecycle approve`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    // 初始状态：待审批
    await expectDrawerStatus(page, /待审批|PENDING_APPROVAL/)

    // 点击「通过」按钮
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.getByRole('button', { name: '通过' }).click()

    // 审批对话框出现
    const dialog = page.getByRole('alertdialog').or(page.getByRole('dialog'))
    await expect(dialog.getByText(/确认通过|审批通过/)).toBeVisible()

    // 填写备注
    await page.getByPlaceholder(/审批备注|填写/).first().fill('LGTM')

    // 确认
    await dialog.getByRole('button', { name: '确认通过' }).click()

    // Toast 出现
    await expect(page.getByText(/审批通过|已通过/)).toBeVisible({ timeout: 10_000 })

    // 状态变为已通过
    await expectDrawerStatus(page, /已通过|APPROVED/)
  })

  // ── 执行 → 状态变化 ──────────────────────────────────────────────────

  test('审批通过后执行 → 状态变为「已完成」', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      `CREATE TABLE ${E2E_TABLE}_exec (id INT PRIMARY KEY)`,
      `${E2E_PREFIX} lifecycle execute`,
    )

    // 先审批通过
    const { apiHelper } = await import('../support/real-test-helpers')
    await apiHelper(page, 'POST', `/tickets/${ticketId}/approve`, { comment: 'auto approve' })

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    // 应该看到执行按钮
    const sheet = page.locator('[data-slot="sheet-content"]')
    const execBtn = sheet.getByRole('button', { name: '执行' })
    await expect(execBtn).toBeVisible()

    // 点击执行
    await execBtn.click()

    // 执行确认对话框
    const dialog = page.getByRole('alertdialog').or(page.getByRole('dialog'))
    await expect(dialog.getByText(/确认执行|工单.*SQL/)).toBeVisible()
    await dialog.getByRole('button', { name: '确认执行' }).click()

    // 等待执行完成（poll API）
    const start = Date.now()
    while (Date.now() - start < 30_000) {
      const ticket = await getTicketViaAPI(page, ticketId)
      if (ticket.status === 'DONE') break
      if (['REJECTED', 'CANCELLED'].includes(ticket.status as string)) break
      await page.waitForTimeout(1_000)
    }

    // 关闭再重新打开抽屉验证状态
    await closeDrawer(page)
    await openTicketDrawer(page, ticketId)
    await expectDrawerStatus(page, /已完成|DONE/)
  })

  // ── 驳回链路 ──────────────────────────────────────────────────────────

  test('驳回 → 抽屉内状态变为「已拒绝」，拒绝按钮消失', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      `DELETE FROM orders WHERE id = 1`,
      `${E2E_PREFIX} lifecycle reject`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')

    // 点击拒绝
    await sheet.getByRole('button', { name: '拒绝' }).click()

    const dialog = page.getByRole('alertdialog').or(page.getByRole('dialog'))
    await dialog.getByPlaceholder(/原因/).fill('SQL 风险太高')
    await dialog.getByRole('button', { name: '确认驳回' }).click()

    // Toast
    await expect(page.getByText(/已驳回|拒绝成功/)).toBeVisible({ timeout: 10_000 })

    // 状态变为已拒绝
    await expectDrawerStatus(page, /已拒绝|REJECTED/)

    // 通过按钮和执行按钮应该不可见
    await expect(sheet.getByRole('button', { name: '通过' })).not.toBeVisible()
    await expect(sheet.getByRole('button', { name: '执行' })).not.toBeVisible()
  })

  // ── 取消链路 ──────────────────────────────────────────────────────────

  test('取消工单 → 状态变为「已取消」', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      `SELECT 1 AS cancel_test`,
      `${E2E_PREFIX} lifecycle cancel`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')

    // 点击取消工单
    const cancelBtn = sheet.getByRole('button', { name: '取消工单' })
    await expect(cancelBtn).toBeVisible()
    await cancelBtn.click()

    const dialog = page.getByRole('alertdialog').or(page.getByRole('dialog'))
    await dialog.getByPlaceholder(/原因/).fill('需求变更')
    await dialog.getByRole('button', { name: '确认取消' }).click()

    // Toast
    await expect(page.getByText(/已取消|取消成功/)).toBeVisible({ timeout: 10_000 })

    // 状态
    await expectDrawerStatus(page, /已取消|CANCELLED/)
  })

  // ── 列表页与抽屉状态同步 ──────────────────────────────────────────────

  test('列表页状态 badge 与抽屉状态一致', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      `SELECT 1 AS sync_test`,
      `${E2E_PREFIX} lifecycle sync`,
    )

    await navigateToTickets(page)

    // 列表中状态
    const row = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await row.waitFor({ state: 'visible', timeout: 10_000 })
    await expect(row.getByText(/待审批|PENDING_APPROVAL/)).toBeVisible()

    // 打开抽屉验证一致
    await row.click()
    await expectDrawerStatus(page, /待审批|PENDING_APPROVAL/)
  })
})
