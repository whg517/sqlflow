import { type Page, type APIRequestContext, request, expect } from '@playwright/test'

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const DEFAULT_USERNAME = process.env.E2E_USERNAME ?? 'admin'
const DEFAULT_PASSWORD = process.env.E2E_PASSWORD ?? 'admin123'

// Track IDs created during a test run so we can clean up afterwards.
const createdDatasourceIds: number[] = []
const createdTicketIds: number[] = []

// ---------------------------------------------------------------------------
// apiRequest — authenticated API helper (Playwright APIRequestContext)
// ---------------------------------------------------------------------------

let _apiContext: APIRequestContext | undefined
let _token: string | undefined

async function getApiContext(): Promise<APIRequestContext> {
  if (!_apiContext) {
    _apiContext = await request.newContext({ baseURL: BASE_URL })
  }
  return _apiContext
}

async function getOrCreateToken(): Promise<string> {
  if (_token) return _token

  const ctx = await getApiContext()
  const resp = await ctx.post('/api/auth/login', {
    data: { username: DEFAULT_USERNAME, password: DEFAULT_PASSWORD },
  })
  expect(resp.ok()).toBeTruthy()
  const body = await resp.json()
  expect(body.code).toBe(0)
  _token = body.data.token as string
  return _token!
}

/**
 * Make an authenticated API request.
 *
 * @example
 *   const resp = await apiRequest('GET', '/api/datasources')
 *   const body = await resp.json()
 */
export async function apiRequest(
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH',
  path: string,
  data?: Record<string, unknown>,
): Promise<import('@playwright/test').APIResponse> {
  const token = await getOrCreateToken()
  const ctx = await getApiContext()
  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
  }
  return ctx.fetch(path, {
    method,
    headers,
    data: data ? JSON.stringify(data) : undefined,
  })
}

// ---------------------------------------------------------------------------
// login — perform real login and persist token in browser localStorage
// ---------------------------------------------------------------------------

/**
 * Authenticate via the real API and store the JWT in the browser's localStorage
 * so that subsequent page navigation uses the real session.
 */
export async function login(page: Page): Promise<void> {
  const token = await getOrCreateToken()

  // Navigate to the app origin first so we can access localStorage.
  await page.goto(BASE_URL + '/login')

  await page.evaluate((jwt) => {
    localStorage.setItem('token', jwt)
  }, token)
}

// ---------------------------------------------------------------------------
// createDataSource — create a test MySQL datasource via real API
// ---------------------------------------------------------------------------

export interface CreateDataSourceParams {
  name?: string
  host?: string
  port?: number
  username?: string
  password?: string
  database?: string
}

/**
 * Create a MySQL datasource via the real backend API.
 * Returns the API response body `data` field (the created datasource object).
 * The ID is tracked for automatic cleanup.
 */
export async function createDataSource(
  params?: CreateDataSourceParams,
): Promise<Record<string, unknown>> {
  const resp = await apiRequest('POST', '/api/datasources', {
    name: params?.name ?? 'e2e-test-datasource',
    type: 'mysql',
    host: params?.host ?? 'mysql-test',
    port: params?.port ?? 3306,
    username: params?.username ?? 'root',
    password: params?.password ?? '123456',
    database: params?.database ?? 'testdb',
  })
  expect(resp.ok()).toBeTruthy()
  const body = await resp.json()
  expect(body.code).toBe(0)
  const ds = body.data as Record<string, unknown>
  createdDatasourceIds.push(ds.id as number)
  return ds
}

// ---------------------------------------------------------------------------
// cleanup — delete all test-created resources
// ---------------------------------------------------------------------------

/**
 * Remove datasources and tickets that were created during the test run.
 * Call this in an `afterAll` hook.
 *
 * Best-effort: swallows errors so one cleanup failure doesn't mask test results.
 */
export async function cleanup(): Promise<void> {
  const token = await getOrCreateToken().catch(() => undefined)
  if (!token) return

  // Clean up datasources
  for (const id of createdDatasourceIds) {
    try {
      await apiRequest('DELETE', `/api/datasources/${id}`)
    } catch {
      // best-effort
    }
  }
  createdDatasourceIds.length = 0

  // Clean up tickets
  for (const id of createdTicketIds) {
    try {
      await apiRequest('DELETE', `/api/tickets/${id}`)
    } catch {
      // best-effort
    }
  }
  createdTicketIds.length = 0
}

// ---------------------------------------------------------------------------
// waitForBackend — poll the health endpoint until it responds
// ---------------------------------------------------------------------------

/**
 * Wait until the backend health endpoint returns 200.
 * Times out after `timeoutMs` (default 30 s).
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
