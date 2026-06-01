/**
 * sql-analysis.spec.ts
 *
 * E2E SQL 解析测试 (SF-QA0034)
 *
 * SQL 解析逻辑在工单创建时自动执行，通过验证创建的工单字段来测试解析结果。
 *
 * 测试范围：
 *   - DDL 语句识别（CREATE, ALTER, DROP, TRUNCATE）
 *   - DML 语句识别（SELECT, INSERT, UPDATE, DELETE）
 *   - affected_tables 提取
 *   - 多语句（单条）
 *   - 边界条件：空 SQL、纯注释、特殊语法
 *
 * 前置：docker-compose.test.yml 环境，e2eadmin 账号可用
 */
import { test, expect, BASE_URL, ADMIN_USER, ADMIN_PASS, loginViaApi, getFirstDatasourceId, apiHelper } from '../support/real-test-helpers'
import type { Page } from '@playwright/test'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

/** Create a ticket and return parsed ticket data. */
async function createAndAnalyze(
  page: Page,
  datasourceId: number,
  sql: string,
  reason: string,
) {
  const { status, data } = await apiHelper(page, 'POST', '/tickets', {
    datasource_id: datasourceId,
    database: 'testdb',
    sql,
    db_type: 'mysql',
    change_reason: reason,
  })
  expect(status).toBe(200)
  const body = data as { code: number; data: Record<string, unknown> }
  expect(body.code).toBe(0)
  return body.data
}

// --- Tests ---

test.describe('SQL 解析 — 语句类型识别', () => {
  let page: Page
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id
  })

  test.afterAll(async () => {
    try { await page.context().close() } catch { /* ignore */ }
  })

  // ── DDL 语句 ──────────────────────────────────────────────────────────

  test('CREATE TABLE — 识别为 CREATE', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'CREATE TABLE e2e_analysis_create (id INT PRIMARY KEY, name VARCHAR(100))',
      'E2E: DDL CREATE',
    )
    expect(ticket.sql_type).toBe('CREATE')
  })

  test('CREATE TABLE IF NOT EXISTS — 识别为 CREATE', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'CREATE TABLE IF NOT EXISTS e2e_analysis_create2 (id INT)',
      'E2E: DDL CREATE IF NOT EXISTS',
    )
    expect(ticket.sql_type).toBe('CREATE')
  })

  test('ALTER TABLE — 识别为 ALTER', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'ALTER TABLE e2e_analysis ADD COLUMN age INT',
      'E2E: DDL ALTER',
    )
    expect(ticket.sql_type).toBe('ALTER')
  })

  test('DROP TABLE — 识别为 DROP', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'DROP TABLE IF EXISTS e2e_analysis_drop',
      'E2E: DDL DROP',
    )
    expect(ticket.sql_type).toBe('DROP')
  })

  test('TRUNCATE TABLE — 识别为 TRUNCATE', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'TRUNCATE TABLE e2e_analysis_trunc',
      'E2E: DDL TRUNCATE',
    )
    expect(ticket.sql_type).toBe('TRUNCATE')
  })

  // ── DML 语句 ──────────────────────────────────────────────────────────

  test('SELECT — 识别为 SELECT', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'SELECT * FROM e2e_analysis WHERE id = 1',
      'E2E: DML SELECT',
    )
    expect(ticket.sql_type).toBe('SELECT')
  })

  test('INSERT — 识别为 INSERT', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'INSERT INTO e2e_analysis (id, name) VALUES (1, "test")',
      'E2E: DML INSERT',
    )
    expect(ticket.sql_type).toBe('INSERT')
  })

  test('UPDATE — 识别为 UPDATE', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'UPDATE e2e_analysis SET name = "updated" WHERE id = 1',
      'E2E: DML UPDATE',
    )
    expect(ticket.sql_type).toBe('UPDATE')
  })

  test('DELETE — 识别为 DELETE', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'DELETE FROM e2e_analysis WHERE id = 1',
      'E2E: DML DELETE',
    )
    expect(ticket.sql_type).toBe('DELETE')
  })

  // ── 特殊语法 ──────────────────────────────────────────────────────────

  test('GRANT — 识别为 GRANT', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      "GRANT SELECT ON e2e_analysis TO 'readonly_user'@'%'",
      'E2E: DDL GRANT',
    )
    expect(ticket.sql_type).toBe('GRANT')
  })

  test('REVOKE — 识别为 REVOKE', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      "REVOKE SELECT ON e2e_analysis FROM 'readonly_user'@'%'",
      'E2E: DDL REVOKE',
    )
    expect(ticket.sql_type).toBe('REVOKE')
  })

  test('REPLACE INTO — 识别为 REPLACE', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'REPLACE INTO e2e_analysis (id, name) VALUES (1, "replaced")',
      'E2E: DML REPLACE',
    )
    expect(ticket.sql_type).toBe('REPLACE')
  })
})

