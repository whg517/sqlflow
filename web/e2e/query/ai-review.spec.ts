import { type Page, type Route, test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI } from '../helpers'

// --- SSE Helpers ---

/**
 * Build an SSE body string with a single `result` event.
 * Used to mock the /api/query/review endpoint responses.
 */
function buildReviewSSE(reviewResult: Record<string, unknown>): string {
  return (
    'event: content\ndata: "Analyzing SQL..."\n\n' +
    'event: result\ndata: ' +
    JSON.stringify(reviewResult) +
    '\n\n'
  )
}

const BLOCKED_REVIEW = {
  risk_level: 'critical',
  risk_score: 95,
  decision: 'blocked',
  summary: '检测到 DROP TABLE 操作，该操作将永久删除表及其所有数据，属于不可逆的高危操作。',
  suggestions: [
    '请确认是否真的需要删除该表',
    '建议先使用 CREATE TABLE ... LIKE 备份表结构',
    '考虑使用 RENAME TABLE 代替 DROP 进行软删除',
  ],
  impact_analysis: '将永久删除 users 表及其所有数据和索引',
  rollback_sql: '',
  warnings: ['此操作不可逆，请谨慎执行'],
  review_source: 'ai',
  reviewed_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + 30000).toISOString(),
  model_used: 'gpt-4',
}

const TICKET_REVIEW = {
  risk_level: 'high',
  risk_score: 80,
  decision: 'ticket',
  summary:
    '检测到无 WHERE 条件的 DELETE 语句，将删除表中所有数据。此操作需要通过工单审批流程。',
  suggestions: [
    '建议添加 WHERE 条件限定删除范围',
    '确认是否需要清空整表数据',
    '可以考虑使用 TRUNCATE TABLE 替代（更高效）',
  ],
  impact_analysis: '将删除 users 表中所有数据，无法恢复',
  rollback_sql: '',
  warnings: ['全表删除操作，请确认已做好数据备份'],
  review_source: 'ai',
  reviewed_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + 30000).toISOString(),
  model_used: 'gpt-4',
}

const CONFIRM_REVIEW = {
  risk_level: 'medium',
  risk_score: 55,
  decision: 'confirm',
  summary: 'ALTER TABLE 操作将修改表结构，属于中等风险操作。请确认变更内容后执行。',
  suggestions: [
    '建议在业务低峰期执行 DDL 操作',
    '确认新字段类型和长度满足业务需求',
  ],
  impact_analysis: '将为 users 表新增 phone 字段 (VARCHAR(20))',
  rollback_sql: 'ALTER TABLE users DROP COLUMN phone;',
  warnings: [],
  review_source: 'ai',
  reviewed_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + 30000).toISOString(),
  model_used: 'gpt-4',
}

const EXECUTE_REVIEW = {
  risk_level: 'low',
  risk_score: 5,
  decision: 'execute',
  summary: '低风险的 SELECT 查询，仅读取数据，无数据修改风险。',
  suggestions: [],
  impact_analysis: 'Read-only query, no data modification',
  rollback_sql: '',
  warnings: [],
  review_source: 'ai',
  reviewed_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + 30000).toISOString(),
  model_used: 'gpt-4',
}

const ALTER_EXECUTE_RESULT = {
  code: 0,
  message: 'ok',
  data: {
    columns: [],
    rows: [],
    total: 0,
    execution_time_ms: 230,
    affected_rows: 0,
    desensitized: false,
    desensitized_fields: [],
    warnings: [],
  },
}

// --- Page Setup Helpers ---

/**
 * Login, navigate to query page, select datasource, and type SQL into the CodeMirror editor.
 */
async function setupQueryPage(page: Page, sql: string) {
  await loginViaUI(page)
  await expect(page).toHaveURL(/\/query/)

  // Select datasource (first option)
  await page.getByRole('combobox').filter({ hasText: '选择数据源' }).click()
  await page.getByRole('option', { name: /test-mysql/ }).click()

  // Wait for CodeMirror editor to mount
  const editor = page.locator('.cm-editor')
  await expect(editor).toBeVisible()

  // Type SQL into the CodeMirror editor via its contenteditable area
  const cmContent = page.locator('.cm-content')
  await cmContent.click()
  await page.keyboard.type(sql, { delay: 10 })
}

/**
 * Click the "执行" button in the StatusBar.
 */
async function clickExecute(page: Page) {
  await page.getByRole('button', { name: '执行' }).click()
}

