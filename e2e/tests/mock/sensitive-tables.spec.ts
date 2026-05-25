/**
 * SF-QA0024: E2E — 敏感表管理
 * Covers: 正常流程 / 异常处理 / 权限校验 / 边界场景
 */
import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, setToken, MOCK_SENSITIVE_TABLES } from '../../support/mock-routes'

// --- In-memory mock state ---
let sensitiveTables = [...MOCK_SENSITIVE_TABLES]
let nextTableId = 100

function mockSensitiveTableApis(page: import('@playwright/test').Page) {
  sensitiveTables = [...MOCK_SENSITIVE_TABLES]
  nextTableId = 100

  // List sensitive tables
  page.route(/\/api\/sensitive-tables(\?.*)?$/, async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: sensitiveTables, total: sensitiveTables.length }),
      })
    } else if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON()
      const newTable = {
        id: nextTableId++,
        datasource_id: body.datasource_id ?? 1,
        table_name: body.table_name,
        column_count: body.column_count ?? 0,
        created_at: new Date().toISOString(),
      }
      sensitiveTables.push(newTable)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok', data: newTable }),
      })
    } else {
      await route.fulfill({ status: 405, contentType: 'application/json', body: '{}' })
    }
  })

  // Delete sensitive table
  page.route(/\/api\/sensitive-tables\/\d+/, async (route) => {
    const url = route.request().url()
    const idMatch = url.match(/\/api\/sensitive-tables\/(\d+)/)
    const tableId = idMatch ? parseInt(idMatch[1]) : 0
    if (route.request().method() === 'DELETE') {
      sensitiveTables = sensitiveTables.filter((t) => t.id !== tableId)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
    } else {
      await route.fulfill({ status: 405, contentType: 'application/json', body: '{}' })
    }
  })
}

test.describe('敏感表管理 — 正常流程', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockSensitiveTableApis(page)
  })

  test('导航到敏感表设置页并验证列表', async ({ page }) => {
    await loginViaUI(page)

    // Navigate to settings → sensitive tables
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '敏感表' }).click()
    await page.waitForURL('**/settings/sensitive')

    // Verify page title
    await expect(page.getByText('敏感表').first()).toBeVisible()

    // Verify table headers
    await expect(page.getByRole('columnheader', { name: '表名' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '敏感列数' })).toBeVisible()

    // Verify table data
    await expect(page.getByText('users')).toBeVisible()
    await expect(page.getByText('payments')).toBeVisible()
  })

  test('添加敏感表', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '敏感表' }).click()
    await page.waitForURL('**/settings/sensitive')

    const initialCount = sensitiveTables.length

    // Click add button
    await page.getByRole('button', { name: /添加|新建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Select datasource
    const dsSelect = page.locator('dialog [role="combobox"]').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    // Select table
    const tableSelect = page.locator('dialog [role="combobox"]').nth(1)
    await tableSelect.click()
    await page.getByRole('option', { name: 'orders' }).click()

    // Save
    await page.getByRole('button', { name: /保存|确认/ }).click()

    // Verify success
    await expect(page.getByText(/添加成功|创建成功/)).toBeVisible()
    expect(sensitiveTables.length).toBeGreaterThan(initialCount)
  })

  test('删除敏感表 — 确认流程', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '敏感表' }).click()
    await page.waitForURL('**/settings/sensitive')

    const initialCount = sensitiveTables.length

    // Click delete on first row
    await page.getByRole('button', { name: '删除' }).first().click()

    // Verify confirmation dialog
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await expect(page.getByText(/确认移除|确定要移除/)).toBeVisible()

    // Confirm
    await page.getByRole('button', { name: /确认|确定/ }).click()

    // Verify success
    await expect(page.getByText(/移除成功|删除成功/)).toBeVisible()
    expect(sensitiveTables.length).toBeLessThan(initialCount)
  })
})

test.describe('敏感表管理 — 异常处理', () => {
  test('重复添加相同表被拒绝', async ({ page }) => {
    mockApiRoutes(page)
    mockSensitiveTableApis(page)

    // Mock POST to reject duplicates
    page.route(/\/api\/sensitive-tables$/, async (route) => {
      if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        if (body.table_name === 'users') {
          await route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({ code: 1, message: '该表已标记为敏感表' }),
          })
        } else {
          await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, message: 'ok' }) })
        }
      } else {
        await route.fulfill()
      }
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '敏感表' }).click()
    await page.waitForURL('**/settings/sensitive')

    await page.getByRole('button', { name: /添加|新建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const dsSelect = page.locator('dialog [role="combobox"]').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const tableSelect = page.locator('dialog [role="combobox"]').nth(1)
    await tableSelect.click()
    await page.getByRole('option', { name: 'users' }).click()

    await page.getByRole('button', { name: /保存|确认/ }).click()

    // Verify duplicate error
    await expect(page.getByText('该表已标记为敏感表')).toBeVisible()
  })

  test('删除敏感表失败时显示错误', async ({ page }) => {
    mockApiRoutes(page)
    mockSensitiveTableApis(page)

    // Mock delete to return error
    page.route(/\/api\/sensitive-tables\/\d+/, async (route) => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ code: 1, message: '删除失败，该表有关联的脱敏规则' }),
      })
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '敏感表' }).click()
    await page.waitForURL('**/settings/sensitive')

    await page.getByRole('button', { name: '删除' }).first().click()
    await page.getByRole('button', { name: /确认|确定/ }).click()

    // Verify error message
    await expect(page.getByText('删除失败，该表有关联的脱敏规则')).toBeVisible()
  })
})

test.describe('敏感表管理 — 权限校验', () => {
  test('非管理员无法访问敏感表页面', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    page.route(/\/api\/sensitive-tables/, async (route) => {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    })

    await setToken(page, 'developer')
    await page.goto('/settings/sensitive')

    await page.waitForURL('**/403**', { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    expect(is403).toBe(true)
  })

  test('developer 在导航中看不到敏感表入口', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await setToken(page, 'developer')

    await page.goto('/settings/datasource')

    const sensitiveLink = page.locator('nav').getByRole('link', { name: '敏感表' })
    await expect(sensitiveLink).not.toBeVisible()
  })

  test('admin 可正常访问敏感表页面', async ({ page }) => {
    mockApiRoutes(page, { role: 'admin' })
    mockSensitiveTableApis(page)

    await setToken(page, 'admin')
    await page.goto('/settings/sensitive')

    await expect(page).toHaveURL(/\/settings\/sensitive/)
    await expect(page.getByText('敏感表').first()).toBeVisible()
  })
})

test.describe('敏感表管理 — 边界场景', () => {
  test('空敏感表列表显示空状态', async ({ page }) => {
    mockApiRoutes(page)
    page.route(/\/api\/sensitive-tables$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: [], total: 0 }),
      })
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '敏感表' }).click()
    await page.waitForURL('**/settings/sensitive')

    await expect(page.getByText(/暂无|没有|空/)).toBeVisible()
  })

  test('取消删除敏感表对话框', async ({ page }) => {
    mockApiRoutes(page)
    mockSensitiveTableApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '敏感表' }).click()
    await page.waitForURL('**/settings/sensitive')

    const initialCount = sensitiveTables.length

    await page.getByRole('button', { name: '删除' }).first().click()
    await expect(page.getByRole('alertdialog')).toBeVisible()

    await page.getByRole('button', { name: '取消' }).click()
    await expect(page.getByRole('alertdialog')).not.toBeVisible()

    expect(sensitiveTables.length).toBe(initialCount)
  })
})
