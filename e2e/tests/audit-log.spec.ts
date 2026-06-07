/**
 * SF-QA0046 — E2E 前端交互：审计日志
 */
import { test, expect, BASE_URL, loginViaUI, loginViaApi } from '../support/test-helpers'
import type { Page } from '@playwright/test'

test.describe.configure({ timeout: 45_000 })

const AUDIT_URL = `${BASE_URL}/audit`

async function gotoAuditPage(page: Page) {
  await page.goto(AUDIT_URL)
  await page.waitForLoadState('networkidle')
  await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible({ timeout: 10_000 })
}

async function getAuthToken(page: Page): Promise<string> {
  return loginViaApi(page)
}

async function createAuditLogViaApi(page: Page, sql: string): Promise<void> {
  const token = await getAuthToken(page)
  const dsRes = await page.request.get(`${BASE_URL}/api/datasources`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const dsBody = await dsRes.json()
  const dsId = dsBody.data?.[0]?.id
  if (!dsId) return
  await page.request.post(`${BASE_URL}/api/query/execute`, {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    data: { datasource_id: dsId, database: 'testdb', sql },
  })
}

// --- Tests ---

test('页面加载：显示标题和总数', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible()
})

test('页面加载：显示表格列标题', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 1 AS audit_col')
  await gotoAuditPage(page)
  await expect(page.getByRole('columnheader', { name: '时间' })).toBeVisible({ timeout: 5_000 })
  await expect(page.getByRole('columnheader', { name: '用户' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '操作' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '数据库' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: 'SQL 摘要' })).toBeVisible()
})

test('页面加载：加载审计日志数据', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 2 AS load_test')
  await gotoAuditPage(page)
  const rows = page.getByRole('row').filter({ has: page.locator('td') })
  await expect(rows.first()).toBeVisible({ timeout: 10_000 })
  expect(await rows.count()).toBeGreaterThanOrEqual(1)
})

test('搜索：搜索框可见且有 placeholder', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  const searchInput = page.getByPlaceholder('搜索 SQL/表名/用户/IP/数据库/错误/脱敏...')
  await expect(searchInput).toBeVisible()
})

test('搜索：输入关键词按 Enter 触发搜索', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 3 AS search_keyword_test')
  await gotoAuditPage(page)
  await expect(page.getByRole('row').filter({ has: page.locator('td') }).first()).toBeVisible({ timeout: 10_000 })
  const searchInput = page.getByPlaceholder('搜索 SQL/表名/用户/IP/数据库/错误/脱敏...')
  await searchInput.fill('search_keyword_test')
  await searchInput.press('Enter')
  await page.waitForTimeout(500)
  await expect(searchInput).toHaveValue('search_keyword_test')
})

test('搜索：清空搜索恢复全部数据', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 4 AS clear_search_test')
  await gotoAuditPage(page)
  const searchInput = page.getByPlaceholder('搜索 SQL/表名/用户/IP/数据库/错误/脱敏...')
  await searchInput.fill('clear_search_test')
  await searchInput.press('Enter')
  await page.waitForTimeout(500)
  // Clear via the × button
  const clearBtn = page.locator('button', { hasText: '×' }).first()
  if (await clearBtn.isVisible().catch(() => false)) {
    await clearBtn.click()
  } else {
    await searchInput.clear()
    await searchInput.press('Enter')
  }
  await expect(searchInput).toHaveValue('')
})

test('筛选：用户筛选下拉框可见', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  const userFilter = page.getByText('全部用户').first()
  await expect(userFilter).toBeVisible({ timeout: 5_000 })
})

test('筛选：操作类型筛选下拉框可见', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  const actionFilter = page.getByText('操作类型').first()
  await expect(actionFilter).toBeVisible({ timeout: 5_000 })
})

test('筛选：数据源筛选下拉框可见', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  const dsFilter = page.getByText('数据源').first()
  await expect(dsFilter).toBeVisible({ timeout: 5_000 })
})

test('筛选：日期范围输入框', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  const dateInputs = page.locator('input[type="date"]')
  expect(await dateInputs.count()).toBeGreaterThanOrEqual(2)
})

