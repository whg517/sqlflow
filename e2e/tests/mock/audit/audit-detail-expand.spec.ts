import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI } from '../../support/mock-routes'

// --- Mock Audit Data with Tickets ---

const MOCK_AUDIT_LOGS = [
  {
    id: 101,
    user_id: 1,
    username: 'admin',
    action: 'UPDATE',
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'UPDATE users SET email = "new@example.com" WHERE id = 42',
    sql_summary: 'UPDATE users SET ...',
    result_rows: 0,
    affected_rows: 1,
    execution_time_ms: 25,
    error_message: '',
    desensitized_fields: 'email',
    ip_address: '192.168.1.100',
    created_at: '2026-05-23T08:00:00.000Z',
    ticket_id: 10,
  },
  {
    id: 102,
    user_id: 2,
    username: 'developer',
    action: 'DELETE',
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'DELETE FROM temp_logs WHERE created_at < "2026-01-01"',
    sql_summary: 'DELETE FROM temp_logs ...',
    result_rows: 0,
    affected_rows: 500,
    execution_time_ms: 120,
    error_message: '',
    desensitized_fields: '',
    ip_address: '192.168.1.200',
    created_at: '2026-05-23T09:30:00.000Z',
    ticket_id: 11,
  },
  {
    id: 103,
    user_id: 1,
    username: 'admin',
    action: 'SELECT',
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'SELECT u.id, u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE o.total > 1000',
    sql_summary: 'SELECT u.id, u.name, o.total ...',
    result_rows: 35,
    affected_rows: 0,
    execution_time_ms: 42,
    error_message: '',
    desensitized_fields: '',
    ip_address: '192.168.1.100',
    created_at: '2026-05-23T10:15:00.000Z',
    ticket_id: null,
  },
]

const MOCK_TICKET_10 = {
  id: 10,
  submitter_id: 1,
  submitter_name: 'admin',
  datasource_id: 1,
  database: 'testdb',
  sql_content: 'UPDATE users SET email = "new@example.com" WHERE id = 42',
  sql_summary: 'UPDATE users SET ...',
  db_type: 'mysql',
  change_reason: 'Update user email address',
  status: 'DONE',
  risk_level: 'low',
  ai_review_result: '',
  reviewer_id: 0,
  reviewer_name: '',
  review_comment: '',
  executed_at: '2026-05-23T08:01:00.000Z',
  created_at: '2026-05-23T07:55:00.000Z',
  updated_at: '2026-05-23T08:01:00.000Z',
}

const MOCK_TICKET_11 = {
  id: 11,
  submitter_id: 2,
  submitter_name: 'developer',
  datasource_id: 1,
  database: 'testdb',
  sql_content: 'DELETE FROM temp_logs WHERE created_at < "2026-01-01"',
  sql_summary: 'DELETE FROM temp_logs ...',
  db_type: 'mysql',
  change_reason: 'Clean up old temp logs',
  status: 'APPROVED',
  risk_level: 'high',
  ai_review_result: '',
  reviewer_id: 1,
  reviewer_name: 'admin',
  review_comment: 'Approved - data older than 6 months',
  executed_at: null,
  created_at: '2026-05-23T09:20:00.000Z',
  updated_at: '2026-05-23T09:25:00.000Z',
}

function mockAuditWithDetails(page: import('@playwright/test').Page) {
  // Override audit-logs endpoint
  page.route(/\/api\/audit-logs(\?.*)?$/, async (route) => {
    if (route.request().method() === 'DELETE') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        message: 'ok',
        data: MOCK_AUDIT_LOGS,
        page: 1,
        page_size: 50,
        total: MOCK_AUDIT_LOGS.length,
      }),
    })
  })

  // Mock ticket detail lookups (by ID)
  page.route(/\/api\/tickets\/10$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, message: 'ok', data: MOCK_TICKET_10 }),
    })
  })

  page.route(/\/api\/tickets\/11$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, message: 'ok', data: MOCK_TICKET_11 }),
    })
  })

  // Tickets list (fallback for ticket search)
  page.route(/\/api\/tickets(\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        message: 'ok',
        data: [MOCK_TICKET_10, MOCK_TICKET_11],
        page: 1,
        page_size: 50,
        total: 2,
      }),
    })
  })
}

