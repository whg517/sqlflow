import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI } from '../../support/mock-routes'

// --- Mock Audit Data ---

const MOCK_AUDIT_LOGS = [
  {
    id: 1,
    user_id: 1,
    username: 'admin',
    action: 'SELECT',
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'SELECT * FROM users WHERE id = 1',
    sql_summary: 'SELECT * FROM users ...',
    result_rows: 10,
    affected_rows: 0,
    execution_time_ms: 15,
    error_message: '',
    desensitized_fields: 'email',
    ip_address: '192.168.1.100',
    created_at: '2026-05-23T10:00:00.000Z',
  },
  {
    id: 2,
    user_id: 1,
    username: 'admin',
    action: 'UPDATE',
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'UPDATE users SET name = "alice" WHERE id = 1',
    sql_summary: 'UPDATE users SET ...',
    result_rows: 0,
    affected_rows: 1,
    execution_time_ms: 25,
    error_message: '',
    desensitized_fields: '',
    ip_address: '192.168.1.100',
    created_at: '2026-05-23T10:05:00.000Z',
  },
  {
    id: 3,
    user_id: 2,
    username: 'developer',
    action: 'DELETE',
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'DELETE FROM logs WHERE created_at < "2026-01-01"',
    sql_summary: 'DELETE FROM logs ...',
    result_rows: 0,
    affected_rows: 500,
    execution_time_ms: 120,
    error_message: '',
    desensitized_fields: '',
    ip_address: '192.168.1.200',
    created_at: '2026-05-23T10:10:00.000Z',
  },
  {
    id: 4,
    user_id: 1,
    username: 'admin',
    action: 'DDL',
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'DROP TABLE temp_sessions',
    sql_summary: 'DROP TABLE temp_sessions',
    result_rows: 0,
    affected_rows: 0,
    execution_time_ms: 45,
    error_message: '',
    desensitized_fields: '',
    ip_address: '192.168.1.100',
    created_at: '2026-05-23T10:15:00.000Z',
  },
  {
    id: 5,
    user_id: 3,
    username: 'dba',
    action: 'INSERT',
    datasource_id: 2,
    database: 'analytics',
    sql_content: 'INSERT INTO events (name, payload) VALUES ("click", \'{"page":"/home"}\')',
    sql_summary: 'INSERT INTO events ...',
    result_rows: 0,
    affected_rows: 1,
    execution_time_ms: 8,
    error_message: '',
    desensitized_fields: '',
    ip_address: '10.0.0.50',
    created_at: '2026-05-23T10:20:00.000Z',
  },
  {
    id: 6,
    user_id: 2,
    username: 'developer',
    action: 'SELECT',
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'SELECT id, name FROM orders WHERE status = "pending"',
    sql_summary: 'SELECT id, name FROM ...',
    result_rows: 30,
    affected_rows: 0,
    execution_time_ms: 22,
    error_message: '',
    desensitized_fields: '',
    ip_address: '192.168.1.200',
    created_at: '2026-05-23T10:25:00.000Z',
  },
]

function makeAuditResponse(data: typeof MOCK_AUDIT_LOGS, total = data.length) {
  return {
    code: 0,
    message: 'ok',
    data,
    page: 1,
    page_size: 50,
    total,
  }
}

