/**
 * E2E — 查询编辑器 UI 基础功能（真实后端）
 * SF-QA0028 batch 2
 */
import { test, expect, loginViaUI, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('查询编辑器（真实后端）', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('查询页工具栏正确渲染', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    // 验证数据源选择器
    await expect(page.getByRole('combobox').first()).toBeVisible()

    // 验证数据库名输入框
    await expect(page.getByPlaceholder(/数据库名/)).toBeVisible()

    // 验证历史按钮
    await expect(page.getByRole('button', { name: '历史' })).toBeVisible()
  })

  test('数据源下拉选项正确展示', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    // 打开数据源选择下拉
    await page.getByRole('combobox').first().click()

    // 验证至少有一个数据源选项
    const options = page.getByRole('option')
    await expect(options.first()).toBeVisible({ timeout: 5000 })
  })

  test('SQL 编辑器 CodeMirror 实例正确渲染', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    // CodeMirror 渲染后会有 .cm-editor 容器
    const editor = page.locator('.cm-editor')
    await expect(editor).toBeVisible()

    // 验证行号显示
    await expect(page.locator('.cm-gutters')).toBeVisible()
    await expect(page.locator('.cm-lineNumbers')).toBeVisible()
  })

  test('在编辑器中输入 SQL 语句', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    const editor = page.locator('.cm-editor')
    await expect(editor).toBeVisible()

    // 点击编辑器聚焦
    await editor.click()

    // 输入 SQL
    await page.keyboard.type('SELECT * FROM sys_user LIMIT 10')

    // 验证输入内容在 CodeMirror 中
    const content = await page.locator('.cm-content').innerText()
    expect(content).toContain('SELECT * FROM sys_user LIMIT 10')
  })

  test('执行简单 SQL 查询', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    // 选择数据源
    const ds = await getFirstDatasourceId(page)
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-editor')
    await expect(editor).toBeVisible()

    // 输入 SQL
    await editor.click()
    await page.keyboard.type('SELECT 1')

    // 点击执行按钮
    await page.getByRole('button', { name: '执行' }).click()

    // 等待结果
    await Promise.race([
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ 行/').waitFor({ timeout: 15_000 }),
    ])

    // 验证查询结果（至少有耗时显示）
    await expect(page.getByText(/\d+ms/)).toBeVisible()
  })

  test('执行查询后结果表格正确展示', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-editor')
    await editor.click()
    await page.keyboard.type('SELECT 1 AS col_a, 2 AS col_b')

    // 执行
    await page.getByRole('button', { name: '执行' }).click()

    await Promise.race([
      page.locator('text=/\\d+ms/').waitFor({ timeout: 15_000 }),
      page.locator('text=/\\d+ 行/').waitFor({ timeout: 15_000 }),
    ])

    // 验证结果表头
    await expect(page.getByRole('columnheader', { name: 'col_a' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'col_b' })).toBeVisible()
  })

  test('执行按钮状态提示：Ctrl+Enter 执行', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    // 未输入 SQL 时应显示快捷键提示
    await expect(page.getByText('Ctrl+Enter 执行')).toBeVisible()
  })

  test('空 SQL 时执行按钮禁用', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    const execBtn = page.getByRole('button', { name: '执行' })
    await expect(execBtn).toBeDisabled()
  })

  test('输入 SQL 后执行按钮可用', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    const editor = page.locator('.cm-editor')
    await editor.click()
    await page.keyboard.type('SELECT 1')

    const execBtn = page.getByRole('button', { name: '执行' })
    await expect(execBtn).toBeEnabled()
  })

  test('查询历史面板打开和关闭', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    // 点击历史按钮
    await page.getByRole('button', { name: '历史' }).click()

    // 历史面板应可见
    await expect(page.locator('[data-history-panel]')).toBeVisible()

    // 再次点击关闭
    await page.getByRole('button', { name: '历史' }).click()
  })

  test('查询编辑器多 Tab 支持', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    // 验证默认 Tab 存在
    await expect(page.getByRole('tab', { name: /查询/ })).toBeVisible()
  })

  test('执行查询后状态栏显示执行时间和行数', async ({ page }) => {
    const ds = await getFirstDatasourceId(page)
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    const editor = page.locator('.cm-editor')
    await editor.click()
    await page.keyboard.type('SELECT * FROM sys_user LIMIT 5')

    await page.getByRole('button', { name: '执行' }).click()

    // 验证状态栏统计信息
    await expect(page.getByText(/\d+ms/)).toBeVisible({ timeout: 15_000 })
    await expect(page.locator('text=/\\d+ 行/')).toBeVisible()
  })
})
