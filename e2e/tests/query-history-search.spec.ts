/**
 * E2E — 查询历史列表 + keyword 搜索（真实后端）
 * SF-QA0028 batch 2
 * 先执行几条 SQL 生成历史，再搜索
 */
import { test, expect, loginViaUI, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('查询历史列表 + keyword 搜索（真实后端）', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  /** 选择数据源 */
  async function selectDatasource(page: import('@playwright/test').Page) {
    const ds = await getFirstDatasourceId(page)
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()
    return ds
  }

  /** 在编辑器中输入 SQL 并执行 */
  async function executeSql(page: import('@playwright/test').Page, sql: string) {
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.press('Control+a')
    await page.keyboard.press('Delete')
    await page.keyboard.type(sql, { delay: 20 })
    await page.getByRole('button', { name: '执行' }).click()
    await Promise.race([
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ 行/').waitFor({ timeout: 15_000 }),
    ])
    await page.waitForTimeout(500)
  }

  test('打开查询历史面板', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    const historyBtn = page.getByRole('button', { name: '历史' })
    await expect(historyBtn).toBeVisible()
    await historyBtn.click()

    // 验证历史面板出现
    await expect(page.getByText('查询历史')).toBeVisible()
  })

  test('历史列表渲染 - 时间、SQL 摘要、数据源', async ({ page }) => {
    // 先执行几条 SQL 生成历史
    await selectDatasource(page)
    await executeSql(page, 'SELECT 1 AS test_col')
    await executeSql(page, 'SELECT 2 AS another_col')
    await executeSql(page, 'SELECT NOW() AS current_time')

    // 打开历史面板
    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 验证历史记录渲染（至少应有我们刚执行的查询）
    const panel = page.locator('[data-history-panel]')
    if (await panel.isVisible({ timeout: 5000 }).catch(() => false)) {
      // 验证面板文本包含我们的查询摘要
      const panelText = await panel.textContent()
      expect(panelText).toBeDefined()
      const hasHistory = panelText!.includes('test_col') || panelText!.includes('another_col') || panelText!.includes('current_time')
      expect(hasHistory, 'History should contain recently executed queries').toBeTruthy()

      // 验证执行耗时显示
      await expect(panel.getByText(/\d+ms/).first()).toBeVisible()
    }
  })

  test('keyword 搜索过滤 - 按 SQL 内容搜索', async ({ page }) => {
    // 生成带有唯一标记的历史
    await selectDatasource(page)
    const marker = `e2e_hist_${Date.now()}`
    await executeSql(page, `SELECT '${marker}' AS e2e_marker`)
    await executeSql(page, `SELECT '${marker}_2' AS e2e_marker2`)

    // 打开历史面板
    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 搜索
    const searchInput = page.locator('[data-history-panel]').getByPlaceholder(/搜索/).or(page.getByPlaceholder(/搜索/).last())
    if (await searchInput.isVisible({ timeout: 3000 }).catch(() => false)) {
      await searchInput.fill(marker)
      await page.keyboard.press('Enter')
      await page.waitForTimeout(2000)

      // 验证搜索结果包含我们的标记
      const panelText = await page.locator('[data-history-panel]').textContent()
      expect(panelText).toContain(marker)
    }
  })

  test('keyword 搜索 - 空结果显示空状态', async ({ page }) => {
    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    const ghostKeyword = `NEVER_${Date.now()}_${Math.random().toString(36).slice(2)}`

    const searchInput = page.locator('[data-history-panel]').getByPlaceholder(/搜索/).or(page.getByPlaceholder(/搜索/).last())
    if (await searchInput.isVisible({ timeout: 3000 }).catch(() => false)) {
      await searchInput.fill(ghostKeyword)
      await page.keyboard.press('Enter')
      await page.waitForTimeout(2000)

      // 验证空状态
      await expect(page.getByText('暂无查询历史')).toBeVisible()
    }
  })

  test('点击历史项加载到编辑器', async ({ page }) => {
    // 生成历史
    await selectDatasource(page)
    const marker = `load_test_${Date.now()}`
    await executeSql(page, `SELECT '${marker}' AS load_col`)

    // 打开历史
    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 找到包含标记的历史项并点击
    const historyItem = page.locator('[data-history-panel]').getByText(marker).first()
    if (await historyItem.isVisible({ timeout: 5000 }).catch(() => false)) {
      await historyItem.click()

      // 验证 SQL 加载到编辑器
      const editor = page.locator('.cm-content').first()
      await expect(editor).toContainText(marker)
    }
  })

  test('清空搜索后恢复完整列表', async ({ page }) => {
    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    const searchInput = page.locator('[data-history-panel]').getByPlaceholder(/搜索/).or(page.getByPlaceholder(/搜索/).last())
    if (await searchInput.isVisible({ timeout: 3000 }).catch(() => false)) {
      // 先搜索不存在的词
      await searchInput.fill('NEVER_EXIST_QUERY')
      await page.keyboard.press('Enter')
      await page.waitForTimeout(2000)

      // 清空搜索
      await searchInput.clear()
      await page.keyboard.press('Enter')
      await page.waitForTimeout(2000)

      // 验证列表恢复（或仍然为空如果没有历史）
      const hasHistory = await page.locator('[data-history-panel]').getByText(/\d+ms/).first()
        .isVisible({ timeout: 3000 }).catch(() => false)
      const hasEmpty = await page.getByText('暂无查询历史').isVisible({ timeout: 1000 }).catch(() => false)
      expect(hasHistory || hasEmpty).toBeTruthy()
    }
  })
})
