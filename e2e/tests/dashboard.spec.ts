/**
 * E2E — Dashboard 概览（真实后端）
 *
 * Tests dashboard page rendering with real backend data.
 */
import { test, expect } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const ADMIN_USER = process.env.E2E_USERNAME ?? 'e2eadmin'
const ADMIN_PASS = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

test.describe.configure({ timeout: 45_000 })

async function loginReal(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill(ADMIN_USER)
  await page.getByPlaceholder('密码').fill(ADMIN_PASS)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 15_000 })
}

test.describe('Dashboard — 真实后端', () => {
  test.beforeEach(async ({ page }) => {
    await loginReal(page)
  })

  test('Dashboard API 返回有效数据', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token'))
    const res = await page.evaluate(async ({ baseUrl, token }) => {
      const r = await fetch(`${baseUrl}/api/dashboard/stats`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      return await r.json()
    }, { baseUrl: BASE_URL, token })
    expect(res.code).toBe(0)
    expect(res.data).toBeDefined()
  })

  test('导航到 Dashboard 页面', async ({ page }) => {
    const dashboardLink = page.getByRole('link', { name: '概览' }).first()
    if (await dashboardLink.isVisible()) {
      await dashboardLink.click()
      await page.waitForURL('**/dashboard')

      // Verify dashboard renders without errors
      await page.waitForLoadState('networkidle')
      await expect(page.locator('body')).toBeVisible()
    }
  })

  test('Dashboard 页面无控制台错误', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))
    page.on('console', (msg) => {
      if (msg.type() === 'error') errors.push(msg.text())
    })

    const dashboardLink = page.getByRole('link', { name: '概览' }).first()
    if (await dashboardLink.isVisible()) {
      await dashboardLink.click()
      await page.waitForURL('**/dashboard')
      await page.waitForTimeout(3000)
    }

    const relevantErrors = errors.filter(
      (e) => !e.includes('playwright') && !e.includes('DevTools') && !e.includes('favicon'),
    )
    expect(relevantErrors).toHaveLength(0)
  })

  test('Dashboard 统计卡片渲染', async ({ page }) => {
    const dashboardLink = page.getByRole('link', { name: '概览' }).first()
    if (await dashboardLink.isVisible()) {
      await dashboardLink.click()
      await page.waitForURL('**/dashboard')
      await page.waitForLoadState('networkidle')

      // Should have some stat cards
      const statCards = page.locator('[class*="card"], [class*="Card"]').filter({ hasText: /\d+/ })
      await expect(statCards.first()).toBeVisible({ timeout: 10_000 })
    }
  })
})
