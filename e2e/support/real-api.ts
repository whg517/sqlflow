/**
 * support/real-api.ts — Real backend API helpers for real E2E tests
 *
 * Migrated from web/e2e-real/helpers.ts.
 * No mocking — all calls hit the real backend.
 */
import { type Page, type APIRequestContext, request, expect } from '@playwright/test'

// --- Config ---

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const DEFAULT_USERNAME = process.env.E2E_USERNAME ?? 'e2e-admin'
const DEFAULT_PASSWORD = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

// Track IDs created during a test run for cleanup.
const createdDatasourceIds: number[] = []
const createdTicketIds: number[] = []

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
): Promise<void> {
  const token = await getOrCreateToken(username, password)

  // Navigate to the app origin first so we can access localStorage.
  await page.goto(BASE_URL + '/login')

  await page.evaluate((jwt) => {
    localStorage.setItem('token', jwt)
  }, token)
}

/**
 * Wait until the backend health endpoint returns 200.
 */
export async function waitForBackend(timeoutMs = 30_000): Promise<void> {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    try {
      const resp = await fetch(`${BASE_URL}/health`)
      if (resp.ok) return
    } catch {
      // not ready yet
    }
    await new Promise((r) => setTimeout(r, 1_000))
  }
  throw new Error(`Backend at ${BASE_URL} did not become healthy within ${timeoutMs} ms`)
}

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
    name: params?.name ?? 'e2e-test-datasource',
    type: 'mysql',
    host: params?.host ?? 'mysql-test',
    port: params?.port ?? 3306,
    username: params?.username ?? 'root',
    password: params?.password ?? DEFAULT_PASSWORD,
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
 * Clean up all test-created resources.
 * Call in afterAll hooks or globalTeardown.
 */
export async function cleanup(): Promise<void> {
  const token = await getOrCreateToken().catch(() => undefined)
  if (!token) return

  for (const id of createdDatasourceIds) {
    try {
      await apiRequest('DELETE', `/api/datasources/${id}`)
    } catch {
      // best-effort
    }
  }
  createdDatasourceIds.length = 0

  for (const id of createdTicketIds) {
    try {
      await apiRequest('DELETE', `/api/tickets/${id}`)
    } catch {
      // best-effort
    }
  }
  createdTicketIds.length = 0
}

/**
 * Reset internal token (e.g., after password change).
 */
export function resetToken(): void {
  _token = undefined
}
