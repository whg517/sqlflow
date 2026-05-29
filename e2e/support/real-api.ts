/**
 * support/real-api.ts — Real backend API helpers for E2E tests (SF-QA0027)
 *
 * All calls hit the real backend — no mocking.
 * Configurable via environment variables (defaults match docker-compose.test.yml).
 */
import { type Page, type APIRequestContext, request, expect } from '@playwright/test'

// --- Config ---

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const DEFAULT_USERNAME = process.env.E2E_USERNAME ?? 'e2e-admin'
const DEFAULT_PASSWORD = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

// Track IDs created during a test run for cleanup.
const createdDatasourceIds: number[] = []
const createdTicketIds: number[] = []
const createdUsernames: string[] = []
const createdMaskRuleIds: number[] = []
const createdSensitiveTableIds: number[] = []

// --- Internal state ---

let _apiContext: APIRequestContext | undefined
let _token: string | undefined

async function getApiContext(): Promise<APIRequestContext> {
  if (!_apiContext) {
    _apiContext = await request.newContext({ baseURL: BASE_URL })
  }
  return _apiContext
}

async function getOrCreateToken(username?: string, password?: string): Promise<string> {
  if (_token) return _token

  const ctx = await getApiContext()
  const resp = await ctx.post('/api/auth/login', {
    data: { username: username ?? DEFAULT_USERNAME, password: password ?? DEFAULT_PASSWORD },
  })
  expect(resp.ok()).toBeTruthy()
  const body = await resp.json()
  expect(body.code).toBe(0)
  _token = body.data.token as string
  return _token!
}

// --- Public API ---

/**
 * Make an authenticated API request.
 */
export async function apiRequest(
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH',
  path: string,
  data?: Record<string, unknown>,
): Promise<import('@playwright/test').APIResponse> {
  const token = await getOrCreateToken()
  const ctx = await getApiContext()
  return ctx.fetch(path, {
    method,
    headers: { Authorization: `Bearer ${token}` },
    data: data ? JSON.stringify(data) : undefined,
  })
}

/**
 * Authenticate via the real API and store JWT in browser localStorage.
 */
export async function login(
  page: Page,
  username?: string,
  password?: string,
): Promise<string> {
  const token = await getOrCreateToken(username, password)

  // Navigate to the app origin first so we can access localStorage.
  await page.goto(BASE_URL + '/login')

  await page.evaluate((jwt) => {
    localStorage.setItem('token', jwt)
  }, token)
  return token
}

/**
 * Login via the real backend and navigate to query page.
 */
export async function loginViaUI(
  page: Page,
  username = DEFAULT_USERNAME,
  password = DEFAULT_PASSWORD,
): Promise<string> {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill(username)
  await page.getByPlaceholder('密码').fill(password)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 15_000 })

  const token = await page.evaluate(() => localStorage.getItem('token'))
  return token ?? ''
}

/**
 * Logout via UI.
 */
export async function logoutViaUI(page: Page): Promise<void> {
  const avatarTrigger = page
    .locator('button')
    .filter({ has: page.locator('[data-slot="avatar-fallback"]') })
    .first()
  await avatarTrigger.click()
  await page.getByText('退出登录').click()
  await page.waitForURL('**/login**', { timeout: 5_000 })
}

/**
 * Wait until the backend health endpoint returns 200.
 */
export async function waitForBackend(timeoutMs = 60_000): Promise<void> {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    try {
      const resp = await fetch(`${BASE_URL}/health`)
      if (resp.ok) return
    } catch {
      // not ready yet
    }
    await new Promise((r) => setTimeout(r, 2_000))
  }
  throw new Error(`Backend at ${BASE_URL} did not become healthy within ${timeoutMs} ms`)
}

// --- Datasource helpers ---

/**
 * Create a MySQL datasource via the real backend API.
 */
export async function createDataSource(
  params?: {
    name?: string
    host?: string
    port?: number
    username?: string
    password?: string
    database?: string
  },
): Promise<Record<string, unknown>> {
  const resp = await apiRequest('POST', '/api/datasources', {
    name: params?.name ?? `e2e-test-ds-${Date.now()}`,
    type: 'mysql',
    host: params?.host ?? 'mysql-test',
    port: params?.port ?? 3306,
    username: params?.username ?? 'root',
    password: params?.password ?? process.env.MYSQL_ROOT_PASSWORD ?? 'e2e-mysql-root-123',
    database: params?.database ?? 'testdb',
  })
  expect(resp.ok()).toBeTruthy()
  const body = await resp.json()
  expect(body.code).toBe(0)
  const ds = body.data as Record<string, unknown>
  createdDatasourceIds.push(ds.id as number)
  return ds
}

/**
 * Find and delete a datasource by name via API.
 */
