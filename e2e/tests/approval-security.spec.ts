/**
 * approval-security.spec.ts
 *
 * E2E 审批安全测试 (SF-QA0036)
 *
 * 韩锐评审要求的安全测试覆盖：
 *
 * 测试范围：
 *   - 多角色权限测试矩阵（developer/dba/admin × 核心操作）
 *   - 水平越权：A 操作 B 的工单（通过 API 验证）
 *   - 垂直越权：developer 直接调审批 API
 *   - 批量审批幂等性（重复提交不产生副作用）
 *   - Token 过期后操作应重定向登录页
 *
 * 前置：docker-compose.test.yml 环境，需要 admin + developer + dba 用户
 */
import { test, expect, loginViaApi, getFirstDatasourceId, getToken } from '../support/real-test-helpers'
import {
  E2E_PREFIX,
  createTicketViaAPI,
  getTicketViaAPI,
  navigateToTickets,
  type Page,
} from '../support/approval-helpers'

test.describe.configure({ timeout: 45_000 })

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'

test.describe('审批安全测试', () => {
  let adminCtx: { page: Page; datasourceId: number }
  let devTicketId: number

  test.beforeAll(async ({ browser }) => {
    // Admin context
    const ctx = await browser.newContext()
    const page = await ctx.newPage()
    await loginViaApi(page)
    const datasourceId = (await getFirstDatasourceId(page)).id

    // Ensure test users exist
    const { apiHelper } = await import('../support/real-test-helpers')
    for (const [role, user] of [
      ['developer', { username: 'e2e-developer', password: 'e2e-test-pass-123' }],
      ['dba', { username: 'e2e-dba', password: 'e2e-test-pass-123' }],
    ] as const) {
      try { await apiHelper(page, 'POST', '/users', { ...user, role }) } catch { /* ok */ }
    }

    // Developer 创建一个工单
    const devToken = await getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (devToken) {
      const res = await page.evaluate(async ({ baseUrl, token, dsId }) => {
        const createRes = await fetch(`${baseUrl}/api/tickets`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
          body: JSON.stringify({
            datasource_id: dsId, database: 'testdb',
            sql: 'SELECT 1 AS dev_security', db_type: 'mysql',
            change_reason: `${E2E_PREFIX} dev security ticket`,
          }),
        })
        return await createRes.json()
      }, { baseUrl: BASE_URL, token: devToken, dsId: datasourceId })
      devTicketId = res.data.id
    }

    adminCtx = { page, datasourceId }
    await ctx.close()
  })

  // ── 垂直越权：developer 调审批 API ─────────────────────────────────

  test('垂直越权：developer 不能审批工单', async ({ page }) => {
    const devToken = await getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (!devToken) { test.skip(); return }

    // 创建一个 developer 的工单
    const createRes = await page.evaluate(async ({ baseUrl, token, dsId }) => {
      const r = await fetch(`${baseUrl}/api/tickets`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({
          datasource_id: dsId, database: 'testdb',
          sql: 'SELECT 1 AS vertical_overflow', db_type: 'mysql',
          change_reason: '越权测试',
        }),
      })
      return { status: r.status, data: await r.json() }
    }, { baseUrl: BASE_URL, token: devToken, dsId: adminCtx.datasourceId })
    const tid = createRes.data.data.id

    // Developer 尝试审批
    const approveRes = await page.evaluate(async ({ baseUrl, token, tid }) => {
      const r = await fetch(`${baseUrl}/api/tickets/${tid}/approve`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ comment: 'unauthorized' }),
      })
      return { status: r.status, data: await r.json() }
    }, { baseUrl: BASE_URL, token: devToken, tid })

    // 应被拒绝
    expect(approveRes.status >= 400 || approveRes.data.code !== 0).toBeTruthy()

    // 工单状态不应变为 APPROVED
    await loginViaApi(page)
    const ticket = await getTicketViaAPI(page, tid)
    expect(ticket.status).not.toBe('APPROVED')
  })

  // ── 垂直越权：developer 不能执行工单 ──────────────────────────────

  test('垂直越权：developer 不能执行未通过的工单', async ({ page }) => {
    const devToken = await getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (!devToken) { test.skip(); return }

    // Developer 创建工单并尝试执行（未经审批）
    const res = await page.evaluate(async ({ baseUrl, token, dsId }) => {
      const createRes = await fetch(`${baseUrl}/api/tickets`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({
          datasource_id: dsId, database: 'testdb',
          sql: 'SELECT 1 AS exec_overflow', db_type: 'mysql',
          change_reason: '执行越权测试',
        }),
      })
      const body = await createRes.json()
      const tid = body.data.id

      const execRes = await fetch(`${baseUrl}/api/tickets/${tid}/execute`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({}),
      })
      return { execStatus: execRes.status, execBody: await execRes.json() }
    }, { baseUrl: BASE_URL, token: devToken, dsId: adminCtx.datasourceId })

    // 执行应被拒绝
    expect(res.execStatus >= 400 || res.execBody.code !== 0).toBeTruthy()
  })

  // ── 水平越权：developer 不能审批别人的工单 ──────────────────────────

  test('水平越权：developer 不能审批 admin 创建的工单', async ({ page }) => {
    await loginViaApi(page)

    // Admin 创建工单
    const adminTicketId = await createTicketViaAPI(
      page, adminCtx.datasourceId,
      'SELECT 1 AS horizontal_overflow',
      `${E2E_PREFIX} horizontal security`,
    )

    // Developer 尝试审批 admin 的工单
    const devToken = await getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (!devToken) { test.skip(); return }

    const approveRes = await page.evaluate(async ({ baseUrl, token, tid }) => {
      const r = await fetch(`${baseUrl}/api/tickets/${tid}/approve`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ comment: 'horizontal unauthorized' }),
      })
      return { status: r.status, data: await r.json() }
    }, { baseUrl: BASE_URL, token: devToken, tid: adminTicketId })

    // 应被拒绝
    expect(approveRes.status >= 400 || approveRes.data.code !== 0).toBeTruthy()
  })

  // ── 批量审批幂等性 ─────────────────────────────────────────────────

  test('批量审批幂等性 — 重复审批不产生副作用', async ({ page }) => {
    await loginViaApi(page)

    const ticketId = await createTicketViaAPI(
      page, adminCtx.datasourceId,
      'SELECT 1 AS idempotent',
      `${E2E_PREFIX} idempotent`,
    )

    // 第一次审批成功
    const { apiHelper } = await import('../support/real-test-helpers')
    const r1 = await apiHelper(page, 'POST', `/tickets/${ticketId}/approve`, { comment: 'first approve' })
    expect(r1.status).toBeLessThan(300)

    // 第二次审批（已 APPROVED 状态再审批）
    const r2 = await apiHelper(page, 'POST', `/tickets/${ticketId}/approve`, { comment: 'second approve' })
    // 应返回错误（状态不再允许审批）
    expect(r2.status >= 400 || (r2.data as { code?: number }).code !== 0).toBeTruthy()

    // 验证状态仍然是 APPROVED
    const ticket = await getTicketViaAPI(page, ticketId)
    expect(ticket.status).toBe('APPROVED')
  })

  // ── Token 过期重定向 ──────────────────────────────────────────────

  test('无效 Token → 访问工单页重定向到登录页', async ({ page }) => {
    // 注入无效 token
    await page.goto(`${BASE_URL}/login`)
    await page.evaluate(() => localStorage.setItem('token', 'invalid-token-12345'))

    // 访问工单页
    await page.goto(`${BASE_URL}/tickets`)

    // 应重定向到登录页
    await page.waitForURL('**/login**', { timeout: 10_000 })
    await expect(page).toHaveURL(/\/login/)
  })

  // ── 多角色权限矩阵 — UI 层面 ──────────────────────────────────────

  test('权限矩阵 — 3 角色工单页面行为差异', async ({ page }) => {
    const roles = ['admin', 'developer', 'dba'] as const
    const credentials: Record<string, [string, string]> = {
      admin: ['e2eadmin', 'e2e-test-pass-123'],
      developer: ['e2e-developer', 'e2e-test-pass-123'],
      dba: ['e2e-dba', 'e2e-test-pass-123'],
    }

    for (const role of roles) {
      const [username, password] = credentials[role]
      const token = await getToken(username, password).catch(() => null)
      if (!token) continue

      await page.goto(`${BASE_URL}/login`)
      await page.evaluate((t) => localStorage.setItem('token', t), token)
      await navigateToTickets(page)

      const isApprover = role === 'admin' || role === 'dba'

      if (isApprover) {
        // 审批人应看到"待我审批"按钮和 checkbox
        await expect(page.getByRole('button', { name: '待我审批' })).toBeVisible({ timeout: 5_000 })
        await expect(page.locator('thead').locator('input[type="checkbox"]').first()).toBeVisible({ timeout: 3_000 })
      } else {
        // developer 不应看到"待我审批"和 checkbox
        await expect(page.getByRole('button', { name: '待我审批' })).not.toBeVisible({ timeout: 5_000 })
        await expect(page.locator('thead').locator('input[type="checkbox"]').first()).not.toBeVisible({ timeout: 3_000 })
      }

      // 所有角色都能看到"我提交的"按钮
      await expect(page.getByRole('button', { name: '我提交的' })).toBeVisible()
    }
  })
})
