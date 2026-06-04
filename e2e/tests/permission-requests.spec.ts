/**
 * permission-requests.spec.ts
 *
 * E2E: 权限申请全流程 — 9 API
 *   POST   /api/permission-requests              — 创建申请
 *   GET    /api/permission-requests/mine          — 我的申请
 *   GET    /api/permission-requests/active        — 我的活跃权限
 *   GET    /api/permission-requests/:id           — 申请详情
 *   GET    /api/permission-requests               — 管理员列表
 *   POST   /api/permission-requests/:id/approve   — 审批通过
 *   POST   /api/permission-requests/:id/reject    — 审批拒绝
 *   POST   /api/permission-requests/:id/revoke    — 撤销权限
 *   POST   /api/permission-requests/expire        — 过期清理
 *
 * 边界：无权限用户看不到管理操作、重复审批
 */
import {
  test,
  expect,
  BASE_URL,
  loginViaUI,
  apiHelper,
  getToken,
  getFirstDatasourceId,
} from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

const E2E_PREFIX = `e2e_perm`

// Track created request IDs for cleanup
const createdIds: number[] = []

// --- Cleanup ---
test.afterAll(async () => {
  if (createdIds.length === 0) return
  try {
    const token = await getToken()
    for (const id of createdIds) {
      // Best-effort revoke/delete
      await fetch(`${BASE_URL}/api/permission-requests/${id}/revoke`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason: 'e2e cleanup' }),
      }).catch(() => {})
    }
  } catch {
    // best-effort
  }
})

test.describe('Permission Requests — Create & List', () => {
  test('should create a permission request', async ({ page }) => {
    await loginViaUI(page)
    const { id: dsId } = await getFirstDatasourceId(page)

    const { status, data } = await apiHelper(page, 'POST', '/permission-requests', {
      datasource_id: dsId,
      database: 'testdb',
      table_name: `${E2E_PREFIX}_table_${Date.now()}`,
      actions: 'SELECT',
      reason: 'E2E test permission request',
      duration_hours: 24,
    })
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: { id: number; status: string } }
    expect(body.code).toBe(0)
    expect(body.data.id).toBeGreaterThan(0)
    expect(body.data.status).toBe('pending')

    createdIds.push(body.data.id)
  })

  test('should list my requests', async ({ page }) => {
    await loginViaUI(page)
    const { id: dsId } = await getFirstDatasourceId(page)

    // Create a request first
    const { data: createData } = await apiHelper(page, 'POST', '/permission-requests', {
      datasource_id: dsId,
      database: 'testdb',
      table_name: `${E2E_PREFIX}_mine_${Date.now()}`,
      actions: 'SELECT,INSERT',
      reason: 'E2E mine list test',
      duration_hours: 1,
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    // List my requests
    const { status, data } = await apiHelper(page, 'GET', '/permission-requests/mine')
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: { items: Array<{ id: number }>; total: number } }
    expect(body.code).toBe(0)
    expect(body.data.items.length).toBeGreaterThanOrEqual(1)
  })

  test('should list active requests', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'GET', '/permission-requests/active')
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: unknown[] }
    expect(body.code).toBe(0)
    expect(Array.isArray(body.data)).toBeTruthy()
  })

  test('should get request detail by ID', async ({ page }) => {
    await loginViaUI(page)
    const { id: dsId } = await getFirstDatasourceId(page)

    // Create
    const { data: createData } = await apiHelper(page, 'POST', '/permission-requests', {
      datasource_id: dsId,
      database: 'testdb',
      table_name: `${E2E_PREFIX}_detail_${Date.now()}`,
      actions: 'SELECT',
      reason: 'E2E detail test',
      duration_hours: 12,
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    // Get detail
    const { status, data } = await apiHelper(page, 'GET', `/permission-requests/${created.data.id}`)
    expect(status).toBeLessThan(300)
    const body = data as {
      code: number
      data: { id: number; status: string; database: string; actions: string; reason: string }
    }
    expect(body.code).toBe(0)
    expect(body.data.id).toBe(created.data.id)
    expect(body.data.status).toBe('pending')
    expect(body.data.database).toBe('testdb')
    expect(body.data.actions).toContain('SELECT')
    expect(body.data.reason).toContain('E2E')
  })

  test('should return 404 for non-existent request', async ({ page }) => {
    await loginViaUI(page)

    const { status } = await apiHelper(page, 'GET', '/permission-requests/999999')
    expect(status).toBe(404)
  })
})