function mockAuditLogs(page: import('@playwright/test').Page, overrideData?: typeof MOCK_AUDIT_LOGS) {
  page.route(/\/api\/audit-logs(\?.*)?$/, async (route) => {
    if (route.request().method() === 'DELETE') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
      return
    }

    const url = route.request().url()
    const urlObj = new URL(url)
    const keyword = urlObj.searchParams.get('keyword')?.toLowerCase() ?? ''

    const allLogs = overrideData ?? MOCK_AUDIT_LOGS
    const filtered = keyword
      ? allLogs.filter(
          (log) =>
            log.sql_content.toLowerCase().includes(keyword) ||
            log.username.toLowerCase().includes(keyword) ||
            log.action.toLowerCase().includes(keyword) ||
            log.database.toLowerCase().includes(keyword) ||
            log.sql_summary.toLowerCase().includes(keyword),
        )
      : allLogs

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(makeAuditResponse(filtered, filtered.length)),
    })
  })

  // Mock audit-logs/search (FTS endpoint) — also keyword-aware
  page.route(/\/api\/audit-logs\/search/, async (route) => {
    const url = route.request().url()
    const urlObj = new URL(url)
    const keyword = urlObj.searchParams.get('keyword')?.toLowerCase() ?? ''

    const allLogs = overrideData ?? MOCK_AUDIT_LOGS
    const filtered = keyword
      ? allLogs.filter(
          (log) =>
            log.sql_content.toLowerCase().includes(keyword) ||
            log.username.toLowerCase().includes(keyword) ||
            log.action.toLowerCase().includes(keyword),
        )
      : allLogs

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(makeAuditResponse(filtered.slice(0, 5), filtered.length)),
    })
  })
}

