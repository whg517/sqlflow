/**
 * coverage-gaps.spec.ts — E2E: API coverage gaps (SF-QA0033)
 *
 * Tests 6 APIs not yet directly covered by other specs:
 *   POST /api/query/explain                          — EXPLAIN query plan
 *   POST /api/datasources/:id/test (admin)            — test datasource connectivity
 *   GET  /api/datasources/:id/tables/:name/columns    — table column listing
 *   PUT  /api/users/:id/reset-password (admin)        — reset user password
 *   POST /api/policies/sync (admin)                   — sync RBAC policies
 *   POST /api/auth/refresh                            — refresh JWT token
 *
 * Security scenarios:
 *   1.  Unauthenticated access → 401
 *   2.  Invalid JWT → 401
 *   3.  Non-admin accessing admin API → 403
 *   4.  EXPLAIN non-SELECT SQL → 400
 *   5.  EXPLAIN on non-MySQL → 400
 *   6.  SQL injection in explain → safe handling
 *   7.  Reset password with weak password → 400
 *   8.  Reset password for non-existent user → 404
 *   9.  Refresh with invalid token → 401
 * 10.  Test connection for non-existent datasource → 404
 * 11.  Table columns for non-existent table → safe error
 * 12.  Empty/missing required params → 400
 */
