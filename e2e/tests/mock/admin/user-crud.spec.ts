import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_USERS } from '../../support/mock-routes'

// --- User Management Mock Data ---

const MOCK_USER_LIST = [
  { id: 1, username: 'admin', role: 'admin', email: 'admin@example.com', status: 'active', created_at: '2026-01-01T00:00:00.000Z' },
  { id: 2, username: 'developer', role: 'developer', email: 'developer@example.com', status: 'active', created_at: '2026-02-15T10:00:00.000Z' },
  { id: 3, username: 'dba_user', role: 'dba', email: 'dba@example.com', status: 'active', created_at: '2026-03-20T08:00:00.000Z' },
  { id: 4, username: 'viewer', role: 'developer', email: 'viewer@example.com', status: 'inactive', created_at: '2026-04-10T14:00:00.000Z' },
]

let users = [...MOCK_USER_LIST]
let nextId = 5

const MOCK_ROLES = [
  { role: 'admin', description: '管理员' },
  { role: 'dba', description: 'DBA' },
  { role: 'developer', description: '开发人员' },
]

function mockUserApis(page: import('@playwright/test').Page) {
  // Reset users state
  users = [...MOCK_USER_LIST]
  nextId = 5

  // List users
  page.route(/\/api\/users(\?.*)?$/, async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: users }),
      })
    } else if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON()
      const newUser = {
        id: nextId++,
        username: body.username,
        role: body.role,
        email: body.email,
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

  // Update / Delete user
  page.route(/\/api\/users\/\d+$/, async (route) => {
    const url = route.request().url()
    const idMatch = url.match(/\/api\/users\/(\d+)/)
    const userId = idMatch ? parseInt(idMatch[1]) : 0

    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON()
      users = users.map((u) =>
        u.id === userId ? { ...u, username: body.username ?? u.username, role: body.role ?? u.role, email: body.email ?? u.email } : u,
      )
      const updated = users.find((u) => u.id === userId)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok', data: updated }),
      })
    } else if (route.request().method() === 'DELETE') {
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

  // Roles
  page.route('**/api/roles**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: MOCK_ROLES }),
    })
  })
}

test.describe('用户管理 CRUD', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockUserApis(page)
  })

  test('导航到用户管理页并验证列表渲染', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 导航到用户管理页
    await page.getByRole('button', { name: '设置' }).click()
    const nav = page.locator('nav')
    await nav.getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 验证页面标题
    await expect(page.getByText('用户管理')).toBeVisible()

    // 验证用户列表渲染
    await expect(page.getByRole('table')).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '用户名' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '角色' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '邮箱' })).toBeVisible()

    // 验证用户数据
    await expect(page.getByText('admin')).toBeVisible()
    await expect(page.getByText('developer')).toBeVisible()
    await expect(page.getByText('dba_user')).toBeVisible()
    await expect(page.getByText('viewer')).toBeVisible()
  })

  test('用户列表显示角色信息', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 验证角色标签
    await expect(page.getByText('管理员')).toBeVisible()
    await expect(page.getByText('开发人员')).toBeVisible()
    await expect(page.getByText('DBA')).toBeVisible()
  })

  test('创建新用户', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 点击创建按钮
    const createBtn = page.getByRole('button', { name: /新建|创建|添加/ })
    await expect(createBtn).toBeVisible()
    await createBtn.click()

    // 验证表单弹窗出现
    await expect(page.getByRole('dialog')).toBeVisible()

    // 填写用户信息
    await page.getByPlaceholder(/用户名/).fill('newuser')
    await page.getByPlaceholder(/邮箱/).fill('newuser@example.com')

    // 选择角色
    const roleSelect = page.getByRole('combobox').filter({ hasText: /选择角色|角色/ })
    await roleSelect.click()
    await page.getByRole('option', { name: /开发人员/ }).click()

    // 提交
    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // 验证创建成功
    await expect(page.getByText('newuser')).toBeVisible()
    await expect(page.getByText('newuser@example.com')).toBeVisible()

    // 验证列表数量增加
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows).toHaveCount(5) // 4 original + 1 new
  })

  test('编辑用户信息', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 找到 viewer 用户行，点击编辑
    const viewerRow = page.getByRole('row', { name: /viewer/ })
    const editBtn = viewerRow.getByRole('button', { name: /编辑/ })
    await expect(editBtn).toBeVisible()
    await editBtn.click()

    // 验证编辑弹窗
    await expect(page.getByRole('dialog')).toBeVisible()

    // 修改邮箱
    const emailInput = page.getByPlaceholder(/邮箱/)
    await emailInput.clear()
    await emailInput.fill('viewer_updated@example.com')

    // 修改角色
    const roleSelect = page.getByRole('combobox').filter({ hasText: /开发人员/ })
    await roleSelect.click()
    await page.getByRole('option', { name: /DBA/ }).click()

    // 提交
    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // 验证修改成功
    await expect(page.getByText('viewer_updated@example.com')).toBeVisible()
  })

  test('删除用户', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 验证 viewer 存在
    await expect(page.getByText('viewer')).toBeVisible()
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows).toHaveCount(4)

    // 点击删除按钮
    const viewerRow = page.getByRole('row', { name: /viewer/ })
    const deleteBtn = viewerRow.getByRole('button', { name: /删除/ })
    await expect(deleteBtn).toBeVisible()
    await deleteBtn.click()

    // 确认删除（确认对话框）
    const confirmBtn = page.getByRole('button', { name: /确认|确定/ })
    if (await confirmBtn.isVisible()) {
      await confirmBtn.click()
    }

    // 验证删除成功
    await expect(page.getByText('viewer')).not.toBeVisible()
    const remainingRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(remainingRows).toHaveCount(3)
  })

  test('角色分配 - 创建用户时选择 DBA 角色', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 创建新用户
    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder(/用户名/).fill('new_dba')
    await page.getByPlaceholder(/邮箱/).fill('newdba@example.com')

    // 选择 DBA 角色
    const roleSelect = page.getByRole('combobox').filter({ hasText: /选择角色|角色/ })
    await roleSelect.click()
    await page.getByRole('option', { name: /DBA/ }).click()

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // 验证新用户的角色
    await expect(page.getByText('new_dba')).toBeVisible()
    // 验证角色标签为 DBA（在同一行）
    const dbaRow = page.getByRole('row', { name: /new_dba/ })
    await expect(dbaRow.getByText('DBA')).toBeVisible()
  })

  test('非管理员无法访问用户管理页', async ({ page }) => {
    // Override auth to return developer role
    page.route('**/api/auth/me', async (route) => {
      const authHeader = await route.request().headerValue('authorization')
      if (authHeader) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 0, data: MOCK_USERS.developer }),
        })
      } else {
        await route.fulfill({ status: 401, contentType: 'application/json', body: '{}' })
      }
    })

    // Override users endpoint to return 403
    page.route(/\/api\/users(\?.*)?$/, async (route) => {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    })

    await loginViaUI(page)

    // 尝试直接导航到用户管理页
    await page.goto('/admin/users')

    // 应该被重定向或显示 403
    await page.waitForURL('**/403**', { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    expect(is403).toBe(true)
  })
})