test.describe('审计日志列表 + keyword 搜索', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockAuditLogs(page)
  })

  test('审计日志列表渲染', async ({ page }) => {
    await loginViaUI(page)

    // 导航到审计页面
    await page.getByRole('link', { name: '审计' }).click()
    await page.waitForURL('**/audit')

    // 验证页面标题
    await expect(page.getByText('审计日志')).toBeVisible()

    // 验证表头列
    await expect(page.getByRole('columnheader', { name: '时间' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '用户' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '操作' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '数据库' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'SQL 摘要' })).toBeVisible()

    // 验证列表渲染 — 检查一些关键数据
    await expect(page.getByText('admin')).toBeVisible()
    await expect(page.getByText('developer')).toBeVisible()
    await expect(page.getByText('testdb')).toBeVisible()

    // 验证分页控件（total=6, pageSize=50 → totalPages=1, 不显示分页）
    // 但分页区域逻辑是 total > pageSize 才显示，这里只有 1 页
    // 验证总数文本存在（如果 total > 1 page）
    // 由于 totalPages = 1，分页不会渲染，验证"导出 CSV"按钮作为页面完整性的替代指标
    await expect(page.getByRole('button', { name: '导出 CSV' })).toBeVisible()
  })

  test('审计日志列表显示分页控件', async ({ page }) => {
    // Mock 返回足够多的数据以显示分页（page_size=50, total=100）
    const manyLogs = Array.from({ length: 100 }, (_, i) => ({
      ...MOCK_AUDIT_LOGS[0],
      id: i + 1,
      created_at: new Date(Date.now() - i * 60000).toISOString(),
    }))
    mockAuditLogs(page, manyLogs)

    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 验证分页控件可见
    await expect(page.getByText('共 100 条，第 1/2 页')).toBeVisible()

    // 验证上一页/下一页按钮
    const prevBtn = page.getByRole('button', { name: '<' })
    const nextBtn = page.getByRole('button', { name: '>' })
    await expect(prevBtn).toBeVisible()
    await expect(prevBtn).toBeDisabled()
    await expect(nextBtn).toBeVisible()
    await expect(nextBtn).toBeEnabled()
  })

  test('keyword 搜索 - 按 SQL 内容搜索', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 输入搜索关键词
    await page.getByPlaceholder('搜索 SQL / 表名...').fill('DROP')
    await page.keyboard.press('Enter')

    // 验证只显示包含 DROP 的记录（id=4: DROP TABLE temp_sessions）
    // admin SELECT 记录不应出现
    await expect(page.getByRole('table').getByText('DROP TABLE temp_sessions')).toBeVisible()
    await expect(page.getByRole('table').getByText('SELECT * FROM users')).not.toBeVisible()
  })

  test('keyword 搜索 - 按用户名搜索', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 输入用户名关键词
    const searchInput = page.getByPlaceholder('搜索 SQL / 表名...')
    await searchInput.fill('admin')
    await page.keyboard.press('Enter')

    // 验证只显示 admin 用户的记录（id=1,2,4 — SELECT, UPDATE, DDL）
    const table = page.getByRole('table')
    await expect(table.getByText('admin')).toBeVisible()
    // developer 用户不应出现
    await expect(table.getByText('developer')).not.toBeVisible()
    // dba 用户不应出现
    await expect(table.getByText('dba')).not.toBeVisible()
  })

  test('keyword 搜索 - 按操作类型搜索', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 输入操作类型关键词（小写也能匹配）
    await page.getByPlaceholder('搜索 SQL / 表名...').fill('select')
    await page.keyboard.press('Enter')

    // 验证只显示 SELECT 操作的记录（id=1,6）
    const table = page.getByRole('table')
    await expect(table.getByText('SELECT')).toBeVisible()
    // DDL 操作不应出现
    await expect(table.getByText('DROP TABLE temp_sessions')).not.toBeVisible()
  })

  test('空搜索结果', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 输入不存在的关键词
    await page.getByPlaceholder('搜索 SQL / 表名...').fill('zzzzzzz')
    await page.keyboard.press('Enter')

    // 验证显示空状态提示
    await expect(page.getByText('暂无审计日志')).toBeVisible()
  })

  test('展开详情', async ({ page }) => {
    // Mock linked ticket lookup
    page.route(/\/api\/tickets(\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: [],
          page: 1,
          page_size: 50,
          total: 0,
        }),
      })
    })

    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 点击第一条记录展开
    await page.getByRole('row', { name: /admin/ }).first().click()

    // 等待展开区域出现
    await expect(page.locator('.audit-expanded-row')).toBeVisible()

    // 验证详情面板显示完整 SQL
    await expect(page.locator('.audit-expanded-row').getByText('SELECT * FROM users WHERE id = 1')).toBeVisible()

    // 验证执行耗时
    await expect(page.locator('.audit-expanded-row').getByText('执行耗时')).toBeVisible()
    await expect(page.locator('.audit-expanded-row').getByText('15ms')).toBeVisible()

    // 验证影响行数
    await expect(page.locator('.audit-expanded-row').getByText('影响行数')).toBeVisible()
    await expect(page.locator('.audit-expanded-row').getByText('0')).toBeVisible()

    // 验证复制按钮存在
    const copyBtn = page.locator('.audit-expanded-row').getByRole('button', { name: '复制' })
    await expect(copyBtn).toBeVisible()
  })

  test('展开详情 - 关联工单链接', async ({ page }) => {
    // Mock linked ticket lookup — 返回匹配的工单
    page.route(/\/api\/tickets(\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: [
            {
              id: 10,
              submitter_id: 1,
              submitter_name: 'admin',
              datasource_id: 1,
              database: 'testdb',
              sql_content: 'SELECT * FROM users WHERE id = 1',
              sql_summary: 'SELECT * FROM users ...',
              db_type: 'mysql',
              change_reason: 'Query user data',
              status: 'DONE',
              risk_level: 'low',
              ai_review_result: '',
              reviewer_id: 0,
              reviewer_name: '',
              review_comment: '',
              executed_at: new Date().toISOString(),
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
          ],
          page: 1,
          page_size: 50,
          total: 1,
        }),
      })
    })

    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 点击第一条记录展开
    await page.getByRole('row', { name: /admin/ }).first().click()

    // 等待关联工单加载
    await expect(page.locator('.audit-expanded-row').getByText('#10')).toBeVisible({ timeout: 5000 })
  })

  test('搜索后清空关键词恢复列表', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 先搜索
    await page.getByPlaceholder('搜索 SQL / 表名...').fill('DROP')
    await page.keyboard.press('Enter')
    await expect(page.getByRole('table').getByText('DROP TABLE temp_sessions')).toBeVisible()

    // 清空搜索框并重新搜索
    const searchInput = page.getByPlaceholder('搜索 SQL / 表名...')
    await searchInput.clear()
    await page.keyboard.press('Enter')

    // 验证所有记录恢复显示
    await expect(page.getByText('admin')).toBeVisible()
    await expect(page.getByText('developer')).toBeVisible()
    await expect(page.getByText('dba')).toBeVisible()
  })
})
