import { test, expect, type Page } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_USERS, MOCK_TOKEN, MOCK_TICKET, MOCK_CREATED_TICKET } from '../helpers'

test.describe('工单审批→执行完整链路', () => {
  test('完整流程：提交工单 → 审批通过 → 执行SQL → 状态变为已完成 → 审计日志新增', async ({ page }) => {
    // ========== 1. Mock 全部 API（带动态状态追踪） ==========

    // Track ticket state through the lifecycle
    let ticketStatus: string = 'PENDING_APPROVAL'
    const auditLogs: Array<Record<string, unknown>> = []

    // Mock comments API
    page.route(/\/api\/tickets\/\d+\/comments/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: [] }),
      })
    })

    // Mock ticket list — returns dynamic status
    page.route(/\/api\/tickets(\?.*)?$/, async (route) => {
      if (route.request().method() === 'POST') {
        // Create ticket
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: {
              ...MOCK_CREATED_TICKET.data,
              id: 100,
              sql_content: 'SELECT COUNT(*) FROM orders;',
              status: 'PENDING_APPROVAL',
            },
          }),
        })
      } else {
        // List tickets — reflect current status
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: [
              {
                ...MOCK_TICKET,
                id: 100,
                sql_content: 'SELECT COUNT(*) FROM orders;',
                sql_summary: 'SELECT COUNT(*) ...',
                status: ticketStatus,
              },
            ],
            page: 1,
            page_size: 50,
            total: 1,
          }),
        })
      }
    })

    // Mock ticket detail — returns dynamic status
    page.route('**/api/tickets/100', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            ...MOCK_TICKET,
            id: 100,
            sql_content: 'SELECT COUNT(*) FROM orders;',
            sql_summary: 'SELECT COUNT(*) ...',
            status: ticketStatus,
            reviewer_id: ticketStatus === 'APPROVED' || ticketStatus === 'DONE' ? 1 : 0,
            reviewer_name: ticketStatus === 'APPROVED' || ticketStatus === 'DONE' ? 'admin' : '',
            executed_at: ticketStatus === 'DONE' ? new Date().toISOString() : null,
          },
        }),
      })
    })

    // Mock ticket approve
    page.route('**/api/tickets/100/approve', async (route) => {
      ticketStatus = 'APPROVED'
      auditLogs.push({
        id: auditLogs.length + 1,
        action: 'approve',
        ticket_id: 100,
        user_id: 1,
        user_name: 'admin',
        detail: '审批通过',
        created_at: new Date().toISOString(),
      })
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            ...MOCK_TICKET,
            id: 100,
            status: 'APPROVED',
            reviewer_id: 1,
            reviewer_name: 'admin',
          },
        }),
      })
    })

    // Mock ticket execute
    page.route('**/api/tickets/100/execute', async (route) => {
      ticketStatus = 'DONE'
      auditLogs.push({
        id: auditLogs.length + 1,
        action: 'execute',
        ticket_id: 100,
        user_id: 2,
        user_name: 'developer',
        detail: 'SQL 执行成功',
        created_at: new Date().toISOString(),
      })
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            ...MOCK_TICKET,
            id: 100,
            status: 'DONE',
            executed_at: new Date().toISOString(),
          },
        }),
      })
    })

    // Mock query execute (for ticket SQL execution)
    page.route('**/api/query/execute', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            columns: ['count(*)'],
            rows: [{ 'count(*)': 1024 }],
            total: 1,
            execution_time_ms: 8,
            affected_rows: 0,
            desensitized: false,
            desensitized_fields: [],
            warnings: [],
          },
        }),
      })
    })

    // Mock audit logs — returns dynamic list
    page.route('**/api/audit-logs', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: auditLogs,
          page: 1,
          page_size: 50,
          total: auditLogs.length,
        }),
      })
    })

    // Default auth + datasource mocks
    mockApiRoutes(page, { role: 'developer' })

    // ========== 2. 普通用户登录并创建工单 ==========

    await loginViaUI(page, 'admin', 'admin123')
    await page.waitForURL('**/query**')

    // Navigate to tickets page
    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')

    // Click create ticket
    await page.getByRole('button', { name: '提交新工单' }).click()
    await page.waitForURL('**/tickets/new**')

    // Fill in ticket form
    await page.getByText('选择数据源').click()
    await page.getByText('test-mysql').click()

    await page.getByPlaceholder('输入数据库名（可选）').fill('testdb')

    await page.getByPlaceholder('输入要执行的 SQL 语句').fill('SELECT COUNT(*) FROM orders;')

    await page.getByPlaceholder(/请说明此次变更的原因/).fill('查询订单总数用于月度报表统计')

    // Submit ticket
    await page.getByRole('button', { name: '提交工单' }).click()

    // Verify success toast and redirect to ticket list
    await expect(page.getByText('工单提交成功')).toBeVisible()
    await page.waitForURL('**/tickets**', { timeout: 5000 })

    // ========== 3. 验证工单列表出现新工单，状态为「待审批」 ==========
    await expect(page.getByText('#100')).toBeVisible()
    await expect(page.getByRole('table').getByText('待审批')).toBeVisible()

    // ========== 4. 审批人打开工单详情并审批通过 ==========
    // Open ticket detail drawer
    await page.getByRole('row', { name: /#100/ }).click()
    await expect(page.getByText('工单 #100')).toBeVisible()

    // Verify initial status in drawer
    await expect(page.locator('[data-slot="sheet-content"]').getByText('待审批')).toBeVisible()

    // Click approve button
    const sheetContent = page.locator('[data-slot="sheet-content"]')
    await sheetContent.getByRole('button', { name: '通过' }).click()

    // Verify approve dialog appears
    await expect(page.getByRole('dialog').getByText('审批通过')).toBeVisible()
    await expect(page.getByRole('dialog').getByText(/确认通过工单 #100/)).toBeVisible()

    // Add approval comment
    await page.getByPlaceholder('填写审批备注...').fill('SQL 安全，已确认')

    // Confirm approval
    await page.getByRole('dialog').getByRole('button', { name: '确认通过' }).click()

    // Verify success toast
    await expect(page.getByText('审批通过')).toBeVisible()

    // Verify status changed to 「已通过」in drawer
    await expect(sheetContent.getByText('已通过')).toBeVisible()

    // Close drawer
    await page.locator('button').filter({ has: page.locator('svg.lucide-x') }).first().click()
    await expect(page.locator('[data-slot="sheet-content"]')).not.toBeVisible()

    // Verify status in list also updated
    await page.reload()
    await expect(page.getByRole('table').getByText('已通过')).toBeVisible()

    // ========== 5. 打开已通过的工单并执行 ==========

    // Open ticket detail
    await page.getByRole('row', { name: /#100/ }).click()
    await expect(page.getByText('工单 #100')).toBeVisible()

    // Verify status is 已通过 and execute button is visible
    const sheetContent2 = page.locator('[data-slot="sheet-content"]')
    await expect(sheetContent2.getByText('已通过')).toBeVisible()
    await expect(sheetContent2.getByRole('button', { name: '执行' })).toBeVisible()

    // Click execute button
    await sheetContent2.getByRole('button', { name: '执行' }).click()

    // Verify execute confirm dialog
    await expect(page.getByRole('dialog').getByText('执行工单')).toBeVisible()
    await expect(page.getByRole('dialog').getByText(/确认执行工单 #100/)).toBeVisible()

    // Confirm execution
    await page.getByRole('dialog').getByRole('button', { name: '确认执行' }).click()

    // Verify success toast
    await expect(page.getByText('工单已执行')).toBeVisible()

    // Verify status changed to 「已完成」
    await expect(sheetContent2.getByText('已完成')).toBeVisible()

    // Close drawer
    await page.locator('button').filter({ has: page.locator('svg.lucide-x') }).first().click()

    // ========== 6. 验证工单列表状态为「已完成」 ==========

    await page.reload()
    await expect(page.getByRole('table').getByText('已完成')).toBeVisible()

    // ========== 7. 验证审计日志新增记录 ==========

    await page.getByRole('link', { name: '审计' }).click()
    await page.waitForURL('**/audit**')

    // Verify audit log entries exist
    await expect(page.getByText('approve')).toBeVisible()
    await expect(page.getByText('execute')).toBeVisible()
    await expect(page.getByText('ticket_id: 100')).toBeVisible()
  })
})