test.describe('Permission Requests — Admin Operations', () => {
  test('admin should list all requests', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'GET', '/permission-requests?page=1&page_size=10')
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: unknown[]; total: number }
    expect(body.code).toBe(0)
  })

  test('should approve a request', async ({ page }) => {
    await loginViaUI(page)
    const { id: dsId } = await getFirstDatasourceId(page)

    // Create
    const { data: createData } = await apiHelper(page, 'POST', '/permission-requests', {
      datasource_id: dsId,
      database: 'testdb',
      table_name: `${E2E_PREFIX}_approve_${Date.now()}`,
      actions: 'SELECT',
      reason: 'E2E approve test',
      duration_hours: 1,
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    // Approve
    const { status, data } = await apiHelper(
      page,
      'POST',
      `/permission-requests/${created.data.id}/approve`,
      { comment: 'E2E auto approve' },
    )
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: { status: string } }
    expect(body.code).toBe(0)
    expect(body.data.status).toBe('approved')

    // Verify via GET
    const { data: getData } = await apiHelper(page, 'GET', `/permission-requests/${created.data.id}`)
    const getBody = getData as { code: number; data: { status: string } }
    expect(getBody.data.status).toBe('approved')
  })

  test('should reject a request', async ({ page }) => {
    await loginViaUI(page)
    const { id: dsId } = await getFirstDatasourceId(page)

    // Create
    const { data: createData } = await apiHelper(page, 'POST', '/permission-requests', {
      datasource_id: dsId,
      database: 'testdb',
      table_name: `${E2E_PREFIX}_reject_${Date.now()}`,
      actions: 'SELECT',
      reason: 'E2E reject test',
      duration_hours: 1,
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    // Reject
    const { status, data } = await apiHelper(
      page,
      'POST',
      `/permission-requests/${created.data.id}/reject`,
      { comment: 'E2E test rejection' },
    )
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: { status: string } }
    expect(body.code).toBe(0)
    expect(body.data.status).toBe('rejected')
  })

  test('should reject duplicate approval', async ({ page }) => {
    await loginViaUI(page)
    const { id: dsId } = await getFirstDatasourceId(page)

    // Create and approve
    const { data: createData } = await apiHelper(page, 'POST', '/permission-requests', {
      datasource_id: dsId,
      database: 'testdb',
      table_name: `${E2E_PREFIX}_dupapprove_${Date.now()}`,
      actions: 'SELECT',
      reason: 'E2E dup approve test',
      duration_hours: 1,
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    // First approve
    await apiHelper(page, 'POST', `/permission-requests/${created.data.id}/approve`, {
      comment: 'First approve',
    })

    // Second approve — should fail with 409
    const { status, data } = await apiHelper(
      page,
      'POST',
      `/permission-requests/${created.data.id}/approve`,
      { comment: 'Duplicate approve' },
    )
    expect(status).toBe(409)
    const body = data as { code: number; message: string }
    expect(body.code).toBe(409)
  })
})

test.describe('Permission Requests — Revoke', () => {
  test('should revoke an approved request', async ({ page }) => {
    await loginViaUI(page)
    const { id: dsId } = await getFirstDatasourceId(page)

    // Create + approve
    const { data: createData } = await apiHelper(page, 'POST', '/permission-requests', {
      datasource_id: dsId,
      database: 'testdb',
      table_name: `${E2E_PREFIX}_revoke_${Date.now()}`,
      actions: 'SELECT',
      reason: 'E2E revoke test',
      duration_hours: 1,
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    await apiHelper(page, 'POST', `/permission-requests/${created.data.id}/approve`, {
      comment: 'Approve for revoke test',
    })

    // Revoke
    const { status, data } = await apiHelper(
      page,
      'POST',
      `/permission-requests/${created.data.id}/revoke`,
      { reason: 'E2E revoke — no longer needed' },
    )
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: { status: string } }
    expect(body.code).toBe(0)
    expect(body.data.status).toBe('revoked')
  })
})

test.describe('Permission Requests — Expire', () => {
  test('should trigger expire cleanup', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'POST', '/permission-requests/expire')
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: { expired_count: number } }
    expect(body.code).toBe(0)
    // expired_count should be a non-negative number
    expect(body.data.expired_count).toBeGreaterThanOrEqual(0)
  })
})

test.describe('Permission Requests — Boundary', () => {
  test('should reject missing datasource_id', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'POST', '/permission-requests', {
      database: 'testdb',
      actions: 'SELECT',
    })
    expect(status).toBe(400)
    const body = data as { code: number; message: string }
    expect(body.message).toContain('数据源')
  })

  test('should reject missing database', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'POST', '/permission-requests', {
      datasource_id: 1,
      actions: 'SELECT',
    })
    expect(status).toBe(400)
    const body = data as { code: number; message: string }
    expect(body.message).toContain('数据库')
  })

  test('should reject missing actions', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'POST', '/permission-requests', {
      datasource_id: 1,
      database: 'testdb',
    })
    expect(status).toBe(400)
    const body = data as { code: number; message: string }
    expect(body.message).toContain('操作')
  })
})
