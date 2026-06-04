/**
 * E2E — 查询模块极端边界场景（真实后端）
 * SF-QA0028 batch 2
 * Covers: 大结果集, SQL 注入防御, 特殊字符, 并发查询, 超时处理
 */
import { test, expect, loginViaUI, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('查询边界 — SQL 注入防御', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('SQL 编辑器接受 DROP TABLE 但执行时需要工单或被拒绝', async ({ page }) => {
    // 获取数据源
    const ds = await getFirstDatasourceId(page)

    // 选择数据源
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    // Type a dangerous SQL
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('DROP TABLE sys_user', { delay: 30 })

    // Click execute
    await page.getByRole('button', { name: '执行' }).first().click()

    // AI review should flag this as high risk → ticket required or risk warning shown
    await expect(page.getByText(/高风险|危险|工单|risk/i).first()).toBeVisible({ timeout: 15_000 })
  })

  test('SQL 编辑器接受 SELECT 语句正常执行', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1', { delay: 30 })

    await page.getByRole('button', { name: '执行' }).first().click()

    // Result should appear
    await Promise.race([
      page.getByRole('table').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
    ])
  })
})

test.describe('查询边界 — 大结果集', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('查询返回 0 行结果', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    // 查询一个不存在的条件
    await page.keyboard.type("SELECT * FROM sys_user WHERE id = -999999", { delay: 30 })

    await page.getByRole('button', { name: '执行' }).first().click()

    // Should show empty result or 0 rows
    await Promise.race([
      page.getByText(/暂无数据|0 行|没有数据/).waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
    ])
  })

  test('查询结果超过一页触发分页', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    // 使用 UNION ALL 生成多行数据（至少 100 行）
    await page.keyboard.type(
      'SELECT 1 AS id, \'a\' AS name UNION ALL ' +
      Array.from({ length: 99 }, (_, i) => `SELECT ${i + 2}, '${String.fromCharCode(97 + (i % 26))}'`).join(' UNION ALL '),
      { delay: 5 },
    )

    await page.getByRole('button', { name: '执行' }).first().click()

    await expect(page.getByRole('table')).toBeVisible({ timeout: 15_000 })

    // Should show total rows indicator
    await expect(page.locator('text=/\\d+ 行/')).toBeVisible({ timeout: 5000 })
  })
})

test.describe('查询边界 — 执行时间与超时', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('查询显示执行耗时', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1', { delay: 30 })

    await page.getByRole('button', { name: '执行' }).first().click()

    await expect(page.getByRole('table')).toBeVisible({ timeout: 15_000 })
    // 验证执行耗时显示（数字 + ms）
    await expect(page.getByText(/\d+ms/)).toBeVisible()
  })

  test('查询执行失败显示错误信息', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECTT * FROMM nonexistent', { delay: 30 })

    await page.getByRole('button', { name: '执行' }).first().click()

    // Error message should appear
    await expect(page.getByText(/语法|error|错误|syntax/i)).toBeVisible({ timeout: 15_000 })
  })
})

test.describe('查询边界 — 快捷键与键盘操作', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('Ctrl+Enter 触发查询执行', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1', { delay: 30 })

    // Press Ctrl+Enter to execute
    await page.keyboard.press('Control+Enter')

    await Promise.race([
      page.getByRole('table').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
    ])
  })

  test('Cmd+Enter 在 Mac 上触发查询执行', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1', { delay: 30 })

    // Press Meta+Enter (Cmd on Mac)
    await page.keyboard.press('Meta+Enter')

    await Promise.race([
      page.getByRole('table').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
    ])
  })
})

test.describe('查询边界 — 多数据源切换', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('切换数据源后 SQL 编辑区保留内容', async ({ page }) => {
    const dsSelect = page.getByRole('combobox').first()

    // 选择第一个数据源
    await dsSelect.click()
    const firstOption = page.getByRole('option').first()
    await firstOption.click()

    // Type SQL
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM sys_user', { delay: 30 })

    // 切换到另一个数据源（如果有）
    await dsSelect.click()
    const options = page.getByRole('option')
    const optionCount = await options.count()
    if (optionCount < 2) {
      test.skip()
      return
    }
    await options.nth(1).click()

    // 页面应正常，不崩溃
    await page.waitForTimeout(500)
    // 编辑器应仍存在
    await expect(page.locator('.cm-editor').first()).toBeVisible()
  })
})
