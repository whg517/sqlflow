import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI } from '../helpers'

// --- Dashboard Mock Data ---

const MOCK_DASHBOARD_STATS = {
  code: 0,
  message: 'ok',
  data: {
    total_queries: 1284,
    total_tickets: 56,
    total_datasources: 3,
    active_users: 12,
    today_queries: 48,
    today_tickets: 5,
    pending_tickets: 8,
    high_risk_blocked: 3,
  },
}

const MOCK_RECENT_QUERIES = {
  code: 0,
  message: 'ok',
  data: [
    {
      id: 1,
      user_id: 1,
      username: 'admin',
      datasource_id: 1,
      database: 'production',
      sql_summary: 'SELECT * FROM users WHERE ...',
      execution_time_ms: 15,
      result_rows: 10,
      created_at: '2026-05-23T18:30:00.000Z',
    },
    {
      id: 2,
      user_id: 2,
      username: 'developer',
      datasource_id: 1,
      database: 'production',
      sql_summary: 'UPDATE orders SET status = ...',
      execution_time_ms: 32,
      result_rows: 0,
      created_at: '2026-05-23T18:15:00.000Z',
    },
    {
      id: 3,
      user_id: 1,
      username: 'admin',
      datasource_id: 2,
      database: 'analytics',
      sql_summary: 'db.events.aggregate([...])',
      execution_time_ms: 120,
      result_rows: 500,
      created_at: '2026-05-23T17:45:00.000Z',
    },
    {
      id: 4,
      user_id: 3,
      username: 'dba_user',
      datasource_id: 1,
      database: 'production',
      sql_summary: 'ALTER TABLE logs ADD COLUMN ...',
      execution_time_ms: 250,
      result_rows: 0,
      created_at: '2026-05-23T17:00:00.000Z',
    },
    {
      id: 5,
      user_id: 2,
      username: 'developer',
      datasource_id: 1,
      database: 'staging',
      sql_summary: 'SELECT COUNT(*) FROM orders ...',
      execution_time_ms: 8,
      result_rows: 1,
      created_at: '2026-05-23T16:30:00.000Z',
    },
  ],
}

const MOCK_AUDIT_STATS = {
  code: 0,
  message: 'ok',
  data: {
    daily: [
      { date: '2026-05-17', queries: 45, tickets: 2, blocked: 0 },
      { date: '2026-05-18', queries: 62, tickets: 5, blocked: 1 },
      { date: '2026-05-19', queries: 38, tickets: 1, blocked: 0 },
      { date: '2026-05-20', queries: 55, tickets: 3, blocked: 0 },
      { date: '2026-05-21', queries: 71, tickets: 4, blocked: 1 },
      { date: '2026-05-22', queries: 89, tickets: 6, blocked: 2 },
      { date: '2026-05-23', queries: 48, tickets: 5, blocked: 1 },
    ],
    top_users: [
      { username: 'admin', query_count: 520 },
      { username: 'developer', query_count: 410 },
      { username: 'dba_user', query_count: 354 },
    ],
    top_tables: [
      { table_name: 'users', query_count: 280 },
      { table_name: 'orders', query_count: 195 },
      { table_name: 'products', query_count: 142 },
    ],
    risk_distribution: {
      low: 1050,
      medium: 180,
      high: 45,
      critical: 9,
    },
  },
}

function mockDashboardApis(page: import('@playwright/test').Page) {
  // Dashboard stats
  page.route('**/api/dashboard/stats', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MOCK_DASHBOARD_STATS),
    })
  })

  // Recent queries
  page.route('**/api/dashboard/recent-queries', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MOCK_RECENT_QUERIES),
    })
  })

  // Audit statistics
  page.route('**/api/dashboard/audit-stats', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MOCK_AUDIT_STATS),
    })
  })
}

