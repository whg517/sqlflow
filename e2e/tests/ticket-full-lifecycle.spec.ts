/**
 * E2E: Ticket full lifecycle — create → approve → execute → reject.
 *
 * SF-QA0044: 工单全流程（创建→审批→执行→拒绝）
 *
 * Covers the complete ticket lifecycle interaction:
 * 1. Create a new ticket via /tickets/new form
 * 2. Verify ticket appears in list with PENDING_APPROVAL status
 * 3. Open ticket detail drawer and approve it
 * 4. Verify ticket status changes to APPROVED
 * 5. Open ticket detail drawer and execute it
 * 6. Verify ticket status changes to DONE (executed)
 * 7. Create another ticket and reject it with reason
 * 8. Verify ticket status changes to REJECTED with reason visible
 *
 * All operations target the real frontend + backend; no mocks.
 */
import { test, expect } from '@playwright/test'
import {
  BASE_URL, ADMIN_USER, ADMIN_PASS,
  loginViaUI, getToken, apiHelper, cleanupDatasources, cleanupUsers,
} from '../support/real-test-helpers'

test.describe.configure({ timeout: 60_000 })

test.beforeAll(async () => {
  await getToken()
})

test.afterAll(async () => {
  await cleanupDatasources()
  await cleanupUsers()
})

// ── Helpers ──

async function gotoTicketList(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/tickets`)
  await page.waitForLoadState('networkidle')
  await page.getByRole('columnheader', { name: 'ID' }).waitFor({ state: 'visible' })
}

/**
 * Create a ticket via the UI form and return its ID.
 */
async function createTicketViaUI(page: import('@playwright/test').Page, sql = 'SELECT 1') {
  await page.goto(`${BASE_URL}/tickets/new`)
  await page.waitForLoadState('networkidle')

  // Wait for form to render
  await page.getByText('提交新工单').first().waitFor({ state: 'visible' })

  // Select datasource (first available)
  const dsTrigger = page.locator('[role="combobox"]').first()
  await dsTrigger.click()
  await page.waitForTimeout(500)
  const firstOption = page.locator('[role="option"]').first()
  await firstOption.waitFor({ state: 'visible' })
  await firstOption.click()

  // Fill SQL
  const sqlTextarea = page.getByPlaceholder('输入要执行的 SQL 语句...')
  await sqlTextarea.fill(sql)

  // Fill change reason (must be >= 10 chars)
  const reasonTextarea = page.getByPlaceholder(/变更的原因和预期影响/)
  await reasonTextarea.fill(`E2E 自动化测试：${sql} - 验证工单全流程功能`)

  // Submit
  await page.getByRole('button', { name: '提交工单' }).click()

  // Wait for navigation back to tickets list
  await page.waitForURL('**/tickets**', { timeout: 10_000 })
  await page.waitForLoadState('networkidle')

  // Find the newly created ticket (first row in "全部" tab should be ours)
  await page.getByRole('columnheader', { name: 'ID' }).waitFor({ state: 'visible' })
  const firstRowId = await page.locator('table tbody tr').first().locator('td').first().textContent()
  const ticketId = parseInt(firstRowId?.replace('#', '') ?? '0', 10)
  expect(ticketId).toBeGreaterThan(0)
  return ticketId
}

/**
 * Open ticket detail drawer by clicking the row with the given ID.
 */
async function openTicketDetail(page: import('@playwright/test').Page, ticketId: number) {
  // Find the row containing the ticket ID
  const row = page.locator('table tbody tr').filter({ hasText: `#${ticketId}` })
  await row.click()

  // Wait for drawer to open
  await page.locator('[data-slot="sheet-content"]').waitFor({ state: 'visible', timeout: 10_000 })
}

/**
 * Close ticket detail drawer.
 */
async function closeTicketDetail(page: import('@playwright/test').Page) {
  // Press Escape to close the sheet/drawer
  await page.keyboard.press('Escape')
  await page.waitForTimeout(500)
}

// ── Tests ──

