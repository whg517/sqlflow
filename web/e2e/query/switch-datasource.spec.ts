import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_DATASOURCES } from '../helpers'

// Extended datasources with 3 sources for switching tests
const EXTENDED_DATASOURCES = [
  { id: 1, name: 'prod-mysql', type: 'mysql', status: 'active' },
  { id: 2, name: 'staging-mysql', type: 'mysql', status: 'active' },
  { id: 3, name: 'prod-mongo', type: 'mongodb', status: 'active' },
]

// Mock results per datasource
const DATASOURCE_RESULTS: Record<number, object> = {
  1: {
    code: 0,
    message: 'ok',
    data: {
      columns: ['id', 'name', 'email', 'department'],
      rows: [
        { id: 1, name: 'Alice Wang', email: 'ali***@prod.com', department: 'Engineering' },
        { id: 2, name: 'Bob Li', email: 'bob***@prod.com', department: 'Product' },
      ],
      total: 2,
      execution_time_ms: 23,
      affected_rows: 0,
      desensitized: true,
      desensitized_fields: ['email'],
      warnings: [],
    },
  },
  2: {
    code: 0,
    message: 'ok',
    data: {
      columns: ['id', 'name', 'email', 'role'],
      rows: [
        { id: 10, name: 'Test User A', email: 'testa@staging.com', role: 'admin' },
        { id: 11, name: 'Test User B', email: 'testb@staging.com', role: 'viewer' },
        { id: 12, name: 'Test User C', email: 'testc@staging.com', role: 'editor' },
      ],
      total: 3,
      execution_time_ms: 12,
      affected_rows: 0,
      desensitized: false,
      desensitized_fields: [],
      warnings: [],
    },
  },
  3: {
    code: 0,
    message: 'ok',
    data: {
      columns: ['_id', 'username', 'created_at', 'profile'],
      rows: [
        { _id: 'obj_001', username: 'mongo_user_1', created_at: '2024-06-01', profile: 'active' },
        { _id: 'obj_002', username: 'mongo_user_2', created_at: '2024-06-02', profile: 'inactive' },
      ],
      total: 2,
      execution_time_ms: 8,
      affected_rows: 0,
      desensitized: false,
      desensitized_fields: [],
      warnings: [],
    },
  },
}

function mockExtendedApis(page: import('@playwright/test').Page) {
  // Override datasources list
  page.route(/\/api\/datasources(\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: EXTENDED_DATASOURCES }),
    })
  })

  // Dynamic query execute based on datasource_id
  page.route('**/api/query/execute', async (route) => {
    const req = route.request().postDataJSON()
    const dsId = req?.datasource_id ?? 1
    const result = DATASOURCE_RESULTS[dsId] ?? DATASOURCE_RESULTS[1]
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(result),
    })
  })
}

