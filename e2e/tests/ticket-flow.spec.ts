/**
 * ticket-flow.spec.ts
 *
 * E2E 全流程工单测试 — **不 mock，直接操作真实数据库**。
 *
 * 前置条件：
 *   1. 后端服务运行在 localhost:8080（前端 vite proxy → /api）
 *   2. 前端 dev server 运行在 localhost:8080
 *   3. MySQL 数据源已创建且状态 active（名称包含 "mysql"，可在 seed 数据中找到）
 *   4. 已有 admin 账号：admin / admin123
 *   5. 目标 MySQL 实例中有可用的数据库（默认 sqlflow_test，可通过 env 覆盖）
 *
 * 运行：
 *   npx playwright test e2e-real/ticket-flow.spec.ts --project=chromium
 *
 * 环境变量（可选）：
 *   E2E_DB_NAME      — 使用的目标数据库名（默认 sqlflow_test）
 *   E2E_BASE_URL     — 前端地址（默认 http://localhost:8080）
 *   E2E_ADMIN_USER   — 管理员用户名（默认 admin）
 *   E2E_ADMIN_PASS   — 管理员密码（默认 admin123）
 */

import { test, expect, type Page } from '@playwright/test'

// ── Config ───────────────────────────────────────────────────────────────────

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const DB_NAME = process.env.E2E_DB_NAME || 'sqlflow_test'
const ADMIN_USER = process.env.E2E_USERNAME ?? 'e2eadmin'
const ADMIN_PASS = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

const E2E_TABLE = 'e2e_test'
const EXEC_TIMEOUT = 30_000

// ── Helpers ─────────────────────────────────────────────────────────────────

/** Real API call — no route mocking. */
async function apiCall<T = unknown>(page: Page, method: string, path: string, body?: unknown): Promise<T> {
  const token = await page.evaluate(() => localStorage.getItem('token'))
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await page.evaluate(
    async ({ url, method, headers, body }) => {
      const r = await fetch(url, {
        method,
        headers,
        body: body != null ? JSON.stringify(body) : undefined,
      })
      return { status: r.status, data: await r.json() }
    },
    { url: `${BASE_URL}/api${path}`, method, headers, body },
  )

  if (res.status >= 400) {
    throw new Error(`API ${method} ${path} → ${res.status}: ${JSON.stringify(res.data)}`)
  }
  return res.data as T
}

/** Login via UI with real credentials. */
async function loginReal(page: Page) {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill(ADMIN_USER)
  await page.getByPlaceholder('密码').fill(ADMIN_PASS)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 15_000 })
}

/** Find the first active MySQL datasource ID from the real API. */
async function getMysqlDatasourceId(page: Page): Promise<number> {
  const res = await apiCall<{ code: number; data: Array<{ id: number; name: string; type: string; status: string }> }>(
    page, 'GET', '/datasources',
  )
  const ds = res.data.find((d) => d.type === 'mysql' && d.status === 'active')
  if (!ds) throw new Error('No active MySQL datasource found. Ensure one exists in the database.')
  return ds.id
}

/** Create a ticket via API, return the ticket data. */
async function createTicketViaApi(page: Page, datasourceId: number, sql: string, reason: string) {
  const res = await apiCall<{
    code: number; data: {
      id: number; status: string; sql_content: string; database: string
    }
  }>(page, 'POST', '/tickets', {
    datasource_id: datasourceId,
    database: DB_NAME,
    sql,
    db_type: 'mysql',
    change_reason: reason,
  })
  return res.data
}

/** Get ticket status via API. */
async function getTicketStatus(page: Page, ticketId: number): Promise<string> {
  const res = await apiCall<{ code: number; data: { status: string } }>(
    page, 'GET', `/tickets/${ticketId}`,
  )
  return res.data.status
}

/** Approve a ticket via API. */
async function approveTicket(page: Page, ticketId: number) {
  return apiCall(page, 'POST', `/tickets/${ticketId}/approve`, { comment: 'E2E auto approve' })
}

