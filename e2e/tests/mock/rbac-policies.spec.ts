/**
 * SF-QA0024: E2E — RBAC 策略管理
 * Covers: 正常流程 / 异常处理 / 权限校验 / 边界场景
 */
import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, setToken, MOCK_USERS } from '../../support/mock-routes'

// --- Mock Policy State ---
let policies = [
  { id: 1, role: 'developer', datasource: 'test-mysql', table: 'users', action: 'select' },
  { id: 2, role: 'developer', datasource: 'test-mysql', table: 'orders', action: 'select' },
  { id: 3, role: 'dba', datasource: 'test-mysql', table: '*', action: '*' },
]
let nextPolicyId = 100

const MOCK_ROLES = [
  { role: 'admin', description: '管理员' },
  { role: 'dba', description: 'DBA' },
  { role: 'developer', description: '开发人员' },
]

function mockPolicyApis(page: import('@playwright/test').Page) {
  policies = [
    { id: 1, role: 'developer', datasource: 'test-mysql', table: 'users', action: 'select' },
    { id: 2, role: 'developer', datasource: 'test-mysql', table: 'orders', action: 'select' },
    { id: 3, role: 'dba', datasource: 'test-mysql', table: '*', action: '*' },
  ]
  nextPolicyId = 100

  // Roles
  page.route('**/api/roles**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: MOCK_ROLES }),
    })
  })

  // Policies list / create
  page.route(/\/api\/policies(\?.*)?$/, async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: policies }),
      })
    } else if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON()
      const newPolicy = {
        id: nextPolicyId++,
        role: body.role,
        datasource: body.datasource,
        table: body.table,
        action: body.action,
      }
      policies.push(newPolicy)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok', data: newPolicy }),
      })
    } else {
      await route.fulfill({ status: 405, contentType: 'application/json', body: '{}' })
    }
  })

  // Delete policy
  page.route(/\/api\/policies\/\d+/, async (route) => {
    const url = route.request().url()
    const idMatch = url.match(/\/api\/policies\/(\d+)/)
    const policyId = idMatch ? parseInt(idMatch[1]) : 0
    if (route.request().method() === 'DELETE') {
      policies = policies.filter((p) => p.id !== policyId)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
    } else if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON()
      policies = policies.map((p) =>
        p.id === policyId ? { ...p, ...body } : p,
      )
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

test.describe('RBAC 策略管理 — 正常流程', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockPolicyApis(page)
  })

  test('导航到策略管理页并验证列表渲染', async ({ page }) => {
    await loginViaUI(page)

    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '权限策略' }).click()
    await page.waitForURL('**/settings/policies')

    // Verify page title
    await expect(page.getByText(/权限策略|策略管理/).first()).toBeVisible()

    // Verify table headers
    await expect(page.getByRole('columnheader', { name: '角色' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '数据源' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '表名' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '操作' })).toBeVisible()

    // Verify policy data
    await expect(page.getByText('developer')).toBeVisible()
    await expect(page.getByText('test-mysql')).toBeVisible()
  })

  test('添加访问控制策略', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '权限策略' }).click()
    await page.waitForURL('**/settings/policies')

    const initialCount = policies.length

    // Click add button
    await page.getByRole('button', { name: /添加|新建|创建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Select role
    const roleSelect = page.locator('dialog [role="combobox"]').first()
    await roleSelect.click()
    await page.getByRole('option', { name: '开发人员' }).click()

    // Select datasource
    const dsSelect = page.locator('dialog [role="combobox"]').nth(1)
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    // Select action
    const actionSelect = page.locator('dialog [role="combobox"]').nth(2)
    await actionSelect.click()
    await page.getByRole('option', { name: 'SELECT' }).click()

    // Save
    await page.getByRole('button', { name: /保存|确认|提交/ }).click()

    // Verify success
    await expect(page.getByText(/添加成功|创建成功/)).toBeVisible()
    expect(policies.length).toBeGreaterThan(initialCount)
  })

  test('删除策略 — 确认流程', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '权限策略' }).click()
    await page.waitForURL('**/settings/policies')

    const initialCount = policies.length

    // Click delete on first policy
    await page.getByRole('button', { name: '删除' }).first().click()

    // Verify confirmation dialog
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await expect(page.getByText(/确认删除|确定要删除/)).toBeVisible()

    // Confirm
    await page.getByRole('button', { name: /确认|确定/ }).click()

    // Verify success
    await expect(page.getByText(/删除成功|移除成功/)).toBeVisible()
    expect(policies.length).toBeLessThan(initialCount)
  })

  test('通配符策略 "*" 的显示', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '权限策略' }).click()
    await page.waitForURL('**/settings/policies')

    // dba role has wildcard policy — verify it's displayed
    // The wildcard table '*' should be rendered
    const dbaRow = page.getByRole('row', { name: /dba/ })
    await expect(dbaRow).toBeVisible()
  })
})

