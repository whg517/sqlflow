/**
 * reports.spec.ts — E2E: Admin report APIs (SF-QA0031)
 *
 * Tests 4 APIs (adminGroup):
 *   GET /api/reports/usage       — usage statistics
 *   GET /api/reports/errors      — error analysis
 *   GET /api/reports/performance — performance trends
 *   GET /api/reports/tickets     — ticket statistics
 */
import { test, expect, BASE_URL, loginViaUI } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('Admin Reports', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should return usage stats', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/reports/usage?days=7`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(200)
    const body: { code: number; data: Record<string, unknown> } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data).toBeTruthy()
  })

  test('should return usage stats for custom days', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/reports/usage?days=30`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(200)
    const body: { code: number } = await res.json()
    expect(body.code).toBe(0)
  })

  test('should return error stats', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/reports/errors?days=7`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(200)
    const body: { code: number; data: Record<string, unknown> } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data).toBeTruthy()
  })

  test('should return performance report', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/reports/performance?days=7`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(200)
    const body: { code: number; data: Record<string, unknown> } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data).toBeTruthy()
  })

  test('should return ticket stats', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/reports/tickets?days=7`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(200)
    const body: { code: number; data: Record<string, unknown> } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data).toBeTruthy()
  })

  test('should handle all 4 report endpoints concurrently', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const headers = { Authorization: `Bearer ${token}` }
    const base = BASE_URL

    const [usageRes, errorRes, perfRes, ticketRes] = await Promise.all([
      page.request.get(`${base}/api/reports/usage?days=7`, { headers }),
      page.request.get(`${base}/api/reports/errors?days=7`, { headers }),
      page.request.get(`${base}/api/reports/performance?days=7`, { headers }),
      page.request.get(`${base}/api/reports/tickets?days=7`, { headers }),
    ])

    expect(usageRes.status()).toBe(200)
    expect(errorRes.status()).toBe(200)
    expect(perfRes.status()).toBe(200)
    expect(ticketRes.status()).toBe(200)
  })
})
