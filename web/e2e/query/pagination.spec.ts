import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_DATASOURCES } from '../helpers'

// Generate 100 mock rows for pagination testing
function generateMockRows(count: number) {
  const rows = []
  for (let i = 1; i <= count; i++) {
    rows.push({
      id: i,
      name: `user_${i}`,
      email: `user${i}@example.com`,
      status: i % 2 === 0 ? 'active' : 'inactive',
      created_at: `2024-01-${String(Math.min(i, 28)).padStart(2, '0')} 00:00:00`,
    })
  }
  return rows
}

const TOTAL_ROWS = 100
const MOCK_ROWS = generateMockRows(TOTAL_ROWS)

const LARGE_QUERY_RESULT = {
  code: 0,
  message: 'ok',
  data: {
    columns: ['id', 'name', 'email', 'status', 'created_at'],
    rows: MOCK_ROWS,
    total: TOTAL_ROWS,
    execution_time_ms: 45,
    affected_rows: 0,
    desensitized: false,
    desensitized_fields: [],
    warnings: [],
  },
}

test.describe('查询结果分页', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)

    // Override execute mock to return 100 rows
    await page.route('**/api/query/execute', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(LARGE_QUERY_RESULT),
      })
    })
  })

  test('分页控件显示：大量结果时展示分页按钮', async ({ page }) => {
    // ========== 1. 登录并选择数据源 ==========
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    // ========== 2. 执行查询 ==========
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM users', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // ========== 3. 验证分页控件显示 ==========
    // 验证总行数信息
    await expect(page.getByText(`共 ${TOTAL_ROWS} 行`)).toBeVisible()

    // 100 行 / 50 每页 = 2 页，应显示页码
    await expect(page.getByText(/第 1\/2 页/)).toBeVisible()

    // 验证分页按钮存在
    await expect(page.getByRole('button', { name: '首页' })).toBeVisible()
    await expect(page.getByRole('button', { name: '上一页' })).toBeVisible()
    await expect(page.getByRole('button', { name: '下一页' })).toBeVisible()
    await expect(page.getByRole('button', { name: '末页' })).toBeVisible()

    // 第一页时首页和上一页应禁用
    await expect(page.getByRole('button', { name: '首页' })).toBeDisabled()
    await expect(page.getByRole('button', { name: '上一页' })).toBeDisabled()

    // 下一页和末页可用
    await expect(page.getByRole('button', { name: '下一页' })).toBeEnabled()
    await expect(page.getByRole('button', { name: '末页' })).toBeEnabled()
  })

  test('默认每页条数为 50', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM users', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 验证默认每页条数选择器显示 50
    const pageSizeSelect = page.locator('select').filter({ hasText: /行\/页/ })
    await expect(pageSizeSelect).toBeVisible()
    await expect(pageSizeSelect).toHaveValue('50')

    // 验证当前页显示 50 行数据（表头行不计入）
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows).toHaveCount(50)
  })

  test('切换到第 2 页验证数据变化', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM users', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // ========== 第 1 页：验证首条数据 ==========
    await expect(page.getByText('user_1')).toBeVisible()
    await expect(page.getByText('user_50')).toBeVisible()

    // ========== 切换到第 2 页 ==========
    await page.getByRole('button', { name: '下一页' }).click()

    // ========== 第 2 页：验证数据变化 ==========
    // 页码信息更新
    await expect(page.getByText(/第 2\/2 页/)).toBeVisible()

    // 第 2 页应显示 user_51 ~ user_100
    await expect(page.getByText('user_51')).toBeVisible()
    await expect(page.getByText('user_100')).toBeVisible()

    // 第 1 页的数据不应存在
    await expect(page.getByText('user_1')).not.toBeVisible()
    await expect(page.getByText('user_50')).not.toBeVisible()

    // 下一页和末页应禁用（已在最后一页）
    await expect(page.getByRole('button', { name: '下一页' })).toBeDisabled()
    await expect(page.getByRole('button', { name: '末页' })).toBeDisabled()

    // 上一页和首页应可用
    await expect(page.getByRole('button', { name: '上一页' })).toBeEnabled()
    await expect(page.getByRole('button', { name: '首页' })).toBeEnabled()
  })

  test('点击首页按钮回到第 1 页', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM users', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 先切到第 2 页
    await page.getByRole('button', { name: '末页' }).click()
    await expect(page.getByText(/第 2\/2 页/)).toBeVisible()

    // 点击首页
    await page.getByRole('button', { name: '首页' }).click()
    await expect(page.getByText(/第 1\/2 页/)).toBeVisible()
    await expect(page.getByText('user_1')).toBeVisible()
  })

  test('切换每页条数后分页重置', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM users', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 默认 50 行/页，2 页
    await expect(page.getByText(/第 1\/2 页/)).toBeVisible()

    // 切换为 100 行/页
    const pageSizeSelect = page.locator('select').filter({ hasText: /行\/页/ })
    await pageSizeSelect.selectOption('100')

    // 100 行/页，100 条数据 = 1 页，不应显示页码
    await expect(page.getByText(/第 \d+\/\d+ 页/)).not.toBeVisible()

    // 应显示全部 100 行数据（ResultTable 的分页只渲染当前页）
    // 但 DOM 中实际渲染的是 pageSize 条
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows).toHaveCount(100)

    // 仍然显示总行数
    await expect(page.getByText(`共 ${TOTAL_ROWS} 行`)).toBeVisible()
  })

  test('少量结果不显示分页按钮', async ({ page }) => {
    // 使用默认 mock（只有 2 条数据），不需要 override
    // 但 beforeEach 中已经 override 了，所以需要在测试中重新 mock
    await page.route('**/api/query/execute', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            columns: ['id', 'name'],
            rows: [{ id: 1, name: 'Alice' }, { id: 2, name: 'Bob' }],
            total: 2,
            execution_time_ms: 5,
            affected_rows: 0,
            desensitized: false,
            desensitized_fields: [],
            warnings: [],
          },
        }),
      })
    })

    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM small_table', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // 2 行数据不足 50 行（默认一页），不应显示分页按钮
    await expect(page.getByText('共 2 行')).toBeVisible()
    await expect(page.getByRole('button', { name: '下一页' })).not.toBeVisible()
    await expect(page.getByRole('button', { name: '上一页' })).not.toBeVisible()
  })
})