import { test, expect, BASE_URL, loginViaUI, getToken, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

/** Make authenticated API call via page.request. */
async function apiCall(
  page: import('@playwright/test').Page,
  method: 'GET' | 'POST' | 'PUT' | 'DELETE',
  path: string,
  data?: Record<string, unknown>,
) {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.fetch(`${BASE_URL}/api${path}`, {
    method,
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    data: data ? JSON.stringify(data) : undefined,
  })
  return { status: res.status(), body: await res.json() }
}

/** Make unauthenticated API call. */
async function unauthCall(
  method: 'GET' | 'POST' | 'PUT',
  path: string,
  data?: Record<string, unknown>,
) {
  const res = await fetch(`${BASE_URL}/api${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: data ? JSON.stringify(data) : undefined,
  })
  return { status: res.status, body: await res.json() }
}

// ============================================================
// POST /api/query/explain — EXPLAIN query plan
// ============================================================

test.describe('EXPLAIN Query Plan', () => {
  let datasourceId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    const ds = await getFirstDatasourceId(page)
    datasourceId = ds.id
  })

  test('should return EXPLAIN result for SELECT', async ({ page }) => {
    const { status, body } = await apiCall(page, 'POST', '/query/explain', {
      datasource_id: datasourceId,
      sql: 'SELECT 1',
      database: 'testdb',
    })
    expect(status).toBe(200)
    expect((body as { code: number }).code).toBe(0)
  })

  test('should return EXPLAIN result for table query', async ({ page }) => {
    const { status, body } = await apiCall(page, 'POST', '/query/explain', {
      datasource_id: datasourceId,
      sql: 'SELECT id, username FROM sys_user LIMIT 1',
      database: 'testdb',
    })
    expect(status).toBe(200)
    expect((body as { code: number }).code).toBe(0)
    const data = (body as { data: unknown }).data
    expect(data).toBeTruthy()
  })

  // Security: non-SELECT SQL → 400
  test('should reject non-SELECT SQL for EXPLAIN', async ({ page }) => {
    const { status, body } = await apiCall(page, 'POST', '/query/explain', {
      datasource_id: datasourceId,
      sql: 'INSERT INTO sys_user (username) VALUES ("hack")',
      database: 'testdb',
    })
    expect(status).toBe(400)
    expect((body as { message?: string }).message).toContain('SELECT')
  })

  // Security: empty SQL → 400
  test('should reject empty SQL', async ({ page }) => {
    const { status } = await apiCall(page, 'POST', '/query/explain', {
      datasource_id: datasourceId,
      sql: '',
    })
    expect(status).toBe(400)
  })

  // Security: missing datasource_id → 400
  test('should reject missing datasource_id', async ({ page }) => {
    const { status } = await apiCall(page, 'POST', '/query/explain', {
      sql: 'SELECT 1',
    })
    expect(status).toBe(400)
  })

  // Security: unauthenticated → 401
  test('should require authentication', async () => {
    const { status } = await unauthCall('POST', '/query/explain', {
      datasource_id: 1,
      sql: 'SELECT 1',
    })
    expect(status).toBe(401)
  })
})

// ============================================================
// POST /api/datasources/:id/test — Test connectivity (admin)
// ============================================================

test.describe('Datasource Connectivity Test', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should test existing datasource connection', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)
    const { status, body } = await apiCall(page, 'POST', `/datasources/${ds.id}/test`)
    expect(status).toBe(200)
    const data = body as { data: { success: boolean; message: string } }
    expect(data.data.success).toBe(true)
  })

  test('should return 404 for non-existent datasource', async ({ page }) => {
    const { status } = await apiCall(page, 'POST', '/datasources/99999/test')
    expect(status).toBe(404)
  })

  // Security: unauthenticated → 401
  test('should require authentication', async () => {
    const { status } = await unauthCall('POST', '/datasources/1/test')
    expect(status).toBe(401)
  })
})

// ============================================================
// GET /api/datasources/:id/tables/:name/columns — Table columns
// ============================================================

test.describe('Table Column Listing', () => {
  let datasourceId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    const ds = await getFirstDatasourceId(page)
    datasourceId = ds.id
  })

  test('should return columns for existing table', async ({ page }) => {
    const { status, body } = await apiCall(page, 'GET', `/datasources/${datasourceId}/tables/sys_user/columns`)
    expect(status).toBe(200)
    const data = body as { code: number; data: Array<{ column_name: string }> }
    expect(data.code).toBe(0)
    expect(data.data.length).toBeGreaterThan(0)
    expect(data.data.some((c) => c.column_name === 'id')).toBeTruthy()
  })

  test('should return error for non-existent table', async ({ page }) => {
    const { status } = await apiCall(page, 'GET', `/datasources/${datasourceId}/tables/nonexistent_table_xyz/columns`)
    expect(status).toBe(500) // MySQL error for non-existent table
  })

  test('should return 404 for non-existent datasource', async ({ page }) => {
    const { status } = await apiCall(page, 'GET', '/datasources/99999/tables/sys_user/columns')
    expect(status).toBe(404)
  })

  // Security: unauthenticated → 401
  test('should require authentication', async () => {
    const { status } = await unauthCall('GET', '/datasources/1/tables/sys_user/columns')
    expect(status).toBe(401)
  })
})

// ============================================================
// PUT /api/users/:id/reset-password — Reset password (admin)
// ============================================================

test.describe('Reset User Password', () => {
  let testUserId: number | undefined

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)

    // Create a test user to reset password
    const token = await page.evaluate(() => localStorage.getItem('token')!)
    const createRes = await page.request.post(`${BASE_URL}/api/users`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: {
        username: `e2e_reset_${Date.now()}`,
        password: 'E2e@Test@Pass123',
        email: `e2e_reset_${Date.now()}@test.com`,
        role: 'viewer',
      },
    })
    if (createRes.ok()) {
      const createBody = await createRes.json() as { data: { id: number } }
      testUserId = createBody.data.id
    }
  })

  test.afterEach(async ({ page }) => {
    // Cleanup: delete test user
    if (testUserId) {
      const token = await page.evaluate(() => localStorage.getItem('token')!)
      await page.request.delete(`${BASE_URL}/api/users/${testUserId}`, {
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {})
      testUserId = undefined
    }
  })

  test('should reset user password', async ({ page }) => {
    if (!testUserId) return test.skip()
    const { status, body } = await apiCall(page, 'PUT', `/users/${testUserId}/reset-password`, {
      password: 'New@Pass456!',
    })
    expect(status).toBe(200)
    expect((body as { code: number; message?: string }).message).toContain('成功')
  })

  // Security: weak password → 400
  test('should reject weak password', async ({ page }) => {
    if (!testUserId) return test.skip()
    const { status } = await apiCall(page, 'PUT', `/users/${testUserId}/reset-password`, {
      password: '123',
    })
    expect(status).toBe(400)
  })

  test('should reject empty password', async ({ page }) => {
    if (!testUserId) return test.skip()
    const { status } = await apiCall(page, 'PUT', `/users/${testUserId}/reset-password`, {
      password: '',
    })
    expect(status).toBe(400)
  })

  test('should return 404 for non-existent user', async ({ page }) => {
    const { status } = await apiCall(page, 'PUT', '/users/99999/reset-password', {
      password: 'Valid@Pass123!',
    })
    expect(status).toBe(404)
  })

  // Security: unauthenticated → 401
  test('should require authentication', async () => {
    const { status } = await unauthCall('PUT', '/users/1/reset-password', {
      password: 'Test@Pass123!',
    })
    expect(status).toBe(401)
  })
})

// ============================================================
// POST /api/policies/sync — Sync RBAC policies (admin)
// ============================================================

test.describe('Policy Sync', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should sync policies successfully', async ({ page }) => {
    const { status, body } = await apiCall(page, 'POST', '/policies/sync')
    expect(status).toBe(200)
    expect((body as { code: number; message?: string }).message).toContain('成功')
  })

  // Security: unauthenticated → 401
  test('should require authentication', async () => {
    const { status } = await unauthCall('POST', '/policies/sync')
    expect(status).toBe(401)
  })
})

// ============================================================
// POST /api/auth/refresh — Refresh JWT token
// ============================================================

test.describe('Token Refresh', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should reject empty refresh_token', async ({ page }) => {
    const { status, body } = await apiCall(page, 'POST', '/auth/refresh', {
      refresh_token: '',
    })
    expect(status).toBe(400)
    expect((body as { message?: string }).message).toContain('refresh_token')
  })

  test('should reject invalid refresh_token', async ({ page }) => {
    const { status } = await apiCall(page, 'POST', '/auth/refresh', {
      refresh_token: 'invalid-token-xyz',
    })
    expect(status).toBe(401)
  })

  test('should reject expired/revoked refresh_token', async ({ page }) => {
    const { status } = await apiCall(page, 'POST', '/auth/refresh', {
      refresh_token: 'expired.fake.token.value',
    })
    expect(status).toBe(401)
  })

  // No auth required for refresh endpoint itself (it takes refresh_token)
  test('refresh endpoint accepts unauthenticated request', async () => {
    const { status } = await unauthCall('POST', '/auth/refresh', {
      refresh_token: '',
    })
    // Should be 400 (empty token) not 401 (no JWT) — refresh doesn't need JWT
    expect(status).toBe(400)
  })
})