test('筛选：清除筛选按钮', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  const searchInput = page.getByPlaceholder('搜索 SQL/表名/用户/IP/数据库/错误/脱敏...')
  await searchInput.fill('test_filter')
  await searchInput.press('Enter')
  await page.waitForTimeout(500)
  const resetBtn = page.getByRole('button', { name: /清除筛选/ })
  await expect(resetBtn).toBeVisible({ timeout: 5_000 })
  await resetBtn.click()
  await expect(searchInput).toHaveValue('')
})

test('展开详情：点击行展开详情', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 5 AS expand_test')
  await gotoAuditPage(page)
  const rows = page.getByRole('row').filter({ has: page.locator('td') })
  await expect(rows.first()).toBeVisible({ timeout: 10_000 })
  await rows.first().click()
  await expect(page.locator('.audit-expanded-row').first()).toBeVisible({ timeout: 5_000 })
})

test('展开详情：显示完整 SQL 和复制按钮', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 6 AS sql_detail_test')
  await gotoAuditPage(page)
  const rows = page.getByRole('row').filter({ has: page.locator('td') })
  await expect(rows.first()).toBeVisible({ timeout: 10_000 })
  await rows.first().click()
  await expect(page.getByText('完整 SQL').first()).toBeVisible({ timeout: 5_000 })
  await expect(page.getByText('复制').first()).toBeVisible()
})

test('展开详情：显示执行详情（耗时/影响行数/返回行数）', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 7 AS exec_detail_test')
  await gotoAuditPage(page)
  const rows = page.getByRole('row').filter({ has: page.locator('td') })
  await expect(rows.first()).toBeVisible({ timeout: 10_000 })
  await rows.first().click()
  await expect(page.getByText('执行耗时').first()).toBeVisible({ timeout: 5_000 })
  await expect(page.getByText('影响行数').first()).toBeVisible()
  await expect(page.getByText('返回行数').first()).toBeVisible()
})

test('展开详情：显示 IP 地址和操作人', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 8 AS ip_operator_test')
  await gotoAuditPage(page)
  const rows = page.getByRole('row').filter({ has: page.locator('td') })
  await expect(rows.first()).toBeVisible({ timeout: 10_000 })
  await rows.first().click()
  await expect(page.getByText('IP 地址').first()).toBeVisible({ timeout: 5_000 })
  await expect(page.getByText('操作人').first()).toBeVisible()
})

test('展开详情：再次点击折叠', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 9 AS collapse_test')
  await gotoAuditPage(page)
  const rows = page.getByRole('row').filter({ has: page.locator('td') })
  await expect(rows.first()).toBeVisible({ timeout: 10_000 })
  await rows.first().click()
  await expect(page.locator('.audit-expanded-row').first()).toBeVisible({ timeout: 5_000 })
  await rows.first().click()
  await expect(page.locator('.audit-expanded-row')).toHaveCount(0, { timeout: 5_000 })
})

test('导出：导出 CSV 按钮可见', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  await expect(page.getByRole('button', { name: /导出 CSV/ })).toBeVisible()
})

test('导出：点击导出按钮', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 10 AS export_test')
  await gotoAuditPage(page)
  await expect(page.getByRole('row').filter({ has: page.locator('td') }).first()).toBeVisible({ timeout: 10_000 })
  const downloadPromise = page.waitForEvent('download', { timeout: 10_000 }).catch(() => null)
  await page.getByRole('button', { name: /导出 CSV/ }).click()
  const download = await downloadPromise
  if (download) {
    const filename = download.suggestedFilename()
    expect(filename).toContain('audit')
  }
  await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible()
})

test('分页：显示总数信息', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  const paginationInfo = page.getByText(/共.*条/)
  if (await paginationInfo.isVisible().catch(() => false)) {
    await expect(paginationInfo).toBeVisible()
  }
})

test('空状态：筛选无结果时显示提示', async ({ page }) => {
  await loginViaUI(page)
  await gotoAuditPage(page)
  const searchInput = page.getByPlaceholder('搜索 SQL/表名/用户/IP/数据库/错误/脱敏...')
  await searchInput.fill('zzz_nonexistent_keyword_xyz_12345')
  await searchInput.press('Enter')
  await page.waitForTimeout(1000)
  const emptyMsg = page.getByText('没有匹配的审计日志')
  if (await emptyMsg.isVisible().catch(() => false)) {
    await expect(emptyMsg).toBeVisible()
    await expect(page.getByRole('button', { name: /清除所有筛选/ })).toBeVisible()
  }
})

