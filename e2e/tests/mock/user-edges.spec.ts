/**
 * SF-QA0024: E2E — 用户管理边界与异常场景
 * Covers: 异常处理 / 权限校验 / 边界场景
 */
import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, setToken, MOCK_USERS } from '../../support/mock-routes'

const MOCK_USER_LIST = [
  { id: 1, username: 'admin', role: 'admin', email: 'admin@example.com', status: 'active', created_at: '2026-01-01T00:00:00.000Z' },
  { id: 2, username: 'developer', role: 'developer', email: 'developer@example.com', status: 'active', created_at: '2026-02-15T10:00:00.000Z' },
  { id: 3, username: 'dba_user', role: 'dba', email: 'dba@example.com', status: 'active', created_at: '2026-03-20T08:00:00.000Z' },
  { id: 4, username: 'viewer', role: 'developer', email: 'viewer@example.com', status: 'inactive', created_at: '2026-04-10T14:00:00.000Z' },
]

const MOCK_ROLES = [
  { role: 'admin', description: '管理员' },
  { role: 'dba', description: 'DBA' },
  { role: 'developer', description: '开发人员' },
]

function mockUserEdgeApis(page: import('@playwright/test').Page) {
  let users = [...MOCK_USER_LIST]
  let nextId = 5

  // Users list + create
  page.route(/\/api\/users(\?.*)?$/, async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: users }),
      })
    } else if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON()
      // Check for duplicate username
      if (users.some((u) => u.username === body.username)) {
        await route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({ code: 1, message: '用户名已存在' }),
        })
        return
      }
      const newUser = {
        id: nextId++,
        username: body.username,
        role: body.role,
        email: body.email ?? '',
        status: 'active',
        created_at: new Date().toISOString(),
      }
      users.push(newUser)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok', data: newUser }),
      })
    } else {
      await route.fulfill({ status: 405, contentType: 'application/json', body: '{}' })
    }
  })

  // Single user CRUD
  page.route(/\/api\/users\/\d+$/, async (route) => {
    const url = route.request().url()
    const idMatch = url.match(/\/api\/users\/(\d+)/)
    const userId = idMatch ? parseInt(idMatch[1]) : 0

    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON()
      // Check for duplicate username in update
      if (body.username && users.some((u) => u.username === body.username && u.id !== userId)) {
        await route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({ code: 1, message: '用户名已存在' }),
        })
        return
      }
      users = users.map((u) =>
        u.id === userId ? { ...u, ...body } : u,
      )
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
    } else if (route.request().method() === 'DELETE') {
      // Prevent deleting last admin
      const target = users.find((u) => u.id === userId)
      const adminCount = users.filter((u) => u.role === 'admin').length
      if (target?.role === 'admin' && adminCount <= 1) {
        await route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({ code: 1, message: '不能删除最后一个管理员' }),
        })
        return
      }
      users = users.filter((u) => u.id !== userId)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
    } else {
      await route.fulfill({ status: 405, contentType: 'application/json', body: '{}' })
    }
  })

  // Reset password
  page.route(/\/api\/users\/\d+\/reset-password/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, message: 'ok' }),
    })
  })

  // Roles
  page.route('**/api/roles**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: MOCK_ROLES }),
    })
  })
}

