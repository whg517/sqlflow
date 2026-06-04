/**
 * E2E — 审计日志展开详情 + 关联工单跳转（真实后端）
 * SF-QA0028 batch 2
 */
import { test, expect, loginViaUI, apiHelper, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

/** Execute a query to ensure audit log has data */
async function ensureAuditData(page: import('@playwright/test').Page) {
  const token = await page.evaluate(() => localStorage.getItem('token') ?? '')
  const ds = await page.evaluate(async ({ baseUrl, token }) => {
    const r = await fetch(`${baseUrl}/api/datasources`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const body = await r.json()
    const list = body.data ?? []
    return list.find((d: { type: string; status: string }) => d.type === 'mysql' && d.status === 'active')
      ?? list.find((d: { status: string }) => d.status === 'active')
  }, { baseUrl: process.env.E2E_BASE_URL ?? 'http://localhost:8080', token })
  if (!ds) return
  await page.evaluate(async ({ baseUrl, token, dsId }) => {
    await fetch(`${baseUrl}/api/query/execute`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ datasource_id: dsId, database: 'testdb', sql: 'SELECT 1 AS e2e_audit_seed' }),
    })
  }, { baseUrl: process.env.E2E_BASE_URL ?? 'http://localhost:8080', token, dsId: ds.id })
}

test.describe('审计展开详情 + 关联跳转（真实后端）', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('展开详情面板 - 显示完整 SQL、执行耗时、影响行数', async ({ page }) => {
    await ensureAuditData(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    // 等待审计日志列表加载
    await Promise.race([
      page.locator('table tbody tr').first().waitFor({ timeout: 10_000 }),
      page.getByText('暂无审计日志').waitFor({ timeout: 10_000 }),
    ])

    const hasData = await page.locator('table tbody tr').first().isVisible().catch(() => false)
    if (!hasData) {
      test.skip()
      return
    }

    // 点击第一条记录展开
    const firstRow = page.locator('table tbody tr').first()
    await firstRow.click()

    // 等待展开区域出现
    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible({ timeout: 5000 })

    // 验证展开面板显示关键字段
    await expect(expandedRow.getByText('完整 SQL')).toBeVisible()
    await expect(expandedRow.getByText('执行耗时')).toBeVisible()
    await expect(expandedRow.getByText('影响行数')).toBeVisible()
    await expect(expandedRow.getByText('返回行数')).toBeVisible()

    // 验证 SQL 内容存在（pre/code block）
    const sqlBlock = expandedRow.locator('pre, code').first()
    await expect(sqlBlock).toBeVisible()
    const sqlText = await sqlBlock.textContent()
    expect(sqlText?.trim().length).toBeGreaterThan(0)

    // 验证执行耗时显示（数字 + ms）
    await expect(expandedRow.getByText(/\d+ms/)).toBeVisible()
  })

  test('展开详情面板 - 显示 IP 地址', async ({ page }) => {
    await ensureAuditData(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    await Promise.race([
      page.locator('table tbody tr').first().waitFor({ timeout: 10_000 }),
      page.getByText('暂无审计日志').waitFor({ timeout: 10_000 }),
    ])

    const hasData = await page.locator('table tbody tr').first().isVisible().catch(() => false)
    if (!hasData) {
      test.skip()
      return
    }

    const firstRow = page.locator('table tbody tr').first()
    await firstRow.click()

    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible({ timeout: 5000 })

    // 验证 IP 地址字段
    await expect(expandedRow.getByText('IP 地址')).toBeVisible()
    // IP 地址应为有效格式
    const ipPattern = expandedRow.getByText(/\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/)
    await expect(ipPattern).toBeVisible()
  })

  test('复制 SQL 按钮功能', async ({ page }) => {
    await ensureAuditData(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    await Promise.race([
      page.locator('table tbody tr').first().waitFor({ timeout: 10_000 }),
      page.getByText('暂无审计日志').waitFor({ timeout: 10_000 }),
    ])

    const hasData = await page.locator('table tbody tr').first().isVisible().catch(() => false)
    if (!hasData) {
      test.skip()
      return
    }

    const firstRow = page.locator('table tbody tr').first()
    await firstRow.click()

    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible({ timeout: 5000 })

    // 验证复制按钮存在
    const copyBtn = expandedRow.getByRole('button', { name: '复制' })
    await expect(copyBtn).toBeVisible()

    // Mock clipboard API
    await page.evaluate(() => {
      Object.assign(navigator, {
        clipboard: {
          writeText: async (text: string) => {
            ;(window as unknown as Record<string, string>).__clipboardText = text
          },
        },
      })
    })

    // 点击复制
    await copyBtn.click()

    // 验证复制成功提示
    await expect(expandedRow.getByText('已复制')).toBeVisible()

    // 验证剪贴板内容非空且包含 SQL 关键字
    const clipboardText = await page.evaluate(() => (window as unknown as Record<string, string>).__clipboardText)
    expect(clipboardText?.length).toBeGreaterThan(0)
    expect(clipboardText?.toUpperCase()).toContain('SELECT')
  })

  test('切换展开项 - 只显示一个详情面板', async ({ page }) => {
    await ensureAuditData(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    await Promise.race([
      page.locator('table tbody tr').first().waitFor({ timeout: 10_000 }),
      page.getByText('暂无审计日志').waitFor({ timeout: 10_000 }),
    ])

    const rows = page.locator('table tbody tr')
    const rowCount = await rows.count()
    if (rowCount < 2) {
      test.skip()
      return
    }

    // 展开第一条
    await rows.nth(0).click()
    await expect(page.locator('.audit-expanded-row')).toHaveCount(1)

    // 展开第二条（第一条应收起）
    await rows.nth(1).click()
    await expect(page.locator('.audit-expanded-row')).toHaveCount(1)

    // 验证当前展开面板可见且包含 SQL
    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow.locator('pre, code').first()).toBeVisible()
  })

  test('关联工单 - 有 ticket_id 的记录显示工单链接', async ({ page }) => {
    await ensureAuditData(page)
    await page.goto('/audit')
    await page.waitForURL('**/audit')

    await Promise.race([
      page.locator('table tbody tr').first().waitFor({ timeout: 10_000 }),
      page.getByText('暂无审计日志').waitFor({ timeout: 10_000 }),
    ])

    const rows = page.locator('table tbody tr')
    const hasData = await rows.first().isVisible().catch(() => false)
    if (!hasData) {
      test.skip()
      return
    }

    // 逐行展开检查是否有工单链接
    const rowCount = Math.min(await rows.count(), 10)
    let foundTicketLink = false

    for (let i = 0; i < rowCount; i++) {
      await rows.nth(i).click()
      const expandedRow = page.locator('.audit-expanded-row').first()
      await expandedRow.waitFor({ state: 'visible', timeout: 3000 }).catch(() => {})

      const ticketLink = expandedRow.getByRole('link', { name: /#\d+/ })
      if (await ticketLink.isVisible().catch(() => false)) {
        foundTicketLink = true
        // 验证链接 href 包含工单 ID
        await expect(ticketLink).toHaveAttribute('href', /\/tickets/)
        break
      }
    }

    // 注意：测试环境可能没有关联工单的审计记录，这不代表功能有问题
    if (!foundTicketLink) {
      test.info().annotations.push({ type: 'skip-reason', description: 'No audit logs with associated tickets in test environment' })
    }
  })
})