test.describe('审计展开详情 + 关联跳转', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockAuditWithDetails(page)
  })

  test('展开详情面板 - 显示完整 SQL、执行耗时、影响行数', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 点击第一条记录展开
    const firstRow = page.getByRole('row', { name: /admin/ }).first()
    await firstRow.click()

    // 等待展开区域出现
    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible()

    // 验证完整 SQL
    await expect(
      expandedRow.getByText('UPDATE users SET email = "new@example.com" WHERE id = 42'),
    ).toBeVisible()

    // 验证执行耗时
    await expect(expandedRow.getByText('25ms')).toBeVisible()

    // 验证影响行数
    await expect(expandedRow.getByText(/影响行数.*1/)).toBeVisible()

    // 验证结果行数
    await expect(expandedRow.getByText(/结果行数.*0/)).toBeVisible()
  })

  test('展开详情面板 - SQL 摘要区分完整 SQL', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 列表行只显示摘要
    await expect(page.getByText('UPDATE users SET ...')).toBeVisible()

    // 点击展开第三条 SELECT 记录
    const selectRow = page.getByRole('row', { name: /SELECT/ }).first()
    await selectRow.click()

    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible()

    // 详情面板显示完整 SQL（包含 JOIN）
    await expect(
      expandedRow.getByText('SELECT u.id, u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id'),
    ).toBeVisible()
  })

  test('展开详情面板 - 显示 IP 地址和脱敏字段', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 展开第一条（有脱敏字段）
    const firstRow = page.getByRole('row', { name: /admin/ }).first()
    await firstRow.click()

    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible()

    // 验证 IP 地址
    await expect(expandedRow.getByText('192.168.1.100')).toBeVisible()

    // 验证脱敏字段标记
    await expect(expandedRow.getByText('email')).toBeVisible()
  })

  test('复制 SQL 按钮功能', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 展开第一条记录
    const firstRow = page.getByRole('row', { name: /admin/ }).first()
    await firstRow.click()

    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible()

    // 验证复制按钮存在
    const copyBtn = expandedRow.getByRole('button', { name: '复制' })
    await expect(copyBtn).toBeVisible()

    // Mock clipboard API
    await page.evaluate(() => {
      Object.assign(navigator, {
        clipboard: {
          writeText: async (text: string) => {
            ;(window as unknown as Record<string, string>).__clipboardText = text
          },
        },
      })
    })

    // 点击复制
    await copyBtn.click()

    // 验证复制成功提示
    await expect(expandedRow.getByText('已复制')).toBeVisible()

    // 验证剪贴板内容
    const clipboardText = await page.evaluate(() => (window as unknown as Record<string, string>).__clipboardText)
    expect(clipboardText).toContain('UPDATE users SET email = "new@example.com" WHERE id = 42')
  })

  test('关联工单链接 - 有 ticket_id 显示工单链接', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 展开第一条记录（ticket_id=10）
    const firstRow = page.getByRole('row', { name: /admin/ }).first()
    await firstRow.click()

    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible()

    // 验证关联工单链接显示
    const ticketLink = expandedRow.getByRole('link', { name: /#10/ })
    await expect(ticketLink).toBeVisible()

    // 验证链接 href 包含工单 ID
    await expect(ticketLink).toHaveAttribute('href', /\/tickets\?id=10/)
  })

  test('关联工单链接 - 点击跳转到工单页', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 展开第二条记录（ticket_id=11）
    const rows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await rows.nth(1).click()

    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible()

    // 点击工单链接
    const ticketLink = expandedRow.getByRole('link', { name: /#11/ })
    await expect(ticketLink).toBeVisible()
    await ticketLink.click()

    // 验证跳转到工单页且 URL 包含工单 ID
    await page.waitForURL('**/tickets**', { timeout: 5000 })
    await expect(page).toHaveURL(/\/tickets\?id=11/)
  })

  test('关联工单链接 - 无 ticket_id 不显示链接', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 展开第三条记录（ticket_id=null，无关联工单）
    const rows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await rows.nth(2).click()

    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible()

    // 不应显示工单链接
    await expect(expandedRow.getByRole('link', { name: /#\d+/ })).not.toBeVisible()
  })

  test('切换展开项 - 只显示一个详情面板', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 展开第一条
    const rows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await rows.nth(0).click()
    await expect(page.locator('.audit-expanded-row')).toHaveCount(1)

    // 展开第二条（第一条应收起）
    await rows.nth(1).click()
    await expect(page.locator('.audit-expanded-row')).toHaveCount(1)

    // 验证当前展开的是第二条的 SQL
    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow.getByText('DELETE FROM temp_logs')).toBeVisible()
  })
})