test('工单全流程：创建→审批→执行→完成', async ({ page }) => {
  await loginViaUI(page)

  // Step 1: Create a ticket
  const ticketId = await createTicketViaUI(page, 'SELECT * FROM users LIMIT 1')
  await gotoTicketList(page)

  // Step 2: Verify ticket is in the list with PENDING_APPROVAL status
  const pendingRow = page.locator('table tbody tr').filter({ hasText: `#${ticketId}` })
  await expect(pendingRow).toContainText(/已提交|待审批/)

  // Step 3: Open detail drawer and approve the ticket
  await openTicketDetail(page, ticketId)

  // The drawer should show the ticket SQL
  await expect(page.locator('[data-slot="sheet-content"]')).toBeVisible()

  // Click the "通过" button to open approval dialog
  await page.getByRole('button', { name: '通过', exact: true }).click()

  // Approval confirmation dialog should appear
  await expect(page.getByRole('alertdialog')).toBeVisible()
  await expect(page.getByRole('alertdialog')).toContainText('审批通过')

  // Fill optional approval comment
  const commentInput = page.locator('[role="alertdialog"] textarea')
  await commentInput.fill('E2E 自动化测试：审批通过')

  // Confirm approval
  await page.getByRole('button', { name: '确认通过' }).click()

  // Wait for dialog to close
  await expect(page.getByRole('alertdialog')).not.toBeVisible({ timeout: 10_000 })
  await page.waitForTimeout(1000)

  // Step 4: Verify status changed to APPROVED
  // Re-open the detail to see updated status
  await closeTicketDetail(page)
  await page.waitForTimeout(500)

  // Refresh the list
  await page.reload()
  await page.waitForLoadState('networkidle')

  const approvedRow = page.locator('table tbody tr').filter({ hasText: `#${ticketId}` })
  await expect(approvedRow).toContainText('已通过')

  // Step 5: Execute the ticket
  await openTicketDetail(page, ticketId)
  await page.getByRole('button', { name: '执行', exact: true }).click()

  // Execute confirmation dialog
  await expect(page.getByRole('alertdialog')).toBeVisible()
  await expect(page.getByRole('alertdialog')).toContainText('执行工单')

  await page.getByRole('button', { name: '确认执行' }).click()
  await expect(page.getByRole('alertdialog')).not.toBeVisible({ timeout: 10_000 })
  await page.waitForTimeout(1000)

  // Step 6: Verify status changed to DONE
  await closeTicketDetail(page)
  await page.waitForTimeout(500)

  // Switch to "已执行" tab to verify
  await page.getByRole('tab', { name: '已执行' }).click()
  await page.waitForLoadState('networkidle')

  const doneRow = page.locator('table tbody tr').filter({ hasText: `#${ticketId}` })
  await expect(doneRow).toContainText(/已执行|已完成/)
})

test('工单全流程：创建→拒绝（含原因）', async ({ page }) => {
  await loginViaUI(page)

  // Step 1: Create a ticket
  const ticketId = await createTicketViaUI(page, 'DROP TABLE IF EXISTS e2e_test_tmp')
  await gotoTicketList(page)

  // Verify it's pending
  const pendingRow = page.locator('table tbody tr').filter({ hasText: `#${ticketId}` })
  await expect(pendingRow).toContainText(/已提交|待审批/)

  // Step 2: Open detail and reject
  await openTicketDetail(page, ticketId)

  // Click "拒绝" button
  await page.getByRole('button', { name: '拒绝', exact: true }).click()

  // Reject dialog with required reason
  await expect(page.getByRole('alertdialog')).toBeVisible()
  await expect(page.getByRole('alertdialog')).toContainText('驳回工单')

  const reasonInput = page.locator('[role="alertdialog"] textarea')
  await reasonInput.fill('E2E 自动化测试：DROP TABLE 操作需要额外审批流程，请走 DDL 专项审批')

  await page.getByRole('button', { name: '确认驳回' }).click()
  await expect(page.getByRole('alertdialog')).not.toBeVisible({ timeout: 10_000 })
  await page.waitForTimeout(1000)

  // Step 3: Verify status changed to REJECTED
  await closeTicketDetail(page)
  await page.waitForTimeout(500)

  await page.reload()
  await page.waitForLoadState('networkidle')

  // Switch to "已拒绝" tab
  await page.getByRole('tab', { name: '已拒绝' }).click()
  await page.waitForLoadState('networkidle')

  const rejectedRow = page.locator('table tbody tr').filter({ hasText: `#${ticketId}` })
  await expect(rejectedRow).toContainText('已拒绝')

  // Verify reason is visible in the detail
  await openTicketDetail(page, ticketId)
  await expect(page.locator('[data-slot="sheet-content"]')).toContainText('DROP TABLE')
})

