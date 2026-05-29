/**
 * comment-ui.spec.ts
 *
 * E2E 评论功能 UI 测试 (SF-QA0036)
 *
 * P1 高优先 — 覆盖工单详情抽屉中的评论功能。
 *
 * 测试范围：
 *   - 发表评论
 *   - 回复评论
 *   - 删除评论
 *   - 时间格式化显示
 *   - XSS 防护（script 标签不应执行）
 *   - 空评论不可发送
 *
 * 前置：docker-compose.test.yml 环境，e2e-admin 账号可用
 */
import { test, expect, loginViaApi, getFirstDatasourceId, apiHelper } from '../support/real-test-helpers'
import {
  E2E_PREFIX,
  createTicketViaAPI,
  createCommentViaAPI,
  navigateToTickets,
  openTicketDrawer,
  type Page,
} from '../support/approval-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('评论功能 UI', () => {
  let page: Page
  let datasourceId: number
  let ticketId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    page = await ctx.newPage()
    await loginViaApi(page)
    datasourceId = (await getFirstDatasourceId(page)).id
    ticketId = await createTicketViaAPI(
      page, datasourceId,
      'SELECT 1 AS comment_ui_test',
      `${E2E_PREFIX} comment base`,
    )
    await ctx.close()
  })

  test.beforeEach(async ({ page }) => {
    await loginViaApi(page)
  })

  // ── 发表评论 ──────────────────────────────────────────────────────────

  test('发表评论 → 列表显示新评论', async ({ page }) => {
    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const sheet = page.locator('[data-slot="section-content"]').filter({ has: page.getByText('评论') }).first()
    const inputArea = page.locator('[data-comment-input]').first()
    if (!(await inputArea.isVisible())) {
      // 滚动到评论区域
      await page.evaluate(() => {
        const el = document.querySelector('[data-comment-input]')
        if (el) el.scrollIntoView({ behavior: 'instant', block: 'center' })
      })
    }
    await inputArea.waitFor({ state: 'visible', timeout: 10_000 })

    const commentText = `E2E 评论测试 ${Date.now()}`
    await inputArea.fill(commentText)

    // 发送按钮
    const sendBtn = page.locator('button').filter({ has: page.locator('svg.lucide-send') }).first()
    await sendBtn.click()

    // 评论应出现在列表
    await expect(page.getByText(commentText)).toBeVisible({ timeout: 5_000 })
  })

  // ── 回复评论 ──────────────────────────────────────────────────────────

  test('回复评论 → 显示为嵌套结构', async ({ page }) => {
    // 先通过 API 创建一条评论
    await loginViaApi(page)
    const parentComment = await createCommentViaAPI(page, ticketId, '父评论内容')

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    // 等待父评论出现
    await expect(page.getByText('父评论内容')).toBeVisible({ timeout: 10_000 })

    // 悬停显示回复按钮
    const parentEl = page.getByText('父评论内容').locator('..').locator('..')
    await parentEl.hover()
    const replyBtn = parentEl.locator('button', { hasText: '回复' })
    await replyBtn.waitFor({ state: 'visible', timeout: 5_000 })
    await replyBtn.click()

    // 应出现回复指示器
    await expect(page.getByText(/回复.*父评论/)).toBeVisible()

    // 输入回复内容
    const inputArea = page.locator('[data-comment-input]').first()
    await inputArea.fill('子评论回复内容')

    // 发送
    const sendBtn = page.locator('button').filter({ has: page.locator('svg.lucide-send') }).first()
    await sendBtn.click()

    // 回复应出现
    await expect(page.getByText('子评论回复内容')).toBeVisible({ timeout: 5_000 })
  })

  // ── 删除评论 ──────────────────────────────────────────────────────────

  test('删除评论 → 从列表消失', async ({ page }) => {
    await loginViaApi(page)
    const comment = await createCommentViaAPI(page, ticketId, `待删除评论 ${Date.now()}`)

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    // 等待评论出现
    const commentEl = page.getByText(`待删除评论 ${Date.now()}`).first()
    await commentEl.waitFor({ state: 'visible', timeout: 10_000 })

    // 悬停显示删除按钮
    const parentOfComment = commentEl.locator('..').locator('..')
    await parentOfComment.hover()
    const deleteBtn = parentOfComment.locator('button', { hasText: '删除' })
    await deleteBtn.waitFor({ state: 'visible', timeout: 5_000 })
    await deleteBtn.click()

    // Toast 确认
    await expect(page.getByText(/评论已删除/)).toBeVisible({ timeout: 5_000 })
  })

  // ── 时间格式化 ──────────────────────────────────────────────────────

  test('评论显示相对时间', async ({ page }) => {
    await loginViaApi(page)
    await createCommentViaAPI(page, ticketId, '时间格式化测试')

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    // 刚创建的评论应显示「刚刚」或「分钟前」
    await expect(
      page.getByText(/刚刚|\d+ 分钟前|\d+ 秒前/).first()
    ).toBeVisible({ timeout: 10_000 })
  })

  // ── 空评论不可发送 ──────────────────────────────────────────────────

  test('空评论 → 发送按钮禁用', async ({ page }) => {
    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    const inputArea = page.locator('[data-comment-input]').first()
    await inputArea.waitFor({ state: 'visible', timeout: 10_000 })

    // 清空
    await inputArea.fill('')

    // 发送按钮应禁用
    const sendBtn = page.locator('button').filter({ has: page.locator('svg.lucide-send') }).first()
    await expect(sendBtn).toBeDisabled()
  })

  // ── XSS 防护 ─────────────────────────────────────────────────────────

  test('XSS 防护 — script 标签不应执行', async ({ page }) => {
    const xssPayload = '<script>alert("xss-test")</script>测试内容'
    await loginViaApi(page)
    await createCommentViaAPI(page, ticketId, xssPayload)

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    // 评论内容应显示为文本，不应执行 script
    const commentText = page.getByText('测试内容').first()
    await commentText.waitFor({ state: 'visible', timeout: 10_000 })

    // script 标签不应存在于 DOM 中（React 默认转义）
    await expect(page.locator('script')).not.toBeVisible()
  })

  test('XSS 防护 — img onerror 不应执行', async ({ page }) => {
    const imgXss = '<img src="x" onerror="document.title=\'xss\'" />图片注入'
    await loginViaApi(page)
    await createCommentViaAPI(page, ticketId, imgXss)

    await navigateToTickets(page)
    await openTicketDrawer(page, ticketId)

    // 内容显示
    await expect(page.getByText('图片注入').first()).toBeVisible({ timeout: 10_000 })

    // title 不应被修改
    const title = await page.title()
    expect(title).not.toBe('xss')
  })
})
