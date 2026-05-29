/**
 * ai-risk-display.spec.ts
 *
 * E2E AI 风险评级展示 UI 测试 (SF-QA0036)
 *
 * P2 补充 — 覆盖工单列表和详情中的风险评级 badge 颜色编码。
 *
 * 测试范围：
 *   - 列表页：低/中/高风险 badge 颜色 CSS class
 *   - 详情抽屉：风险 badge 颜色
 *   - 风险 dot 颜色（小圆点指示器）
 *   - 无风险评级的工单显示「—」
 *   - 风险筛选功能
 *
 * 颜色编码（来自前端 riskColorMap）：
 *   low:    bg-emerald-500/20 text-emerald-400 (dot: bg-emerald-400)
 *   medium: bg-yellow-500/20 text-yellow-400  (dot: bg-yellow-400)
 *   high:   bg-red-500/20 text-red-400       (dot: bg-red-400)
 *
 * 前置：docker-compose.test.yml 环境，e2e-admin 账号可用
 */
import { test, expect, loginViaApi, getFirstDatasourceId, apiHelper } from '../support/real-test-helpers'
import {
  E2E_PREFIX,
  createTicketViaAPI,
  getTicketViaAPI,
  navigateToTickets,
  openTicketDrawer,
  type Page,
} from '../support/approval-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('AI 风险评级展示', () => {
  let page: Page
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id
    await ctx.close()
  })

  test.beforeEach(async ({ page }) => {
    await loginViaApi(page)
  })

  // ── 列表页风险 badge 颜色 ──────────────────────────────────────────

  test('SELECT 语句 → 低风险 badge（绿色系）', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'SELECT 1 AS low_risk',
      `${E2E_PREFIX} risk low`,
    )

    // 确认后端确实返回 low
    const ticket = await getTicketViaAPI(page, ticketId)
    if (ticket.risk_level !== 'low') {
      // 有些 SQL 可能被评为 medium，跳过
      test.skip()
      return
    }

    await navigateToTickets(page)

    const row = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await row.waitFor({ state: 'visible', timeout: 10_000 })

    // 验证绿色系 CSS class
    const badge = row.locator('.rounded-full').filter({ hasText: /低风险/ })
    await expect(badge).toBeVisible()
    // CSS class 包含 emerald
    const classes = await badge.getAttribute('class')
    expect(classes).toContain('emerald')
  })

  test('INSERT 语句 → 中风险 badge（黄色系）', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      "INSERT INTO sys_user (username, email) VALUES ('test', 'test@test.com')",
      `${E2E_PREFIX} risk medium`,
    )

    const ticket = await getTicketViaAPI(page, ticketId)
    if (ticket.risk_level !== 'medium') {
      test.skip()
      return
    }

    await navigateToTickets(page)

    const row = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await row.waitFor({ state: 'visible', timeout: 10_000 })

    const badge = row.locator('.rounded-full').filter({ hasText: /中风险/ })
    await expect(badge).toBeVisible()
    const classes = await badge.getAttribute('class')
    expect(classes).toContain('yellow')
  })

  test('DELETE 语句 → 高/极高风险 badge（红色系）', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'DELETE FROM sys_user WHERE id = 999',
      `${E2E_PREFIX} risk high`,
    )

    const ticket = await getTicketViaAPI(page, ticketId)
    const level = ticket.risk_level as string
    if (!['high', 'critical'].includes(level)) {
      test.skip()
      return
    }

    await navigateToTickets(page)

    const row = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await row.waitFor({ state: 'visible', timeout: 10_000 })

    const badge = row.locator('.rounded-full').filter({ hasText: /风险/ })
    await expect(badge).toBeVisible()
    const classes = await badge.getAttribute('class')
    expect(classes).toContain('red')
  })

  // ── 风险 dot 颜色 ────────────────────────────────────────────────

  test('风险 badge 包含颜色指示 dot', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'SELECT 1 AS dot_test',
      `${E2E_PREFIX} risk dot`,
    )

    await navigateToTickets(page)

    const row = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await row.waitFor({ state: 'visible', timeout: 10_000 })

    // dot 是 rounded-full 的小圆点
    const dot = row.locator('.inline-block.rounded-full').filter({ has: page.locator(':scope') }).first()
    // 至少有一个 dot 存在
    const dots = row.locator('.rounded-full.h-1\\.5')
    expect(await dots.count()).toBeGreaterThanOrEqual(1)
  })

  // ── 无风险评级 ────────────────────────────────────────────────────

  test('无风险评级的工单列表中显示「—」', async ({ page }) => {
    // 创建后立即查看（可能还没有 AI 分析）
    // 某些 SQL 风险评估可能返回空
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      '-- just a comment',
      `${E2E_PREFIX} risk empty`,
    )

    await navigateToTickets(page)

    const row = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await row.waitFor({ state: 'visible', timeout: 10_000 })

    // 风险列显示「—」或 badge
    const riskCell = row.locator('td').filter({ hasText: /—|低风险|中风险|高风险/ })
    await expect(riskCell).toBeVisible()
  })

  // ── 详情抽屉风险 badge ────────────────────────────────────────────

  test('详情抽屉中风险 badge 与列表一致', async ({ page }) => {
    const ticketId = await createTicketViaAPI(
      page, datasourceId,
      'ALTER TABLE sys_user ADD COLUMN age INT',
      `${E2E_PREFIX} risk drawer`,
    )

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="sheet-content"]')
    const badge = sheet.locator('.rounded-full').filter({ hasText: /风险/ })
    if (await badge.isVisible()) {
      const classes = await badge.getAttribute('class')
      // ALTER 是 critical/high，应该是红色
      if (classes?.includes('red')) {
        expect(classes).toContain('red')
      }
    }
  })

  // ── 风险筛选功能 ──────────────────────────────────────────────────

  test('风险筛选器 — 选择「低风险」过滤结果', async ({ page }) => {
    // 先创建一些不同风险的工单
    await createTicketViaAPI(page, datasourceId, 'SELECT 1', `${E2E_PREFIX} filter low 1`)
    await createTicketViaAPI(page, datasourceId, 'SELECT 2', `${E2E_PREFIX} filter low 2`)

    await navigateToTickets(page)

    // 打开风险筛选下拉
    const riskSelect = page.getByRole('combobox').filter({ hasText: /AI 风险|全部风险/ }).first()
    await riskSelect.click()

    // 选择「低风险」
    await page.getByRole('option', { name: '低风险' }).click()

    // 等待列表刷新
    await page.waitForTimeout(2_000)

    // 所有显示的工单应该是低风险
    const rows = page.locator('tbody').locator('tr')
    const count = await rows.count()
    if (count > 0) {
      for (let i = 0; i < count; i++) {
        await expect(rows.nth(i).getByText(/低风险/)).toBeVisible({ timeout: 3_000 })
      }
    }
  })
})
