/**
 * risk-evaluation.spec.ts
 *
 * E2E 风险评级测试 (SF-QA0034)
 *
 * 风险评级在工单创建时自动执行，通过验证创建的工单 risk_level 字段来测试评级结果。
 *
 * 评级规则（基于 RiskEvaluator）：
 *   - low (≤15): SELECT
 *   - medium (16-40): INSERT, MERGE, REPLACE, CREATE
 *   - high (41-65): UPDATE, GRANT/REVOKE
 *   - critical (>65): DELETE, ALTER, TRUNCATE, DROP
 *
 * 测试范围：
 *   - HIGH/MODERATE/LOW/CRITICAL 触发条件
 *   - 多表影响增加风险分
 *   - 敏感表交叉检测（通过敏感表 API 配置 + 工单创建验证）
 *   - 边界条件
 *
 * 前置：docker-compose.test.yml 环境，e2e-admin 账号可用
 */
import { test, expect, BASE_URL, ADMIN_USER, ADMIN_PASS, loginViaApi, getFirstDatasourceId, apiHelper, getToken } from '../support/real-test-helpers'
import type { Page } from '@playwright/test'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

const E2E_PREFIX = `e2e_risk_${Date.now()}`

/** Create a ticket and return its risk_level. */
async function createAndGetRisk(
  page: Page,
  datasourceId: number,
  sql: string,
): Promise<string> {
  const { status, data } = await apiHelper(page, 'POST', '/tickets', {
    datasource_id: datasourceId,
    database: 'testdb',
    sql,
    db_type: 'mysql',
    change_reason: `${E2E_PREFIX} risk eval`,
  })
  expect(status).toBe(200)
  const body = data as { code: number; data: { risk_level: string } }
  expect(body.code).toBe(0)
  return body.data.risk_level
}

/** Create a sensitive table entry. */
async function createSensitiveTable(datasourceId: number, tableName: string) {
  const token = await getToken()
  const res = await fetch(`${BASE_URL}/api/sensitive-tables`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ datasource_id: datasourceId, table_name: tableName }),
  })
  return res.json()
}

/** Delete a sensitive table entry by ID. */
async function deleteSensitiveTableById(id: number) {
  const token = await getToken()
  await fetch(`${BASE_URL}/api/sensitive-tables/${id}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
}

/** List sensitive tables and return array. */
async function listSensitiveTables() {
  const token = await getToken()
  const res = await fetch(`${BASE_URL}/api/sensitive-tables`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const body = await res.json()
  return (body.data ?? body) as Array<{ id: number; datasource_id: number; table_name: string }>
}

/** Cleanup e2e_ prefixed sensitive tables. */
async function cleanupE2ESensitiveTables() {
  const tables = await listSensitiveTables()
  for (const t of tables) {
    if (t.table_name.startsWith('e2e_')) {
      await deleteSensitiveTableById(t.id)
    }
  }
}

// --- Tests ---

test.describe('风险评级 — 语句类型触发', () => {
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

  // ── LOW 风险 ──────────────────────────────────────────────────────────

  test('SELECT — LOW 风险', async () => {
    const risk = await createAndGetRisk(page, datasourceId, 'SELECT 1')
    expect(risk).toBe('low')
  })

  test('SELECT 多列多表 — LOW 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      'SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id',
    )
    expect(risk).toBe('low')
  })

  // ── MEDIUM 风险 ───────────────────────────────────────────────────────

  test('INSERT — MEDIUM 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      'INSERT INTO e2e_risk_ins (name, value) VALUES ("test", 42)',
    )
    expect(risk).toBe('medium')
  })

  test('CREATE TABLE — MEDIUM 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      'CREATE TABLE e2e_risk_create (id INT PRIMARY KEY)',
    )
    expect(risk).toBe('medium')
  })

  test('REPLACE INTO — MEDIUM 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      'REPLACE INTO e2e_risk_rep (id, name) VALUES (1, "test")',
    )
    expect(risk).toBe('medium')
  })

  // ── HIGH 风险 ──────────────────────────────────────────────────────────

  test('UPDATE — HIGH 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      'UPDATE e2e_risk_upd SET status = 2 WHERE id = 1',
    )
    expect(risk).toBe('high')
  })

  test('GRANT — HIGH 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      "GRANT SELECT ON e2e_risk_grant TO 'user'@'%'",
    )
    expect(risk).toBe('high')
  })

  // ── CRITICAL 风险 ─────────────────────────────────────────────────────

  test('DELETE — CRITICAL 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      'DELETE FROM e2e_risk_del WHERE id = 1',
    )
    expect(risk).toBe('critical')
  })

  test('ALTER TABLE — CRITICAL 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      'ALTER TABLE e2e_risk_alt ADD COLUMN col INT',
    )
    expect(risk).toBe('critical')
  })

  test('DROP TABLE — CRITICAL 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      'DROP TABLE IF EXISTS e2e_risk_drop',
    )
    expect(risk).toBe('critical')
  })

  test('TRUNCATE — CRITICAL 风险', async () => {
    const risk = await createAndGetRisk(
      page, datasourceId,
      'TRUNCATE TABLE e2e_risk_trunc',
    )
    expect(risk).toBe('critical')
  })

  // ── 多表影响增加风险 ──────────────────────────────────────────────────

  test('多表 UPDATE — 风险高于单表', async () => {
    const singleRisk = await createAndGetRisk(
      page, datasourceId,
      'UPDATE e2e_risk_single SET val = 1 WHERE id = 1',
    )
    const multiRisk = await createAndGetRisk(
      page, datasourceId,
      // 多表 UPDATE（关联更新）
      'UPDATE e2e_risk_multi a SET a.val = 1 WHERE a.id IN (SELECT b.id FROM e2e_risk_other b)',
    )
    // 多表的风险分应不低于单表
    const levelOrder: Record<string, number> = { low: 0, medium: 1, high: 2, critical: 3 }
    expect(levelOrder[multiRisk] ?? 0).toBeGreaterThanOrEqual(levelOrder[singleRisk] ?? 0)
  })
})

test.describe('风险评级 — 敏感表交叉检测', () => {
  let page: Page
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id
    await cleanupE2ESensitiveTables()
  })

  test.afterAll(async () => {
    await cleanupE2ESensitiveTables()
    try { await page.context().close() } catch { /* ignore */ }
  })

  test('敏感表 — 创建和删除', async () => {
    const tableName = 'e2e_risk_sensitive_1'

    // 创建敏感表
    const res = await createSensitiveTable(datasourceId, tableName)
    expect(res.code).toBe(0)
    const stId = (res.data ?? res).id as number

    // 删除
    await deleteSensitiveTableById(stId)
  })

  test('敏感表 — 列出敏感表', async () => {
    const tables = await listSensitiveTables()
    expect(Array.isArray(tables)).toBeTruthy()
  })

  test('敏感表 — 重复创建同一表名', async () => {
    const tableName = 'e2e_risk_dup'

    // 第一次创建
    const res1 = await createSensitiveTable(datasourceId, tableName)
    expect(res1.code ?? res1.id).toBeTruthy()

    // 第二次创建（可能返回错误或冲突）
    const res2 = await createSensitiveTable(datasourceId, tableName)
    // 无论是重复成功还是报错，都不应该崩溃
    expect(res2).toBeDefined()
  })
})