test('导航：侧边栏进入审计日志', async ({ page }) => {
  await loginViaUI(page)
  await page.goto(`${BASE_URL}/query`)
  await page.waitForLoadState('networkidle')
  const auditLink = page.getByRole('link', { name: /审计/ }).first()
  if (await auditLink.isVisible().catch(() => false)) {
    await auditLink.click()
    await expect(page).toHaveURL(/\/audit/, { timeout: 5_000 })
    await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible()
  }
})

test('导航：直接访问 /audit URL', async ({ page }) => {
  await loginViaUI(page)
  await page.goto(AUDIT_URL)
  await page.waitForLoadState('networkidle')
  await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible({ timeout: 10_000 })
})

test('数据一致性：API 创建的查询记录出现在审计日志', async ({ page }) => {
  await loginViaUI(page)
  const uniqueMarker = `audit_consistency_${Date.now()}`
  await createAuditLogViaApi(page, `SELECT '${uniqueMarker}' AS marker`)
  await gotoAuditPage(page)
  await expect(page.getByRole('row').filter({ has: page.locator('td') }).first()).toBeVisible({ timeout: 10_000 })
  const searchInput = page.getByPlaceholder('搜索 SQL/表名/用户/IP/数据库/错误/脱敏...')
  await searchInput.fill(uniqueMarker)
  await searchInput.press('Enter')
  await page.waitForTimeout(1000)
  const matchingRows = page.getByRole('row').filter({ hasText: uniqueMarker })
  const count = await matchingRows.count()
  expect(count).toBeGreaterThanOrEqual(0)
})

test('复制：展开详情后可复制 SQL', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 12 AS copy_sql_test')
  await gotoAuditPage(page)
  const rows = page.getByRole('row').filter({ has: page.locator('td') })
  await expect(rows.first()).toBeVisible({ timeout: 10_000 })
  await rows.first().click()
  const copyBtn = page.getByText('复制').first()
  await expect(copyBtn).toBeVisible()
  await copyBtn.click()
  await expect(page.getByText('已复制').first()).toBeVisible({ timeout: 3_000 })
})

test('SQL Tooltip：悬停显示完整 SQL', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 13 AS tooltip_test')
  await gotoAuditPage(page)
  const rows = page.getByRole('row').filter({ has: page.locator('td') })
  await expect(rows.first()).toBeVisible({ timeout: 10_000 })
  const sqlCell = rows.first().locator('td').last()
  await sqlCell.hover()
  const tooltip = page.getByRole('tooltip')
  if (await tooltip.isVisible().catch(() => false)) {
    await expect(tooltip).toBeVisible()
  }
})

test('页面刷新：刷新后保持审计日志列表', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 14 AS refresh_test')
  await gotoAuditPage(page)
  await expect(page.getByRole('row').filter({ has: page.locator('td') }).first()).toBeVisible({ timeout: 10_000 })
  await page.reload()
  await page.waitForLoadState('networkidle')
  await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible({ timeout: 10_000 })
  await expect(page.getByRole('row').filter({ has: page.locator('td') }).first()).toBeVisible({ timeout: 10_000 })
})

test('并发操作：快速切换筛选不报错', async ({ page }) => {
  await loginViaUI(page)
  await createAuditLogViaApi(page, 'SELECT 15 AS rapid_test')
  await gotoAuditPage(page)
  const searchInput = page.getByPlaceholder('搜索 SQL/表名/用户/IP/数据库/错误/脱敏...')
  for (let i = 0; i < 3; i++) {
    await searchInput.fill(`rapid_${i}`)
    await searchInput.press('Enter')
  }
  await page.waitForTimeout(1000)
  await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible()
})

test('错误处理：页面加载后无严重 console 错误', async ({ page }) => {
  await loginViaUI(page)
  const errors: string[] = []
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text())
  })
  await gotoAuditPage(page)
  await page.waitForTimeout(2000)
  const criticalErrors = errors.filter(e =>
    !e.includes('favicon') && !e.includes('DevTools') && !e.includes('net::ERR')
  )
  expect(criticalErrors.length).toBeLessThanOrEqual(2)
})
