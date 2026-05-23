import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_HISTORY } from '../helpers'

// --- Extended History Mock ---

const EXTENDED_HISTORY = [
  {
    id: 1,
    user_id: 1,
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'SELECT * FROM users WHERE status = "active" LIMIT 50',
    sql_summary: 'SELECT * FROM users ...',
    db_type: 'mysql',
    execution_time: 15,
    result_rows: 10,
    affected_rows: 0,
    created_at: '2026-05-23T18:00:00.000Z',
  },
  {
    id: 2,
    user_id: 1,
    datasource_id: 2,
    database: 'analytics',
    sql_content: 'db.events.find({ status: "completed" }).sort({ created_at: -1 }).limit(20)',
    sql_summary: 'db.events.find({ ...',
    db_type: 'mongodb',
    execution_time: 8,
    result_rows: 20,
    affected_rows: 0,
    created_at: '2026-05-23T17:30:00.000Z',
  },
  {
    id: 3,
    user_id: 2,
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'UPDATE orders SET status = "shipped" WHERE id IN (100, 101, 102)',
    sql_summary: 'UPDATE orders SET ...',
    db_type: 'mysql',
    execution_time: 32,
    result_rows: 0,
    affected_rows: 3,
    created_at: '2026-05-23T17:00:00.000Z',
  },
  {
    id: 4,
    user_id: 1,
    datasource_id: 1,
    database: 'testdb',
    sql_content: 'SELECT o.id, u.name, o.total FROM orders o JOIN users u ON o.user_id = u.id WHERE o.total > 500',
    sql_summary: 'SELECT o.id, u.name, ...',
    db_type: 'mysql',
    execution_time: 45,
    result_rows: 35,
    affected_rows: 0,
    created_at: '2026-05-23T16:00:00.000Z',
  },
  {
    id: 5,
    user_id: 1,
    datasource_id: 1,
    database: 'production',
    sql_content: 'INSERT INTO audit_logs (action, user_id) VALUES ("login", 1)',
    sql_summary: 'INSERT INTO audit_logs ...',
    db_type: 'mysql',
    execution_time: 5,
    result_rows: 0,
    affected_rows: 1,
    created_at: '2026-05-23T15:00:00.000Z',
  },
]

function mockExtendedHistory(page: import('@playwright/test').Page) {
  page.route(/\/api\/query\/history(\?.*)?$/, async (route) => {
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

    const filtered = keyword
      ? EXTENDED_HISTORY.filter(
          (h) =>
            h.sql_content.toLowerCase().includes(keyword) ||
            h.database.toLowerCase().includes(keyword) ||
            h.sql_summary.toLowerCase().includes(keyword) ||
            h.db_type.toLowerCase().includes(keyword),
        )
      : EXTENDED_HISTORY

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        message: 'ok',
        data: filtered,
        page: 1,
        page_size: 50,
        total: filtered.length,
      }),
    })
  })
}

