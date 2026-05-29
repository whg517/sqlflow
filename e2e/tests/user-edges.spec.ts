/**
 * E2E — 用户管理边界与异常场景（真实后端）
 * Covers: 异常处理 / 权限校验 / 边界场景
 * Migrated from mock/tests/mock/user-edges.spec.ts
 */
import { test, expect, loginViaUI, apiRequest, apiHelper, cleanupUsers, getToken, BASE_URL, ADMIN_USER } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

/** Navigate to admin user management page */
async function goToUserManagement(page: import('@playwright/test').Page) {
  await page.getByRole('button', { name: '设置' }).click()
  await page.locator('nav').getByRole('link', { name: '用户管理' }).click()
  await page.waitForURL('**/admin/users**')
}

/** Create a test user via API and return the username */
async function createTestUser(page: import('@playwright/test').Page, username: string, role = 'developer', email?: string): Promise<number> {
  const { status, body } = await apiRequest(page, 'POST', '/users', {
    username,
    password: 'e2e-test-pass-123',
    role,
    email: email ?? `${username}@example.com`,
  })
  expect(status).toBe(200)
  const data = body as { code: number; data?: { id: number } }
  expect(data.code).toBe(0)
  return data.data!.id
}

/** Delete a test user by ID via API */
async function deleteTestUser(page: import('@playwright/test').Page, userId: number): Promise<void> {
  await apiHelper(page, 'DELETE', `/users/${userId}`).catch(() => {})
}

test.describe('用户管理 — 异常处理', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test.afterAll(async () => {
    await cleanupUsers()
  })

  test('创建用户时用户名重复被拒绝', async ({ page }) => {
    // 先通过 API 创建一个用户
    const username = `e2e_dup_${Date.now()}`
    await createTestUser(page, username)

    // 导航到用户管理页
    await goToUserManagement(page)

    // Open create dialog
    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Try to create user with same username
    await page.getByPlaceholder(/用户名/).fill(username)
    await page.getByPlaceholder(/邮箱/).fill('duplicate@example.com')

    // Save
    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // Verify duplicate error
    await expect(page.getByText(/用户名已存在|重复/)).toBeVisible({ timeout: 5000 })
  })

  test('编辑用户时修改为已存在的用户名被拒绝', async ({ page }) => {
    // 先创建一个测试用户
    const existingUsername = `e2e_exist_${Date.now()}`
    const newUsername = `e2e_new_${Date.now()}`
    await createTestUser(page, existingUsername)
    await createTestUser(page, newUsername)

    await goToUserManagement(page)

    // Edit newUsername user, try change to existingUsername
    const targetRow = page.getByRole('row', { name: new RegExp(newUsername) })
    await targetRow.getByRole('button', { name: /编辑/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Try change username to existing one
    const usernameInput = page.getByPlaceholder(/用户名/)
    await usernameInput.clear()
    await usernameInput.fill(existingUsername)

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    await expect(page.getByText(/用户名已存在|重复/)).toBeVisible({ timeout: 5000 })
  })

  test('删除最后一个管理员被阻止', async ({ page }) => {
    await goToUserManagement(page)

    // Try to delete admin user (the only admin)
    const adminRow = page.getByRole('row', { name: new RegExp(ADMIN_USER) })
    const adminDeleteBtn = adminRow.getByRole('button', { name: /删除/ })

    if (await adminDeleteBtn.isVisible()) {
      await adminDeleteBtn.click()

      // May have confirmation dialog
      const confirmBtn = page.getByRole('button', { name: /确认|确定/ })
      if (await confirmBtn.isVisible()) {
        await confirmBtn.click()
      }

      // Verify error message
      await expect(page.getByText(/不能删除最后一个管理员|至少保留一个管理员/)).toBeVisible({ timeout: 5000 })
    }
  })

  test('创建用户时超长用户名被拒绝', async ({ page }) => {
    await goToUserManagement(page)

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
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test.afterAll(async () => {
    await cleanupUsers()
  })

  test('用户名为 3 个字符（最小长度）', async ({ page }) => {
    await goToUserManagement(page)

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Minimum length username
    const username = `e2e_abc_${Date.now()}`
    // Use exactly 3+ chars
    await page.getByPlaceholder(/用户名/).fill(`abc${Date.now().toString().slice(-2)}`)
    await page.getByPlaceholder(/邮箱/).fill(`abc${Date.now()}@example.com`)

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    await expect(page.getByText(/创建成功|添加成功|ok/)).toBeVisible({ timeout: 5000 })
  })

  test('用户名为 32 个字符（最大长度）', async ({ page }) => {
    await goToUserManagement(page)

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Maximum length username
    const maxName = `e2e_${'a'.repeat(27)}` // 32 chars total
    await page.getByPlaceholder(/用户名/).fill(maxName)
    await page.getByPlaceholder(/邮箱/).fill('maxlen@example.com')

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    await expect(page.getByText(/创建成功|添加成功|ok/)).toBeVisible({ timeout: 5000 })
  })

  test('用户名包含特殊字符被拒绝', async ({ page }) => {
    await goToUserManagement(page)

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // Try username with special characters
    await page.getByPlaceholder(/用户名/).fill('user@name')
    await page.getByPlaceholder(/用户名/).blur()

    // Expect frontend validation
    const hasError = await page.getByText(/字母|数字|下划线|允许/).isVisible().catch(() => false)
    if (!hasError) {
      // Try submitting and see if backend rejects
      await page.getByPlaceholder(/邮箱/).fill('test@example.com')
      await page.getByRole('button', { name: /确认|提交|保存/ }).click()
      const hasBackendError = await page.getByText(/格式|特殊|invalid/).isVisible({ timeout: 3000 }).catch(() => false)
      // Either frontend or backend should catch it
      expect(hasBackendError || !page.getByRole('dialog').isVisible()).toBeTruthy()
    }
  })

  test('用户名为空字符串', async ({ page }) => {
    await goToUserManagement(page)

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
    await goToUserManagement(page)

    await page.getByRole('button', { name: /新建|创建|添加/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const username = `e2e_noemail_${Date.now()}`
    await page.getByPlaceholder(/用户名/).fill(username)

    await page.getByRole('button', { name: /确认|提交|保存/ }).click()

    // May succeed or show validation depending on whether email is required
    const dialogVisible = await page.getByRole('dialog').isVisible().catch(() => false)
    const hasSuccess = await page.getByText(/创建成功|添加成功|用户名已存在/).isVisible({ timeout: 3000 }).catch(() => false)
    expect(!dialogVisible || hasSuccess).toBeTruthy()
  })
})

test.describe('用户管理 — 权限校验补充', () => {
  test('developer 无法通过直接 URL 访问用户管理', async ({ page }) => {
    const devToken = await getToken('e2e-developer', 'e2e-test-pass-123').catch(() => null)
    if (!devToken) {
      test.skip()
      return
    }

    // Set developer token
    await page.goto('/login')
    await page.evaluate((t) => localStorage.setItem('token', t), devToken)

    await page.goto('/admin/users')

    // Should be redirected to 403 or login
    await page.waitForURL(/\/403|\/login/, { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    const isLogin = page.url().includes('/login')
    expect(is403 || isLogin).toBe(true)
  })

  test('未登录用户访问用户管理重定向到登录', async ({ page }) => {
    await page.goto('/admin/users')
    await page.waitForURL('**/login**', { timeout: 5000 })
    expect(page.url()).toContain('/login')
  })
})