test.describe('SQL 解析 — affected_tables 提取', () => {
  let page: Page
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id
  })

  test.afterAll(async () => {
    try { await page.context().close() } catch { /* ignore */ }
  })

  test('SELECT ... FROM — 提取表名', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'SELECT id, name FROM users WHERE status = 1',
      'E2E: table extraction SELECT',
    )
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables).toContain('users')
  })

  test('SELECT ... JOIN — 提取多表', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id',
      'E2E: table extraction JOIN',
    )
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables).toContain('users')
    expect(tables).toContain('orders')
  })

  test('INSERT INTO — 提取目标表', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'INSERT INTO products (name, price) VALUES ("widget", 9.99)',
      'E2E: table extraction INSERT',
    )
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables).toContain('products')
  })

  test('UPDATE — 提取目标表', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'UPDATE inventory SET stock = 0 WHERE product_id = 42',
      'E2E: table extraction UPDATE',
    )
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables).toContain('inventory')
  })

  test('DELETE FROM — 提取目标表', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'DELETE FROM logs WHERE created_at < "2025-01-01"',
      'E2E: table extraction DELETE',
    )
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables).toContain('logs')
  })

  test('CREATE TABLE — 提取新表名', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'CREATE TABLE e2e_new_table (id INT PRIMARY KEY)',
      'E2E: table extraction CREATE',
    )
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables).toContain('e2e_new_table')
  })

  test('带反引号的表名 — 正确提取', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'SELECT * FROM `my_table` WHERE `my_table`.id = 1',
      'E2E: backtick table name',
    )
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables).toContain('my_table')
  })
})

test.describe('SQL 解析 — 边界条件', () => {
  let page: Page
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id
  })

  test.afterAll(async () => {
    try { await page.context().close() } catch { /* ignore */ }
  })

  test('纯注释 — 识别为 OTHER', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      '-- This is a comment\n/* multi line comment */',
      'E2E: comment only',
    )
    expect(ticket.sql_type).toBe('OTHER')
  })

  test('带注释的 SQL — 正确识别', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      '-- Get active users\nSELECT * FROM users WHERE active = 1',
      'E2E: SQL with comments',
    )
    expect(ticket.sql_type).toBe('SELECT')
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables).toContain('users')
  })

  test('多 JOIN — 提取所有表', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'SELECT a.name, b.total, c.category FROM accounts a LEFT JOIN orders b ON a.id = b.user_id RIGHT JOIN categories c ON b.cat_id = c.id',
      'E2E: multi-join table extraction',
    )
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables.length).toBeGreaterThanOrEqual(3)
    expect(tables).toContain('accounts')
    expect(tables).toContain('orders')
    expect(tables).toContain('categories')
  })

  test('子查询 — 提取外层和子查询表', async () => {
    const ticket = await createAndAnalyze(
      page, datasourceId,
      'SELECT * FROM users WHERE id IN (SELECT user_id FROM orders WHERE total > 100)',
      'E2E: subquery table extraction',
    )
    const tables: string[] = typeof ticket.affected_tables === 'string'
      ? JSON.parse(ticket.affected_tables as string)
      : (ticket.affected_tables as string[] || [])
    expect(tables).toContain('users')
    expect(tables).toContain('orders')
  })
})