test.describe('RBAC 策略管理 — 异常处理', () => {
  test('重复策略被拒绝', async ({ page }) => {
    mockApiRoutes(page)
    mockPolicyApis(page)

    // Mock POST to reject duplicates
    await page.route(/\/api\/policies$/, async (route) => {
      if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        if (body.role === 'developer' && body.datasource === 'test-mysql' && body.action === 'select') {
          await route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({ code: 1, message: '该角色已存在相同策略' }),
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
    await page.locator('nav').getByRole('link', { name: '权限策略' }).click()
    await page.waitForURL('**/settings/policies')

    await page.getByRole('button', { name: /添加|新建|创建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const roleSelect = page.locator('dialog [role="combobox"]').first()
    await roleSelect.click()
    await page.getByRole('option', { name: '开发人员' }).click()

    const dsSelect = page.locator('dialog [role="combobox"]').nth(1)
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const actionSelect = page.locator('dialog [role="combobox"]').nth(2)
    await actionSelect.click()
    await page.getByRole('option', { name: 'SELECT' }).click()

    await page.getByRole('button', { name: /保存|确认|提交/ }).click()

    await expect(page.getByText('该角色已存在相同策略')).toBeVisible()
  })

  test('添加策略时未选择角色不能提交', async ({ page }) => {
    mockApiRoutes(page)
    mockPolicyApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '权限策略' }).click()
    await page.waitForURL('**/settings/policies')

    await page.getByRole('button', { name: /添加|新建|创建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Submit without selecting required fields
    const saveBtn = page.getByRole('button', { name: /保存|确认|提交/ })
    // Button should be disabled or form should show validation
    const isDisabled = await saveBtn.isDisabled().catch(() => true)
    if (!isDisabled) {
      await saveBtn.click()
      await expect(page.getByText(/请选择|必填|必须/)).toBeVisible()
    }
  })
})

test.describe('RBAC 策略管理 — 权限校验', () => {
  test('非管理员无法访问策略管理页', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    page.route('**/api/policies', async (route) => {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    })

    await setToken(page, 'developer')
    await page.goto('/settings/policies')

    await page.waitForURL('**/403**', { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    expect(is403).toBe(true)
  })

  test('developer 在设置导航中看不到权限策略入口', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await setToken(page, 'developer')

    await page.goto('/settings/datasource')

    const policyLink = page.locator('nav').getByRole('link', { name: '权限策略' })
    await expect(policyLink).not.toBeVisible()
  })

  test('admin 可正常访问策略管理页', async ({ page }) => {
    mockApiRoutes(page, { role: 'admin' })
    mockPolicyApis(page)

    await setToken(page, 'admin')
    await page.goto('/settings/policies')

    await expect(page).toHaveURL(/\/settings\/policies/)
    await expect(page.getByText(/权限策略|策略管理/).first()).toBeVisible()
  })
})

test.describe('RBAC 策略管理 — 边界场景', () => {
  test('空策略列表显示空状态', async ({ page }) => {
    mockApiRoutes(page)
    page.route('**/api/policies', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: [] }),
      })
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '权限策略' }).click()
    await page.waitForURL('**/settings/policies')

    await expect(page.getByText(/暂无|没有|空/)).toBeVisible()
  })

  test('取消删除策略对话框', async ({ page }) => {
    mockApiRoutes(page)
    mockPolicyApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '权限策略' }).click()
    await page.waitForURL('**/settings/policies')

    const initialCount = policies.length

    await page.getByRole('button', { name: '删除' }).first().click()
    await expect(page.getByRole('alertdialog')).toBeVisible()

    await page.getByRole('button', { name: '取消' }).click()
    await expect(page.getByRole('alertdialog')).not.toBeVisible()

    expect(policies.length).toBe(initialCount)
  })

  test('添加 DBA 角色的全表全操作策略', async ({ page }) => {
    mockApiRoutes(page)
    mockPolicyApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '权限策略' }).click()
    await page.waitForURL('**/settings/policies')

    await page.getByRole('button', { name: /添加|新建|创建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Select DBA role
    const roleSelect = page.locator('dialog [role="combobox"]').first()
    await roleSelect.click()
    await page.getByRole('option', { name: 'DBA' }).click()

    // Select datasource
    const dsSelect = page.locator('dialog [role="combobox"]').nth(1)
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    // Select * (wildcard) action
    const actionSelect = page.locator('dialog [role="combobox"]').nth(2)
    await actionSelect.click()
    await page.getByRole('option', { name: 'ALL' }).click()

    await page.getByRole('button', { name: /保存|确认|提交/ }).click()
    await expect(page.getByText(/添加成功|创建成功/)).toBeVisible()
  })
})

test.describe('RBAC 角色菜单可见性', () => {
  test('admin 首页侧边栏显示所有菜单项', async ({ page }) => {
    mockApiRoutes(page, { role: 'admin' })
    await setToken(page, 'admin')

    await page.goto('/query')

    // Admin should see all navigation items
    await expect(page.getByRole('link', { name: '查询' })).toBeVisible()
    await expect(page.getByRole('link', { name: '工单' })).toBeVisible()
    await expect(page.getByRole('link', { name: '审计' })).toBeVisible()
    await expect(page.getByRole('button', { name: '设置' })).toBeVisible()
  })

  test('developer 首页侧边栏只显示有限菜单项', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await setToken(page, 'developer')

    await page.goto('/query')

    // Developer should see query and tickets
    await expect(page.getByRole('link', { name: '查询' })).toBeVisible()
    await expect(page.getByRole('link', { name: '工单' })).toBeVisible()

    // Developer might see audit but with limited access
    // Settings might be hidden
    const settingsBtn = page.getByRole('button', { name: '设置' })
    const isSettingsVisible = await settingsBtn.isVisible().catch(() => false)
    // If settings is visible, only non-admin items should be shown
    if (isSettingsVisible) {
      await settingsBtn.click()
      const adminOnlyLink = page.locator('nav').getByRole('link', { name: '用户管理' })
      await expect(adminOnlyLink).not.toBeVisible()
    }
  })

  test('dba 首页侧边栏包含审计菜单', async ({ page }) => {
    mockApiRoutes(page, { role: 'dba' })
    await setToken(page, 'dba')

    await page.goto('/query')

    await expect(page.getByRole('link', { name: '审计' })).toBeVisible()
  })
})
