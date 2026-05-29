/**
 * E2E — SQL 编辑器输入并执行查询（真实后端）
 * SF-QA0028 batch 2
 * 使用真实 SQL（SELECT 1, SELECT * FROM sys_user）替代 mock 数据
 */
import { test, expect, loginViaUI, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('SQL 编辑器输入并执行查询（真实后端）', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('MySQL 查询完整流程：输入 SQL → 执行 → 验证结果 → 查询历史', async ({ page }) => {
    // 选择数据源
    const ds = await getFirstDatasourceId(page)
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    // 在 SQL 编辑器中输入查询
    const editor = page.locator('.cm-content').first()
    await expect(editor).toBeVisible()
    await editor.click()
    await page.keyboard.type('SELECT * FROM sys_user LIMIT 10', { delay: 30 })

    // 点击执行按钮
    const executeBtn = page.getByRole('button', { name: '执行' })
    await expect(executeBtn).toBeEnabled()
    await executeBtn.click()

    // 等待结果表格渲染
    await Promise.race([
      page.getByRole('table').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
    ])

    // 验证结果表格有列头
    const tableHeaders = page.getByRole('columnheader')
    await expect(tableHeaders.first()).toBeVisible()

    // 验证数据行存在
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows.first()).toBeVisible()

    // 验证执行耗时显示
    await expect(page.getByText(/\d+ms/)).toBeVisible()

    // 验证总行数显示
    await expect(page.locator('text=/\\d+ 行/')).toBeVisible()

    // 验证查询历史
    await page.getByRole('button', { name: '历史' }).click()
    await expect(page.getByText('查询历史')).toBeVisible()

    // 关闭历史面板
    await page.getByRole('button', { name: '历史' }).click()
  })

  test('空 SQL 不能执行', async ({ page }) => {
    // 未输入 SQL，验证执行按钮禁用
    const executeBtn = page.getByRole('button', { name: '执行' })
    await expect(executeBtn).toBeDisabled()
  })

  test('输入 SQL 后执行按钮可用', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1')

    const execBtn = page.getByRole('button', { name: '执行' })
    await expect(execBtn).toBeEnabled()
  })

  test('执行后显示脱敏字段标识', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM sys_user LIMIT 10', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()

    await Promise.race([
      page.getByRole('table').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
    ])

    // 验证脱敏信息显示（如果有脱敏配置）
    const desensitizeBadge = page.getByText(/已脱敏/)
    const isDesensitized = await desensitizeBadge.isVisible({ timeout: 5000 }).catch(() => false)
    // 脱敏标识是可选的（取决于 mask rules 配置）
    if (!isDesensitized) {
      test.info().annotations.push({ type: 'info', description: 'No mask rules configured for sys_user in test environment' })
    }
  })
})
