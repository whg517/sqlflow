import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, setToken, MOCK_TOKEN } from './helpers'

test.describe('认证与登录流程', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
  })

  test('登录页面正确渲染', async ({ page }) => {
    await page.goto('/login')

    // 验证品牌标识
    await expect(page.getByText('SQLFlow', { exact: true })).toBeVisible()
    await expect(page.getByText('SQL 审批管理平台')).toBeVisible()

    // 验证表单元素
    await expect(page.getByPlaceholder('用户名')).toBeVisible()
    await expect(page.getByPlaceholder('密码')).toBeVisible()
    await expect(page.getByRole('button', { name: '登 录' })).toBeVisible()

    // 验证版权信息
    await expect(page.getByText('© 2026 SQLFlow')).toBeVisible()
  })

  test('使用正确凭据成功登录', async ({ page }) => {
    await loginViaUI(page)

    // 验证跳转到查询页
    await expect(page).toHaveURL(/\/query/)
    await expect(page.getByText('SQLFlow', { exact: false }).first()).toBeVisible()
  })

  test('空表单提交显示验证错误', async ({ page }) => {
    await page.goto('/login')

    // 不输入任何内容直接点击登录
    await page.getByRole('button', { name: '登 录' }).click()

    // 验证前端验证错误提示
    await expect(page.getByText('请输入用户名')).toBeVisible()
    await expect(page.getByText('请输入密码')).toBeVisible()
  })

  test('用户名过短显示验证错误', async ({ page }) => {
    await page.goto('/login')

    await page.getByPlaceholder('用户名').fill('ab')
    await page.getByPlaceholder('用户名').blur()

    await expect(page.getByText('用户名需 3-32 个字符')).toBeVisible()
  })

  test('密码过短显示验证错误', async ({ page }) => {
    await page.goto('/login')

    await page.getByPlaceholder('密码').fill('short')
    await page.getByPlaceholder('密码').blur()

    await expect(page.getByText('密码需 8-128 个字符')).toBeVisible()
  })

  test('错误凭据显示服务端错误', async ({ page }) => {
    // 覆盖 mock：让登录请求返回非 200 状态码，触发客户端 catch
    await page.route('**/api/auth/login', async (route) => {
      await route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ code: 1, message: '用户名或密码错误' }),
      })
    })

    await page.goto('/login')

    await page.getByPlaceholder('用户名').fill('wronguser')
    await page.getByPlaceholder('密码').fill('wrongpassword123')
    await page.getByRole('button', { name: '登 录' }).click()

    // 验证服务端返回的错误消息被显示
    await expect(page.getByText('用户名或密码错误')).toBeVisible()
  })

  test('未登录访问受保护页面重定向到登录页', async ({ page }) => {
    await page.goto('/query')
    await page.waitForURL('**/login**')
    await expect(page).toHaveURL(/\/login/)
  })

  test('未登录访问工单页重定向到登录页', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/login**')
    await expect(page).toHaveURL(/\/login/)
  })

  test('未登录访问设置页重定向到登录页', async ({ page }) => {
    await page.goto('/settings')
    await page.waitForURL('**/login**')
    await expect(page).toHaveURL(/\/login/)
  })

  test('无效 token 触发 401 自动跳转到登录页', async ({ page }) => {
    await page.goto('/login')
    await page.evaluate(() => {
      localStorage.setItem('token', 'invalid-expired-token')
    })

    await page.goto('/query')

    // 等待页面被 401 拦截器重定向到登录页
    await page.waitForFunction(() => window.location.pathname === '/login', { timeout: 10000 })
    expect(page.url()).toContain('/login')
  })

  test('登录成功后 token 被保存到 localStorage', async ({ page }) => {
    await page.goto('/login')
    await page.getByPlaceholder('用户名').fill('admin')
    await page.getByPlaceholder('密码').fill('admin123')
    await page.getByRole('button', { name: '登 录' }).click()

    await page.waitForURL('**/query**')

    const token = await page.evaluate(() => localStorage.getItem('token'))
    expect(token).toBe(MOCK_TOKEN)
  })

  test('登录后登出清除 token', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 确保没有已打开的下拉菜单
    await page.locator('[data-radix-popper-content-wrapper]').first().waitFor({ state: 'hidden' }).catch(() => {})

    // 点击头像按钮打开下拉菜单
    const avatarTrigger = page.locator('button').filter({ has: page.locator('[data-slot="avatar-fallback"]') }).first()
    await avatarTrigger.click()

    // 点击退出登录
    await page.getByText('退出登录').click()

    // 验证跳转到登录页
    await page.waitForURL('**/login**', { timeout: 5000 })
    await expect(page).toHaveURL(/\/login/)

    // 验证 token 已清除
    const token = await page.evaluate(() => localStorage.getItem('token'))
    expect(token).toBeNull()
  })

  test('登录按钮在提交时显示加载状态', async ({ page }) => {
    // 延迟 API 响应以观察加载状态
    await page.route('**/api/auth/login', async (route) => {
      await new Promise((r) => setTimeout(r, 500))
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: { token: MOCK_TOKEN } }),
      })
    })

    await page.goto('/login')
    await page.getByPlaceholder('用户名').fill('admin')
    await page.getByPlaceholder('密码').fill('admin123')
    await page.getByRole('button', { name: '登 录' }).click()

    // 验证按钮显示加载状态
    await expect(page.getByRole('button', { name: '登录中...' })).toBeVisible()

    // 等待加载完成
    await page.waitForURL('**/query**')
  })

  test('Tab 键在表单字段间正确切换', async ({ page }) => {
    await page.goto('/login')

    const usernameInput = page.getByPlaceholder('用户名')
    const passwordInput = page.getByPlaceholder('密码')
    const loginButton = page.getByRole('button', { name: '登 录' })

    await usernameInput.focus()
    await page.keyboard.press('Tab')

    // 焦点应移到密码框
    await expect(passwordInput).toBeFocused()

    await page.keyboard.press('Tab')

    // 焦点应移到登录按钮
    await expect(loginButton).toBeFocused()
  })

  test('登录页 URL 直接访问不重定向', async ({ page }) => {
    await page.goto('/login')
    await expect(page).toHaveURL(/\/login/)
    await expect(page.getByText('SQLFlow', { exact: true })).toBeVisible()
  })
})
