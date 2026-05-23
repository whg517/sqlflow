import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_DATASOURCES } from '../helpers'

// --- MongoDB Mock Data ---

const MONGO_EXECUTE_RESULT = {
  code: 0,
  message: 'ok',
  data: {
    columns: ['_id', 'name', 'email', 'age', 'status', 'created_at'],
    rows: [
      {
        _id: '60d5ec49f1b2c3a1b8e4a123',
        name: 'Alice Chen',
        email: 'ali***@example.com',
        age: 28,
        status: 'active',
        created_at: '2026-01-15T08:00:00.000Z',
      },
      {
        _id: '60d5ec49f1b2c3a1b8e4a124',
        name: 'Bob Li',
        email: 'b**@example.com',
        age: 35,
        status: 'inactive',
        created_at: '2026-02-20T10:30:00.000Z',
      },
      {
        _id: '60d5ec49f1b2c3a1b8e4a125',
        name: 'Carol Wang',
        email: 'carol***@example.com',
        age: 42,
        status: 'active',
        created_at: '2026-03-10T14:00:00.000Z',
      },
    ],
    total: 3,
    execution_time_ms: 8,
    affected_rows: 0,
    desensitized: true,
    desensitized_fields: ['email'],
    warnings: [],
  },
}

const MONGO_COLLECTIONS = ['users', 'orders', 'products', 'events', 'analytics']

function mockMongoApis(page: import('@playwright/test').Page) {
  // Override execute to return MongoDB result
  page.route('**/api/query/execute', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MONGO_EXECUTE_RESULT),
    })
  })

  // Override tables endpoint to return MongoDB collections
  page.route('**/api/datasources/*/tables', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: MONGO_COLLECTIONS }),
    })
  })

  // Override query review to auto-execute (low risk)
  page.route('**/api/query/review', async (route) => {
    const body =
      'event: content\ndata: "Analyzing query..."\n\n' +
      'event: result\ndata: ' +
      JSON.stringify({
        risk_level: 'low',
        risk_score: 5,
        decision: 'execute',
        summary: 'Low risk find query',
        suggestions: [],
        impact_analysis: 'Read-only query',
        rollback_sql: '',
        warnings: [],
        review_source: 'ai',
        reviewed_at: new Date().toISOString(),
        expires_at: new Date(Date.now() + 30000).toISOString(),
        model_used: 'gpt-4',
      }) +
      '\n\n'
    await route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body,
    })
  })
}

test.describe('MongoDB 编辑器查询', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockMongoApis(page)
  })

  test('切换到 MongoDB 数据源后编辑器切换为 MongoDB 模式', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 选择 MySQL 数据源先验证 SQL 编辑器
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    // 验证 SQL 编辑器可见
    await expect(page.locator('.cm-content').first()).toBeVisible()

    // 切换到 MongoDB 数据源
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mongo/ }).click()

    // 验证 MongoDB 标识
    await expect(page.getByText('MongoDB')).toBeVisible()

    // 验证集合名输入框出现
    await expect(page.getByPlaceholder('集合名 (collection)')).toBeVisible()

    // 验证 MongoDB 操作选择器
    await expect(page.getByRole('combobox').filter({ hasText: 'find' })).toBeVisible()

    // 验证 SQL 编辑器的集合名输入不存在于 MySQL 模式
    // (这个验证通过"切换回 MySQL"测试确认，见下方)
  })

  test('输入 MongoDB 查询参数', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 直接选择 MongoDB 数据源
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mongo/ }).click()

    // 输入集合名
    const collectionInput = page.getByPlaceholder('集合名 (collection)')
    await expect(collectionInput).toBeVisible()
    await collectionInput.fill('users')

    // 在 Filter 编辑器中输入查询条件
    const filterEditor = page.locator('.cm-content').first()
    await filterEditor.click()
    await page.keyboard.type('{ status: "active" }', { delay: 20 })

    // 验证编辑器包含输入的查询
    await expect(filterEditor).toContainText('status')
    await expect(filterEditor).toContainText('active')
  })

  test('执行 MongoDB 查询并验证结果', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 选择 MongoDB 数据源
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mongo/ }).click()

    // 填写查询参数
    await page.getByPlaceholder('集合名 (collection)').fill('users')

    const filterEditor = page.locator('.cm-content').first()
    await filterEditor.click()
    await page.keyboard.type('{}', { delay: 20 })

    // 执行查询
    const executeBtn = page.getByRole('button', { name: '执行' })
    await expect(executeBtn).toBeEnabled()
    await executeBtn.click()

    // 等待结果
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 验证 MongoDB 特有的 _id 列
    await expect(page.getByRole('columnheader', { name: '_id' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'name' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'email' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'age' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'status' })).toBeVisible()

    // 验证数据内容
    await expect(page.getByText('60d5ec49f1b2c3a1b8e4a123')).toBeVisible()
    await expect(page.getByText('Alice Chen')).toBeVisible()
    await expect(page.getByText('Bob Li')).toBeVisible()
    await expect(page.getByText('Carol Wang')).toBeVisible()

    // 验证行数
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows).toHaveCount(3)

    // 验证执行耗时
    await expect(page.getByText('8ms')).toBeVisible()

    // 验证总行数
    await expect(page.getByText('3 行')).toBeVisible()
  })

  test('MongoDB 结果脱敏标识', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mongo/ }).click()

    await page.getByPlaceholder('集合名 (collection)').fill('users')
    const filterEditor = page.locator('.cm-content').first()
    await filterEditor.click()
    await page.keyboard.type('{}', { delay: 20 })

    await page.getByRole('button', { name: '执行' }).click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 验证脱敏信息
    await expect(page.getByText('已脱敏')).toBeVisible()
    // 验证 email 列数据被脱敏
    await expect(page.getByText('ali***@example.com')).toBeVisible()
  })

  test('MongoDB 操作类型切换', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mongo/ }).click()

    // 验证默认操作类型为 find
    const operationSelect = page.getByRole('combobox').filter({ hasText: 'find' })
    await expect(operationSelect).toBeVisible()

    // 切换操作类型为 aggregate
    await operationSelect.click()
    await page.getByRole('option', { name: 'aggregate' }).click()

    // 验证操作类型已切换
    await expect(page.getByRole('combobox').filter({ hasText: 'aggregate' })).toBeVisible()

    // 切换为 count
    await operationSelect.click()
    await page.getByRole('option', { name: 'count' }).click()
    await expect(page.getByRole('combobox').filter({ hasText: 'count' })).toBeVisible()
  })

  test('从 MongoDB 切换回 MySQL 编辑器', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()

    // 先选 MongoDB
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mongo/ }).click()
    await expect(page.getByText('MongoDB')).toBeVisible()
    await expect(page.getByPlaceholder('集合名 (collection)')).toBeVisible()

    // 切换回 MySQL
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    // MongoDB 标识消失
    await expect(page.getByText('MongoDB')).not.toBeVisible()
    // 集合名输入消失
    await expect(page.getByPlaceholder('集合名 (collection)')).not.toBeVisible()

    // SQL 编辑器存在
    await expect(page.locator('.cm-content').first()).toBeVisible()
  })
})
