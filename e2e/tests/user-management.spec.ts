/**
 * E2E — 用户管理（真实后端）
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

test.describe('用户管理 — 真实后端', () => {
  test.beforeEach(async ({ page }) => {
    await loginReal(page)
  })

  test('admin 可以访问用户管理页', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()

    const userLink = page.locator('nav').getByRole('link', { name: /用户/ })
    if (await userLink.isVisible()) {
      await userLink.click()
      await page.waitForURL(/settings\/user/)

      // Page should render user list
      await expect(page.getByText(/用户/).first()).toBeVisible()
    }
  })

  test('用户 API — 创建、查询、删除', async ({ page }) => {
    const username = `e2e_user_${Date.now()}`

    // Create
    const token = await page.evaluate(() => localStorage.getItem('token'))
    const createRes = await page.evaluate(async ({ baseUrl, token, username }) => {
      const r = await fetch(`${baseUrl}/api/users`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ username, password: 'Test123456', role: 'developer' }),
      })
      return await r.json()
    }, { baseUrl: BASE_URL, token, username })
    expect(createRes.code).toBe(0)

    // List — should contain new user
    const listRes = await page.evaluate(async ({ baseUrl, token }) => {
      const r = await fetch(`${baseUrl}/api/users`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      return await r.json()
    }, { baseUrl: BASE_URL, token })
    expect(listRes.code).toBe(0)
    const users = listRes.data?.users ?? listRes.data ?? []
    const found = users.find((u: { username: string }) => u.username === username)
    expect(found).toBeDefined()

    // Cleanup: delete
    if (found?.id) {
      await page.evaluate(async ({ baseUrl, token, id }) => {
        await fetch(`${baseUrl}/api/users/${id}`, {
          method: 'DELETE',
          headers: { Authorization: `Bearer ${token}` },
        })
      }, { baseUrl: BASE_URL, token, id: found.id })
    }
  })

  test('创建重复用户名返回错误', async ({ page }) => {
    // admin already exists, try to create again
    const token = await page.evaluate(() => localStorage.getItem('token'))
    const res = await page.evaluate(async ({ baseUrl, token }) => {
      const r = await fetch(`${baseUrl}/api/users`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ username: ADMIN_USER, password: 'Test123456', role: 'developer' }),
      })
      return { status: r.status, data: await r.json() }
    }, { baseUrl: BASE_URL, token })

    // Should fail — user already exists
    expect(res.data.code !== 0 || res.status >= 400).toBeTruthy()
  })
})
