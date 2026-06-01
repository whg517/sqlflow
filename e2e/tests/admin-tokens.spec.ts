/**
 * admin-tokens.spec.ts — E2E: API Token management (SF-QA0031)
 *
 * Tests APIs:
 *   authGroup: POST/GET /api/tokens, GET /api/tokens/stats, DELETE /api/tokens/:id
 *   adminGroup: GET /api/admin/tokens, DELETE /api/admin/tokens/:id
 *
 * Security: token only returned at creation, list responses show masked prefix.
 */
import { test, expect, BASE_URL, loginViaUI } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

const TEST_PREFIX = 'e2e-tok-'

async function getToken(): Promise<string> {
  const res = await fetch(`${BASE_URL}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: process.env.E2E_USERNAME ?? 'e2eadmin',
      password: process.env.E2E_PASSWORD ?? 'e2e-test-pass-123',
    }),
  })
  const body: { code: number; data: { token: string } } = await res.json()
  return body.data.token
}

test.describe('Token Management — User scope', () => {
  let createdTokenId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test.afterEach(async ({ page }) => {
    if (createdTokenId) {
      const token = await page.evaluate(() => localStorage.getItem('token')!)
      await page.request.delete(`${BASE_URL}/api/tokens/${createdTokenId}`, {
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {})
      createdTokenId = 0
    }
  })

  test('should create token and return plain token once', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.post(`${BASE_URL}/api/tokens`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: {
        name: `${TEST_PREFIX}${Date.now()}`,
        description: 'E2E test token',
        scopes: ['query:read', 'query:write'],
        expires_days: 30,
      },
    })
    expect(res.status()).toBe(201)
    const body: { code: number; data: { id: number; token: string; token_prefix: string } } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data.token).toBeTruthy() // plain token only at creation
    expect(body.data.token_prefix).toBeTruthy()
    createdTokenId = body.data.id
  })

  test('should reject create with empty name', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.post(`${BASE_URL}/api/tokens`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: { name: '', scopes: ['query:read'] },
    })
    expect(res.status()).toBe(400)
  })

  test('should reject create with empty scopes', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.post(`${BASE_URL}/api/tokens`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: { name: `${TEST_PREFIX}${Date.now()}`, scopes: [] },
    })
    expect(res.status()).toBe(400)
  })

  test('should list my tokens', async ({ page }) => {
    // Create a token first
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const createRes = await page.request.post(`${BASE_URL}/api/tokens`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: { name: `${TEST_PREFIX}${Date.now()}`, scopes: ['query:read'] },
    })
    const createBody = await createRes.json() as { data: { id: number } }
    createdTokenId = createBody.data.id

    // List
    const res = await page.request.get(`${BASE_URL}/api/tokens`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.status()).toBe(200)
    const body: { code: number; data: Array<{ id: number }> } = await res.json()
    expect(body.code).toBe(0)
    expect(body.data.length).toBeGreaterThanOrEqual(1)
  })

  test('should get token stats', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(`${BASE_URL}/api/tokens/stats`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.status()).toBe(200)
    const body: { code: number; data: { total_tokens: number; active_tokens: number } } = await res.json()
    expect(body.code).toBe(0)
    expect(typeof body.data.total_tokens).toBe('number')
    expect(typeof body.data.active_tokens).toBe('number')
  })

  test('should revoke my token', async ({ page }) => {
    // Create
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const createRes = await page.request.post(`${BASE_URL}/api/tokens`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: { name: `${TEST_PREFIX}${Date.now()}`, scopes: ['query:read'], expires_days: 1 },
    })
    const createBody = await createRes.json() as { data: { id: number } }
    const tokenId = createBody.data.id

    // Revoke
    const res = await page.request.delete(`${BASE_URL}/api/tokens/${tokenId}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.status()).toBe(200)
  })

  test('should return 404 when revoking non-existent token', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.delete(`${BASE_URL}/api/tokens/99999`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.status()).toBe(404)
  })
})

test.describe('Token Management — Admin scope', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should list all tokens (admin)', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.get(`${BASE_URL}/api/admin/tokens?page=1&page_size=10`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.status()).toBe(200)
    const body: { code: number } = await res.json()
    expect(body.code).toBe(0)
  })

  test('should revoke any token as admin', async ({ page }) => {
    // Create a user token first
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const createRes = await page.request.post(`${BASE_URL}/api/tokens`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: { name: `${TEST_PREFIX}admin-revoke-${Date.now()}`, scopes: ['query:read'], expires_days: 1 },
    })
    const createBody = await createRes.json() as { data: { id: number } }
    const tokenId = createBody.data.id

    // Admin revoke
    const res = await page.request.delete(`${BASE_URL}/api/admin/tokens/${tokenId}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.status()).toBe(200)
  })

  test('should return 404 for admin revoke of non-existent token', async ({ page }) => {
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const res = await page.request.delete(`${BASE_URL}/api/admin/tokens/99999`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.status()).toBe(404)
  })
})