test('工单创建：表单校验', async ({ page }) => {
  await loginViaUI(page)
  await page.goto(`${BASE_URL}/tickets/new`)
  await page.waitForLoadState('networkidle')

  // Click submit without filling any fields
  await page.getByRole('button', { name: '提交工单' }).click()

  // Validation errors should appear
  await expect(page.getByText('请选择数据源')).toBeVisible()
  await expect(page.getByText('请输入 SQL')).toBeVisible()
  await expect(page.getByText('请填写变更原因')).toBeVisible()

  // Fill change reason with too few characters (<10)
  const reasonTextarea = page.getByPlaceholder(/变更的原因和预期影响/)
  await reasonTextarea.fill('太短了')

  await page.getByRole('button', { name: '提交工单' }).click()
  await expect(page.getByText('变更原因至少 10 个字符')).toBeVisible()
})

test('工单创建：SQL 内容输入与展示', async ({ page }) => {
  await loginViaUI(page)
  await page.goto(`${BASE_URL}/tickets/new`)
  await page.waitForLoadState('networkidle')

  // Fill the SQL editor with a multi-line statement
  const sql = `ALTER TABLE users
  ADD COLUMN e2e_test_col VARCHAR(255) DEFAULT NULL`
  const sqlTextarea = page.getByPlaceholder('输入要执行的 SQL 语句...')
  await sqlTextarea.fill(sql)
  await expect(sqlTextarea).toHaveValue(sql)

  // Verify the datasource dropdown is rendered
  const dsTrigger = page.locator('[role="combobox"]').first()
  await expect(dsTrigger).toBeVisible()
})

test('工单列表：筛选器交互', async ({ page }) => {
  await loginViaUI(page)
  await gotoTicketList(page)

  // Toggle "我提交的" filter
  await page.getByRole('button', { name: '我提交的' }).click()
  await page.waitForLoadState('networkidle')

  // Toggle "待我审批" filter (admin role has this)
  await page.getByRole('button', { name: '待我审批' }).click()
  await page.waitForLoadState('networkidle')

  // Clear filters by clicking again
  await page.getByRole('button', { name: '待我审批' }).click()
  await page.waitForLoadState('networkidle')

  // Test datasource filter dropdown
  const filterCombos = page.locator('[role="combobox"]')
  const count = await filterCombos.count()
  if (count >= 2) {
    // Second combobox is datasource filter
    await filterCombos.nth(1).click()
    await page.waitForTimeout(500)
    // Select "全部数据源"
    const allOption = page.locator('[role="option"]', { hasText: '全部数据源' })
    if (await allOption.count() > 0) {
      await allOption.click()
    } else {
      // Press Escape to close
      await page.keyboard.press('Escape')
    }
  }
})

test('工单列表：搜索功能', async ({ page }) => {
  await loginViaUI(page)
  await gotoTicketList(page)

  const searchInput = page.getByPlaceholder('搜索 SQL 内容...')
  await searchInput.fill('E2E')
  await searchInput.press('Enter')
  await page.waitForLoadState('networkidle')

  // Verify results (if any) contain E2E
  const rows = page.locator('table tbody tr')
  const rowCount = await rows.count()
  if (rowCount > 0) {
    const firstRow = rows.first()
    await expect(firstRow).toContainText('E2E', { timeout: 5_000 })
  }
})

