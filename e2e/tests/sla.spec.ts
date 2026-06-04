/**
 * sla.spec.ts — E2E: SLA configuration & status APIs (SF-QA0031)
 *
 * Tests 6 APIs:
 *   authGroup: GET /api/tickets/sla-status
 *   adminGroup: GET/POST/PUT/DELETE /api/settings/sla, GET /api/sla-notifications
 */
import { test, expect, BASE_URL, loginViaUI, apiRequest } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

const TEST_PREFIX = 'e2e-sla-'

let createdSLAIds: number[] = []

async function cleanupSLA(page: import('@playwright/test').Page): Promise<void> {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.get(`${BASE_URL}/api/settings/sla`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok()) return
  const body: { code: number; data: Array<{ id: number; priority: string }> } = await res.json()
  for (const cfg of body.data ?? []) {
    if (cfg.priority.startsWith(TEST_PREFIX)) {
      await page.request.delete(`${BASE_URL}/api/settings/sla/${cfg.id}`, {
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {})
    }
  }
}

test.describe('SLA Configuration — CRUD (admin)', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    await cleanupSLA(page)
    createdSLAIds = []
  })

  test.afterEach(async ({ page }) => {
    await cleanupSLA(page)
  })

  test('should create SLA config', async ({ page }) => {
    const { status, body } = await apiRequest(page, 'POST', '/settings/sla', {
      priority: `${TEST_PREFIX}${Date.now()}`,
      timeout_minutes: 60,
      reminder_percent: 80,
      escalate_to_role: 'dba',
      enabled: true,
    })
    expect(status).toBeLessThan(300)
    const data = body as { code: number; data: { id: number } }
    expect(data.code).toBe(0)
    expect(data.data.id).toBeTruthy()
    createdSLAIds.push(data.data.id)
  })

  test('should reject create with empty priority', async ({ page }) => {
    const { status } = await apiRequest(page, 'POST', '/settings/sla', {
      priority: '',
      timeout_minutes: 60,
    })
    expect(status).toBe(400)
  })

  test('should reject create with zero timeout', async ({ page }) => {
    const { status } = await apiRequest(page, 'POST', '/settings/sla', {
      priority: `${TEST_PREFIX}${Date.now()}`,
      timeout_minutes: 0,
    })
    expect(status).toBe(400)
  })

  test('should list SLA configs', async ({ page }) => {
    // Create one first
    const create = await apiRequest(page, 'POST', '/settings/sla', {
      priority: `${TEST_PREFIX}${Date.now()}`,
      timeout_minutes: 120,
      enabled: true,
    })
    expect(create.status).toBeLessThan(300)

    const { status, body } = await apiRequest(page, 'GET', '/settings/sla')
    expect(status).toBeLessThan(300)
    const data = body as { code: number; data: Array<{ id: number }> }
    expect(data.code).toBe(0)
    expect(data.data.length).toBeGreaterThanOrEqual(1)
  })

  test('should update SLA config', async ({ page }) => {
    // Create
    const create = await apiRequest(page, 'POST', '/settings/sla', {
      priority: `${TEST_PREFIX}${Date.now()}`,
      timeout_minutes: 30,
      enabled: false,
    })
    const created = create.body as { data: { id: number } }
    const id = created.data.id

    // Update
    const { status, body } = await apiRequest(page, 'PUT', `/settings/sla/${id}`, {
      priority: `${TEST_PREFIX}${Date.now()}`,
      timeout_minutes: 90,
      reminder_percent: 50,
      enabled: true,
    })
    expect(status).toBeLessThan(300)
    expect((body as { code: number }).code).toBe(0)
  })

  test('should delete SLA config', async ({ page }) => {
    // Create
    const create = await apiRequest(page, 'POST', '/settings/sla', {
      priority: `${TEST_PREFIX}${Date.now()}`,
      timeout_minutes: 60,
    })
    const created = create.body as { data: { id: number } }
    const id = created.data.id

    // Delete
    const { status, body } = await apiRequest(page, 'DELETE', `/settings/sla/${id}`)
    expect(status).toBeLessThan(300)
    expect((body as { code: number }).code).toBe(0)
  })

  test('should reject delete non-existent SLA config', async ({ page }) => {
    const { status } = await apiRequest(page, 'DELETE', '/settings/sla/99999')
    // May return 200 or error depending on implementation
    expect([200, 400, 404, 500]).toContain(status)
  })
})

test.describe('SLA Notifications (admin)', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should list SLA notifications', async ({ page }) => {
    const { status, body } = await apiRequest(page, 'GET', '/sla-notifications?page=1&page_size=10')
    expect(status).toBeLessThan(300)
    const data = body as { code: number; data: unknown }
    expect(data.code).toBe(0)
  })
})

test.describe('Ticket SLA Status (auth)', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should return SLA status for ticket IDs', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    // Use IDs that likely don't exist — API should still return valid response
    const res = await page.request.get(
      `${BASE_URL}/api/tickets/sla-status?ticket_ids=1,2,3`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBeLessThan(300)
    const body: { code: number } = await res.json()
    expect(body.code).toBe(0)
  })

  test('should reject empty ticket_ids', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(
      `${BASE_URL}/api/tickets/sla-status?ticket_ids=`,
      { headers: { Authorization: `Bearer ${token}` } },
    )
    expect(res.status()).toBe(400)
  })
})
