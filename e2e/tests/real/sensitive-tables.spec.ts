/**
 * E2E — 敏感表管理（真实后端）
 * SF-QA0027: Migrated from mock/tests/mock/sensitive-tables.spec.ts
 */
import { test, expect } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const ADMIN_USER = process.env.E2E_USERNAME ?? 'e2e-admin'
const ADMIN_PASS = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

test.describe.configure({ timeout: 45_000 })

async function loginReal(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill(ADMIN_USER)
  await page.getByPlaceholder('密码').fill(ADMIN_PASS)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 15_000 })
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

test.describe('敏感表管理 — 真实后端', () => {
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

  test('导航到敏感表页并验证渲染', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()

    const stLink = page.locator('nav').getByRole('link', { name: '敏感表' })
    if (await stLink.isVisible()) {
      await stLink.click()
      await page.waitForURL('**/settings/sensitive')

      // Page should render without errors
      await expect(page.getByText(/敏感/).first()).toBeVisible()
    }
  })

  test('敏感表 API CRUD', async ({ page }) => {
    // Create
    const token = await page.evaluate(() => localStorage.getItem('token'))
    const createRes = await page.evaluate(async ({ baseUrl, token, dsId }) => {
      const r = await fetch(`${baseUrl}/api/sensitive-tables`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ datasource_id: dsId, table_name: 'sys_user' }),
      })
      return await r.json()
    }, { baseUrl: BASE_URL, token, dsId: datasourceId })
    expect(createRes.code).toBe(0)

    // List
    const listRes = await page.evaluate(async ({ baseUrl, token }) => {
      const r = await fetch(`${baseUrl}/api/sensitive-tables`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      return await r.json()
    }, { baseUrl: BASE_URL, token })
    expect(listRes.code).toBe(0)
    const tables = listRes.data ?? []
    expect(tables.length).toBeGreaterThan(0)

    // Cleanup: delete created entry
    if (createRes.data?.id) {
      await page.evaluate(async ({ baseUrl, token, id }) => {
        await fetch(`${baseUrl}/api/sensitive-tables/${id}`, {
          method: 'DELETE',
          headers: { Authorization: `Bearer ${token}` },
        })
      }, { baseUrl: BASE_URL, token, id: createRes.data.id })
    }
  })
})
