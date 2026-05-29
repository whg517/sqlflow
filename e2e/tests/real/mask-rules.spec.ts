/**
 * E2E — 脱敏规则管理（真实后端）
 * SF-QA0027: Migrated from mock/tests/mock/mask-rules.spec.ts
 *
 * Tests mask rule CRUD, permission checks, and boundary scenarios.
 */
import { test, expect } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const ADMIN_USER = process.env.E2E_USERNAME ?? 'e2e-admin'
const ADMIN_PASS = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

async function loginReal(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill(ADMIN_USER)
  await page.getByPlaceholder('密码').fill(ADMIN_PASS)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 15_000 })
}

async function maskRuleApi(
  page: import('@playwright/test').Page,
  method: string,
  path: string,
  body?: unknown,
) {
  const token = await page.evaluate(() => localStorage.getItem('token'))
  return page.evaluate(async ({ baseUrl, token, method, path, body }) => {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    }
    const r = await fetch(`${baseUrl}/api${path}`, {
      method,
      headers,
      body: body != null ? JSON.stringify(body) : undefined,
    })
    return { status: r.status, data: await r.json() }
  }, { baseUrl: BASE_URL, token, method, path, body })
}

async function getFirstActiveDatasourceId(page: import('@playwright/test').Page): Promise<number> {
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
  return ds?.id ?? 0
}

// --- Tests ---

test.describe('脱敏规则管理 — 真实后端', () => {
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const context = await browser.newContext()
    const page = await context.newPage()
    await loginReal(page)
    datasourceId = await getFirstActiveDatasourceId(page)
    await page.close()
  })

  test.beforeEach(async ({ page }) => {
    await loginReal(page)
  })

  test('导航到脱敏规则页并验证渲染', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    await expect(page.getByText('脱敏规则').first()).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '表名' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '列名' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '脱敏类型' })).toBeVisible()
  })

  test('脱敏规则 API CRUD — 创建、查询、删除', async ({ page }) => {
    const uniqueCol = `e2e_mask_${Date.now()}`

    // Create
    const createRes = await maskRuleApi(page, 'POST', '/mask-rules', {
      datasource_id: datasourceId,
      table_name: 'sys_user',
      column_name: uniqueCol,
      mask_type: 'partial',
      mask_length: 3,
      sensitivity: 'high',
      description: 'E2E test mask rule',
    })
    expect(createRes.data.code).toBe(0)
    const ruleId = createRes.data.data?.id
    expect(ruleId).toBeGreaterThan(0)

    // List — should contain new rule
    const listRes = await maskRuleApi(page, 'GET', '/mask-rules')
    expect(listRes.data.code).toBe(0)
    const rules = listRes.data.data ?? []
    const found = rules.find((r: { id: number; column_name: string }) => r.id === ruleId)
    expect(found).toBeDefined()
    expect(found.column_name).toBe(uniqueCol)

    // Delete
    const delRes = await maskRuleApi(page, 'DELETE', `/mask-rules/${ruleId}`)
    expect(delRes.data.code).toBe(0)
  })

  test('空规则列表显示空状态', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    // If no rules, should show empty state
    const emptyText = page.getByText(/暂无|没有/)
    const rows = page.getByRole('row')
    if (!(await rows.nth(1).isVisible().catch(() => false))) {
      await expect(emptyText).toBeVisible()
    }
  })

  test('admin 可以访问脱敏规则页', async ({ page }) => {
    await page.goto('/settings/mask')
    await expect(page).toHaveURL(/\/settings\/mask/)
  })
})
