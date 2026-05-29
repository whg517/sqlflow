/**
 * query-snapshot-diff.spec.ts — E2E: Query snapshots & diff (SF-QA0029)
 *
 * Tests 5 snapshot APIs against the real backend:
 *   POST   /api/query/snapshots           — create snapshot from history
 *   GET    /api/query/snapshots           — list snapshots
 *   GET    /api/query/snapshots/:id       — get snapshot detail
 *   DELETE /api/query/snapshots/:id       — delete snapshot
 *   POST   /api/query/compare              — compare two snapshots
 *
 * Coverage: create → list → detail → compare → delete
 * Edge cases: same snapshot compare, schema mismatch, empty list, non-existent ID
 */
import { test, expect, BASE_URL, loginViaUI, apiRequest, cleanupDatasources, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

/** Execute SQL via the query API and return the history entry. */
async function executeQuery(
  page: import('@playwright/test').Page,
  sql: string,
  datasourceId: number,
): Promise<{ status: number; data: { code: number; data: { history_id?: number } } }> {
  const { status, body } = await apiRequest(page, 'POST', '/query/execute', {
    datasource_id: datasourceId,
    sql,
    database: 'testdb',
  })
  return { status, data: body as { code: number; data: { history_id?: number } } }
}

/** Create a snapshot from a query history ID. */
async function createSnapshot(
  page: import('@playwright/test').Page,
  queryHistoryId: number,
): Promise<{ status: number; data: { code: number; data: { id: number; sql: string } } }> {
  const { status, body } = await apiRequest(page, 'POST', '/query/snapshots', {
    query_history_id: queryHistoryId,
  })
  return { status, data: body as { code: number; data: { id: number; sql: string } } }
}

/** List snapshots. */
async function listSnapshots(page: import('@playwright/test').Page, page_ = 1, pageSize = 20) {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.get(`${BASE_URL}/api/query/snapshots?page=${page_}&page_size=${pageSize}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return { status: res.status(), data: await res.json() }
}

/** Get snapshot detail. */
async function getSnapshot(page: import('@playwright/test').Page, id: number) {
  const { status, body } = await apiRequest(page, 'GET', `/query/snapshots/${id}`)
  return { status, data: body as { code: number; data: { id: number; sql: string; columns: string[]; rows: unknown[] } } }
}

/** Delete a snapshot. */
async function deleteSnapshot(page: import('@playwright/test').Page, id: number) {
  const { status, body } = await apiRequest(page, 'DELETE', `/query/snapshots/${id}`)
  return { status, data: body as { code: number } }
}

/** Compare two snapshots. */
async function compareSnapshots(
  page: import('@playwright/test').Page,
  leftId: number,
  rightId: number,
): Promise<{ status: number; data: { code: number; message?: string; data?: unknown } }> {
  const { status, body } = await apiRequest(page, 'POST', '/query/compare', {
    left_snapshot_id: leftId,
    right_snapshot_id: rightId,
  })
  return { status, data: body as { code: number; message?: string; data?: unknown } }
}

// --- Tests ---

test.describe('Query Snapshot — Create & List', () => {
  let snapshotIds: number[] = []
  let datasourceId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    const ds = await getFirstDatasourceId(page)
    datasourceId = ds.id
  })

  test.afterEach(async ({ page }) => {
    for (const id of snapshotIds) {
      await deleteSnapshot(page, id).catch(() => {})
    }
    snapshotIds = []
  })

  test('should create snapshot from query history', async ({ page }) => {
    // Execute a query first to create history
    const sql = 'SELECT id, username, email FROM sys_user LIMIT 5'
    const execResult = await executeQuery(page, sql, datasourceId)
    expect(execResult.status).toBe(200)
    expect(execResult.data.code).toBe(0)
    expect(execResult.data.data.history_id).toBeTruthy()

    // Create snapshot from that history
    const snap = await createSnapshot(page, execResult.data.data.history_id!)
    expect(snap.status).toBe(200)
    expect(snap.data.code).toBe(0)
    expect(snap.data.data.id).toBeTruthy()
    snapshotIds.push(snap.data.data.id)
  })

  test('should list snapshots', async ({ page }) => {
    // Create two snapshots
    const sql1 = 'SELECT 1 as e2e_snap_a'
    const sql2 = 'SELECT 1 as e2e_snap_b'
    const exec1 = await executeQuery(page, sql1, datasourceId)
    const exec2 = await executeQuery(page, sql2, datasourceId)
    const snap1 = await createSnapshot(page, exec1.data.data.history_id!)
    const snap2 = await createSnapshot(page, exec2.data.data.history_id!)
    snapshotIds.push(snap1.data.data.id, snap2.data.data.id)

    // List should contain both
    const { status, data } = await listSnapshots(page)
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    const list = data.data as Array<{ id: number }>
    expect(list.length).toBeGreaterThanOrEqual(2)
    expect(list.some((s) => s.id === snap1.data.data.id)).toBeTruthy()
    expect(list.some((s) => s.id === snap2.data.data.id)).toBeTruthy()
  })

  test('should return empty list when no snapshots exist', async ({ page }) => {
    // Clean up existing snapshots first
    const { data: listData } = await listSnapshots(page, 1, 100)
    const list = (listData.data as Array<{ id: number }>) ?? []
    for (const s of list) {
      await deleteSnapshot(page, s.id).catch(() => {})
    }

    const { status, data } = await listSnapshots(page)
    expect(status).toBe(200)
    const emptyList = (data.data as Array<unknown>) ?? []
    expect(emptyList.length).toBe(0)
  })
})

test.describe('Query Snapshot — Detail', () => {
  let snapshotId: number
  let datasourceId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    const ds = await getFirstDatasourceId(page)
    datasourceId = ds.id

    // Execute + create snapshot
    const exec = await executeQuery(page, 'SELECT id, username FROM sys_user LIMIT 3', datasourceId)
    const snap = await createSnapshot(page, exec.data.data.history_id!)
    snapshotId = snap.data.data.id
  })

  test.afterEach(async ({ page }) => {
    await deleteSnapshot(page, snapshotId).catch(() => {})
  })

  test('should get snapshot detail', async ({ page }) => {
    const { status, data } = await getSnapshot(page, snapshotId)
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    expect(data.data.id).toBe(snapshotId)
    expect(data.data.columns).toBeTruthy()
    expect(Array.isArray(data.data.rows)).toBeTruthy()
  })

  test('should return 404 for non-existent snapshot', async ({ page }) => {
    const { status } = await getSnapshot(page, 99999)
    expect(status).toBe(404)
  })
})

test.describe('Query Snapshot — Delete', () => {
  let snapshotId: number
  let datasourceId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    const ds = await getFirstDatasourceId(page)
    datasourceId = ds.id

    const exec = await executeQuery(page, 'SELECT 1 as e2e_delete_snap', datasourceId)
    const snap = await createSnapshot(page, exec.data.data.history_id!)
    snapshotId = snap.data.data.id
  })

  test('should delete snapshot', async ({ page }) => {
    // Verify exists
    const before = await getSnapshot(page, snapshotId)
    expect(before.status).toBe(200)

    // Delete
    const { status, data } = await deleteSnapshot(page, snapshotId)
    expect(status).toBe(200)
    expect(data.code).toBe(0)

    // Should be gone
    const after = await getSnapshot(page, snapshotId)
    expect(after.status).toBe(404)
  })

  test('should return 404 when deleting non-existent snapshot', async ({ page }) => {
    const { status } = await deleteSnapshot(page, 99999)
    expect(status).toBe(404)
  })
})

test.describe('Query Snapshot — Compare', () => {
  let snapAId: number
  let snapBId: number
  let datasourceId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    const ds = await getFirstDatasourceId(page)
    datasourceId = ds.id

    // Create two snapshots with different schemas
    const execA = await executeQuery(page, 'SELECT id, username FROM sys_user LIMIT 3', datasourceId)
    const snapA = await createSnapshot(page, execA.data.data.history_id!)

    const execB = await executeQuery(page, 'SELECT id, email, phone FROM sys_user LIMIT 3', datasourceId)
    const snapB = await createSnapshot(page, execB.data.data.history_id!)

    snapAId = snapA.data.data.id
    snapBId = snapB.data.data.id
  })

  test.afterEach(async ({ page }) => {
    await deleteSnapshot(page, snapAId).catch(() => {})
    await deleteSnapshot(page, snapBId).catch(() => {})
  })

  test('should compare two snapshots', async ({ page }) => {
    const { status, data } = await compareSnapshots(page, snapAId, snapBId)
    // Schema mismatch is expected (different columns)
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    expect(data.data).toBeTruthy()
  })

  test('should return error when comparing same snapshot', async ({ page }) => {
    const { status, data } = await compareSnapshots(page, snapAId, snapAId)
    expect(status).toBe(400)
    expect(data.code).toBe(400)
  })

  test('should return 404 when comparing non-existent snapshot', async ({ page }) => {
    const { status } = await compareSnapshots(page, snapAId, 99999)
    expect(status).toBe(404)
  })
})

test.describe('Query Snapshot — Create Errors', () => {
  let datasourceId: number

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
    const ds = await getFirstDatasourceId(page)
    datasourceId = ds.id
  })

  test('should return error when query_history_id is 0', async ({ page }) => {
    const { status, data } = await createSnapshot(page, 0)
    expect(status).toBe(400)
  })

  test('should return 404 when query history does not exist', async ({ page }) => {
    const { status } = await createSnapshot(page, 99999)
    expect(status).toBe(404)
  })
})
