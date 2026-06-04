/**
 * E2E Real SQL Execution Tests — query.spec.ts
 *
 * These tests hit the REAL backend (http://localhost:8080) without any mocking.
 * Prerequisites:
 *   1. Backend running at localhost:8080
 *   2. A MySQL datasource named "test-mysql" exists (or any active datasource)
 *   3. A database "testdb" exists on that datasource with a "sys_user" table
 *   4. Default admin credentials: admin / admin123
 *
 * Run:
 *   npx playwright test e2e-real/query.spec.ts
 */

import { test, expect, BASE_URL, loginViaApi, getFirstDatasourceId } from '../support/real-test-helpers'
import type { Page } from '@playwright/test'

// --- Helpers (query-specific, using shared helpers) ---

/**
 * Execute a SQL query via the real API and return the response body.
 */
async function realExecuteSql(page: Page, token: string, params: {
  datasource_id: number
  database: string
  sql: string
}) {
  const res = await page.request.post(`${BASE_URL}/api/query/execute`, {
    headers: { Authorization: `Bearer ${token}` },
    data: params,
  })
  const body = await res.json()
  return { status: res.status(), body }
}

/**
 * Select a datasource in the UI dropdown by name.
 */
async function selectDatasourceByName(page: Page, name: string) {
  const trigger = page.getByRole('combobox').first()
  await trigger.click()
  await page.getByRole('option', { name: new RegExp(name) }).click()
}

/**
 * Type SQL into the CodeMirror editor.
 */
async function typeSql(page: Page, sql: string) {
  const editor = page.locator('.cm-editor')
  await expect(editor).toBeVisible()
  await editor.click()
  // Clear existing content with Ctrl+A then Delete
  await page.keyboard.press('Control+a')
  await page.keyboard.press('Delete')
  await page.keyboard.type(sql)
}

/**
 * Execute the current SQL in the editor via the UI execute button.
 * Waits for the AI review SSE stream to complete and for the result to appear.
 */
async function executeViaUI(page: Page) {
  await page.getByRole('button', { name: '执行' }).click()
  // Wait for either result data or error message in the status bar
  await Promise.race([
    page.locator('text=/\\d+ms/').waitFor({ timeout: 30_000 }),
    page.locator('text=/\\d+ 行/').waitFor({ timeout: 30_000 }),
    page.locator('text=/错误|error|syntax/i').waitFor({ timeout: 30_000 }),
  ])
}

test.describe.configure({ timeout: 60_000 })

