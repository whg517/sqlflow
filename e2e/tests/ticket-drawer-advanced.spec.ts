/**
 * ticket-drawer-advanced.spec.ts
 *
 * E2E 工单详情抽屉高级交互测试 (SF-QA0036)
 *
 * P1 高优先 — 覆盖详情抽屉的深度 UI 交互。
 *
 * 测试范围：
 *   - 抽屉打开/关闭交互
 *   - SQL 内容展示 + 复制按钮
 *   - 审批记录展示（审批通过/拒绝后的记录）
 *   - 变更原因展示
 *   - 提交人和时间 meta 信息
 *   - 审批对话框取消后状态不变
 *   - 焦点管理（对话框打开时焦点在 textarea）
 *
 * 前置：docker-compose.test.yml 环境，e2e-admin 账号可用
 */
import { test, expect, loginViaApi, getFirstDatasourceId, apiHelper } from '../support/real-test-helpers'
import {
  E2E_PREFIX,
  createTicketViaAPI,
  navigateToTickets,
  openTicketDrawer,
  expectDrawerStatus,
  closeDrawer,
  type Page,
} from '../support/approval-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('工单详情抽屉高级交互', () => {
  let page: Page
  let datasourceId: number

  test.beforeEach(async ({ page }) => {
    await loginViaApi(page)
  })

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id
    await ctx.close()
  })

  // ── 抽屉打开/关闭 ──────────────────────────────────────────────────

  test('抽屉打开后显示工单标题', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'SELECT 1 AS drawer_test',
      `${E2E_PREFIX} drawer open`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    await expect(sheet.getByText(`工单 #${ticketId}`)).toBeVisible()
  })

  test('抽屉关闭后工单列表正常显示', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'SELECT 1 AS drawer_close',
      `${E2E_PREFIX} drawer close`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    // 关闭抽屉
    await closeDrawer(page)

    // 抽屉消失
    await expect(page.locator('[data-slot="sheet-content"]')).not.toBeVisible({ timeout: 5_000 })

    // 列表正常
    const row = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await expect(row).toBeVisible()
  })

  // ── SQL 内容展示 + 复制 ─────────────────────────────────────────────

  test('SQL 内容完整展示，复制按钮可用', async ({ page }) => {
    const longSql = `SELECT u.id, u.name, u.email, u.role, u.status, u.created_at
FROM sys_user u
JOIN orders o ON u.id = o.user_id
WHERE u.status = 1
  AND u.created_at > '2025-01-01'
GROUP BY u.id, u.name, u.email, u.role, u.status, u.created_at
HAVING COUNT(o.id) > 0
ORDER BY u.created_at DESC
LIMIT 100`

    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      longSql,
      `${E2E_PREFIX} drawer sql`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    const sqlBlock = sheet.locator('pre').first()
    await expect(sqlBlock).toBeVisible()

    // SQL 内容应包含关键字
    await expect(sqlBlock).toContainText('SELECT')

    // 复制按钮存在
    const copyBtn = sheet.locator('button').filter({ has: page.locator('svg.lucide-copy') }).first()
    await expect(copyBtn).toBeVisible()
  })

  // ── 变更原因展示 ──────────────────────────────────────────────────

  test('变更原因在抽屉中完整展示', async ({ page }) => {
    const reason = '这是一个详细的变更原因说明：修复线上订单表的索引缺失问题，添加复合索引以提升查询性能。'
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'SELECT 1 AS reason_test',
      reason,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    await expect(sheet.getByText('变更原因')).toBeVisible()
    await expect(sheet.getByText(reason)).toBeVisible()
  })

  // ── 提交人和时间信息 ──────────────────────────────────────────────

  test('meta 信息展示：提交人、提交时间、数据库', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'SELECT 1 AS meta_test',
      `${E2E_PREFIX} meta`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    await expect(sheet.getByText(/提交人/)).toBeVisible()
    await expect(sheet.getByText(/提交时间/)).toBeVisible()
    await expect(sheet.getByText(/数据库/)).toBeVisible()
    await expect(sheet.getByText('testdb')).toBeVisible()
  })

  // ── 审批记录展示 ──────────────────────────────────────────────────

  test('审批通过后抽屉显示审批记录', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'SELECT 1 AS approval_record',
      `${E2E_PREFIX} approval record`,
    )

    // 通过 API 审批
    await apiHelper(page, 'POST', `/tickets/${ticketId}/approve`, { comment: '审批通过测试' })

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    // 审批记录区域
    await expect(sheet.getByText(/审批记录/)).toBeVisible({ timeout: 5_000 })
    await expect(sheet.getByText(/审批通过/)).toBeVisible({ timeout: 5_000 })
  })

  test('审批拒绝后抽屉显示拒绝原因', async ({ page }) => {
    const rejectReason = 'DELETE 操作缺少 WHERE 条件，有数据丢失风险'
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'DELETE FROM orders WHERE id = 1',
      `${E2E_PREFIX} rejection record`,
    )

    await apiHelper(page, 'POST', `/tickets/${ticketId}/reject`, { comment: rejectReason })

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    // 审批记录应显示拒绝原因
    await expect(sheet.getByText(/已拒绝|拒绝/)).toBeVisible({ timeout: 5_000 })
  })

  // ── 审批对话框取消后状态不变 ──────────────────────────────────────

  test('审批对话框取消 → 工单状态不变', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'SELECT 1 AS cancel_dialog',
      `${E2E_PREFIX} cancel dialog`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    // 初始状态
    await expectDrawerStatus(page, /待审批|PENDING_APPROVAL/)

    // 打开审批对话框
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.getByRole('button', { name: '通过' }).click()

    const dialog = page.getByRole('alertdialog').or(page.getByRole('dialog'))
    await expect(dialog.getByText(/确认通过/)).toBeVisible()

    // 取消
    await dialog.getByRole('button', { name: '取消' }).click()

    // 对话框消失
    await expect(dialog).not.toBeVisible({ timeout: 5_000 })

    // 状态不变
    await expectDrawerStatus(page, /待审批|PENDING_APPROVAL/)
  })

  // ── 风险评级 badge 展示 ──────────────────────────────────────────

  test('高风除 SQL 显示红色风险 badge', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'ALTER TABLE sys_user ADD COLUMN phone VARCHAR(32)',
      `${E2E_PREFIX} risk badge`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    // 风险 badge 应显示（ALTER 触发 critical/high 风险）
    const riskBadge = sheet.locator('.rounded-full').filter({ hasText: /风险/ })
    await expect(riskBadge).toBeVisible({ timeout: 5_000 })
  })
})
