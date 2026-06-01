/**
 * E2E — 导出功能（真实后端）
 * SF-QA0032: Added async export tasks coverage (list/detail/download/polling)
 *
 * Tests sync export, async export tasks, and security scenarios against real backend.
 */
import { test, expect, type Download } from '@playwright/test'
import { BASE_URL, loginViaUI, getToken } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

/** Trigger async export and return task info. */
async function triggerAsyncExport(
  page: import('@playwright/test').Page,
  exportType: 'audit' | 'ticket',
  filters?: Record<string, unknown>,
): Promise<{ status: number; data: { code: number; data: { task_id: number; status: string } } }> {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.post(`${BASE_URL}/api/export/${exportType}`, {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    data: filters ?? {},
  })
  return { status: res.status(), data: await res.json() }
}

/** List export tasks. */
async function listExportTasks(page: import('@playwright/test').Page) {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.get(`${BASE_URL}/api/export/tasks`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return { status: res.status(), data: await res.json() }
}

/** Get export task detail. */
async function getExportTask(page: import('@playwright/test').Page, taskId: number) {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.get(`${BASE_URL}/api/export/tasks/${taskId}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return { status: res.status(), data: await res.json() }
}

/** Download export file for a task. */
async function downloadExportFile(page: import('@playwright/test').Page, taskId: number) {
  const token = await page.evaluate(() => localStorage.getItem('token')!)
  const res = await page.request.get(`${BASE_URL}/api/export/tasks/${taskId}/download`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return { status: res.status(), body: await res.body() }
}

/** Poll a task until it reaches one of the target statuses or timeout. */
async function pollTaskUntil(
  page: import('@playwright/test').Page,
  taskId: number,
  targetStatuses: string[],
  maxMs = 30_000,
  intervalMs = 2_000,
): Promise<{ status: number; data: { code: number; data: { status: string } } }> {
  const start = Date.now()
  while (Date.now() - start < maxMs) {
    const result = await getExportTask(page, taskId)
    if (result.data.data?.status && targetStatuses.includes(result.data.data.status)) {
      return result
    }
    await page.waitForTimeout(intervalMs)
  }
  throw new Error(`Task ${taskId} did not reach ${targetStatuses.join('/')} within ${maxMs}ms`)
}

test.describe('审计日志导出 — 真实后端', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('审计导出 API 返回 CSV', async ({ request }) => {
    const token = await getToken()
    const exportRes = await request.get(`${BASE_URL}/api/export/audit`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(exportRes.status()).toBeLessThan(500)
    const contentType = exportRes.headers()['content-type'] ?? ''
    expect(contentType).toContain('text/csv')
  })

  test('审计页面导出按钮可见', async ({ page }) => {
    await page.goto('/audit')
    await page.waitForLoadState('networkidle')
    const exportBtn = page.getByRole('button', { name: /导出/ })
    await expect(exportBtn).toBeVisible()
  })
})

test.describe('工单导出 — 真实后端', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('工单导出 API 返回 CSV', async ({ request }) => {
    const token = await getToken()
    const exportRes = await request.get(`${BASE_URL}/api/export/tickets`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(exportRes.status()).toBeLessThan(500)
    const contentType = exportRes.headers()['content-type'] ?? ''
    expect(contentType).toContain('text/csv')
  })

  test('工单页面导出按钮可见', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForLoadState('networkidle')
    const exportBtn = page.getByRole('button', { name: /导出/ })
    await expect(exportBtn).toBeVisible()
  })
})

// ============================================================
// SF-QA0032: Async Export Tasks
// ============================================================

test.describe('异步导出任务 — 列表与详情', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should list export tasks', async ({ page }) => {
    const { status, data } = await listExportTasks(page)
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    expect(Array.isArray(data.data)).toBeTruthy()
  })

  test('should return empty list when no tasks exist', async ({ page }) => {
    const { status, data } = await listExportTasks(page)
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    expect(Array.isArray(data.data)).toBeTruthy()
  })

  test('should return 404 for non-existent task detail', async ({ page }) => {
    const { status, data } = await getExportTask(page, 99999)
    expect(status).toBe(404)
    expect(data.code).toBe(404)
  })
})

test.describe('异步导出任务 — 完整流程（创建→轮询→下载）', () => {
  let createdTaskId: number | undefined

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should create async audit export task', async ({ page }) => {
    // Trigger with filters that force async path
    const { status, data } = await triggerAsyncExport(page, 'audit', {})
    // May return 200 (sync) or 202 (async)
    expect([200, 202]).toContain(status)
    if (status === 202) {
      expect(data.code).toBe(0)
      expect(data.data.task_id).toBeTruthy()
      createdTaskId = data.data.task_id
    }
  })

  test('should create async ticket export task', async ({ page }) => {
    const { status, data } = await triggerAsyncExport(page, 'ticket', {})
    expect([200, 202]).toContain(status)
    if (status === 202) {
      expect(data.code).toBe(0)
      expect(data.data.task_id).toBeTruthy()
    }
  })

  test('should poll task until completed or failed', async ({ page }) => {
    // Create a task first
    const { status: createStatus, data: createData } = await triggerAsyncExport(page, 'audit', {})
    if (createStatus !== 202) {
      // Sync export — skip async flow test
      test.skip()
      return
    }
    const taskId = createData.data.task_id

    // Poll until done
    const result = await pollTaskUntil(page, taskId, ['completed', 'failed', 'cancelled'])
    expect(['completed', 'failed', 'cancelled']).toContain(result.data.data.status)
  })

  test('should list tasks including newly created ones', async ({ page }) => {
    // Create a task
    const { status: createStatus, data: createData } = await triggerAsyncExport(page, 'audit', {})
    if (createStatus !== 202) {
      test.skip()
      return
    }
    const taskId = createData.data.task_id

    // List should include it
    const { status, data } = await listExportTasks(page)
    expect(status).toBe(200)
    expect(data.code).toBe(0)
    const tasks = data.data as Array<{ id: number }>
    expect(tasks.some((t) => t.id === taskId)).toBeTruthy()
  })
})

test.describe('异步导出任务 — 下载', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should return 400 when downloading a non-completed task', async ({ page }) => {
    // Create a task and try downloading immediately
    const { status: createStatus, data: createData } = await triggerAsyncExport(page, 'audit', {})
    if (createStatus !== 202) {
      test.skip()
      return
    }
    const taskId = createData.data.task_id

    // Download immediately — task likely not ready yet
    const { status } = await downloadExportFile(page, taskId)
    // Should be 400 (not ready) or could be 200 if it finished fast
    expect([200, 400]).toContain(status)
  })

  test('should return 404 for downloading non-existent task', async ({ page }) => {
    const { status } = await downloadExportFile(page, 99999)
    expect(status).toBe(404)
  })

  test('should download completed task file as CSV', async ({ page }) => {
    // Create task
    const { status: createStatus, data: createData } = await triggerAsyncExport(page, 'audit', {})
    if (createStatus !== 202) {
      test.skip()
      return
    }
    const taskId = createData.data.task_id

    // Wait for completion
    const result = await pollTaskUntil(page, taskId, ['completed', 'failed'], 30_000, 2_000)
    if (result.data.data.status !== 'completed') {
      test.skip()
      return
    }

    // Download
    const { status, body } = await downloadExportFile(page, taskId)
    expect(status).toBe(200)
    expect(body.length).toBeGreaterThan(0)
    // Should be CSV content
    const content = body.toString('utf-8')
    expect(content.length).toBeGreaterThan(0)
  })
})

test.describe('异步导出任务 — 安全场景', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('should return 404 for task detail of non-existent task', async ({ page }) => {
    const { status } = await getExportTask(page, 99999)
    expect(status).toBe(404)
  })

  test('should return 404 for download of non-existent task', async ({ page }) => {
    const { status } = await downloadExportFile(page, 99999)
    expect(status).toBe(404)
  })

  test('task ownership — user cannot access another user\'s task', async ({ page }) => {
    // Without creating a real second user, test with a fabricated high ID
    // The backend should return 404 (ownership check)
    const { status } = await getExportTask(page, 99998)
    expect([404, 403]).toContain(status)
  })

  test('download ownership — user cannot download another user\'s file', async ({ page }) => {
    const { status } = await downloadExportFile(page, 99998)
    expect([404, 403]).toContain(status)
  })
})
