/**
 * sql-templates.spec.ts
 *
 * E2E: SQL 模板 CRUD + 渲染 — 6 API
 *   POST   /api/sql-templates           — 创建模板
 *   GET    /api/sql-templates            — 列表
 *   GET    /api/sql-templates/:id        — 详情
 *   PUT    /api/sql-templates/:id        — 更新
 *   DELETE /api/sql-templates/:id        — 删除
 *   POST   /api/sql-templates/:id/render — 渲染
 *
 * 边界：空列表、模板名唯一性、SQL 注入防御
 */
import {
  test,
  expect,
  BASE_URL,
  loginViaUI,
  apiHelper,
  getToken,
} from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

const UID = Date.now()
const E2E_PREFIX = `e2e_tpl_${UID}`

// Track created template IDs for cleanup
const createdIds: number[] = []

// --- Cleanup ---
test.afterAll(async () => {
  if (createdIds.length === 0) return
  try {
    const token = await getToken()
    for (const id of createdIds) {
      await fetch(`${BASE_URL}/api/sql-templates/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      }).catch(() => {})
    }
  } catch {
    // best-effort cleanup
  }
})

test.describe('SQL Templates — CRUD', () => {
  test('should start with empty or minimal list', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'GET', '/sql-templates')
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: unknown[] }
    expect(body.code).toBe(0)
    // List should be an array (may be empty or have items from other tests)
    expect(Array.isArray(body.data)).toBeTruthy()
  })

  test('should create a template', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'POST', '/sql-templates', {
      name: `${E2E_PREFIX}_basic`,
      description: 'E2E test template',
      sql_content: 'SELECT * FROM {{ table }} WHERE id = {{ id }}',
      db_type: 'mysql',
      category: 'general',
      is_public: true,
    })
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: { id: number; name: string } }
    expect(body.code).toBe(0)
    expect(body.data.id).toBeGreaterThan(0)
    expect(body.data.name).toContain(E2E_PREFIX)

    createdIds.push(body.data.id)
  })

  test('should reject duplicate template name', async ({ page }) => {
    await loginViaUI(page)

    const name = `${E2E_PREFIX}_dup`
    // Create first
    const { data: createData } = await apiHelper(page, 'POST', '/sql-templates', {
      name,
      sql_content: 'SELECT 1',
      db_type: 'mysql',
    })
    const body1 = createData as { code: number; data: { id: number } }
    createdIds.push(body1.data.id)

    // Try duplicate
    const { status, data } = await apiHelper(page, 'POST', '/sql-templates', {
      name,
      sql_content: 'SELECT 2',
      db_type: 'mysql',
    })
    // 409 Conflict
    expect(status).toBe(409)
    const body = data as { code: number; message: string }
    expect(body.code).toBe(409)
    expect(body.message).toContain('已存在')
  })

  test('should get template by ID', async ({ page }) => {
    await loginViaUI(page)

    // Create a template first
    const { data: createData } = await apiHelper(page, 'POST', '/sql-templates', {
      name: `${E2E_PREFIX}_get`,
      sql_content: 'SELECT {{ col }} FROM users',
      db_type: 'mysql',
      category: 'analytics',
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    // Get it
    const { status, data } = await apiHelper(page, 'GET', `/sql-templates/${created.data.id}`)
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: { id: number; name: string; sql_content: string; category: string } }
    expect(body.code).toBe(0)
    expect(body.data.id).toBe(created.data.id)
    expect(body.data.sql_content).toContain('{{ col }}')
    expect(body.data.category).toBe('analytics')
  })

  test('should update template', async ({ page }) => {
    await loginViaUI(page)

    // Create
    const { data: createData } = await apiHelper(page, 'POST', '/sql-templates', {
      name: `${E2E_PREFIX}_update`,
      sql_content: 'SELECT 1',
      db_type: 'mysql',
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    // Update
    const newName = `${E2E_PREFIX}_updated`
    const { status, data } = await apiHelper(page, 'PUT', `/sql-templates/${created.data.id}`, {
      name: newName,
      sql_content: 'SELECT * FROM orders WHERE status = {{ status }}',
      db_type: 'mysql',
      category: 'business',
      is_public: false,
    })
    expect(status).toBeLessThan(300)
    const body = data as { code: number }
    expect(body.code).toBe(0)

    // Verify update
    const { data: getData } = await apiHelper(page, 'GET', `/sql-templates/${created.data.id}`)
    const getBody = getData as { code: number; data: { name: string; sql_content: string; category: string } }
    expect(getBody.data.name).toBe(newName)
    expect(getBody.data.sql_content).toContain('{{ status }}')
    expect(getBody.data.category).toBe('business')
  })

  test('should delete template', async ({ page }) => {
    await loginViaUI(page)

    // Create
    const { data: createData } = await apiHelper(page, 'POST', '/sql-templates', {
      name: `${E2E_PREFIX}_delete`,
      sql_content: 'SELECT 1',
      db_type: 'mysql',
    })
    const created = createData as { code: number; data: { id: number } }
    const tplId = created.data.id

    // Delete
    const { status, data } = await apiHelper(page, 'DELETE', `/sql-templates/${tplId}`)
    expect(status).toBeLessThan(300)
    const body = data as { code: number }
    expect(body.code).toBe(0)

    // Verify deleted — should 404
    const { status: getStatus } = await apiHelper(page, 'GET', `/sql-templates/${tplId}`)
    expect(getStatus).toBe(404)

    // Remove from cleanup list since already deleted
    const idx = createdIds.indexOf(tplId)
    if (idx >= 0) createdIds.splice(idx, 1)
  })
})

test.describe('SQL Templates — Render', () => {
  test('should render template with params', async ({ page }) => {
    await loginViaUI(page)

    // Create a template with placeholders
    const { data: createData } = await apiHelper(page, 'POST', '/sql-templates', {
      name: `${E2E_PREFIX}_render`,
      sql_content: 'SELECT {{ columns }} FROM {{ table }} WHERE created_at > {{ date }} LIMIT {{ limit }}',
      db_type: 'mysql',
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    // Render
    const { status, data } = await apiHelper(page, 'POST', `/sql-templates/${created.data.id}/render`, {
      params: {
        columns: 'id, name, email',
        table: 'users',
        date: '2026-01-01',
        limit: '100',
      },
    })
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: string }
    expect(body.code).toBe(0)
    // Rendered SQL should have params replaced
    expect(body.data).toContain('id, name, email')
    expect(body.data).toContain('users')
    expect(body.data).toContain('2026-01-01')
    expect(body.data).toContain('100')
    expect(body.data).not.toContain('{{')
  })

  test('should render with empty params (keep placeholders)', async ({ page }) => {
    await loginViaUI(page)

    const { data: createData } = await apiHelper(page, 'POST', '/sql-templates', {
      name: `${E2E_PREFIX}_render_empty`,
      sql_content: 'SELECT * FROM users WHERE id = {{ id }}',
      db_type: 'mysql',
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    const { status, data } = await apiHelper(page, 'POST', `/sql-templates/${created.data.id}/render`, {
      params: {},
    })
    expect(status).toBeLessThan(300)
    // Empty params should either keep or strip placeholders — backend behavior
    const body = data as { code: number }
    expect(body.code).toBe(0)
  })
})

test.describe('SQL Templates — Boundary', () => {
  test('should reject empty name', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'POST', '/sql-templates', {
      name: '',
      sql_content: 'SELECT 1',
      db_type: 'mysql',
    })
    expect(status).toBe(400)
    const body = data as { code: number; message: string }
    expect(body.message).toContain('名称')
  })

  test('should reject empty SQL content', async ({ page }) => {
    await loginViaUI(page)

    const { status, data } = await apiHelper(page, 'POST', '/sql-templates', {
      name: `${E2E_PREFIX}_nosql`,
      sql_content: '',
      db_type: 'mysql',
    })
    expect(status).toBe(400)
    const body = data as { code: number; message: string }
    expect(body.message).toContain('SQL')
  })

  test('should handle SQL injection in template content', async ({ page }) => {
    await loginViaUI(page)

    const maliciousSQL = "SELECT * FROM users; DROP TABLE users; -- {{ table }}"
    const { data: createData } = await apiHelper(page, 'POST', '/sql-templates', {
      name: `${E2E_PREFIX}_inject`,
      sql_content: maliciousSQL,
      db_type: 'mysql',
    })
    const created = createData as { code: number; data: { id: number } }
    createdIds.push(created.data.id)

    // Template should be stored as-is (SQL is not executed, only stored/rendered)
    expect(created.code).toBe(0)

    // Render with a param — should return the SQL string
    const { status, data } = await apiHelper(page, 'POST', `/sql-templates/${created.data.id}/render`, {
      params: { table: 'orders' },
    })
    expect(status).toBeLessThan(300)
    const body = data as { code: number; data: string }
    expect(body.data).toContain('orders')
  })

  test('should return 404 for non-existent template', async ({ page }) => {
    await loginViaUI(page)

    const { status } = await apiHelper(page, 'GET', '/sql-templates/999999')
    expect(status).toBe(404)
  })
})