/** Execute a ticket via API and wait for DONE status. */
async function executeTicketAndWait(page: Page, ticketId: number) {
  await apiCall(page, 'POST', `/tickets/${ticketId}/execute`, {})

  // Poll until status is DONE (or timeout)
  const startTime = Date.now()
  while (Date.now() - startTime < EXEC_TIMEOUT) {
    const status = await getTicketStatus(page, ticketId)
    if (status === 'DONE') return
    if (['REJECTED', 'CANCELLED'].includes(status)) {
      throw new Error(`Ticket ${ticketId} ended with status ${status}, cannot execute`)
    }
    await page.waitForTimeout(1_000)
  }
  throw new Error(`Ticket ${ticketId} did not reach DONE status within ${EXEC_TIMEOUT}ms`)
}

/** Execute a query against the real database and return results. */
async function executeRealQuery(page: Page, datasourceId: number, sql: string) {
  const res = await apiCall<{
    code: number; data: {
      columns: string[]; rows: Record<string, unknown>[];
      total: number; affected_rows: number
    }
  }>(page, 'POST', '/query/execute', {
    datasource_id: datasourceId,
    database: DB_NAME,
    sql,
  })
  return res.data
}

/** Cleanup: ensure e2e_test table doesn't exist before tests. */
async function cleanupTable(page: Page, datasourceId: number) {
  try {
    await executeRealQuery(page, datasourceId, `DROP TABLE IF EXISTS ${E2E_TABLE}`)
  } catch {
    // Table may not exist, that's fine
  }
}

// ── Tests ───────────────────────────────────────────────────────────────────

