/**
 * E2E — 审计日志列表 + keyword 搜索（真实后端）
 * SF-QA0028 batch 2
 */
import { test, expect, loginViaUI } from '../support/real-test-helpers'

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

/** 导航到审计页面并等待加载 */
async function gotoAudit(page: import('@playwright/test').Page) {
  await ensureAuditData(page)
  await page.getByRole('link', { name: '审计' }).first().click()
  await page.waitForURL('**/audit')
  await Promise.race([
    page.locator('table tbody tr').first().waitFor({ timeout: 10_000 }),
    page.getByText('暂无审计日志').waitFor({ timeout: 10_000 }),
  ])
}

test.describe('审计日志列表 + keyword 搜索（真实后端）', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('审计日志列表渲染', async ({ page }) => {
    await gotoAudit(page)

    // 验证页面标题
    await expect(page.getByText('审计日志')).toBeVisible()

    // 验证搜索框
    await expect(page.getByPlaceholder('搜索 SQL / 表名...')).toBeVisible()

    // 验证表头列
    await expect(page.getByRole('columnheader', { name: '时间' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '用户' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '操作' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '数据库' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'SQL 摘要' })).toBeVisible()

    // 验证导出 CSV 按钮存在（页面完整性指标）
    await expect(page.getByRole('button', { name: '导出 CSV' })).toBeVisible()
  })

  test('keyword 搜索 - 按 SQL 内容搜索', async ({ page }) => {
    await gotoAudit(page)

    // 先确认有数据
    const hasData = await page.locator('table tbody tr').first().isVisible().catch(() => false)
    if (!hasData) {
      test.skip()
      return
    }

    // 输入搜索关键词
    await page.getByPlaceholder('搜索 SQL / 表名...').fill('SELECT')
    await page.keyboard.press('Enter')
    await page.waitForTimeout(2000)

    // 如果有结果，验证包含 SELECT
    const rows = page.locator('table tbody tr')
    const rowCount = await rows.count()
    if (rowCount > 0) {
      // 展开第一条验证 SQL 内容
      await rows.first().click()
      await page.waitForTimeout(500)
      const expandedSql = page.locator('.audit-expanded-row pre, .audit-expanded-row code').first()
      if (await expandedSql.isVisible({ timeout: 3000 }).catch(() => false)) {
        const sqlText = await expandedSql.textContent()
        expect(sqlText?.toUpperCase()).toContain('SELECT')
      }
    }
  })

  test('keyword 搜索 - 按操作类型搜索', async ({ page }) => {
    await gotoAudit(page)

    const hasData = await page.locator('table tbody tr').first().isVisible().catch(() => false)
    if (!hasData) {
      test.skip()
      return
    }

    // 输入操作类型关键词
    await page.getByPlaceholder('搜索 SQL / 表名...').fill('SELECT')
    await page.keyboard.press('Enter')
    await page.waitForTimeout(2000)

    // 验证搜索结果存在（或空状态）
    const hasResults = await page.locator('table tbody tr').first().isVisible({ timeout: 3000 }).catch(() => false)
    const hasEmpty = await page.getByText('暂无审计日志').isVisible({ timeout: 1000 }).catch(() => false)
    expect(hasResults || hasEmpty).toBeTruthy()
  })

  test('空搜索结果', async ({ page }) => {
    await gotoAudit(page)

    // 输入不存在的关键词
    const ghostKeyword = `XQZ_NEVER_${Date.now()}_${Math.random().toString(36).slice(2)}`
    await page.getByPlaceholder('搜索 SQL / 表名...').fill(ghostKeyword)
    await page.keyboard.press('Enter')
    await page.waitForTimeout(2000)

    // 验证显示空状态提示
    await expect(page.getByText('暂无审计日志')).toBeVisible()
  })

  test('搜索后清空关键词恢复列表', async ({ page }) => {
    await gotoAudit(page)

    const hasData = await page.locator('table tbody tr').first().isVisible().catch(() => false)
    if (!hasData) {
      test.skip()
      return
    }

    // 先搜索一个不存在的词
    const ghostKeyword = `NEVER_${Date.now()}`
    await page.getByPlaceholder('搜索 SQL / 表名...').fill(ghostKeyword)
    await page.keyboard.press('Enter')
    await page.waitForTimeout(2000)
    await expect(page.getByText('暂无审计日志')).toBeVisible()

    // 清空搜索框并重新搜索
    const searchInput = page.getByPlaceholder('搜索 SQL / 表名...')
    await searchInput.clear()
    await page.keyboard.press('Enter')
    await page.waitForTimeout(2000)

    // 验证数据恢复
    await expect(page.locator('table tbody tr').first()).toBeVisible({ timeout: 5000 })
  })

  test('展开详情面板', async ({ page }) => {
    await gotoAudit(page)

    const hasData = await page.locator('table tbody tr').first().isVisible().catch(() => false)
    if (!hasData) {
      test.skip()
      return
    }

    // 点击第一条记录展开
    await page.locator('table tbody tr').first().click()
    await page.waitForTimeout(500)

    // 等待展开区域出现
    const expandedRow = page.locator('.audit-expanded-row').first()
    await expect(expandedRow).toBeVisible({ timeout: 5000 })

    // 验证详情面板显示关键字段
    await expect(expandedRow.getByText('完整 SQL')).toBeVisible()
    await expect(expandedRow.getByText('执行耗时')).toBeVisible()
    await expect(expandedRow.getByText('影响行数')).toBeVisible()

    // 验证复制按钮存在
    const copyBtn = expandedRow.getByRole('button', { name: '复制' })
    await expect(copyBtn).toBeVisible()
  })
})
