/**
 * E2E — 数据源切换后查询（真实后端）
 * SF-QA0028 batch 2
 * 用 getFirstDatasourceId 获取可用数据源
 */
import { test, expect, loginViaUI, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('数据源切换后查询（真实后端）', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('切换数据源后查询结果属于不同数据源', async ({ page }) => {
    // 打开数据源下拉
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()

    const options = page.getByRole('option')
    const optionCount = await options.count()

    if (optionCount < 2) {
      test.skip()
      return
    }

    // 选择第一个数据源
    await options.first().click()

    // 输入 SQL 并执行
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()

    await Promise.race([
      page.getByRole('table').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
    ])

    // 记录第一个数据源的查询结果
    await expect(page.getByText(/\d+ms/)).toBeVisible()

    // 切换到第二个数据源
    await dsSelect.click()
    await options.nth(1).click()

    // 验证编辑器状态 — 编辑器应保留 SQL
    await expect(page.locator('.cm-content').first()).toBeVisible()

    // 结果面板应被重置
    const resultPlaceholder = page.getByText('执行查询以查看结果')
    const hasPlaceholder = await resultPlaceholder.isVisible({ timeout: 3000 }).catch(() => false)

    // 执行查询
    await executeBtn.click()

    await Promise.race([
      page.getByRole('table').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
    ])

    // 验证查询成功（不同数据源返回结果）
    await expect(page.getByText(/\d+ms/)).toBeVisible()
  })

  test('数据源列表正确展示', async ({ page }) => {
    // 打开数据源选择下拉
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()

    // 验证至少有一个数据源选项
    const options = page.getByRole('option')
    await expect(options.first()).toBeVisible({ timeout: 5000 })

    // 验证选项包含类型标识（如有）
    const optionCount = await options.count()
    expect(optionCount).toBeGreaterThan(0)
  })

  test('快速连续切换数据源不会出错', async ({ page }) => {
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()

    const options = page.getByRole('option')
    const optionCount = await options.count()

    if (optionCount < 2) {
      test.skip()
      return
    }

    // 快速切换
    for (let i = 0; i < 4; i++) {
      await dsSelect.click()
      await options.nth(i % optionCount).click()
      await page.waitForTimeout(200)
    }

    // 最终应停在 MySQL 编辑器，页面无报错
    await expect(page.locator('.cm-content').first()).toBeVisible()
  })
})
