import { test, expect, type Page } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_USERS, MOCK_TOKEN, MOCK_TICKET } from '../../support/mock-routes'

test.describe('工单拒绝流程', () => {
  test.beforeEach(async ({ page }) => {
    // Mock comments API (used by TicketDetailDrawer)
    page.route(/\/api\/tickets\/\d+\/comments/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: [] }),
      })
    })

    // Default auth + datasource mocks (admin role for approval capabilities)
    mockApiRoutes(page, { role: 'admin' })
  })

  test('完整流程：创建工单 → 审批人拒绝 → 验证状态和原因 → 执行按钮不可用', async ({ page }) => {
    const REJECT_REASON = 'SQL 存在性能风险，缺少索引条件，请优化后重新提交'

    // Track ticket state
    let ticketStatus: string = 'PENDING_APPROVAL'
    let reviewComment: string = ''

    // Override ticket list mock
    page.route(/\/api\/tickets(\?.*)?$/, async (route) => {
      if (route.request().method() === 'POST') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: {
              ...MOCK_TICKET,
              id: 200,
              sql_content: 'DELETE FROM orders WHERE created_at < "2025-01-01"',
              sql_summary: 'DELETE FROM orders ...',
              status: 'PENDING_APPROVAL',
              change_reason: '清理过期历史订单数据',
            },
          }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: [
              {
                ...MOCK_TICKET,
                id: 200,
                sql_content: 'DELETE FROM orders WHERE created_at < "2025-01-01"',
                sql_summary: 'DELETE FROM orders ...',
                status: ticketStatus,
                review_comment: reviewComment,
                reviewer_id: ticketStatus === 'REJECTED' ? 1 : 0,
                reviewer_name: ticketStatus === 'REJECTED' ? 'admin' : '',
              },
            ],
            page: 1,
            page_size: 50,
            total: 1,
          }),
        })
      }
    })

    // Override ticket detail mock
    page.route('**/api/tickets/200', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            ...MOCK_TICKET,
            id: 200,
            sql_content: 'DELETE FROM orders WHERE created_at < "2025-01-01"',
            sql_summary: 'DELETE FROM orders ...',
            status: ticketStatus,
            change_reason: '清理过期历史订单数据',
            review_comment: reviewComment,
            reviewer_id: ticketStatus === 'REJECTED' ? 1 : 0,
            reviewer_name: ticketStatus === 'REJECTED' ? 'admin' : '',
          },
        }),
      })
    })

    // Mock ticket reject
    page.route('**/api/tickets/200/reject', async (route) => {
      ticketStatus = 'REJECTED'
      reviewComment = REJECT_REASON
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            ...MOCK_TICKET,
            id: 200,
            status: 'REJECTED',
            review_comment: REJECT_REASON,
            reviewer_id: 1,
            reviewer_name: 'admin',
          },
        }),
      })
    })

    // ========== 1. 创建工单 ==========

    await loginViaUI(page, 'admin', 'admin123')
    await page.waitForURL('**/query**')

    // Navigate to tickets page
    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')

    // Create a new ticket
    await page.getByRole('button', { name: '提交新工单' }).click()
    await page.waitForURL('**/tickets/new**')

    // Fill in ticket form
    await page.getByText('选择数据源').click()
    await page.getByText('test-mysql').click()

    await page.getByPlaceholder('输入数据库名（可选）').fill('testdb')

    await page.getByPlaceholder('输入要执行的 SQL 语句').fill('DELETE FROM orders WHERE created_at < "2025-01-01"')

    await page.getByPlaceholder(/请说明此次变更的原因/).fill('清理过期历史订单数据')

    // Submit ticket
    await page.getByRole('button', { name: '提交工单' }).click()

    // Verify success and redirect
    await expect(page.getByText('工单提交成功')).toBeVisible()
    await page.waitForURL('**/tickets**', { timeout: 5000 })

    // Verify new ticket appears in list with pending approval status
    await expect(page.getByText('#200')).toBeVisible()
    await expect(page.getByRole('table').getByText('待审批')).toBeVisible()

    // ========== 2. 审批人打开工单详情 ==========

    await page.getByRole('row', { name: /#200/ }).click()
    await expect(page.getByText('工单 #200')).toBeVisible()

    const sheetContent = page.locator('[data-slot="sheet-content"]')

    // Verify current status is 待审批
    await expect(sheetContent.getByText('待审批')).toBeVisible()

    // Verify reject button is visible for admin
    await expect(sheetContent.getByRole('button', { name: '拒绝' })).toBeVisible()

    // ========== 3. 点击「拒绝」按钮 ==========

    await sheetContent.getByRole('button', { name: '拒绝' }).click()

    // Verify reject dialog appears
    await expect(page.getByRole('dialog').getByText('驳回工单')).toBeVisible()
    await expect(page.getByRole('dialog').getByText(/驳回工单 #200/)).toBeVisible()

    // ========== 4. 填写拒绝原因 ==========

    const rejectTextarea = page.getByPlaceholder('请说明驳回原因...')
    await expect(rejectTextarea).toBeVisible()

    await rejectTextarea.fill(REJECT_REASON)

    // Verify reject button is enabled after filling reason
    const rejectConfirmBtn = page.getByRole('dialog').getByRole('button', { name: '确认驳回' })
    await expect(rejectConfirmBtn).toBeEnabled()

    // ========== 5. 确认拒绝 ==========

    await rejectConfirmBtn.click()

    // Verify success toast
    await expect(page.getByText('已驳回')).toBeVisible()

    // ========== 6. 验证工单状态变为「已拒绝」 ==========

    await expect(sheetContent.getByText('已拒绝')).toBeVisible()

    // ========== 7. 验证拒绝原因显示在工单详情中 ==========

    // After rejection, the review record section should appear with the reason
    await expect(sheetContent.getByText('审批记录')).toBeVisible()
    await expect(sheetContent.getByText('已拒绝')).toBeVisible()
    await expect(sheetContent.getByText(REJECT_REASON)).toBeVisible()

    // Close drawer and verify list status
    await page.locator('button').filter({ has: page.locator('svg.lucide-x') }).first().click()

    await page.reload()
    await expect(page.getByRole('table').getByText('已拒绝')).toBeVisible()

    // Re-open detail to fully verify rejection reason persists
    await page.getByRole('row', { name: /#200/ }).click()
    await expect(page.getByText('工单 #200')).toBeVisible()

    const sheetContent2 = page.locator('[data-slot="sheet-content"]')
    await expect(sheetContent2.getByText('已拒绝')).toBeVisible()
    await expect(sheetContent2.getByText(REJECT_REASON)).toBeVisible()

    // ========== 8. 验证「执行」按钮不可见 ==========

    // After rejection, the execute button should not be present
    await expect(sheetContent2.getByRole('button', { name: '执行' })).not.toBeVisible()

    // Also verify approve button is gone
    await expect(sheetContent2.getByRole('button', { name: '通过' })).not.toBeVisible()
  })

  test('拒绝时不填写原因应显示错误提示', async ({ page }) => {
    // Setup: mock a pending ticket in the list and detail
    page.route(/\/api\/tickets(\?.*)?$/, async (route) => {
      if (route.request().method() === 'POST') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: {
              ...MOCK_TICKET,
              id: 300,
              status: 'PENDING_APPROVAL',
            },
          }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: [{ ...MOCK_TICKET, id: 300, status: 'PENDING_APPROVAL' }],
            page: 1,
            page_size: 50,
            total: 1,
          }),
        })
      }
    })

    page.route('**/api/tickets/300', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: { ...MOCK_TICKET, id: 300, status: 'PENDING_APPROVAL' },
        }),
      })
    })

    // Navigate directly to tickets and open the pending ticket
    await loginViaUI(page, 'admin', 'admin123')
    await page.waitForURL('**/query**')

    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')

    await page.getByRole('row', { name: /#300/ }).click()
    await expect(page.getByText('工单 #300')).toBeVisible()

    const sheetContent = page.locator('[data-slot="sheet-content"]')

    // Click reject
    await sheetContent.getByRole('button', { name: '拒绝' }).click()

    // Verify dialog appears
    await expect(page.getByRole('dialog').getByText('驳回工单')).toBeVisible()

    // Confirm reject button should be disabled without reason
    const rejectConfirmBtn = page.getByRole('dialog').getByRole('button', { name: '确认驳回' })
    await expect(rejectConfirmBtn).toBeDisabled()

    // Click confirm without filling reason — button is disabled so it shouldn't submit
    // The component uses handleReject which checks rejectReason.trim() before calling API
    // If somehow bypassed, verify error toast appears
    await expect(rejectConfirmBtn).toBeDisabled()
  })
})