test.describe('数据源切换后查询', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockExtendedApis(page)
  })

  test('MySQL 数据源 A → MySQL 数据源 B：切换后查询结果属于不同数据源', async ({ page }) => {
    // ========== 1. 登录 ==========
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // ========== 2. 选择数据源 A（prod-mysql）==========
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /prod-mysql/ }).click()

    // ========== 3. 执行查询，验证结果属于 A ==========
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM users', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 验证 prod-mysql 的特征数据
    await expect(page.getByRole('columnheader', { name: 'department' })).toBeVisible()
    await expect(page.getByText('Alice Wang')).toBeVisible()
    await expect(page.getByText('Engineering')).toBeVisible()
    await expect(page.getByText('23ms')).toBeVisible()
    await expect(page.getByText('2 行')).toBeVisible()

    // ========== 4. 切换到数据源 B（staging-mysql）==========
    await dsSelect.click()
    await page.getByRole('option', { name: /staging-mysql/ }).click()

    // ========== 5. 验证编辑器状态 ==========
    // 编辑器应保留之前输入的 SQL（根据 updateTabDatasource 的实现，不自动清空 SQL）
    // 但结果面板应被重置
    await expect(page.getByText('执行查询以查看结果')).toBeVisible()

    // 验证仍为 MySQL 编辑器（SQL 编辑器存在）
    await expect(page.locator('.cm-content').first()).toBeVisible()

    // ========== 6. 执行查询，验证结果属于 B ==========
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 验证 staging-mysql 的特征数据
    await expect(page.getByRole('columnheader', { name: 'role' })).toBeVisible()
    await expect(page.getByText('Test User A')).toBeVisible()
    await expect(page.getByText('admin')).toBeVisible()
    await expect(page.getByText('12ms')).toBeVisible()
    await expect(page.getByText('3 行')).toBeVisible()

    // 数据源 A 的特征数据不应存在
    await expect(page.getByText('Alice Wang')).not.toBeVisible()
    await expect(page.getByText('Engineering')).not.toBeVisible()
  })

  test('MySQL → MongoDB：自动切换编辑器类型', async ({ page }) => {
    // ========== 1. 登录 ==========
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // ========== 2. 选择 MySQL 数据源 ==========
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /prod-mysql/ }).click()

    // 验证 SQL 编辑器可见
    await expect(page.locator('.cm-content').first()).toBeVisible()

    // 在 SQL 编辑器中输入查询
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM mysql_table', { delay: 30 })

    // 执行 MySQL 查询
    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })
    await expect(page.getByText('Alice Wang')).toBeVisible()

    // ========== 3. 切换到 MongoDB 数据源 ==========
    await dsSelect.click()
    await page.getByRole('option', { name: /prod-mongo/ }).click()

    // ========== 4. 验证自动切换到 MongoDB 编辑器 ==========
    // 应显示 MongoDB 标识
    await expect(page.getByText('MongoDB')).toBeVisible()

    // 应显示集合名输入
    await expect(page.getByPlaceholder('集合名 (collection)')).toBeVisible()

    // 应显示操作类型选择器
    await expect(page.getByRole('combobox').filter({ hasText: 'find' })).toBeVisible()

    // ========== 5. 验证结果面板重置 ==========
    await expect(page.getByText('执行查询以查看结果')).toBeVisible()

    // MySQL 结果不应存在
    await expect(page.getByText('Alice Wang')).not.toBeVisible()

    // ========== 6. 执行 MongoDB 查询 ==========
    await page.getByPlaceholder('集合名 (collection)').fill('users')

    // Filter 编辑器输入查询条件
    const filterEditor = page.locator('.cm-content').first()
    await filterEditor.click()
    await page.keyboard.type('{}', { delay: 30 })

    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 验证 MongoDB 结果
    await expect(page.getByRole('columnheader', { name: '_id' })).toBeVisible()
    await expect(page.getByText('obj_001')).toBeVisible()
    await expect(page.getByText('mongo_user_1')).toBeVisible()
    await expect(page.getByText('8ms')).toBeVisible()
  })

  test('MongoDB → MySQL：切换回 SQL 编辑器', async ({ page }) => {
    // ========== 1. 登录 ==========
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // ========== 2. 直接选择 MongoDB 数据源 ==========
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /prod-mongo/ }).click()

    // 验证 MongoDB 编辑器
    await expect(page.getByText('MongoDB')).toBeVisible()
    await expect(page.getByPlaceholder('集合名 (collection)')).toBeVisible()

    // 输入 MongoDB 查询
    await page.getByPlaceholder('集合名 (collection)').fill('orders')
    const filterEditor = page.locator('.cm-content').first()
    await filterEditor.click()
    await page.keyboard.type('{}', { delay: 30 })

    // 执行
    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })
    await expect(page.getByText('obj_001')).toBeVisible()

    // ========== 3. 切换回 MySQL ==========
    await dsSelect.click()
    await page.getByRole('option', { name: /staging-mysql/ }).click()

    // ========== 4. 验证切回 SQL 编辑器 ==========
    // MongoDB 标识应消失
    await expect(page.getByText('MongoDB')).not.toBeVisible()

    // SQL CodeMirror 编辑器应存在
    await expect(page.locator('.cm-content').first()).toBeVisible()

    // 集合名输入不应存在
    await expect(page.getByPlaceholder('集合名 (collection)')).not.toBeVisible()

    // 结果面板应重置
    await expect(page.getByText('执行查询以查看结果')).toBeVisible()
    await expect(page.getByText('obj_001')).not.toBeVisible()

    // ========== 5. 执行 MySQL 查询 ==========
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM staging_table', { delay: 30 })

    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 验证 MySQL 结果
    await expect(page.getByText('Test User A')).toBeVisible()
    await expect(page.getByText('12ms')).toBeVisible()
  })

  test('数据源列表正确展示 3 个选项', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 打开数据源选择下拉
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()

    // 验证 3 个数据源选项
    await expect(page.getByRole('option', { name: /prod-mysql/ })).toBeVisible()
    await expect(page.getByRole('option', { name: /staging-mysql/ })).toBeVisible()
    await expect(page.getByRole('option', { name: /prod-mongo/ })).toBeVisible()

    // 验证类型标识
    await expect(page.getByText('(mysql)')).toBeVisible()
    await expect(page.getByText('(mongodb)')).toBeVisible()
  })

  test('快速连续切换数据源不会出错', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()

    // 快速切换：prod-mysql → staging-mysql → prod-mongo → prod-mysql
    await dsSelect.click()
    await page.getByRole('option', { name: /prod-mysql/ }).click()

    await dsSelect.click()
    await page.getByRole('option', { name: /staging-mysql/ }).click()

    await dsSelect.click()
    await page.getByRole('option', { name: /prod-mongo/ }).click()

    await dsSelect.click()
    await page.getByRole('option', { name: /prod-mysql/ }).click()

    // 最终应停在 MySQL 编辑器，页面无报错
    await expect(page.locator('.cm-content').first()).toBeVisible()
    await expect(page.getByText('MongoDB')).not.toBeVisible()
  })
})
