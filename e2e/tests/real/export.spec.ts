/**
 * E2E — 导出功能（真实后端）
 * SF-QA0027: Migrated from mock/tests/mock/export/export.spec.ts
 *
 * Tests audit log export and ticket export against real backend.
 */
import { test, expect, type Download } from '@playwright/test'

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

test.describe('审计日志导出 — 真实后端', () => {
  test.beforeEach(async ({ page }) => {
    await loginReal(page)
  })

  test('审计导出 API 返回 CSV', async ({ request }) => {
    // Login via API
    const loginRes = await request.post(`${BASE_URL}/api/auth/login`, {
      data: { username: ADMIN_USER, password: ADMIN_PASS },
    })
    const loginBody = await loginRes.json()
    const token = loginBody.data.token

    const exportRes = await request.get(`${BASE_URL}/api/export/audit`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    // Should return 200 with CSV content or empty result
    expect(exportRes.status()).toBeLessThan(500)
    const contentType = exportRes.headers()['content-type'] ?? ''
    expect(contentType).toContain('text/csv')
  })

  test('审计页面导出按钮可见', async ({ page }) => {
    await page.goto('/audit')
    await page.waitForLoadState('networkidle')

    const exportBtn = page.getByRole('button', { name: /导出/ })
    await expect(exportBtn).toBeVisible()
  })
})

test.describe('工单导出 — 真实后端', () => {
  test.beforeEach(async ({ page }) => {
    await loginReal(page)
  })

  test('工单导出 API 返回 CSV', async ({ request }) => {
    const loginRes = await request.post(`${BASE_URL}/api/auth/login`, {
      data: { username: ADMIN_USER, password: ADMIN_PASS },
    })
    const loginBody = await loginRes.json()
    const token = loginBody.data.token

    const exportRes = await request.get(`${BASE_URL}/api/export/tickets`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(exportRes.status()).toBeLessThan(500)
    const contentType = exportRes.headers()['content-type'] ?? ''
    expect(contentType).toContain('text/csv')
  })

  test('工单页面导出按钮可见', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForLoadState('networkidle')

    const exportBtn = page.getByRole('button', { name: /导出/ })
    await expect(exportBtn).toBeVisible()
  })
})