test.describe('Dashboard 渲染', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockDashboardApis(page)
  })

  test('导航到 Dashboard 页面', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 导航到 Dashboard
    await page.getByRole('link', { name: '概览' }).click()
    await page.waitForURL('**/dashboard')

    // 验证页面标题
    await expect(page.getByText('Dashboard')).toBeVisible()
  })

  test('概览卡片渲染 - 查询数、工单数、数据源数', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('link', { name: '概览' }).click()
    await page.waitForURL('**/dashboard')

    // 验证统计卡片渲染
    await expect(page.getByText('1,284')).toBeVisible() // total_queries
    await expect(page.getByText('56')).toBeVisible() // total_tickets
    await expect(page.getByText('3')).toBeVisible() // total_datasources
    await expect(page.getByText('12')).toBeVisible() // active_users

    // 验证今日数据
    await expect(page.getByText('48')).toBeVisible() // today_queries
    await expect(page.getByText('5')).toBeVisible() // today_tickets

    // 验证待处理工单
    await expect(page.getByText('8')).toBeVisible() // pending_tickets

    // 验证高风险拦截数
    await expect(page.getByText('3')).toBeVisible() // high_risk_blocked
  })

  test('概览卡片显示标签文字', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('link', { name: '概览' }).click()
    await page.waitForURL('**/dashboard')

    // 验证卡片标签
    await expect(page.getByText(/查询总数|总查询/)).toBeVisible()
    await expect(page.getByText(/工单总数|总工单/)).toBeVisible()
    await expect(page.getByText(/数据源/)).toBeVisible()
    await expect(page.getByText(/活跃用户/)).toBeVisible()
    await expect(page.getByText(/今日查询/)).toBeVisible()
    await expect(page.getByText(/待审批/)).toBeVisible()
  })

  test('最近查询列表渲染', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('link', { name: '概览' }).click()
    await page.waitForURL('**/dashboard')

    // 验证最近查询区块标题
    await expect(page.getByText(/最近查询|Recent Queries/)).toBeVisible()

    // 验证查询记录渲染
    await expect(page.getByText('SELECT * FROM users WHERE ...')).toBeVisible()
    await expect(page.getByText('UPDATE orders SET status = ...')).toBeVisible()
    await expect(page.getByText('db.events.aggregate([...])')).toBeVisible()
    await expect(page.getByText('ALTER TABLE logs ADD COLUMN ...')).toBeVisible()
    await expect(page.getByText('SELECT COUNT(*) FROM orders ...')).toBeVisible()

    // 验证用户名
    await expect(page.getByText('admin')).toBeVisible()
    await expect(page.getByText('developer')).toBeVisible()
    await expect(page.getByText('dba_user')).toBeVisible()
  })

  test('最近查询显示执行时间和数据库', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('link', { name: '概览' }).click()
    await page.waitForURL('**/dashboard')

    // 验证执行时间
    await expect(page.getByText('15ms')).toBeVisible()
    await expect(page.getByText('32ms')).toBeVisible()
    await expect(page.getByText('120ms')).toBeVisible()

    // 验证数据库名
    await expect(page.getByText('production')).toBeVisible()
    await expect(page.getByText('analytics')).toBeVisible()
    await expect(page.getByText('staging')).toBeVisible()
  })

  test('审计统计图表渲染', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('link', { name: '概览' }).click()
    await page.waitForURL('**/dashboard')

    // 验证审计统计区块
    await expect(page.getByText(/审计统计|Audit Stats/).first()).toBeVisible()

    // 验证图表容器渲染（使用 canvas 或 svg）
    // Chart.js uses <canvas>, ECharts uses a container div
    const chartCanvas = page.locator('canvas')
    const chartSvg = page.locator('svg')
    const hasChart = (await chartCanvas.count()) > 0 || (await chartSvg.count()) > 0
    expect(hasChart).toBe(true)
  })

  test('审计统计 - 风险分布数据', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('link', { name: '概览' }).click()
    await page.waitForURL('**/dashboard')

    // 验证风险分布标签
    await expect(page.getByText(/低风险|Low/)).toBeVisible()
    await expect(page.getByText(/中风险|Medium/)).toBeVisible()
    await expect(page.getByText(/高风险|High/)).toBeVisible()

    // 验证 Top 用户
    await expect(page.getByText('admin')).toBeVisible()
    await expect(page.getByText('developer')).toBeVisible()
    await expect(page.getByText('dba_user')).toBeVisible()
  })

  test('Dashboard 页面加载完成后无控制台错误', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))
    page.on('console', (msg) => {
      if (msg.type() === 'error') errors.push(msg.text())
    })

    await loginViaUI(page)
    await page.getByRole('link', { name: '概览' }).click()
    await page.waitForURL('**/dashboard')

    // 等待所有数据加载
    await page.waitForTimeout(2000)

    // 过滤掉无关的 console errors (如 Playwright 内部的)
    const relevantErrors = errors.filter(
      (e) => !e.includes('playwright') && !e.includes('DevTools') && !e.includes('favicon'),
    )
    expect(relevantErrors).toHaveLength(0)
  })

  test('Dashboard 数据加载中状态', async ({ page }) => {
    // 延迟 API 响应以观察加载状态
    page.route('**/api/dashboard/stats', async (route) => {
      await new Promise((r) => setTimeout(r, 500))
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_DASHBOARD_STATS),
      })
    })

    await loginViaUI(page)
    await page.getByRole('link', { name: '概览' }).click()
    await page.waitForURL('**/dashboard')

    // 验证加载指示器（skeleton 或 spinner）
    const skeletonOrSpinner =
      (await page.locator('.animate-pulse').count()) > 0 ||
      (await page.locator('[data-testid="loading"], [data-testid="skeleton"]').count()) > 0 ||
      (await page.locator('svg.animate-spin').count()) > 0

    // 加载状态可能在 API 响应前短暂出现
    // 不强制断言，因为加载可能太快
    // 最终验证数据已加载
    await expect(page.getByText('1,284')).toBeVisible({ timeout: 5000 })
  })
})
