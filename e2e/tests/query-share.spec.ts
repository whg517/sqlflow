/**
 * query-share.spec.ts — E2E: Query sharing (SF-QA0029)
 *
 * Tests 5 share APIs against the real backend:
 *   POST   /api/query/share          — create share link
 *   GET    /api/query/share          — list my shares
 *   GET    /s/:token                 — get shared result (public, no auth)
 *   POST   /s/:token/verify          — verify share password (public)
 *   DELETE /api/query/share/:id      — revoke share
 *
 * Coverage: create → list → access public page → password verify → revoke
 * Edge cases: empty list, password-protected, wrong password, revoked link
 */
import { test, expect, BASE_URL, loginViaUI, cleanupDatasources } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

/** Create a share via page.request (authenticated). */
async function createShare(
  page: import('@playwright/test').Page,
  body: {
    columns?: string[]
    rows?: Array<Record<string, unknown>>
    expires_in_hours?: number
    password?: string
    sql_summary?: string
    datasource_name?: string
  },
): Promise<{ status: number; data: { code: number; data: { id: number; token: string; expires_at: string } } }> {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.post(`${BASE_URL}/api/query/share`, {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    data: body,
  })
  return { status: res.status(), data: await res.json() }
}

/** List shares via API. */
async function listShares(page: import('@playwright/test').Page) {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.get(`${BASE_URL}/api/query/share`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return { status: res.status(), data: await res.json() }
}

/** Revoke a share by ID. */
async function revokeShare(page: import('@playwright/test').Page, id: number) {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.delete(`${BASE_URL}/api/query/share/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return { status: res.status(), data: await res.json() }
}

/** Access public share page (no auth). */
async function getPublicShare(token: string) {
  const res = await fetch(`${BASE_URL}/s/${token}`)
  return { status: res.status, data: await res.json() }
}

/** Verify share password (public). */
async function verifySharePassword(token: string, password: string) {
  const res = await fetch(`${BASE_URL}/s/${token}/verify`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  })
  return { status: res.status, data: await res.json() }
}

const TEST_COLUMNS = ['id', 'username', 'email']
const TEST_ROWS: Array<Record<string, unknown>> = [
  { id: 1, username: 'e2e_test_user', email: 'e2e@test.com' },
  { id: 2, username: 'e2e_test_user2', email: 'e2e2@test.com' },
]

// --- Tests ---

test.describe('Query Share — API', () => {
  let createdShareIds: number[] = []

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test.afterEach(async ({ page }) => {
    // cleanup: revoke any shares created in this test
    for (const id of createdShareIds) {
      await revokeShare(page, id).catch(() => {})
    }
    createdShareIds = []
  })

  test('should create a share link', async ({ page }) => {
    const { status, data } = await createShare(page, {
      columns: TEST_COLUMNS,
      rows: TEST_ROWS,
      sql_summary: 'SELECT * FROM sys_user',
      datasource_name: 'e2e-mysql',
    })
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    expect(data.data.id).toBeTruthy()
    expect(data.data.token).toBeTruthy()
    createdShareIds.push(data.data.id)
  })

  test('should create a share with password', async ({ page }) => {
    const { status, data } = await createShare(page, {
      columns: TEST_COLUMNS,
      rows: TEST_ROWS,
      password: 'e2e-share-pass',
    })
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    expect(data.data.token).toBeTruthy()
    createdShareIds.push(data.data.id)
  })

  test('should list shares', async ({ page }) => {
    // Create a share first
    const createRes = await createShare(page, {
      columns: TEST_COLUMNS,
      rows: TEST_ROWS,
      sql_summary: 'e2e-share-list-test',
    })
    expect(createRes.status).toBe(200)
    createdShareIds.push(createRes.data.data.id)

    // List should include the share
    const { status, data } = await listShares(page)
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    const list = data.data as Array<{ id: number }>
    expect(list.length).toBeGreaterThanOrEqual(1)
    expect(list.some((s) => s.id === createRes.data.data.id)).toBeTruthy()
  })

  test('should list empty when no shares exist for fresh context', async ({ page }) => {
    // Delete all e2e shares first
    const { data: listData } = await listShares(page)
    const list = (listData.data as Array<{ id: number }>) ?? []
    for (const s of list) {
      await revokeShare(page, s.id).catch(() => {})
    }

    const { status, data } = await listShares(page)
    expect(status).toBe(200)
    const emptyList = data.data as unknown[]
    expect(emptyList.length).toBe(0)
  })
})

test.describe('Query Share — Public Access (no auth)', () => {
  let shareToken: string
  let shareId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    // Create a share without password
    const { data } = await createShare(page, {
      columns: TEST_COLUMNS,
      rows: TEST_ROWS,
      sql_summary: 'e2e-public-access',
      expires_in_hours: 1,
    })
    shareId = data.data.id
    shareToken = data.data.token
  })

  test.afterEach(async ({ page }) => {
    await revokeShare(page, shareId).catch(() => {})
  })

  test('should access shared result via public token', async () => {
    const { status, data } = await getPublicShare(shareToken)
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    expect(data.data.columns).toEqual(TEST_COLUMNS)
    expect((data.data.rows as unknown[]).length).toBe(2)
  })

  test('should return 404 for non-existent token', async () => {
    const { status, data } = await getPublicShare('nonexistent-token-xyz')
    expect(status).toBe(404)
    expect(data.code).toBe(404)
  })
})

test.describe('Query Share — Password Protection', () => {
  let shareToken: string
  let shareId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    const { data } = await createShare(page, {
      columns: TEST_COLUMNS,
      rows: TEST_ROWS,
      password: 'e2e-protected-pass',
      expires_in_hours: 1,
    })
    shareId = data.data.id
    shareToken = data.data.token
  })

  test.afterEach(async ({ page }) => {
    await revokeShare(page, shareId).catch(() => {})
  })

  test('should reject wrong password with 401', async () => {
    const { status, data } = await verifySharePassword(shareToken, 'wrong-password')
    expect(status).toBe(401)
    expect(data.code).toBe(401)
  })

  test('should accept correct password', async () => {
    const { status, data } = await verifySharePassword(shareToken, 'e2e-protected-pass')
    expect(status).toBe(200)
    expect(data.code).toBe(0)
  })
})

test.describe('Query Share — Revoke', () => {
  let shareToken: string
  let shareId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    const { data } = await createShare(page, {
      columns: TEST_COLUMNS,
      rows: TEST_ROWS,
      sql_summary: 'e2e-revoke-test',
      expires_in_hours: 1,
    })
    shareId = data.data.id
    shareToken = data.data.token
  })

  test('should revoke share and deny public access', async ({ page }) => {
    // Verify it exists first
    const before = await getPublicShare(shareToken)
    expect(before.status).toBe(200)

    // Revoke
    const { status, data } = await revokeShare(page, shareId)
    expect(status).toBe(200)
    expect(data.code).toBe(0)

    // Public access should fail
    const after = await getPublicShare(shareToken)
    expect(after.status).toBe(410)
    expect(after.data.code).toBe(410)
  })

  test('should return error when revoking non-existent share', async ({ page }) => {
    const { status } = await revokeShare(page, 99999)
    expect(status).toBe(400)
  })
})
