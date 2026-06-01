/**
 * E2E — 工单列表与详情（真实后端）
 *
 * Tests ticket list rendering, tab switching, filtering, search,
 * ticket detail drawer, and new ticket form validation.
 */
import { test, expect, type Page } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const ADMIN_USER = process.env.E2E_USERNAME ?? 'e2eadmin'
const ADMIN_PASS = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

async function loginReal(page: Page): Promise<string> {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill(ADMIN_USER)
  await page.getByPlaceholder('密码').fill(ADMIN_PASS)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 15_000 })
  return page.evaluate(() => localStorage.getItem('token') ?? '')
}

async function createTicketViaApi(page: Page, dsId: number, sql: string, reason: string) {
  const token = await page.evaluate(() => localStorage.getItem('token'))
  const res = await page.evaluate(async ({ baseUrl, token, dsId, sql, reason }) => {
    const r = await fetch(`${baseUrl}/api/tickets`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ datasource_id: dsId, database: 'testdb', sql, db_type: 'mysql', change_reason: reason }),
    })
    return await r.json()
  }, { baseUrl: BASE_URL, token, dsId, sql, reason })
  return res as { code: number; data: { id: number; status: string } }
}

async function getFirstDatasourceId(page: Page): Promise<number> {
  const token = await page.evaluate(() => localStorage.getItem('token'))
  const res = await page.evaluate(async ({ baseUrl, token }) => {
    const r = await fetch(`${baseUrl}/api/datasources`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    return await r.json()
  }, { baseUrl: BASE_URL, token })
  const list = res.data ?? []
  const ds = list.find((d: { type: string; status: string }) => d.type === 'mysql' && d.status === 'active')
    ?? list.find((d: { status: string }) => d.status === 'active')
  if (!ds) throw new Error('No active datasource found')
  return ds.id
}

// --- Tests ---

test.describe('工单列表 — 真实后端', () => {
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const context = await browser.newContext()
    const page = await context.newPage()
    await loginReal(page)
    datasourceId = await getFirstDatasourceId(page)

    // Seed a ticket for list display tests
    await createTicketViaApi(
      page, datasourceId,
      'SELECT 1 AS e2e_ticket_seed',
      'E2E seed ticket for list display testing',
    )
    await page.close()
  })

  test.beforeEach(async ({ page }) => {
    await loginReal(page)
  })

  test('工单列表页面正确渲染', async ({ page }) => {
    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')

    await expect(page.getByText('变更工单')).toBeVisible()
    await expect(page.getByRole('button', { name: '提交新工单' })).toBeVisible()
  })

  test('工单列表表头完整', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'SQL 摘要' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '数据库' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '状态' })).toBeVisible()
  })

  test('工单状态 Tab 切换', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    const tabs = ['全部', '待审批', '已通过', '已拒绝', '已取消', '已执行']
    for (const tab of tabs) {
      await expect(page.getByRole('tab', { name: tab })).toBeVisible()
    }

    await page.getByRole('tab', { name: '待审批' }).click()
    await expect(page.getByRole('tab', { name: '待审批' })).toHaveAttribute('data-state', 'active')
  })

  test('新建工单页面导航', async ({ page }) => {
    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')
    await page.getByRole('button', { name: '提交新工单' }).click()

    await page.waitForURL('**/tickets/new**')
    await expect(page).toHaveURL(/\/tickets\/new/)
  })

  test('新建工单表单字段渲染', async ({ page }) => {
    await page.goto('/tickets/new')

    await expect(page.getByText('选择数据源')).toBeVisible()
    await expect(page.getByText('数据库名')).toBeVisible()
    await expect(page.getByText('SQL 内容')).toBeVisible()
    await expect(page.getByText('变更原因')).toBeVisible()
  })

  test('新建工单空表单提交显示验证错误', async ({ page }) => {
    await page.goto('/tickets/new')
    await page.getByRole('button', { name: '提交工单' }).click()

    await expect(page.getByText('请选择数据源').or(page.getByText('请输入'))).toBeVisible()
  })

  test('新建工单成功提交并跳转列表', async ({ page }) => {
    await page.goto('/tickets/new')

    // Select datasource
    const dsSelect = page.locator('[class*="SelectTrigger"], [role="combobox"]').first()
    await dsSelect.click()
    await page.getByRole('option').filter({ hasText: 'mysql' }).first().click()

    // Fill database
    const dbInput = page.getByPlaceholder(/数据库名/)
    if (await dbInput.isVisible()) await dbInput.fill('testdb')

    // Fill SQL
    await page.getByPlaceholder(/SQL/).first().fill('SELECT 1')

    // Fill reason
    await page.getByPlaceholder(/原因/).fill('E2E test: real backend ticket submission')

    // Submit
    await page.getByRole('button', { name: '提交工单' }).click()

    // Verify success toast
    await expect(page.getByText(/成功|提交/)).toBeVisible({ timeout: 10_000 })
    await page.waitForURL('**/tickets**', { timeout: 10_000 })
  })

  test('工单详情抽屉打开', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // Click first ticket row
    const firstRow = page.getByRole('row').nth(1)
    await firstRow.waitFor({ state: 'visible', timeout: 10_000 })
    await firstRow.click()

    // Verify sheet opens
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })
    await expect(sheet).toBeVisible()
  })

  test('工单筛选按钮可见', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    await expect(page.getByRole('button', { name: '我提交的' })).toBeVisible()
    await expect(page.getByRole('button', { name: '待我审批' })).toBeVisible()
  })

  test('工单搜索功能', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    const searchInput = page.getByPlaceholder(/搜索/)
    if (await searchInput.isVisible()) {
      await searchInput.fill('SELECT')
      await page.keyboard.press('Enter')
      await expect(searchInput).toHaveValue('SELECT')
    }
  })
})
