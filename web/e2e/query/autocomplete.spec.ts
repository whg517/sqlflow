import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_DATASOURCES } from '../helpers'

// --- Autocomplete Mock Data ---

const TABLE_SCHEMA = {
  tables: [
    {
      name: 'users',
      columns: [
        { name: 'id', type: 'int', comment: '主键' },
        { name: 'name', type: 'varchar(100)', comment: '用户名' },
        { name: 'email', type: 'varchar(200)', comment: '邮箱' },
        { name: 'status', type: 'varchar(20)', comment: '状态' },
        { name: 'created_at', type: 'datetime', comment: '创建时间' },
        { name: 'updated_at', type: 'datetime', comment: '更新时间' },
      ],
    },
    {
      name: 'orders',
      columns: [
        { name: 'id', type: 'int', comment: '主键' },
        { name: 'user_id', type: 'int', comment: '用户ID' },
        { name: 'total', type: 'decimal(10,2)', comment: '订单金额' },
        { name: 'status', type: 'varchar(20)', comment: '订单状态' },
        { name: 'created_at', type: 'datetime', comment: '创建时间' },
      ],
    },
  ],
}

const SQL_KEYWORDS = ['SELECT', 'FROM', 'WHERE', 'AND', 'OR', 'INSERT', 'UPDATE', 'DELETE', 'JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'INNER JOIN', 'GROUP BY', 'ORDER BY', 'HAVING', 'LIMIT', 'OFFSET', 'UNION', 'DISTINCT', 'AS', 'ON', 'SET', 'VALUES', 'INTO', 'ALTER', 'DROP', 'CREATE', 'TABLE', 'INDEX']

function mockAutocompleteApis(page: import('@playwright/test').Page) {
  // Mock table schema endpoint for autocomplete
  page.route('**/api/datasources/1/columns', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: TABLE_SCHEMA }),
    })
  })

  // Mock single table columns
  page.route(/\/api\/datasources\/1\/tables\/(\w+)\/columns/, async (route) => {
    const url = route.request().url()
    const tableNameMatch = url.match(/\/tables\/(\w+)\/columns/)
    const tableName = tableNameMatch?.[1] ?? 'users'

    const table = TABLE_SCHEMA.tables.find((t) => t.name === tableName)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 0,
        data: table ? table.columns : [],
      }),
    })
  })

  // Mock keyword autocomplete
  page.route('**/api/query/autocomplete', async (route) => {
    const url = route.request().url()
    const urlObj = new URL(url)
    const type = urlObj.searchParams.get('type') ?? 'keyword'
    const prefix = urlObj.searchParams.get('prefix') ?? ''

    let suggestions: { label: string; type: string }[] = []

    if (type === 'keyword') {
      suggestions = SQL_KEYWORDS
        .filter((kw) => kw.toLowerCase().startsWith(prefix.toLowerCase()))
        .map((kw) => ({ label: kw, type: 'keyword' }))
    } else if (type === 'table') {
      suggestions = TABLE_SCHEMA.tables
        .filter((t) => t.name.toLowerCase().startsWith(prefix.toLowerCase()))
        .map((t) => ({ label: t.name, type: 'table' }))
    } else if (type === 'column') {
      // For column suggestions, return all columns from all tables
      const allColumns = TABLE_SCHEMA.tables.flatMap((t) =>
        t.columns.map((c) => ({ label: `${c.name}`, type: 'column' })),
      )
      suggestions = allColumns.filter((c) => c.label.toLowerCase().startsWith(prefix.toLowerCase()))
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: suggestions }),
    })
  })
}

test.describe('SQL 自动补全触发', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockAutocompleteApis(page)
  })

  test('输入 SELECT 触发关键字补全', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 选择数据源
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    // 点击编辑器
    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入 "SEL" 触发补全
    await page.keyboard.type('SEL', { delay: 100 })

    // 验证补全菜单出现，包含 SELECT
    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    await expect(autocompleteMenu).toBeVisible({ timeout: 3000 })

    // 验证 SELECT 选项存在
    await expect(autocompleteMenu.getByText('SELECT')).toBeVisible()
  })

  test('输入表名后输入 "." 触发字段名补全', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入表名 + "."
    await page.keyboard.type('users.', { delay: 50 })

    // 验证补全菜单出现，包含 users 表的字段
    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    await expect(autocompleteMenu).toBeVisible({ timeout: 3000 })

    // 验证字段名补全项
    await expect(autocompleteMenu.getByText('id')).toBeVisible()
    await expect(autocompleteMenu.getByText('name')).toBeVisible()
    await expect(autocompleteMenu.getByText('email')).toBeVisible()
    await expect(autocompleteMenu.getByText('status')).toBeVisible()
    await expect(autocompleteMenu.getByText('created_at')).toBeVisible()
  })

  test('Ctrl+Space 手动触发补全', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入部分关键字但不触发自动补全
    await page.keyboard.type('WHE', { delay: 50 })

    // 手动触发补全
    await page.keyboard.press('Control+Space')

    // 验证补全菜单出现
    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    await expect(autocompleteMenu).toBeVisible({ timeout: 3000 })

    // 验证 WHERE 选项
    await expect(autocompleteMenu.getByText('WHERE')).toBeVisible()
  })

  test('上下键选择补全项', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入 "SE" 触发补全（SELECT, SET）
    await page.keyboard.type('SE', { delay: 100 })

    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    await expect(autocompleteMenu).toBeVisible({ timeout: 3000 })

    // 默认选中第一项（SELECT）
    await expect(autocompleteMenu.locator('[aria-selected="true"]').first()).toContainText('SELECT')

    // 按下键选择第二项
    await page.keyboard.press('ArrowDown')
    await expect(autocompleteMenu.locator('[aria-selected="true"]').first()).toContainText('SET')

    // 按上键回到第一项
    await page.keyboard.press('ArrowUp')
    await expect(autocompleteMenu.locator('[aria-selected="true"]').first()).toContainText('SELECT')
  })

  test('Enter 确认补全', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入 "SEL"
    await page.keyboard.type('SEL', { delay: 100 })

    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    await expect(autocompleteMenu).toBeVisible({ timeout: 3000 })

    // 按 Enter 确认补全
    await page.keyboard.press('Enter')

    // 验证补全已应用（编辑器中应该有 SELECT）
    await expect(editor).toContainText('SELECT')

    // 补全菜单应关闭
    await expect(autocompleteMenu).not.toBeVisible()
  })

  test('Escape 关闭补全菜单', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 触发补全
    await page.keyboard.type('SEL', { delay: 100 })

    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    await expect(autocompleteMenu).toBeVisible({ timeout: 3000 })

    // 按 Escape 关闭
    await page.keyboard.press('Escape')

    // 补全菜单应关闭
    await expect(autocompleteMenu).not.toBeVisible()

    // 编辑器中应保留原始输入
    await expect(editor).toContainText('SEL')
  })

  test('空输入不触发补全', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 点击编辑器但不输入
    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    await expect(autocompleteMenu).not.toBeVisible()
  })
})
