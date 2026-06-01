/**
 * support/real-test-helpers.ts — Shared helpers for unified real E2E tests (SF-QA0028)
 *
 * Eliminates per-spec login boilerplate. All specs import from here:
 *   import { test, expect, BASE_URL, ADMIN_USER, ADMIN_PASS, loginViaUI, apiHelper, getFirstDatasourceId } from '../support/real-test-helpers'
 */
import { test as base, expect, request, type Page } from '@playwright/test'

// --- Config (defaults match docker-compose.test.yml) ---

export const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
export const ADMIN_USER = process.env.E2E_USERNAME ?? 'e2eadmin'
export const ADMIN_PASS = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

// --- Re-export base test & expect ---

export { expect }

/**
 * Extended test base with built-in login helper and describe.configure for 45s timeout.
 */
export const test = base.extend<{ authenticatedPage: Page }>({})

// --- Login helpers ---

/**
 * Login via real backend UI — fill username/password and click login button.
 * Waits for redirect to /query.
 */
export async function loginViaUI(page: Page, username = ADMIN_USER, password = ADMIN_PASS): Promise<string> {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill(username)
  await page.getByPlaceholder('密码').fill(password)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 15_000 })
  return page.evaluate(() => localStorage.getItem('token') ?? '')
}

/**
 * Login via real backend API — inject token into localStorage.
 * Returns the JWT token.
 */
export async function loginViaApi(page: Page, username = ADMIN_USER, password = ADMIN_PASS): Promise<string> {
  const loginRes = await page.request.post(`${BASE_URL}/api/auth/login`, {
    data: { username, password },
  })
  expect(loginRes.ok(), `Login failed: ${loginRes.status()}`).toBeTruthy()
  const body: { code: number; data: { token: string } } = await loginRes.json()
  expect(body.code).toBe(0)
  const token = body.data.token

  await page.goto(`${BASE_URL}/login`)
  await page.evaluate((t) => localStorage.setItem('token', t), token)
  return token
}

/**
 * Get a JWT token via fetch (no page needed, for beforeAll hooks).
 */
export async function getToken(username = ADMIN_USER, password = ADMIN_PASS): Promise<string> {
  const res = await fetch(`${BASE_URL}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  const body: { code: number; data: { token: string } } = await res.json()
  if (body.code !== 0) throw new Error(`getToken failed for ${username}`)
  return body.data.token
}

/**
 * Make an authenticated API call using fetch inside page.evaluate.
 * Returns parsed JSON response.
 */
export async function apiHelper(
  page: Page,
  method: string,
  path: string,
  body?: unknown,
): Promise<{ status: number; data: Record<string, unknown> }> {
  return page.evaluate(
    async ({ baseUrl, method, path, body }) => {
      const token = localStorage.getItem('token')
      const headers: Record<string, string> = { 'Content-Type': 'application/json' }
      if (token) headers['Authorization'] = `Bearer ${token}`
      const r = await fetch(`${baseUrl}/api${path}`, {
        method,
        headers,
        body: body != null ? JSON.stringify(body) : undefined,
      })
      return { status: r.status, data: await r.json() }
    },
    { baseUrl: BASE_URL, method, path, body },
  )
}

/**
 * Make an authenticated API call using Playwright request context (page.request).
 */
export async function apiRequest(
  page: Page,
  method: 'GET' | 'POST' | 'PUT' | 'DELETE',
  path: string,
  data?: Record<string, unknown>,
): Promise<{ status: number; body: unknown }> {
  const token = await page.evaluate(() => localStorage.getItem('token'))
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`
  const res = await page.request.fetch(`${BASE_URL}/api${path}`, {
    method,
    headers,
    data: data ? JSON.stringify(data) : undefined,
  })
  return { status: res.status(), body: await res.json() }
}

// --- Datasource helpers ---

/**
 * Get the first active datasource ID and name from the real backend.
 */
export async function getFirstDatasourceId(page: Page): Promise<{ id: number; name: string }> {
  const token = await page.evaluate(() => localStorage.getItem('token'))
  const res: { code: number; data: Array<{ id: number; name: string; type: string; status: string }> } =
    await page.evaluate(async ({ baseUrl, token }) => {
      const r = await fetch(`${baseUrl}/api/datasources`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      return await r.json()
    }, { baseUrl: BASE_URL, token })
  expect(res.code).toBe(0)
  const list = res.data ?? []
  const ds = list.find((d) => d.type === 'mysql' && d.status === 'active')
    ?? list.find((d) => d.status === 'active')
  expect(ds, 'No active datasource found').toBeTruthy()
  return { id: ds!.id, name: ds!.name }
}

/**
 * Create an e2e-prefixed datasource for test isolation.
 */
export async function createTestDatasource(page: Page, params?: { name?: string }): Promise<number> {
  const dsName = params?.name ?? `e2e-ds-${Date.now()}`
  const { status, body } = await apiRequest(page, 'POST', '/datasources', {
    name: dsName,
    type: 'mysql',
    host: 'mysql-test',
    port: 3306,
    username: 'root',
    password: process.env.MYSQL_ROOT_PASSWORD ?? 'e2e-mysql-root-123',
    database: 'testdb',
  })
  expect(status).toBe(200)
  const data = body as { code: number; data: { id: number } }
  expect(data.code).toBe(0)
  return data.data.id
}

/**
 * Delete a datasource by name (best-effort).
 */
export async function deleteDatasourceByName(page: Page, name: string): Promise<void> {
  try {
    await apiHelper(page, 'GET', '/datasources').then(async ({ data }) => {
      const list = (data as { data?: Array<{ id: number; name: string }> }).data ?? []
      const ds = list.find((d) => d.name === name)
      if (ds) await apiHelper(page, 'DELETE', `/datasources/${ds.id}`)
    })
  } catch {
    // best-effort
  }
}

// --- Cleanup ---

/**
 * Delete all e2e-prefixed datasources. Call in afterAll.
 */
export async function cleanupDatasources(page?: Page): Promise<void> {
  try {
    const token = await getToken()
    const res = await fetch(`${BASE_URL}/api/datasources`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (!res.ok) return
    const body: { data: Array<{ id: number; name: string }> } = await res.json()
    for (const ds of body.data ?? []) {
      if (ds.name.startsWith('e2e-')) {
        await fetch(`${BASE_URL}/api/datasources/${ds.id}`, {
          method: 'DELETE',
          headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        }).catch(() => {})
      }
    }
  } catch {
    // best-effort
  }
}

/**
 * Delete all e2e-prefixed users. Call in afterAll.
 */
export async function cleanupUsers(): Promise<void> {
  try {
    const token = await getToken()
    const res = await fetch(`${BASE_URL}/api/users`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (!res.ok) return
    const body: { data: { users: Array<{ id: number; username: string }> } } = await res.json()
    for (const user of body.data?.users ?? []) {
      if (user.username.startsWith('e2e_')) {
        await fetch(`${BASE_URL}/api/users/${user.id}`, {
          method: 'DELETE',
          headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        }).catch(() => {})
      }
    }
  } catch {
    // best-effort
  }
}