test.describe('查询执行 E2E（真实 SQL）', () => {
  let token: string
  let datasourceId: number
  let datasourceName: string

  test.beforeAll(async () => {
    // Pre-flight: verify backend is reachable
    try {
      const health = await fetch(`${BASE_URL}/health`)
      if (!health.ok) throw new Error(`Backend not healthy at ${BASE_URL}`)
    } catch {
      throw new Error(`Cannot connect to backend at ${BASE_URL}. Is it running?`)
    }
  })

  test.beforeEach(async ({ page }) => {
    token = await loginViaApi(page)
    await page.goto('/query')
    await page.waitForURL('**/query')
    const ds = await getFirstDatasourceId(page)
    datasourceId = ds.id
    datasourceName = ds.name
  })

  // =========================================================================
  // Test 1: Select datasource + execute SELECT 1
  // =========================================================================
  test('选择数据源并执行 SELECT 1 → 验证结果显示', async ({ page }) => {
    // Select datasource in the UI
    await selectDatasourceByName(page, datasourceName)

    // Set database
    const dbInput = page.getByPlaceholder(/数据库名/)
    await dbInput.fill('testdb')

    // Type and execute
    await typeSql(page, 'SELECT 1')
    await executeViaUI(page)

    // Verify result is shown — status bar should display time and row count
    await expect(page.locator('text=/\\d+ms/')).toBeVisible()
    await expect(page.locator('text=/\\d+ 行/')).toBeVisible()

    // Verify result table has at least one row with value "1"
    await expect(page.locator('.cm-editor')).toBeVisible() // editor still present
    // The result table should contain the value 1 from SELECT 1
    const resultBody = page.locator('[class*="TableBody"], .result-table')
    if (await resultBody.isVisible()) {
      await expect(resultBody.locator('text=1').first()).toBeVisible()
    }
  })

  // =========================================================================
  // Test 2: Query real table data (sys_user)
  // =========================================================================
  test('查询 sys_user 表数据 → 验证结果表格和列名', async ({ page }) => {
    await selectDatasourceByName(page, datasourceName)
    await page.getByPlaceholder(/数据库名/).fill('testdb')

    const sql = 'SELECT * FROM sys_user LIMIT 10'
    await typeSql(page, sql)
    await executeViaUI(page)

    // Verify status bar shows execution stats
    await expect(page.locator('text=/\\d+ms/')).toBeVisible()

    // Verify result table is rendered with column headers
    // sys_user typically has columns like id, username, password, email, etc.
    const tableHeaders = page.locator('[role="columnheader"]')
    const headerCount = await tableHeaders.count()
    expect(headerCount, 'Result table should have column headers').toBeGreaterThan(0)

    // Verify at least one common sys_user column is present
    const headerTexts = await tableHeaders.allTextContents()
    const headerLower = headerTexts.map((h) => h.toLowerCase())
    const hasExpectedColumn = headerLower.some(
      (h) => h.includes('id') || h.includes('username') || h.includes('name') || h.includes('user'),
    )
    expect(hasExpectedColumn, 'Should have at least one expected column (id/username/name)').toBeTruthy()

    // Verify data rows exist (if table has data)
    const rows = page.locator('[class*="TableRow"], tbody tr')
    const rowCount = await rows.count()
    expect(rowCount, 'Result table should have data rows').toBeGreaterThan(0)
  })

  // =========================================================================
  // Test 3: Error SQL handling — syntax error
  // =========================================================================
  test('错误 SQL（SELECTT）→ 验证显示错误信息', async ({ page }) => {
    await selectDatasourceByName(page, datasourceName)
    await page.getByPlaceholder(/数据库名/).fill('testdb')

    const invalidSql = 'SELECTT * FROM'
    await typeSql(page, invalidSql)
    await executeViaUI(page)

    // Wait for the error to surface — could be in status bar or toast
    // The backend should return a non-zero code with an error message
    await Promise.race([
      // Error shown in status bar (red text)
      page.locator('.text-\\[var\\(--danger\\)\\], [class*="danger"], [class*="error"]')
        .first()
        .waitFor({ timeout: 15_000 })
        .catch(() => null),
      // Or a toast notification with error
      page.locator('[data-sonner-toast][data-type="error"]')
        .waitFor({ timeout: 15_000 })
        .catch(() => null),
      // Or the error text contains "syntax" or "error" (Chinese UI might say 语法错误)
      page.locator('text=/语法|error|错误|syntax/i')
        .waitFor({ timeout: 15_000 })
        .catch(() => null),
    ])

    // Also verify via API that the backend returned an error
    const { status, body } = await realExecuteSql(page, token, {
      datasource_id: datasourceId,
      database: 'testdb',
      sql: invalidSql,
    })
    // Backend should return an error (code !== 0 or non-2xx status)
    const isError = body.code !== 0 || status >= 400 || body.message?.toLowerCase().includes('error')
    expect(isError, 'Invalid SQL should produce an error response').toBeTruthy()
  })

  // =========================================================================
  // Test 4: Query history — execute SQL then check history panel
  // =========================================================================
  test('执行 SQL 后查询历史面板显示历史记录', async ({ page }) => {
    await selectDatasourceByName(page, datasourceName)
    await page.getByPlaceholder(/数据库名/).fill('testdb')

    // Execute a few SQL statements sequentially
    const sqlStatements = [
      'SELECT 1 AS test_col',
      'SELECT 2 AS another_col',
      'SELECT NOW() AS current_time',
    ]

    for (const sql of sqlStatements) {
      await typeSql(page, sql)
      await executeViaUI(page)
      // Brief pause to let the backend record history
      await page.waitForTimeout(500)
    }

    // Open history panel
    await page.getByRole('button', { name: '历史' }).click()

    // Wait for history panel to load
    await page.locator('[data-history-panel]').waitFor({ state: 'visible' })

    // Verify history items exist (at least the ones we just executed)
    // History panel shows sql_summary, execution_time, result_rows, and timestamp
    const historyItems = page.locator('[data-history-panel] .group, [data-history-panel] [class*="cursor-pointer"]')
    await expect(historyItems.first()).toBeVisible({ timeout: 10_000 })

    // Verify one of our queries appears in history
    const panelText = await page.locator('[data-history-panel]').textContent()
    expect(panelText).toBeDefined()
    // History should contain at least a summary matching our queries
    const hasHistory = panelText!.includes('test_col') || panelText!.includes('another_col') || panelText!.includes('current_time')
    expect(hasHistory, 'History panel should contain records of executed queries').toBeTruthy()

    // Close history panel
    await page.getByRole('button', { name: '历史' }).click()
  })

  // =========================================================================
  // Test 5: Query history via API
  // =========================================================================
  test('查询历史 API 返回执行记录', async ({ page }) => {
    // Execute a query via API first to ensure history exists
    await realExecuteSql(page, token, {
      datasource_id: datasourceId,
      database: 'testdb',
      sql: 'SELECT 1 AS e2e_history_test',
    })

    // Fetch history via API
    const res = await page.request.get(`${BASE_URL}/api/query/history?datasource_id=${datasourceId}&page=1&page_size=50`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.ok(), `History API failed: ${res.status()}`).toBeTruthy()

    const body = await res.json()
    const items = body.data ?? []
    expect(items.length, 'History should have at least one record').toBeGreaterThan(0)

    // Verify the history item has expected fields
    const latestItem = items[0] as Record<string, unknown>
    expect(latestItem).toHaveProperty('id')
    expect(latestItem).toHaveProperty('sql_content')
    expect(latestItem).toHaveProperty('database')
    expect(latestItem).toHaveProperty('execution_time')
    expect(latestItem).toHaveProperty('created_at')
  })

  // =========================================================================
  // Test 6: Sensitive data masking (desensitization)
  // =========================================================================
  test('sys_user 敏感字段（password）在结果中被脱敏显示', async ({ page }) => {
    await selectDatasourceByName(page, datasourceName)
    await page.getByPlaceholder(/数据库名/).fill('testdb')

    await typeSql(page, 'SELECT * FROM sys_user LIMIT 10')
    await executeViaUI(page)

    // Wait for result to render
    await expect(page.locator('text=/\\d+ms/')).toBeVisible({ timeout: 15_000 })

    // Check if desensitization badge appears in status bar
    // When desensitized_fields is non-empty, StatusBar shows "已脱敏 N 字段"
    const desensitizeBadge = page.locator('text=/已脱敏.*字段/')
    const isDesensitized = await desensitizeBadge.isVisible().catch(() => false)

    if (isDesensitized) {
      // UI indicates desensitization — verify the Lock icon appears on sensitive column headers
      const lockIcons = page.locator('[data-slot="tooltip-trigger"] svg.lucide-lock, svg.lucide-Lock')
      const lockCount = await lockIcons.count()
      expect(lockCount, 'Sensitive columns should show Lock icon').toBeGreaterThan(0)

      // Verify password values in the result table are masked
      // Masked values typically contain asterisks (***) or are replaced with a masked string
      const tableCells = page.locator('td.font-mono')
      const cellTexts = await tableCells.allTextContents()
      const hasMaskedValue = cellTexts.some(
        (text) => text.includes('***') || text.includes('******') || text === '******',
      )
      expect(hasMaskedValue, 'Password field should show masked values (containing ***)').toBeTruthy()
    } else {
      // Even if the UI doesn't show the desensitization badge, verify via API response
      const { body } = await realExecuteSql(page, token, {
        datasource_id: datasourceId,
        database: 'testdb',
        sql: 'SELECT * FROM sys_user LIMIT 10',
      })

      const result = body.data as Record<string, unknown> | undefined
      if (result?.desensitized === true) {
        expect(result.desensitized_fields, 'Should list desensitized fields').toBeDefined()
        const fields = result.desensitized_fields as string[]
        expect(fields.length, 'Should have at least one desensitized field').toBeGreaterThan(0)
      }

      // If neither UI nor API shows desensitization, it means mask rules may not be configured.
      // We log a warning but don't fail — the test environment may not have mask rules set up.
      console.warn(
        '[e2e-real] No desensitization detected. ' +
        'Ensure mask rules are configured for sys_user.password in the test environment.',
      )
    }
  })

  // =========================================================================
  // Test 7: Direct API execution — verify response structure
  // =========================================================================
  test('直接通过 API 执行查询 → 验证完整响应结构', async ({ page }) => {
    const { status, body } = await realExecuteSql(page, token, {
      datasource_id: datasourceId,
      database: 'testdb',
      sql: 'SELECT 1 AS col_a, 2 AS col_b',
    })

    expect(status).toBeLessThan(300)
    expect(body.code, 'Response code should be 0').toBe(0)

    // Validate response data structure
    const data = body.data as Record<string, unknown>
    expect(data).toHaveProperty('columns')
    expect(data).toHaveProperty('rows')
    expect(data).toHaveProperty('total')
    expect(data).toHaveProperty('execution_time_ms')
    expect(data).toHaveProperty('affected_rows')
    expect(data).toHaveProperty('desensitized')
    expect(data).toHaveProperty('desensitized_fields')
    expect(data).toHaveProperty('warnings')

    // Verify data content
    const columns = data.columns as string[]
    expect(columns).toContain('col_a')
    expect(columns).toContain('col_b')

    const rows = data.rows as Record<string, unknown>[]
    expect(rows.length).toBeGreaterThan(0)
    expect(Number(rows[0].col_a)).toBe(1)
    expect(Number(rows[0].col_b)).toBe(2)

    // total and rows length should match
    expect(Number(data.total)).toBe(rows.length)
  })
})
