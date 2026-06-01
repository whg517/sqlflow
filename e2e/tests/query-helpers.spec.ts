/**
 * E2E — 查询辅助功能（真实后端）
 * SF-QA0027: Covers query history, query execution error handling,
 */
import { test, expect } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const ADMIN_USER = process.env.E2E_USERNAME ?? 'e2e-admin'
const ADMIN_PASS = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

test.describe.configure({ timeout: 60_000 })

// --- Helpers ---

async function loginAndInjectToken(page: import('@playwright/test').Page): Promise<string> {
  const loginRes = await page.request.post(`${BASE_URL}/api/auth/login`, {
    data: { username: ADMIN_USER, password: ADMIN_PASS },
  })
  expect(loginRes.ok()).toBeTruthy()
  const body = await loginRes.json()
  const token = body.data.token

  await page.goto('/')
  await page.evaluate((t) => localStorage.setItem('token', t), token)
  return token
}

async function getFirstDatasourceId(page: import('@playwright/test').Page): Promise<{ id: number; name: string }> {
  const res = await page.request.get(`${BASE_URL}/api/datasources`)
  expect(res.ok()).toBeTruthy()
  const body = await res.json()
  const list: Array<{ id: number; name: string; type: string; status: string }> = body.data ?? []
  const ds = list.find((d) => d.status === 'active' && d.type === 'mysql')
    ?? list.find((d) => d.status === 'active')
  expect(ds).toBeTruthy()
  return { id: ds!.id, name: ds!.name }
}

// --- Tests ---

test.describe('查询辅助功能 — 真实后端', () => {
  let token: string
  let datasourceId: number

  test.beforeEach(async ({ page }) => {
    token = await loginAndInjectToken(page)
    const ds = await getFirstDatasourceId(page)
    datasourceId = ds.id
  })

  test('查询历史 API 返回记录', async ({ page }) => {
    // Execute a query first to ensure history exists
    await page.request.post(`${BASE_URL}/api/query/execute`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { datasource_id: datasourceId, database: 'testdb', sql: 'SELECT 1 AS e2e_history_helper' },
    })

    // Fetch history
    const historyRes = await page.request.get(`${BASE_URL}/api/query/history?datasource_id=${datasourceId}&page=1&page_size=50`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(historyRes.ok()).toBeTruthy()
    const historyBody = await historyRes.json()
    const items = historyBody.data ?? []
    expect(items.length).toBeGreaterThan(0)

    // Verify item structure
    const item = items[0] as Record<string, unknown>
    expect(item).toHaveProperty('id')
    expect(item).toHaveProperty('sql_content')
    expect(item).toHaveProperty('execution_time')
  })

  test('错误 SQL 返回错误响应', async ({ page }) => {
    const res = await page.request.post(`${BASE_URL}/api/query/execute`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { datasource_id: datasourceId, database: 'testdb', sql: 'SELECTT * FROM invalid_syntax' },
    })
    const body = await res.json()
    // Backend should return error for invalid SQL
    expect(body.code !== 0 || !res.ok()).toBeTruthy()
  })

  test('数据源表列表 API 返回正确数据', async ({ page }) => {
    const res = await page.request.get(`${BASE_URL}/api/datasources/${datasourceId}/tables`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.ok()).toBeTruthy()
    const body = await res.json()
    expect(body.code).toBe(0)
    const tables: string[] = body.data ?? []
    expect(tables.length).toBeGreaterThan(0)
    // Known tables from init.sql
    expect(tables).toContain('sys_user')
  })

  test('数据源表列 API 返回正确结构', async ({ page }) => {
    const res = await page.request.get(`${BASE_URL}/api/datasources/${datasourceId}/tables/sys_user/columns`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(res.ok()).toBeTruthy()
    const body = await res.json()
    expect(body.code).toBe(0)
    const columns = body.data ?? []
    expect(columns.length).toBeGreaterThan(0)
    // Should have known columns
    const colNames = columns.map((c: Record<string, unknown>) => c.column_name ?? c.name)
    expect(colNames).toContain('id')
    expect(colNames).toContain('username')
  })

  test('查询结果导出 API', async ({ page }) => {
    const res = await page.request.post(`${BASE_URL}/api/query/export`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: {
        datasource_id: datasourceId,
        database: 'testdb',
        sql: 'SELECT 1 AS export_test',
        format: 'csv',
      },
    })
    expect(res.status()).toBeLessThan(500)
    const contentType = res.headers()['content-type'] ?? ''
    expect(contentType).toContain('text/csv')
  })
})
