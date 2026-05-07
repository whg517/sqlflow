import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, setToken } from './helpers'

test.describe('Login → Query → Ticket 完整流程', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
  })

  test('完整流程：登录 → 查询 → 创建工单', async ({ page }) => {
    // ========== 1. 登录 ==========
    await page.goto('/login')

    // 验证登录页元素
    await expect(page.getByText('SQLFlow', { exact: true })).toBeVisible()
    await expect(page.getByText('SQL 审批管理平台')).toBeVisible()
    await expect(page.getByPlaceholder('用户名')).toBeVisible()
    await expect(page.getByPlaceholder('密码')).toBeVisible()

    // 输入凭据并提交
    await page.getByPlaceholder('用户名').fill('admin')
    await page.getByPlaceholder('密码').fill('admin123')
    await page.getByRole('button', { name: '登 录' }).click()

    // 验证跳转到查询页
    await page.waitForURL('**/query**')
    await expect(page).toHaveURL(/\/query/)

    // ========== 2. 查询页 ==========
    // 验证侧边栏导航
    await expect(page.getByText('SQLFlow', { exact: false }).first()).toBeVisible()
    await expect(page.getByRole('link', { name: '查询' })).toBeVisible()

    // 验证顶部搜索框
    await expect(page.getByText('搜索...')).toBeVisible()

    // 验证用户头像下拉
    const avatarButton = page.locator('button').filter({ hasText: /^A$/ }).first()
    await expect(avatarButton).toBeVisible()

    // ========== 3. 导航到工单页 ==========
    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')
    await expect(page).toHaveURL(/\/tickets/)

    // 验证工单页标题
    await expect(page.getByText('变更工单')).toBeVisible()

    // 验证工单列表有数据
    await expect(page.getByText('#1')).toBeVisible()

    // ========== 4. 新建工单 ==========
    await page.getByRole('button', { name: '提交新工单' }).click()
    await page.waitForURL('**/tickets/new**')

    // 验证新工单页标题
    await expect(page.getByText('提交新工单')).toBeVisible()

    // 填写工单表单
    // 选择数据源
    await page.getByText('选择数据源').click()
    await page.getByText('test-mysql').click()

    // 输入数据库
    await page.getByPlaceholder('输入数据库名').fill('testdb')

    // 输入 SQL
    await page.getByPlaceholder('输入要执行的 SQL 语句').fill('SELECT 1')

    // 输入变更原因
    await page.getByPlaceholder('请说明此次变更的原因').fill('Test ticket for E2E testing flow')

    // 提交
    await page.getByRole('button', { name: '提交工单' }).click()

    // 验证跳转回工单列表
    await page.waitForURL('**/tickets**', { timeout: 5000 })
    await expect(page).toHaveURL(/\/tickets/)
  })

  test('未登录访问受保护页面跳转到登录页', async ({ page }) => {
    // 不设置 token，直接访问 /query
    await page.goto('/query')
    await page.waitForURL('**/login**')
    await expect(page).toHaveURL(/\/login/)
  })

  test('登录后 401 自动跳转到登录页', async ({ page }) => {
    // 设置无效 token
    await page.goto('/login')
    await page.evaluate(() => {
      localStorage.setItem('token', 'invalid-token')
    })

    // 访问受保护页面，API /auth/me 返回 401 会触发 window.location.href = '/login'
    await page.goto('/query')

    // 等待页面被 401 拦截器重定向到登录页
    await page.waitForFunction(() => window.location.pathname === '/login', { timeout: 10000 })
    expect(page.url()).toContain('/login')
  })

  test('登录页表单验证', async ({ page }) => {
    await page.goto('/login')

    // 不输入任何内容直接点击登录
    await page.getByRole('button', { name: '登 录' }).click()

    // 验证验证错误提示
    await expect(page.getByText('请输入用户名')).toBeVisible()
    await expect(page.getByText('请输入密码')).toBeVisible()
  })

  test('登录后登出清除 token', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 点击头像下拉
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
})
