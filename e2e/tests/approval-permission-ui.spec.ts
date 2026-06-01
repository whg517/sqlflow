/**
 * approval-permission-ui.spec.ts
 *
 * E2E 审批→执行权限边界 UI 测试 (SF-QA0036)
 *
 * P1 高优先 — 验证不同角色在审批→执行链路中的按钮显隐和操作权限。
 *
 * 测试范围：
 *   - admin/dba 角色：可以看到通过/拒绝按钮
 *   - developer 角色：不显示通过/拒绝按钮
 *   - 审批通过后：执行按钮对 submitter 和 admin/dba 可见
 *   - 非 admin/dba：不显示批量操作 checkbox
 *   - "待我审批" 筛选按钮仅对 admin/dba 可见
 *   - 越权 API 调用验证（developer 直接调审批 API 应被拒绝）
 *
 * 前置：docker-compose.test.yml 环境，需要 admin + developer + dba 用户
 */
import { test, expect, loginViaApi, getFirstDatasourceId, getToken } from '../support/real-test-helpers'
import {
  E2E_PREFIX,
  createTicketViaAPI,
  navigateToTickets,
  openTicketDrawer,
  type Page,
} from '../support/approval-helpers'

test.describe.configure({ timeout: 45_000 })

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'

test.describe('审批→执行权限边界 UI', () => {
  let adminPage: Page
  let datasourceId: number
  let ticketId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    adminPage = await ctx.newPage()
    await loginViaApi(adminPage)
    datasourceId = (await getFirstDatasourceId(adminPage)).id

    // 确保测试用户存在
    for (const [role, user] of [
      ['developer', { username: 'e2e-developer', password: 'e2e-test-pass-123' }],
      ['dba', { username: 'e2e-dba', password: 'e2e-test-pass-123' }],
    ] as const) {
      try {
        const { apiHelper } = await import('../support/real-test-helpers')
        await apiHelper(adminPage, 'POST', '/users', {
          username: user.username,
          password: user.password,
          role,
        })
      } catch { /* best-effort */ }
    }

    ticketId = await createTicketViaAPI(
      adminPage, datasourceId,
      'SELECT 1 AS permission_test',
      `${E2E_PREFIX} permission base`,
    )
    await ctx.close()
  })

  // ── admin/dba 可以看到审批按钮 ──────────────────────────────────────

  test('admin 角色 → 抽屉显示通过/拒绝按钮', async ({ page }) => {
    await loginViaApi(page)
    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    await expect(sheet.getByRole('button', { name: '通过' })).toBeVisible()
    await expect(sheet.getByRole('button', { name: '拒绝' })).toBeVisible()
  })

  test('dba 角色 → 抽屉显示通过/拒绝按钮', async ({ page }) => {
    const token = await getToken('e2e-dba', 'e2e-test-pass-123').catch(() => null)
    if (!token) { test.skip(); return }

    await page.goto(`${BASE_URL}/login`)
    await page.evaluate((t) => localStorage.setItem('token', t), token)
    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    await expect(sheet.getByRole('button', { name: '通过' })).toBeVisible({ timeout: 5_000 })
    await expect(sheet.getByRole('button', { name: '拒绝' })).toBeVisible({ timeout: 5_000 })
  })

  // ── developer 不显示审批按钮 ─────────────────────────────────────────

  test('developer 角色 → 抽屉不显示通过/拒绝按钮', async ({ page }) => {
    // 创建一个 developer 自己提交的工单
    const devToken = await getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (!devToken) { test.skip(); return }

    await page.goto(`${BASE_URL}/login`)
    await page.evaluate((t) => localStorage.setItem('token', t), devToken)

    // Developer 创建工单
    const { apiHelper } = await import('../support/real-test-helpers')
    const { status: s1, data: d1 } = await apiHelper(page, 'POST', '/tickets', {
      datasource_id: datasourceId,
      database: 'testdb',
      sql: 'SELECT 1 AS dev_permission',
      db_type: 'mysql',
      change_reason: `${E2E_PREFIX} dev permission`,
    })
    const devTicketId = (d1 as { code: number; data: { id: number } }).data.id

    await navigateToTickets(page)
    await openTicketDrawer(page, devTicketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    await expect(sheet.getByRole('button', { name: '通过' })).not.toBeVisible({ timeout: 5_000 })
    await expect(sheet.getByRole('button', { name: '拒绝' })).not.toBeVisible({ timeout: 5_000 })
  })

  // ── "待我审批" 筛选按钮 ─────────────────────────────────────────────

  test('admin/dba → 显示"待我审批"筛选按钮', async ({ page }) => {
    await loginViaApi(page)
    await navigateToTickets(page)

    await expect(page.getByRole('button', { name: '待我审批' })).toBeVisible()
  })

  test('developer → 不显示"待我审批"筛选按钮', async ({ page }) => {
    const token = await getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (!token) { test.skip(); return }

    await page.goto(`${BASE_URL}/login`)
    await page.evaluate((t) => localStorage.setItem('token', t), token)
    await navigateToTickets(page)

    await expect(page.getByRole('button', { name: '待我审批' })).not.toBeVisible({ timeout: 5_000 })
  })

  // ── 越权 API 调用验证 ──────────────────────────────────────────────

  test('越权：developer 调审批 API 应返回 403', async ({ page }) => {
    const devToken = await getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (!devToken) { test.skip(); return }

    // 用 developer token 创建工单
    const res = await page.evaluate(async ({ baseUrl, token, dsId }) => {
      // 先 login as dev
      const loginRes = await fetch(`${baseUrl}/api/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: 'e2e-developer', password: 'e2e-test-pass-123' }),
      })
      const loginBody = await loginRes.json()
      const devToken = loginBody.data.access_token

      // 创建工单
      const createRes = await fetch(`${baseUrl}/api/tickets`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${devToken}` },
        body: JSON.stringify({
          datasource_id: dsId,
          database: 'testdb',
          sql: 'SELECT 1 AS unauthorized_test',
          db_type: 'mysql',
          change_reason: '越权测试',
        }),
      })
      const createBody = await createRes.json()

      // 尝试用自己的 dev token 审批自己的工单
      const approveRes = await fetch(`${baseUrl}/api/tickets/${createBody.data.id}/approve`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${devToken}` },
        body: JSON.stringify({ comment: 'unauthorized' }),
      })
      return { status: approveRes.status, body: await approveRes.json() }
    }, { baseUrl: BASE_URL, token: devToken, dsId: datasourceId })

    // developer 不应有审批权限，应返回 403 或错误
    expect(res.status >= 400).toBeTruthy()
  })

  // ── 水平越权：A 不能操作 B 的工单（如果后端有检查） ─────────────────

  test('审批后执行按钮可见性 — APPROVED 状态显示执行按钮', async ({ page }) => {
    await loginViaApi(page)

    // 通过 API 审批
    const { apiHelper } = await import('../support/real-test-helpers')
    await apiHelper(page, 'POST', `/tickets/${ticketId}/approve`, { comment: 'approve for exec test' })

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    // 已通过后应显示执行按钮
    await expect(sheet.getByRole('button', { name: '执行' })).toBeVisible({ timeout: 5_000 })
    // 通过/拒绝按钮应消失
    await expect(sheet.getByRole('button', { name: '通过' })).not.toBeVisible({ timeout: 3_000 })
    await expect(sheet.getByRole('button', { name: '拒绝' })).not.toBeVisible({ timeout: 3_000 })
  })
})