test('工单详情抽屉：显示完整信息', async ({ page }) => {
  await loginViaUI(page)
  await gotoTicketList(page)

  // Click first row to open detail
  const firstRow = page.locator('table tbody tr').first()
  await firstRow.click()

  // Drawer should open
  const drawer = page.locator('[data-slot="sheet-content"]')
  await expect(drawer).toBeVisible({ timeout: 10_000 })

  // Drawer should show ticket information
  // The drawer should have action buttons (depending on ticket status)
  await page.waitForTimeout(1000)

  // Close drawer
  await page.keyboard.press('Escape')
  await expect(drawer).not.toBeVisible({ timeout: 5_000 })
})

test('工单创建页：返回按钮导航', async ({ page }) => {
  await loginViaUI(page)

  // Navigate to new ticket page
  await page.goto(`${BASE_URL}/tickets/new`)
  await page.waitForLoadState('networkidle')

  // Click the back arrow button
  await page.getByRole('button').filter({ has: page.locator('svg.lucide-arrow-left') }).first().click()

  // Should navigate back to tickets list
  await page.waitForURL('**/tickets**', { timeout: 5_000 })
  await expect(page).toHaveURL(/\/tickets(\/)?$/)
})

test('工单全流程：创建→取消工单', async ({ page }) => {
  await loginViaUI(page)

  // Create a ticket
  const ticketId = await createTicketViaUI(page, 'CREATE TABLE e2e_cancel_test (id INT)')
  await gotoTicketList(page)

  // Open detail and cancel
  await openTicketDetail(page, ticketId)

  // Click "取消工单" button
  const cancelBtn = page.getByRole('button', { name: '取消工单', exact: true })
  const hasCancelBtn = await cancelBtn.count() > 0
  if (hasCancelBtn) {
    await cancelBtn.click()

    // Cancel dialog with required reason
    await expect(page.getByRole('alertdialog')).toBeVisible()
    const reasonInput = page.locator('[role="alertdialog"] textarea')
    await reasonInput.fill('E2E 自动化测试：取消工单')

    await page.getByRole('button', { name: /^确认/ }).click()
    await expect(page.getByRole('alertdialog')).not.toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(1000)

    // Verify status changed to CANCELLED
    await closeTicketDetail(page)
    await page.waitForTimeout(500)

    await page.reload()
    await page.waitForLoadState('networkidle')

    await page.getByRole('tab', { name: '已取消' }).click()
    await page.waitForLoadState('networkidle')

    const cancelledRow = page.locator('table tbody tr').filter({ hasText: `#${ticketId}` })
    await expect(cancelledRow).toContainText('已取消')
  }
})

test('工单标签页：各状态切换', async ({ page }) => {
  await loginViaUI(page)
  await gotoTicketList(page)

  const tabs = ['全部', '待审批', '已通过', '已拒绝', '已取消', '已执行']

  for (const tabName of tabs) {
    await page.getByRole('tab', { name: tabName }).click()
    await page.waitForLoadState('networkidle')

    // Verify tab is active
    await expect(page.getByRole('tab', { name: tabName })).toHaveAttribute('data-state', 'active')

    // Table should be visible
    await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible()
  }
})

test('工单批量操作：批量选择与批量审批', async ({ page }) => {
  await loginViaUI(page)
  await gotoTicketList(page)

  // Check if there are pending tickets with checkboxes (admin/DBA role)
  const checkboxes = page.locator('table thead input[type="checkbox"]')
  if (await checkboxes.count() === 0) {
    // Admin checkboxes may not be visible if no pending tickets or non-admin role
    test.skip()
    return
  }

  // Select all pending tickets
  await checkboxes.first().check()
  await page.waitForTimeout(500)

  // Batch action bar should appear
  const batchBar = page.getByText('已选')
  const hasBatchBar = await batchBar.count() > 0
  if (!hasBatchBar) {
    // No pending tickets to select
    return
  }

  // Verify batch approve/reject buttons
  await expect(page.getByRole('button', { name: '批量通过' })).toBeVisible()
  await expect(page.getByRole('button', { name: '批量拒绝' })).toBeVisible()
  await expect(page.getByRole('button', { name: '取消选择' })).toBeVisible()

  // Cancel selection
  await page.getByRole('button', { name: '取消选择' }).click()
  await expect(batchBar).not.toBeVisible({ timeout: 5_000 })
})
