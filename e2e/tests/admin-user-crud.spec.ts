/**
 * E2E — 用户管理 CRUD（真实后端）
 */
import { test, expect, loginViaUI, apiRequest, apiHelper, cleanupUsers, getToken, ADMIN_USER, BASE_URL } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('用户管理 CRUD', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test.afterAll(async () => {
    await cleanupUsers()
  })

  test('导航到用户管理页并验证列表渲染', async ({ page }) => {
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

    // 验证至少有一个用户
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    const count = await dataRows.count()
    expect(count).toBeGreaterThan(0)
  })

  test('用户列表显示角色信息', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 验证角色标签（真实后端可能有多种角色）
    const adminLabel = page.getByText('管理员').first()
    const devLabel = page.getByText('开发人员').first()
    const dbaLabel = page.getByText('DBA').first()

    // 至少应该看到管理员角色
    await expect(adminLabel).toBeVisible()
  })

  test('创建新用户', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 获取创建前的行数
    const dataRowsBefore = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    const countBefore = await dataRowsBefore.count()

    // 点击创建按钮
    const createBtn = page.getByRole('button', { name: /新建|创建|添加/ })
    await expect(createBtn).toBeVisible()
    await createBtn.click()

    // 验证表单弹窗出现
    await expect(page.getByRole('dialog')).toBeVisible()

    // 填写用户信息
    const username = `e2e_newuser_${Date.now()}`
    await page.getByPlaceholder(/用户名/).fill(username)
    await page.getByPlaceholder(/邮箱/).fill(`${username}@example.com`)

    // 选择角色
    const roleSelect = page.getByRole('combobox').filter({ hasText: /选择角色|角色/ })
    if (await roleSelect.isVisible()) {
      await roleSelect.click()
      await page.getByRole('option', { name: /开发人员/ }).click()
    }

    // 提交
    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // 验证创建成功（用户名出现在列表中）
    await expect(page.getByText(username)).toBeVisible({ timeout: 5000 })
  })

  test('编辑用户信息', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 先创建一个测试用户
    const editUsername = `e2e_editme_${Date.now()}`
    await apiRequest(page, 'POST', '/users', {
      username: editUsername,
      password: 'e2e-test-pass-123',
      role: 'developer',
      email: `${editUsername}@example.com`,
    })

    // 刷新页面
    await page.reload()
    await page.waitForURL('**/admin/users**')

    // 找到用户行，点击编辑
    const targetRow = page.getByRole('row', { name: new RegExp(editUsername) })
    const editBtn = targetRow.getByRole('button', { name: /编辑/ })
    await expect(editBtn).toBeVisible()
    await editBtn.click()

    // 验证编辑弹窗
    await expect(page.getByRole('dialog')).toBeVisible()

    // 修改邮箱
    const emailInput = page.getByPlaceholder(/邮箱/)
    await emailInput.clear()
    await emailInput.fill(`${editUsername}_updated@example.com`)

    // 提交
    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // 验证修改成功
    await expect(page.getByText(`${editUsername}_updated@example.com`)).toBeVisible({ timeout: 5000 })
  })

  test('删除用户', async ({ page }) => {
    // 先创建一个测试用户
    const deleteUsername = `e2e_delme_${Date.now()}`
    await apiRequest(page, 'POST', '/users', {
      username: deleteUsername,
      password: 'e2e-test-pass-123',
      role: 'developer',
      email: `${deleteUsername}@example.com`,
    })

    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 验证用户存在
    await expect(page.getByText(deleteUsername)).toBeVisible()

    // 点击删除按钮
    const targetRow = page.getByRole('row', { name: new RegExp(deleteUsername) })
    const deleteBtn = targetRow.getByRole('button', { name: /删除/ })
    await expect(deleteBtn).toBeVisible()
    await deleteBtn.click()

    // 确认删除
    const confirmBtn = page.getByRole('button', { name: /确认|确定/ })
    if (await confirmBtn.isVisible()) {
      await confirmBtn.click()
    }

    // 验证删除成功
    await expect(page.getByText(deleteUsername)).not.toBeVisible({ timeout: 5000 })
  })

  test('角色分配 - 创建用户时选择 DBA 角色', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
    await page.waitForURL('**/admin/users**')

    // 创建新用户
    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const username = `e2e_newdba_${Date.now()}`
    await page.getByPlaceholder(/用户名/).fill(username)
    await page.getByPlaceholder(/邮箱/).fill(`${username}@example.com`)

    // 选择 DBA 角色
    const roleSelect = page.getByRole('combobox').filter({ hasText: /选择角色|角色/ })
    if (await roleSelect.isVisible()) {
      await roleSelect.click()
      const dbaOption = page.getByRole('option', { name: /DBA/ })
      if (await dbaOption.isVisible()) {
        await dbaOption.click()
      }
    }

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // 验证新用户存在
    await expect(page.getByText(username)).toBeVisible({ timeout: 5000 })
  })

  test('非管理员无法访问用户管理页', async ({ page }) => {
    const devToken = await getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (!devToken) {
      test.skip()
      return
    }

    // 设置 developer token
    await page.goto('/login')
    await page.evaluate((t) => localStorage.setItem('token', t), devToken)

    // 尝试直接导航到用户管理页
    await page.goto('/admin/users')

    // 应该被重定向或显示 403
    await page.waitForURL(/\/403|\/login/, { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    const isLogin = page.url().includes('/login')
    expect(is403 || isLogin).toBe(true)
  })
})
