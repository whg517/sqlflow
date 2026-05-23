import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_QUERY_RESULT, MOCK_DATASOURCES } from '../../e2e/helpers'

test.describe('查询编辑器', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
  })

  test('查询页工具栏正确渲染', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 验证数据源选择器
    await expect(page.getByRole('combobox').first()).toBeVisible()

    // 验证数据库名输入框
    await expect(page.getByPlaceholder(/数据库名/)).toBeVisible()

    // 验证历史按钮
    await expect(page.getByRole('button', { name: '历史' })).toBeVisible()
  })

  test('数据源下拉选项正确展示', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 打开数据源选择下拉
    await page.getByRole('combobox').first().click()

    // 验证 mock 数据源选项
    await expect(page.getByRole('option', { name: /test-mysql/ })).toBeVisible()
    await expect(page.getByRole('option', { name: /test-mongo/ })).toBeVisible()
  })

  test('SQL 编辑器 CodeMirror 实例正确渲染', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // CodeMirror 渲染后会有 .cm-editor 容器
    const editor = page.locator('.cm-editor')
    await expect(editor).toBeVisible()

    // 验证行号显示
    await expect(page.locator('.cm-gutters')).toBeVisible()
    await expect(page.locator('.cm-lineNumbers')).toBeVisible()
  })

  test('在编辑器中输入 SQL 语句', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const editor = page.locator('.cm-editor')
    await expect(editor).toBeVisible()

    // 点击编辑器聚焦
    await editor.click()

    // 输入 SQL
    await page.keyboard.type('SELECT * FROM users LIMIT 10')

    // 验证输入内容在 CodeMirror 中
    const content = await page.locator('.cm-content').innerText()
    expect(content).toContain('SELECT * FROM users LIMIT 10')
  })

  test('执行简单 SQL 查询', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const editor = page.locator('.cm-editor')
    await expect(editor).toBeVisible()

    // 输入 SQL
    await editor.click()
    await page.keyboard.type('SELECT id, name, email FROM users')

    // 点击执行按钮
    await page.getByRole('button', { name: '执行' }).click()

    // 等待 AI review 流式响应完成
    // mock 返回 low risk → decision: 'execute' → 自动执行
    await expect(page.getByText('15ms')).toBeVisible({ timeout: 10000 })

    // 验证查询结果
    await expect(page.getByText('2 行')).toBeVisible()
  })

  test('执行查询后结果表格正确展示', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const editor = page.locator('.cm-editor')
    await editor.click()
    await page.keyboard.type('SELECT 1')

    // 执行
    await page.getByRole('button', { name: '执行' }).click()

    // 等待结果
    await expect(page.getByText('15ms')).toBeVisible({ timeout: 10000 })

    // 验证结果表头（mock 数据 columns: id, name, email）
    await expect(page.getByRole('columnheader', { name: 'id' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'name' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'email' })).toBeVisible()

    // 验证数据行
    await expect(page.getByText('Alice')).toBeVisible()
    await expect(page.getByText('Bob')).toBeVisible()
  })

  test('查询结果脱敏标记显示', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const editor = page.locator('.cm-editor')
    await editor.click()
    await page.keyboard.type('SELECT * FROM users')

    await page.getByRole('button', { name: '执行' }).click()

    // mock 返回 desensitized: true, desensitized_fields: ['email']
    await expect(page.getByText('已脱敏 1 字段')).toBeVisible({ timeout: 10000 })
  })

  test('执行按钮状态提示：Ctrl+Enter 执行', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 未输入 SQL 时应显示快捷键提示
    await expect(page.getByText('Ctrl+Enter 执行')).toBeVisible()
  })

  test('空 SQL 时执行按钮禁用', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 未输入 SQL，执行按钮应禁用（editor 默认空，按钮 disabled）
    // 但在有 datasource 选择后，空 SQL 时执行按钮仍 disabled
    const execBtn = page.getByRole('button', { name: '执行' })
    // 注意：mock 会自动选中第一个 datasource，所以按钮可能不是 disabled
    // 如果有 datasource 但没有 SQL，按钮应该是 disabled
    // 不过 CodeMirror 初始化时可能有空字符串，需确认 canExecute 逻辑
    // canExecute = !executing && !!sql.trim() && !!datasourceId
    // 空字符串 trim 后为 falsy → disabled
    await expect(execBtn).toBeDisabled()
  })

  test('输入 SQL 后执行按钮可用', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const editor = page.locator('.cm-editor')
    await editor.click()
    await page.keyboard.type('SELECT 1')

    const execBtn = page.getByRole('button', { name: '执行' })
    await expect(execBtn).toBeEnabled()
  })

  test('查询历史面板打开和关闭', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 点击历史按钮
    await page.getByRole('button', { name: '历史' }).click()

    // 历史面板应可见
    await expect(page.locator('[data-history-panel]')).toBeVisible()

    // 再次点击关闭
    await page.getByRole('button', { name: '历史' }).click()
  })

  test('查询编辑器多 Tab 支持', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 验证默认 Tab 存在（Tab 1）
    await expect(page.getByRole('tab', { name: /查询/ })).toBeVisible()
  })

  test('执行查询后状态栏显示执行时间和行数', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const editor = page.locator('.cm-editor')
    await editor.click()
    await page.keyboard.type('SELECT * FROM users')

    await page.getByRole('button', { name: '执行' }).click()

    // 验证状态栏统计信息（mock 返回 execution_time_ms: 15, total: 2）
    await expect(page.getByText('15ms')).toBeVisible({ timeout: 10000 })
    await expect(page.getByText('2 行')).toBeVisible()
  })
})
