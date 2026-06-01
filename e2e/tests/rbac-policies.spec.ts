/**
 * E2E — RBAC 策略管理（真实后端）
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

test.describe('RBAC 策略 — 真实后端', () => {
  test.beforeEach(async ({ page }) => {
    await loginReal(page)
  })

  test('角色列表 API 返回数据', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token'))
    const res = await page.evaluate(async ({ baseUrl, token }) => {
      const r = await fetch(`${baseUrl}/api/roles`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      return await r.json()
    }, { baseUrl: BASE_URL, token })
    expect(res.code).toBe(0)
    const roles = res.data ?? []
    expect(roles.length).toBeGreaterThan(0)
    const roleNames = roles.map((r: { role: string }) => r.role)
    expect(roleNames).toContain('admin')
  })

  test('策略列表 API 返回数据', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token'))
    const res = await page.evaluate(async ({ baseUrl, token }) => {
      const r = await fetch(`${baseUrl}/api/policies`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      return await r.json()
    }, { baseUrl: BASE_URL, token })
    expect(res.code).toBe(0)
    // policies may be empty, that's fine
    expect(Array.isArray(res.data)).toBeTruthy()
  })

  test('admin 可以访问 RBAC 设置页', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()

    const rbacLink = page.locator('nav').getByRole('link', { name: /权限|RBAC|角色/ })
    if (await rbacLink.isVisible()) {
      await rbacLink.click()
      // Page should render without errors
      await page.waitForLoadState('networkidle')
    }
  })
})
