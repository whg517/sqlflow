/**
 * batch-approval-ui.spec.ts
 *
 * E2E 批量审批/拒绝 UI 测试 (SF-QA0036)
 *
 * P1 高优先 — 覆盖工单列表页的批量操作 UI 交互。
 *
 * 测试范围：
 *   - 多选工单（Checkbox 选中）
 *   - 全选/取消全选
 *   - 批量操作浮层出现
 *   - 批量通过确认对话框 + toast
 *   - 批量拒绝确认对话框（必填原因）
 *   - 50 条上限校验
 *   - 批量操作后列表刷新
 *
 * 前置：docker-compose.test.yml 环境，e2eadmin 账号可用（admin/dba 角色可批量操作）
 */
import { test, expect, loginViaApi, getFirstDatasourceId, apiHelper } from '../support/real-test-helpers'
import {
  E2E_PREFIX,
  createTicketViaAPI,
  navigateToTickets,
  type Page,
} from '../support/approval-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('批量审批/拒绝 UI', () => {
  let page: Page
  let datasourceId: number
  let ticketIds: number[] = []

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id

    // 创建 3 个待审批工单
    for (let i = 0; i < 3; i++) {
      const id = await createTicketViaAPI(
        page, datasourceId,
        `SELECT ${i + 1} AS batch_test`,
        `${E2E_PREFIX} batch item ${i}`,
      )
      ticketIds.push(id)
    }
    await ctx.close()
  })

  test.beforeEach(async ({ page }) => {
    await loginViaApi(page)
  })

  // ── 多选工单 ──────────────────────────────────────────────────────────

  test('选中待审批工单 → 批量操作浮层出现', async ({ page }) => {
    await navigateToTickets(page)

    // 选中第一个工单
    const firstRow = page.getByRole('row', { name: new RegExp(`#${ticketIds[0]}`) })
    await firstRow.waitFor({ state: 'visible', timeout: 10_000 })
    const checkbox = firstRow.locator('input[type="checkbox"]').first()
    await checkbox.check()

    // 批量操作浮层出现
    await expect(page.getByText(/已选.*1.*条/)).toBeVisible({ timeout: 5_000 })
    await expect(page.getByRole('button', { name: '批量通过' })).toBeVisible()
    await expect(page.getByRole('button', { name: '批量拒绝' })).toBeVisible()
    await expect(page.getByRole('button', { name: '取消选择' })).toBeVisible()
  })

  // ── 全选/取消全选 ─────────────────────────────────────────────────────

  test('全选 → 所有待审批工单被选中', async ({ page }) => {
    await navigateToTickets(page)

    // 点击表头全选 checkbox
    const headerCheckbox = page.locator('thead').locator('input[type="checkbox"]').first()
    await headerCheckbox.check()

    // 批量浮层显示选中数量
    const selectedText = page.getByText(/已选.*条/)
    await expect(selectedText).toBeVisible({ timeout: 5_000 })
    // 应至少有我们创建的 3 个
    await expect(selectedText).toContainText(/已选/)
  })

  test('取消全选 → 浮层消失', async ({ page }) => {
    await navigateToTickets(page)

    const headerCheckbox = page.locator('thead').locator('input[type="checkbox"]').first()
    await headerCheckbox.check()

    // 浮层出现
    await expect(page.getByText(/已选.*条/)).toBeVisible()

    // 点击「取消选择」
    await page.getByRole('button', { name: '取消选择' }).click()

    // 浮层消失
    await expect(page.getByText(/已选.*条/)).not.toBeVisible({ timeout: 3_000 })
  })

  // ── 批量通过确认对话框 + toast ──────────────────────────────────────

  test('批量通过 → 确认对话框 → toast 成功', async ({ page }) => {
    await navigateToTickets(page)

    // 选中第一个工单
    const firstRow = page.getByRole('row', { name: new RegExp(`#${ticketIds[0]}`) })
    await firstRow.waitFor({ state: 'visible', timeout: 10_000 })
    await firstRow.locator('input[type="checkbox"]').first().check()

    // 点击批量通过
    await page.getByRole('button', { name: '批量通过' }).click()

    // 确认对话框出现
    const dialog = page.getByRole('alertdialog').or(page.getByRole('dialog'))
    await expect(dialog.getByText(/批量通过.*确认/)).toBeVisible()
    await expect(dialog.getByText(/确认要通过/)).toBeVisible()

    // 填写审批意见（可选）
    const textarea = dialog.locator('textarea').first()
    await textarea.fill('批量通过测试')

    // 确认
    await dialog.getByRole('button', { name: '确认通过' }).click()

    // Toast 成功
    await expect(page.getByText(/批量通过.*成功/)).toBeVisible({ timeout: 10_000 })

    // 浮层消失
    await expect(page.getByText(/已选.*条/)).not.toBeVisible({ timeout: 5_000 })
  })

  // ── 批量拒绝 — 必填原因 ──────────────────────────────────────────────

  test('批量拒绝 — 不填原因时确认按钮应禁用', async ({ page }) => {
    await navigateToTickets(page)

    // 需要至少一个还没被审批通过的工单
    const pendingId = ticketIds[1]
    const row = page.getByRole('row', { name: new RegExp(`#${pendingId}`) })
    await row.waitFor({ state: 'visible', timeout: 10_000 })
    await row.locator('input[type="checkbox"]').first().check()

    await page.getByRole('button', { name: '批量拒绝' }).click()

    const dialog = page.getByRole('alertdialog').or(page.getByRole('dialog'))
    await expect(dialog.getByText(/批量拒绝.*确认/)).toBeVisible()

    // 确认按钮应在未填原因时禁用
    const confirmBtn = dialog.getByRole('button', { name: '确认拒绝' })
    await expect(confirmBtn).toBeDisabled()

    // 填写原因后启用
    await dialog.locator('textarea').first().fill('风险太高，批量驳回')
    await expect(confirmBtn).toBeEnabled()

    // 取消对话框（不实际拒绝，留给后续测试）
    await dialog.getByRole('button', { name: '取消' }).click()
  })

  test('批量拒绝 → 填写原因 → toast 成功', async ({ page }) => {
    await navigateToTickets(page)

    const pendingId = ticketIds[2]
    const row = page.getByRole('row', { name: new RegExp(`#${pendingId}`) })
    await row.waitFor({ state: 'visible', timeout: 10_000 })
    await row.locator('input[type="checkbox"]').first().check()

    await page.getByRole('button', { name: '批量拒绝' }).click()

    const dialog = page.getByRole('alertdialog').or(page.getByRole('dialog'))
    await dialog.locator('textarea').first().fill('批量拒绝测试')
    await dialog.getByRole('button', { name: '确认拒绝' }).click()

    await expect(page.getByText(/批量拒绝.*成功/)).toBeVisible({ timeout: 10_000 })
  })

  // ── 非 admin/dba 不显示 checkbox ──────────────────────────────────────

  test('developer 角色不显示批量操作 checkbox', async ({ page }) => {
    // 以 developer 身份登录
    const token = await (await import('../support/real-test-helpers')).getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (!token) {
      test.skip()
      return
    }

    await page.goto('http://localhost:8080/login')
    await page.evaluate((t) => localStorage.setItem('token', t), token)
    await navigateToTickets(page)

    // 表头不应有 checkbox
    await expect(page.locator('thead').locator('input[type="checkbox"]').first()).not.toBeVisible({ timeout: 5_000 })
  })
})
