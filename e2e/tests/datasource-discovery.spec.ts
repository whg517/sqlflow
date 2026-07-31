import { test, expect } from '@playwright/test'
import {
  BASE_URL,
  ensureTestUsers,
  loginAsRole,
} from '../support/approval-helpers'
import { apiHelper } from '../support/real-test-helpers'

test.describe('authenticated datasource discovery', () => {
  test.beforeEach(async ({ page }) => {
    await ensureTestUsers(page)
  })

  for (const role of ['developer', 'dba'] as const) {
    test(`${role} can discover safe active datasource summaries`, async ({ page }) => {
      expect(await loginAsRole(page, role)).toBeTruthy()

      const result = await apiHelper(page, 'GET', '/datasources/available')
      expect(result.status).toBe(200)

      const response = result.data as {
        code: number
        data: Array<Record<string, unknown>>
      }
      expect(response.code).toBe(0)
      expect(response.data.length).toBeGreaterThan(0)
      for (const datasource of response.data) {
        expect(Object.keys(datasource).sort()).toEqual(['id', 'name', 'status', 'type'])
        expect(datasource.status).toBe('active')
      }

      const adminResult = await apiHelper(page, 'GET', '/datasources')
      expect(adminResult.status).toBe(403)

      const discoveryResponse = page.waitForResponse(
        (res) => res.url().includes('/api/datasources/available') && res.request().method() === 'GET',
      )
      await page.goto(`${BASE_URL}/query`)
      expect((await discoveryResponse).status()).toBe(200)
      await expect(page.getByText('e2e-shared-mysql').first()).toBeVisible()
    })
  }

  test('anonymous request is rejected', async ({ request }) => {
    const response = await request.get(`${BASE_URL}/api/datasources/available`)
    expect(response.status()).toBe(401)
  })
})