test.describe('查询历史列表 + keyword 搜索', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockExtendedHistory(page)
  })

  test('打开查询历史面板', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 打开历史面板
    const historyBtn = page.getByRole('button', { name: '历史' })
    await expect(historyBtn).toBeVisible()
    await historyBtn.click()

    // 验证历史面板出现
    await expect(page.getByText('查询历史')).toBeVisible()
  })

  test('历史列表渲染 - 时间、SQL 摘要、数据源、状态', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 打开历史面板
    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 验证历史记录渲染
    // SQL 摘要
    await expect(page.getByText('SELECT * FROM users ...')).toBeVisible()
    await expect(page.getByText('db.events.find({ ...')).toBeVisible()
    await expect(page.getByText('UPDATE orders SET ...')).toBeVisible()

    // 数据源/数据库
    await expect(page.getByText('testdb')).toBeVisible()
    await expect(page.getByText('analytics')).toBeVisible()

    // 时间信息（相对时间或绝对时间）
    // 验证至少显示了时间信息
    const historyPanel = page.getByText('查询历史').locator('..')
    await expect(historyPanel.locator('text=/\\d{2}:\\d{2}/')).toBeVisible()

    // 执行耗时
    await expect(page.getByText('15ms')).toBeVisible()
    await expect(page.getByText('8ms')).toBeVisible()
  })

  test('历史列表按时间倒序排列', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 最新的记录 (id=1, 18:00) 应该在最上面
    const historyItems = page.locator('[data-testid="history-item"]')
    const firstItem = historyItems.first()
    await expect(firstItem).toContainText('SELECT * FROM users ...')

    // 最旧的记录 (id=5, 15:00) 应该在最下面
    const lastItem = historyItems.last()
    await expect(lastItem).toContainText('INSERT INTO audit_logs ...')
  })

  test('keyword 搜索过滤 - 按 SQL 内容搜索', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 输入搜索关键词
    const searchInput = page.getByPlaceholder(/搜索/)
    await searchInput.fill('orders')
    await page.keyboard.press('Enter')

    // 验证只显示包含 orders 的记录
    await expect(page.getByText('UPDATE orders SET ...')).toBeVisible()
    await expect(page.getByText('SELECT o.id, u.name, ...')).toBeVisible()

    // 不包含 orders 的记录不应出现
    await expect(page.getByText('db.events.find')).not.toBeVisible()
    await expect(page.getByText('INSERT INTO audit_logs')).not.toBeVisible()
  })

  test('keyword 搜索过滤 - 按数据库名搜索', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 按数据库名搜索
    const searchInput = page.getByPlaceholder(/搜索/)
    await searchInput.fill('analytics')
    await page.keyboard.press('Enter')

    // 验证只显示 analytics 数据库的记录
    await expect(page.getByText('db.events.find({ ...')).toBeVisible()
    await expect(page.getByText('testdb').first()).not.toBeVisible()
  })

  test('keyword 搜索 - 空结果显示空状态', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 输入不存在的关键词
    const searchInput = page.getByPlaceholder(/搜索/)
    await searchInput.fill('zzzzzznotexist')
    await page.keyboard.press('Enter')

    // 验证空状态
    await expect(page.getByText('暂无查询历史')).toBeVisible()
  })

  test('点击历史项加载到编辑器', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 点击第一条历史记录
    const historyItem = page.getByText('SELECT * FROM users ...').first()
    await historyItem.click()

    // 验证 SQL 加载到编辑器
    const editor = page.locator('.cm-content').first()
    await expect(editor).toContainText('SELECT * FROM users WHERE status = "active" LIMIT 50')

    // 验证历史面板关闭（或至少 SQL 已加载）
    // 编辑器应显示完整的 SQL 而非摘要
    await expect(editor).toContainText('WHERE status')
  })

  test('清空历史', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 验证历史列表有数据
    await expect(page.getByText('SELECT * FROM users ...')).toBeVisible()

    // 点击清空按钮
    const clearBtn = page.getByRole('button', { name: /清空/ })
    await expect(clearBtn).toBeVisible()
    await clearBtn.click()

    // 确认清空（如果有确认对话框）
    const confirmBtn = page.getByRole('button', { name: '确认' })
    if (await confirmBtn.isVisible()) {
      await confirmBtn.click()
    }

    // 验证历史已清空
    await expect(page.getByText('暂无查询历史')).toBeVisible()
  })

  test('清空搜索后恢复完整列表', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 先搜索
    const searchInput = page.getByPlaceholder(/搜索/)
    await searchInput.fill('orders')
    await page.keyboard.press('Enter')
    await expect(page.getByText('UPDATE orders SET ...')).toBeVisible()

    // 清空搜索
    await searchInput.clear()
    await page.keyboard.press('Enter')

    // 验证所有记录恢复
    await expect(page.getByText('SELECT * FROM users ...')).toBeVisible()
    await expect(page.getByText('db.events.find({ ...')).toBeVisible()
    await expect(page.getByText('INSERT INTO audit_logs ...')).toBeVisible()
  })
})
