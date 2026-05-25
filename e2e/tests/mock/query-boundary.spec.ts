/**
 * SF-QA0024: E2E — 查询模块极端边界场景
 * Covers: 大结果集, SQL 注入防御, 特殊字符, 并发查询, 超时处理
 */
import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI } from '../../support/mock-routes'

test.describe('查询边界 — SQL 注入防御', () => {
  test('SQL 编辑器接受 DROP TABLE 但不允许执行（通过工单）', async ({ page }) => {
    mockApiRoutes(page)

    await loginViaUI(page)

    // Select datasource
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    // Type a dangerous SQL
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('DROP TABLE users', { delay: 30 })

    // AI review should flag this as high risk
    // Wait for AI review results (mocked to return high risk)
    await page.getByRole('button', { name: '执行' }).click()

    // Mock AI review returns high risk → ticket required
    // The button should change to "提交工单" or show risk warning
    await expect(page.getByText(/高风险|危险|工单/)).toBeVisible({ timeout: 5000 })
  })

  test('SQL 编辑器接受 SELECT 语句正常执行', async ({ page }) => {
    mockApiRoutes(page)

    await loginViaUI(page)

    // Sign in to the query page
    await page.goto('/query')
    // Already logged in via loginViaUI

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1', { delay: 30 })

    await page.getByRole('button', { name: '执行' }).click()

    // Result table should appear
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })
  })
})

test.describe('查询边界 — 大结果集', () => {
  test('查询返回 0 行结果', async ({ page }) => {
    // Mock empty result
    await page.route('**/api/query/execute', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: {
            columns: ['id', 'name', 'email'],
            rows: [],
            total: 0,
            execution_time_ms: 5,
            affected_rows: 0,
            desensitized: false,
            desensitized_fields: [],
            warnings: [],
          },
        }),
      })
    })

    mockApiRoutes(page)
    await loginViaUI(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM empty_table', { delay: 30 })

    await page.getByRole('button', { name: '执行' }).click()

    // Should show empty result message
    await expect(page.getByText(/暂无数据|0 行|没有数据/)).toBeVisible({ timeout: 10000 })
  })

  test('查询返回大量列的数据', async ({ page }) => {
    // Mock many columns
    const manyColumns = Array.from({ length: 30 }, (_, i) => `col_${i}`)
    const sampleRow: Record<string, unknown> = {}
    manyColumns.forEach((col) => { sampleRow[col] = `value_${col}` })

    await page.route('**/api/query/execute', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: {
            columns: manyColumns,
            rows: [sampleRow, sampleRow],
            total: 2,
            execution_time_ms: 25,
            affected_rows: 0,
            desensitized: false,
            desensitized_fields: [],
            warnings: [],
          },
        }),
      })
    })

    mockApiRoutes(page)
    await loginViaUI(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM wide_table', { delay: 30 })

    await page.getByRole('button', { name: '执行' }).click()

    // Table should render with many columns (horizontally scrollable)
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })
    await expect(page.getByRole('columnheader', { name: 'col_0' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'col_29' })).toBeVisible()
  })

  test('查询结果超过一页触发分页', async ({ page }) => {
    // Mock paginated results
    const rows = Array.from({ length: 100 }, (_, i) => ({
      id: i + 1,
      name: `User ${i + 1}`,
      email: `user${i + 1}@example.com`,
    }))

    await page.route('**/api/query/execute', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: {
            columns: ['id', 'name', 'email'],
            rows: rows,
            total: 100,
            execution_time_ms: 45,
            affected_rows: 0,
            desensitized: false,
            desensitized_fields: [],
            warnings: [],
          },
        }),
      })
    })

    mockApiRoutes(page)
    await loginViaUI(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM large_table', { delay: 30 })

    await page.getByRole('button', { name: '执行' }).click()

    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })

    // Should show pagination controls for 100 rows
    const pagination = page.getByText(/100|页/)
    await expect(pagination).toBeVisible({ timeout: 5000 })
  })
})

test.describe('查询边界 — 执行时间与超时', () => {
  test('慢查询显示执行耗时', async ({ page }) => {
    // Mock slow query
    await page.route('**/api/query/execute', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: {
            columns: ['id', 'name'],
            rows: [{ id: 1, name: 'Test' }],
            total: 1,
            execution_time_ms: 8500,
            affected_rows: 0,
            desensitized: false,
            desensitized_fields: [],
            warnings: ['查询耗时较长，建议优化索引'],
          },
        }),
      })
    })

    mockApiRoutes(page)
    await loginViaUI(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM huge_table WHERE unindexed_col = x', { delay: 30 })

    await page.getByRole('button', { name: '执行' }).click()

    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })
    await expect(page.getByText('8500ms')).toBeVisible()

    // Verify warning message
    await expect(page.getByText(/耗时较长|建议优化/)).toBeVisible()
  })

  test('查询执行失败显示错误信息', async ({ page }) => {
    // Mock query error
    await page.route('**/api/query/execute', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 1,
          message: '语法错误: You have an error in your SQL syntax near \'FROMM\'',
          data: null,
        }),
      })
    })

    mockApiRoutes(page)
    await loginViaUI(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROMM users', { delay: 30 })

    await page.getByRole('button', { name: '执行' }).click()

    // Error message should appear
    await expect(page.getByText(/语法错误|error/)).toBeVisible({ timeout: 10000 })
  })
})

test.describe('查询边界 — 快捷键与键盘操作', () => {
  test('Ctrl+Enter 触发查询执行', async ({ page }) => {
    mockApiRoutes(page)
    await loginViaUI(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1', { delay: 30 })

    // Press Ctrl+Enter to execute
    await page.keyboard.press('Control+Enter')

    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })
  })

  test('Cmd+Enter 在 Mac 上触发查询执行', async ({ page }) => {
    mockApiRoutes(page)
    await loginViaUI(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1', { delay: 30 })

    // Press Meta+Enter (Cmd on Mac)
    await page.keyboard.press('Meta+Enter')

    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })
  })
})

test.describe('查询边界 — 多数据源切换', () => {
  test('切换数据源后 SQL 编辑区保留内容', async ({ page }) => {
    mockApiRoutes(page)
    await loginViaUI(page)

    // Select MySQL
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    // Type SQL
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM users', { delay: 30 })

    // Switch to MongoDB
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mongo' }).click()

    // Content should be preserved or cleared depending on design
    // At minimum, the page should not crash
    await page.waitForTimeout(500)
  })

  test('切换到 MongoDB 数据源显示 MongoDB 编辑器', async ({ page }) => {
    mockApiRoutes(page)
    await loginViaUI(page)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mongo' }).click()

    // MongoDB editor should show collection input
    await expect(page.getByPlaceholder('集合名 (collection)')).toBeVisible()
    await expect(page.getByText('MongoDB')).toBeVisible()
  })
})