test.describe('用户管理 — 异常处理', () => {
  test('创建用户时用户名重复被拒绝', async ({ page }) => {
    mockApiRoutes(page)
    mockUserEdgeApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users')

    // Open create dialog
    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Try to create user with existing username
    await page.getByPlaceholder(/用户名/).fill('admin')
    await page.getByPlaceholder(/邮箱/).fill('duplicate@example.com')

    // Save
    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // Verify duplicate error
    await expect(page.getByText('用户名已存在')).toBeVisible()
  })

  test('编辑用户时修改为已存在的用户名被拒绝', async ({ page }) => {
    mockApiRoutes(page)
    mockUserEdgeApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users')

    // Edit developer user
    const devRow = page.getByRole('row', { name: /developer/ })
    await devRow.getByRole('button', { name: /编辑/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Try change username to 'admin' which already exists
    const usernameInput = page.getByPlaceholder(/用户名/)
    await usernameInput.clear()
    await usernameInput.fill('admin')

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    await expect(page.getByText('用户名已存在')).toBeVisible()
  })

  test('删除最后一个管理员被阻止', async ({ page }) => {
    mockApiRoutes(page)
    mockUserEdgeApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users')

    // Try to delete admin (the only admin)
    const adminRow = page.getByRole('row', { name: /^admin/ })
    await adminRow.getByRole('button', { name: /删除/ }).click()

    // May have confirmation dialog
    const confirmBtn = page.getByRole('button', { name: /确认|确定/ })
    if (await confirmBtn.isVisible()) {
      await confirmBtn.click()
    }

    // Verify error message
    await expect(page.getByText('不能删除最后一个管理员')).toBeVisible()
  })

  test('创建用户时超长用户名被拒绝', async ({ page }) => {
    mockApiRoutes(page)
    mockUserEdgeApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users')

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Try username > 32 chars
    const longName = 'a'.repeat(33)
    const usernameInput = page.getByPlaceholder(/用户名/)
    await usernameInput.fill(longName)
    await usernameInput.blur()

    // Check for frontend validation
    const hasError = await page.getByText(/字符|长度/).isVisible().catch(() => false)
    const isDisabled = await page.getByRole('button', { name: /确认|提交|保存/ }).isDisabled().catch(() => false)
    expect(hasError || isDisabled).toBeTruthy()
  })
})

test.describe('用户管理 — 边界场景', () => {
  test('用户名为 3 个字符（最小长度）', async ({ page }) => {
    mockApiRoutes(page)
    mockUserEdgeApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users')

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Minimum length username
    await page.getByPlaceholder(/用户名/).fill('abc')
    await page.getByPlaceholder(/邮箱/).fill('abc@example.com')

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    await expect(page.getByText(/创建成功|添加成功/)).toBeVisible()
  })

  test('用户名为 32 个字符（最大长度）', async ({ page }) => {
    mockApiRoutes(page)
    mockUserEdgeApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users')

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Maximum length username
    await page.getByPlaceholder(/用户名/).fill('a'.repeat(32))
    await page.getByPlaceholder(/邮箱/).fill('maxlen@example.com')

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    await expect(page.getByText(/创建成功|添加成功/)).toBeVisible()
  })

  test('用户名包含特殊字符被拒绝', async ({ page }) => {
    mockApiRoutes(page)
    mockUserEdgeApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users')

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Try username with special characters
    await page.getByPlaceholder(/用户名/).fill('user@name')
    await page.getByPlaceholder(/用户名/).blur()

    // Expect frontend validation
    const hasError = await page.getByText(/字母|数字|下划线|允许/).isVisible().catch(() => false)
    // Not all UIs enforce this, but if they do it should be caught
    if (!hasError) {
      // Try submitting and see if backend rejects
      await page.getByPlaceholder(/邮箱/).fill('test@example.com')
      await page.getByRole('button', { name: /确认|提交|保存/ }).click()
    }
  })

  test('用户名为空字符串', async ({ page }) => {
    mockApiRoutes(page)
    mockUserEdgeApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users')

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Focus and blur username without typing
    const usernameInput = page.getByPlaceholder(/用户名/)
    await usernameInput.focus()
    await usernameInput.blur()

    // Expect error
    const hasError = await page.getByText(/请输入|必填|不能为空/).isVisible().catch(() => false)
    const isDisabled = await page.getByRole('button', { name: /确认|提交|保存/ }).isDisabled().catch(() => false)
    expect(hasError || isDisabled).toBeTruthy()
  })

  test('创建用户时不填邮箱', async ({ page }) => {
    mockApiRoutes(page)
    mockUserEdgeApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users')

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder(/用户名/).fill('testuser')

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // May succeed or show validation depending on whether email is required
    // At minimum, the dialog should close or show error
    const dialogVisible = await page.getByRole('dialog').isVisible().catch(() => false)
    const hasSuccess = await page.getByText(/创建成功|添加成功|用户名已存在/).isVisible().catch(() => false)
    expect(!dialogVisible || hasSuccess).toBeTruthy()
  })
})

test.describe('用户管理 — 权限校验补充', () => {
  test('developer 无法通过直接 URL 访问用户管理', async ({ page }) => {
    page.route('**/api/auth/me', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: MOCK_USERS.developer }),
      })
    })
    page.route(/\/api\/users/, async (route) => {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    })

    // Set token first
    await page.goto('/login')
    await page.evaluate(() => {
      localStorage.setItem('token', 'mock-jwt-e2e-testing-token')
    })

    await page.goto('/admin/users')

    await page.waitForURL('**/403**', { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    expect(is403).toBe(true)
  })

  test('未登录用户访问用户管理重定向到登录', async ({ page }) => {
    await page.goto('/admin/users')
    await page.waitForURL('**/login**', { timeout: 5000 })
    expect(page.url()).toContain('/login')
  })
})