export async function deleteDatasourceByName(name: string): Promise<void> {
  const token = await getOrCreateToken()
  const ctx = await getApiContext()
  const resp = await ctx.fetch('/api/datasources', {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!resp.ok()) return
  const body = await resp.json()
  const list = (body.data ?? []) as Array<{ id: number; name: string }>
  const ds = list.find((d) => d.name === name)
  if (ds) {
    await apiRequest('DELETE', `/api/datasources/${ds.id}`)
  }
}

// --- User helpers ---

/**
 * Create a user via API. Tracks for cleanup.
 */
export async function createTestUser(params: {
  username: string
  password?: string
  role?: string
}): Promise<Record<string, unknown>> {
  const resp = await apiRequest('POST', '/api/users', {
    username: params.username,
    password: params.password ?? 'Test123456',
    role: params.role ?? 'developer',
  })
  expect(resp.ok()).toBeTruthy()
  const body = await resp.json()
  expect(body.code).toBe(0)
  createdUsernames.push(params.username)
  return body.data as Record<string, unknown>
}

/**
 * Delete a user by username via API (best-effort).
 */
export async function deleteTestUser(username: string): Promise<void> {
  const token = await getOrCreateToken()
  const ctx = await getApiContext()
  const resp = await ctx.fetch('/api/users', {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!resp.ok()) return
  const body = await resp.json()
  const list = ((body.data?.users ?? body.data) ?? []) as Array<{ id: number; username: string }>
  const user = list.find((u) => u.username === username)
  if (user) {
    await apiRequest('DELETE', `/api/users/${user.id}`)
  }
}

// --- Settings helpers ---

export async function getSettings(): Promise<Record<string, unknown>> {
  const resp = await apiRequest('GET', '/api/settings')
  expect(resp.ok()).toBeTruthy()
  const body = await resp.json()
  return (body.data ?? {}) as Record<string, unknown>
}

export async function updateAISettings(data: Record<string, unknown>): Promise<void> {
  const resp = await apiRequest('PUT', '/api/settings/ai', data)
  expect(resp.ok()).toBeTruthy()
}

export async function updateDingtalkSettings(data: Record<string, unknown>): Promise<void> {
  const resp = await apiRequest('PUT', '/api/settings/dingtalk', data)
  expect(resp.ok()).toBeTruthy()
}

export async function testDingtalkNotify(): Promise<Record<string, unknown>> {
  const resp = await apiRequest('POST', '/api/settings/dingtalk/test', {})
  const body = await resp.json()
  return body as Record<string, unknown>
}

export async function testAIConnection(): Promise<Record<string, unknown>> {
  const resp = await apiRequest('POST', '/api/ai-config/test', {})
  const body = await resp.json()
  return body as Record<string, unknown>
}

// --- Mask rule helpers ---

export async function createMaskRule(data: {
  datasource_id: number
  table_name: string
  column_name: string
  mask_type: string
  mask_length?: number
  sensitivity: string
  description?: string
}): Promise<Record<string, unknown>> {
  const resp = await apiRequest('POST', '/api/mask-rules', data)
  const body = await resp.json()
  if (body.code === 0 && body.data?.id) {
    createdMaskRuleIds.push(body.data.id as number)
  }
  return body as Record<string, unknown>
}

export async function deleteMaskRule(id: number): Promise<void> {
  await apiRequest('DELETE', `/api/mask-rules/${id}`)
}

// --- Sensitive table helpers ---

export async function createSensitiveTable(data: {
  datasource_id: number
  table_name: string
}): Promise<Record<string, unknown>> {
  const resp = await apiRequest('POST', '/api/sensitive-tables', data)
  const body = await resp.json()
  if (body.code === 0 && body.data?.id) {
    createdSensitiveTableIds.push(body.data.id as number)
  }
  return body as Record<string, unknown>
}

// --- Cleanup ---

/**
 * Clean up all test-created resources.
 * Call in afterAll hooks or globalTeardown.
 */
export async function cleanup(): Promise<void> {
  const token = await getOrCreateToken().catch(() => undefined)
  if (!token) return

  for (const id of createdDatasourceIds) {
    try { await apiRequest('DELETE', `/api/datasources/${id}`) } catch { /* best-effort */ }
  }
  createdDatasourceIds.length = 0

  for (const id of createdTicketIds) {
    try { await apiRequest('DELETE', `/api/tickets/${id}`) } catch { /* best-effort */ }
  }
  createdTicketIds.length = 0

  for (const username of createdUsernames) {
    try { await deleteTestUser(username) } catch { /* best-effort */ }
  }
  createdUsernames.length = 0

  for (const id of createdMaskRuleIds) {
    try { await deleteMaskRule(id) } catch { /* best-effort */ }
  }
  createdMaskRuleIds.length = 0

  for (const id of createdSensitiveTableIds) {
    try { await apiRequest('DELETE', `/api/sensitive-tables/${id}`) } catch { /* best-effort */ }
  }
  createdSensitiveTableIds.length = 0
}

/**
 * Reset internal token (e.g., after password change).
 */
export function resetToken(): void {
  _token = undefined
}