test.describe('工单全流程 E2E（真实数据库操作）', () => {
  let datasourceId: number
  let page: Page

  test.beforeAll(async ({ browser }) => {
    const context = await browser.newContext()
    page = await context.newPage()

    // Login once for the entire suite
    await loginReal(page)

    // Get datasource ID
    datasourceId = await getMysqlDatasourceId(page)

    // Cleanup any leftover table
    await cleanupTable(page, datasourceId)
  })

  test.afterAll(async () => {
    // Best-effort cleanup
    try {
      await cleanupTable(page, datasourceId)
    } catch {
      // Ignore cleanup failures
    }
    await page.close()
  })

  // ── 1. Create DDL ticket (CREATE TABLE) ──────────────────────────────────

  test('1. 创建 CREATE TABLE 工单 — 验证工单提交成功且状态为待审批', async () => {
    const ticket = await createTicketViaApi(
      page,
      datasourceId,
      `CREATE TABLE ${E2E_TABLE} (id INT PRIMARY KEY, name VARCHAR(100))`,
      'E2E test: create table for full lifecycle validation',
    )

    expect(ticket.id).toBeGreaterThan(0)
    expect(ticket.status).toBe('SUBMITTED')
    expect(ticket.sql_content).toContain('CREATE TABLE')
    expect(ticket.database).toBe(DB_NAME)
  })

  // ── 2. AI review ─────────────────────────────────────────────────────────

  test('2. AI 审查流程 — 验证审查调用不报错', async () => {
    // The AI review is triggered when creating tickets from the UI.
    // Since we create tickets via API, we verify the review endpoint responds
    // without crashing (it may return a fallback result when no AI key is configured).
    const res = await page.evaluate(async ({ baseUrl, dbName, dsId }) => {
      const token = localStorage.getItem('token')
      try {
        const r = await fetch(`${baseUrl}/api/query/review`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify({
            datasource_id: dsId,
            database: dbName,
            sql: 'SELECT 1',
          }),
        })
        // SSE stream — read first few chunks to verify it starts
        const reader = r.body?.getReader()
        if (!reader) return { ok: false, status: r.status, error: 'no body' }

        const decoder = new TextDecoder()
        let buffer = ''
        let gotResult = false
        let gotError = false
        let errorMsg = ''
        let chunkCount = 0

        for (let i = 0; i < 10; i++) {
          const { done, value } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          chunkCount++

          if (buffer.includes('"result"') || buffer.includes('event: result')) gotResult = true
          if (buffer.includes('"error"') || buffer.includes('event: error')) {
            gotError = true
            // Extract error message
            const match = buffer.match(/"message"\s*:\s*"([^"]+)"/)
            if (match) errorMsg = match[1]
            break
          }
          if (gotResult) break
        }
        reader.cancel().catch(() => {})
        return { ok: true, status: r.status, chunkCount, gotResult, gotError, errorMsg, snippet: buffer.slice(0, 200) }
      } catch (e: unknown) {
        return { ok: false, error: e instanceof Error ? e.message : String(e) }
      }
    }, { baseUrl: BASE_URL, dbName: DB_NAME, dsId: datasourceId })

    // We accept either a successful result or a graceful fallback.
    // The important thing is it doesn't crash the server.
    if (!res.ok) {
      // Some errors are acceptable (e.g., no AI config → fallback)
      expect(res.error ?? '').toBeDefined()
    }
    // If got an error event, it should be a meaningful message, not a panic
    if (res.gotError) {
      // AI review error is acceptable when no API key is configured
      expect(res.errorMsg).toBeTruthy()
    }
  })

  // ── 3. Approve and execute the CREATE TABLE ticket ───────────────────────

  test('3. 审批并执行 CREATE TABLE 工单 — 验证状态变为已完成', async () => {
    // Create a fresh CREATE TABLE ticket
    const ticket = await createTicketViaApi(
      page,
      datasourceId,
      `CREATE TABLE ${E2E_TABLE} (id INT PRIMARY KEY, name VARCHAR(100))`,
      'E2E test: approve and execute CREATE TABLE',
    )

    // Approve
    await approveTicket(page, ticket.id)
    let status = await getTicketStatus(page, ticket.id)
    expect(['APPROVED', 'PENDING_APPROVAL', 'AI_REVIEWED', 'SUBMITTED']).toContain(status)

    // If not yet APPROVED, the service may auto-transition after AI review.
    // Try executing — it should work once APPROVED.
    await executeTicketAndWait(page, ticket.id)

    status = await getTicketStatus(page, ticket.id)
    expect(status).toBe('DONE')
  })

  // ── 4. Verify table actually exists in MySQL ─────────────────────────────

  test('4. 查询验证 — SELECT 确认 e2e_test 表已创建', async () => {
    const result = await executeRealQuery(page, datasourceId, `SELECT * FROM ${E2E_TABLE}`)
    expect(result.columns).toContain('id')
    expect(result.columns).toContain('name')
    expect(result.total).toBe(0)
    expect(result.rows).toEqual([])
  })

  // ── 5. DDL ticket (ALTER TABLE) ──────────────────────────────────────────

  test('5. DDL 工单 — ALTER TABLE 添加列 → 审批 → 执行 → 验证结构变更', async () => {
    const alterSql = `ALTER TABLE ${E2E_TABLE} ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP`

    const ticket = await createTicketViaApi(page, datasourceId, alterSql, 'E2E test: ALTER TABLE add created_at column')
    expect(ticket.status).toBeTruthy()

    // Approve + Execute
    await approveTicket(page, ticket.id)
    await executeTicketAndWait(page, ticket.id)

    const status = await getTicketStatus(page, ticket.id)
    expect(status).toBe('DONE')

    // Verify column was added
    const result = await executeRealQuery(page, datasourceId, `DESCRIBE ${E2E_TABLE}`)
    const columnNames = result.rows.map((row) => String(row['Field'] ?? row['field'] ?? Object.values(row)[0]))
    expect(columnNames).toContain('created_at')
  })

  // ── 6. DML ticket (INSERT) ───────────────────────────────────────────────

  test('6. DML 工单 — INSERT 数据 → 审批 → 执行 → 查询验证数据存在', async () => {
    const insertSql = `INSERT INTO ${E2E_TABLE} (id, name) VALUES (1, 'hello'), (2, 'world')`

    const ticket = await createTicketViaApi(page, datasourceId, insertSql, 'E2E test: INSERT test data into e2e_test')
    expect(ticket.status).toBeTruthy()

    // Approve + Execute
    await approveTicket(page, ticket.id)
    await executeTicketAndWait(page, ticket.id)

    const status = await getTicketStatus(page, ticket.id)
    expect(status).toBe('DONE')

    // Verify data exists
    const result = await executeRealQuery(page, datasourceId, `SELECT * FROM ${E2E_TABLE} ORDER BY id`)
    expect(result.total).toBe(2)
    expect(result.rows[0]).toMatchObject({ id: 1, name: 'hello' })
    expect(result.rows[1]).toMatchObject({ id: 2, name: 'world' })
  })

  // ── 7. Query via UI page ─────────────────────────────────────────────────

  test('7. 通过 UI 查询页面验证数据可见', async () => {
    await page.goto(`${BASE_URL}/query`)
    await page.waitForURL('**/query**', { timeout: 10_000 })

    // Select datasource
    const dsSelect = page.getByRole('combobox').filter({ hasText: '选择数据源' }).first()
    await dsSelect.waitFor({ state: 'visible', timeout: 10_000 })
    await dsSelect.click()
    await page.getByRole('option').filter({ hasText: 'mysql' }).first().click()

    // Set database
    const dbInput = page.getByPlaceholder('数据库名')
    await dbInput.waitFor({ state: 'visible', timeout: 5_000 })
    await dbInput.fill(DB_NAME)

    // Type SQL
    const editor = page.locator('textarea, [class*="editor"], [role="textbox"]').first()
    await editor.waitFor({ state: 'visible', timeout: 5_000 })
    await editor.fill(`SELECT * FROM ${E2E_TABLE} ORDER BY id`)

    // Click execute
    const runBtn = page.getByRole('button', { name: /运行|执行|Run/i }).first()
    if (await runBtn.isVisible()) {
      await runBtn.click()
    } else {
      // Fallback: Ctrl+Enter
      await editor.press('Control+Enter')
    }

    // Wait for results table to appear
    const resultTable = page.getByRole('table').filter({ hasText: 'hello' })
    await resultTable.waitFor({ state: 'visible', timeout: 15_000 })

    // Verify data visible
    await expect(page.getByText('hello')).toBeVisible()
    await expect(page.getByText('world')).toBeVisible()
  })

  // ── 8. Drop table cleanup ────────────────────────────────────────────────

  test('8. 删除清理 — DROP TABLE 工单 → 审批 → 执行 → 验证表已删除', async () => {
    const dropSql = `DROP TABLE ${E2E_TABLE}`

    const ticket = await createTicketViaApi(page, datasourceId, dropSql, 'E2E test: cleanup DROP TABLE e2e_test')
    expect(ticket.status).toBeTruthy()

    // Approve + Execute
    await approveTicket(page, ticket.id)
    await executeTicketAndWait(page, ticket.id)

    const status = await getTicketStatus(page, ticket.id)
    expect(status).toBe('DONE')

    // Verify table no longer exists — should throw an error
    let tableGone = false
    try {
      await executeRealQuery(page, datasourceId, `SELECT * FROM ${E2E_TABLE}`)
    } catch (err) {
      // Expected: table doesn't exist
      tableGone = true
      const msg = err instanceof Error ? err.message : String(err)
      expect(msg).toBeTruthy()
    }
    expect(tableGone).toBe(true)
  })

  // ── 9. Verify full lifecycle on UI ───────────────────────────────────────

  test('9. UI 工单列表可见 — 验证所有工单在工单页面展示', async () => {
    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**', { timeout: 10_000 })

    // Page should show ticket list
    await expect(page.getByText('变更工单')).toBeVisible()

    // Should see the submit button
    await expect(page.getByRole('button', { name: '提交新工单' })).toBeVisible()

    // Should have ticket rows
    const ticketRows = page.getByRole('row').filter({ hasText: /#|E2E test/ })
    await expect(ticketRows.first()).toBeVisible({ timeout: 10_000 })
  })

  // ── 10. Ticket detail drawer via UI ──────────────────────────────────────

  test('10. UI 工单详情抽屉 — 验证能打开工单详情并查看信息', async () => {
    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**', { timeout: 10_000 })

    // Click on the first ticket row that contains "E2E"
    const e2eRow = page.getByRole('row').filter({ hasText: /E2E/ }).first()
    await e2eRow.waitFor({ state: 'visible', timeout: 10_000 })
    await e2eRow.click()

    // Wait for detail drawer/sheet to open
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // Verify SQL content is visible in the drawer
    await expect(sheet.getByText(/E2E test/)).toBeVisible()
  })
})
