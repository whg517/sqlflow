/**
 * performance.spec.ts — E2E: Performance monitoring APIs (SF-QA0031)
 *
 * Tests 2 APIs (authGroup):
 *   GET /api/query/performance/slow  — list slow queries
 *   GET /api/query/performance/stats — performance stats
 */
import { test, expect, BASE_URL, loginViaUI } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('Performance Monitoring', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should return slow query list', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/query/performance/slow?threshold=100&page=1&page_size=10`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBeLessThan(300)
    const body: { code: number; data: unknown[]; total?: number } = await res.json()
    expect(body.code).toBe(0)
    expect(Array.isArray(body.data)).toBeTruthy()
  })

  test('should return performance stats', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/query/performance/stats?days=7`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBeLessThan(300)
    const body: { code: number; data: Record<string, unknown> } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data).toBeTruthy()
  })

  test('should return empty list when threshold is very high', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/query/performance/slow?threshold=99999999`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBeLessThan(300)
    const body: { code: number; data: unknown[] } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data.length).toBe(0)
  })

  test('should accept custom threshold and date range', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/query/performance/slow?threshold=500&start_date=2024-01-01&end_date=2030-12-31`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBeLessThan(300)
    const body: { code: number } = await res.json()
    expect(body.code).toBe(0)
  })

  test('should accept custom days for stats', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/query/performance/stats?days=30`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBeLessThan(300)
    const body: { code: number; data: Record<string, unknown> } = await res.json()
    expect(body.code).toBe(0)
  })
})