test.describe('AI 审查流程', () => {
  test.beforeEach(async ({ page }) => {
    // Set up default mocks, then we override review route per test
    mockApiRoutes(page)
  })

  test('DROP TABLE 触发 AI 审查并被拦截', async ({ page }) => {
    // Override the review endpoint to return blocked
    await page.route('**/api/query/review', async (route: Route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: buildReviewSSE(BLOCKED_REVIEW),
      })
    })

    await setupQueryPage(page, 'DROP TABLE users;')

    // Click execute — triggers AI review
    await clickExecute(page)

    // Verify AI review indicator appears
    await expect(page.getByText('AI 评审中...')).toBeVisible()

    // Wait for review to complete
    await expect(page.getByText('AI 评审完成')).toBeVisible()

    // Verify blocked decision message
    await expect(page.getByText('操作被安全规则拦截，禁止执行')).toBeVisible()

    // Verify risk level badge shows high/critical risk
    await expect(page.getByText(/高风险|危险/).first()).toBeVisible()

    // Verify the rejection summary is displayed
    await expect(
      page.getByText('检测到 DROP TABLE 操作，该操作将永久删除表及其所有数据'),
    ).toBeVisible()

    // Verify SQL was NOT executed — result area should not show query data
    // The result area should be empty or show no table data
    await expect(page.getByText('Alice')).not.toBeVisible()

    // Verify the impact analysis is shown
    await expect(
      page.getByText(/将永久删除 users 表及其所有数据/),
    ).toBeVisible()

    // Verify warnings are shown
    await expect(page.getByText('此操作不可逆，请谨慎执行')).toBeVisible()
  })

  test('DELETE 无 WHERE 触发审查并需提交工单', async ({ page }) => {
    // Override the review endpoint to return ticket (high risk, needs approval)
    await page.route('**/api/query/review', async (route: Route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: buildReviewSSE(TICKET_REVIEW),
      })
    })

    await setupQueryPage(page, 'DELETE FROM users;')

    // Click execute — triggers AI review
    await clickExecute(page)

    // Verify AI review starts
    await expect(page.getByText('AI 评审中...')).toBeVisible()

    // Wait for review to complete
    await expect(page.getByText('AI 评审完成')).toBeVisible()

    // Verify ticket decision message
    await expect(page.getByText('高风险操作，需提交工单审批')).toBeVisible()

    // Verify the review summary mentions the risk
    await expect(
      page.getByText(/无 WHERE 条件的 DELETE 语句/),
    ).toBeVisible()

    // Verify "提交工单" button is present
    await expect(page.getByRole('button', { name: '提交工单' })).toBeVisible()

    // Verify SQL was NOT executed — no query result data
    await expect(page.getByText('Alice')).not.toBeVisible()

    // Verify suggestions count
    await expect(page.getByText(/3 条建议/)).toBeVisible()

    // Expand suggestions to verify content
    await page.getByText(/3 条建议/).click()
    await expect(page.getByText('建议添加 WHERE 条件限定删除范围')).toBeVisible()
  })

  test('高风险 SQL 通过审查后可确认执行', async ({ page }) => {
    // Override the review endpoint to return confirm (medium risk, needs manual confirmation)
    await page.route('**/api/query/review', async (route: Route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: buildReviewSSE(CONFIRM_REVIEW),
      })
    })

    // Override execute endpoint to return ALTER TABLE result (no rows)
    await page.route('**/api/query/execute', async (route: Route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(ALTER_EXECUTE_RESULT),
      })
    })

    await setupQueryPage(page, 'ALTER TABLE users ADD COLUMN phone VARCHAR(20);')

    // Click execute — triggers AI review
    await clickExecute(page)

    // Verify AI review starts
    await expect(page.getByText('AI 评审中...')).toBeVisible()

    // Wait for review to complete
    await expect(page.getByText('AI 评审完成')).toBeVisible()

    // Verify confirm decision message
    await expect(page.getByText('高风险查询，请确认后执行')).toBeVisible()

    // Verify "确认执行" button is present
    await expect(page.getByRole('button', { name: '确认执行' })).toBeVisible()

    // Verify rollback SQL is displayed
    await expect(page.getByText(/ALTER TABLE users DROP COLUMN phone/)).toBeVisible()

    // Click "确认执行" to proceed
    await page.getByRole('button', { name: '确认执行' }).click()

    // Verify query executes — execution time should appear in status bar
    await expect(page.getByText(/230ms/)).toBeVisible()
  })

  test('安全 SQL 不触发审查直接执行', async ({ page }) => {
    // Override the review endpoint to return execute (low risk, auto-execute)
    await page.route('**/api/query/review', async (route: Route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: buildReviewSSE(EXECUTE_REVIEW),
      })
    })

    await setupQueryPage(page, 'SELECT * FROM users WHERE id = 1;')

    // Click execute
    await clickExecute(page)

    // Verify AI review indicator briefly appears then auto-executes
    await expect(page.getByText('AI 评审中...')).toBeVisible()

    // The review completes and auto-executes (decision: 'execute' with 1s delay)
    // Verify query result is displayed
    await expect(page.getByText('Alice')).toBeVisible({ timeout: 5000 })
    await expect(page.getByText('Bob')).toBeVisible()

    // Verify no blocking review UI remains (no "确认执行", no "提交工单", no "禁止执行")
    await expect(page.getByRole('button', { name: '确认执行' })).not.toBeVisible()
    await expect(page.getByRole('button', { name: '提交工单' })).not.toBeVisible()
    await expect(page.getByText('操作被安全规则拦截')).not.toBeVisible()

    // Verify result metadata (execution time)
    await expect(page.getByText(/15ms/)).toBeVisible()
  })
})
