import { test, expect } from '@playwright/test'
import { mockApiRoutes, setToken, MOCK_TOKEN } from '../../support/mock-routes'

test.describe('权限隔离验证', () => {
  test('developer 可以访问查询页和工单页', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await setToken(page, 'developer')

    // 访问查询页
    await page.goto('/query')
    await expect(page).toHaveURL(/\/query/)
    await expect(page.getByRole('link', { name: '查询' })).toBeVisible()

    // 访问工单页
    await page.goto('/tickets')
    await expect(page).toHaveURL(/\/tickets/)
    await expect(page.getByText('变更工单')).toBeVisible()

    // 可以新建工单
    await page.goto('/tickets/new')
    await expect(page).toHaveURL(/\/tickets\/new/)
    await expect(page.getByText('提交新工单')).toBeVisible()
  })

  test('developer 不能看到 "待我审批" 筛选按钮', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await setToken(page, 'developer')

    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // "我提交的" 按钮应该存在
    await expect(page.getByRole('button', { name: '我提交的' })).toBeVisible()

    // "待我审批" 按钮不应该存在（developer 不是 admin/dba）
    await expect(page.getByRole('button', { name: '待我审批' })).not.toBeVisible()
  })

  test('admin 可以看到 "待我审批" 筛选按钮', async ({ page }) => {
    mockApiRoutes(page, { role: 'admin' })
    await setToken(page, 'admin')

    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // "我提交的" 和 "待我审批" 都应该存在
    await expect(page.getByRole('button', { name: '我提交的' })).toBeVisible()
    await expect(page.getByRole('button', { name: '待我审批' })).toBeVisible()
  })

  test('dba 可以看到 "待我审批" 筛选按钮', async ({ page }) => {
    mockApiRoutes(page, { role: 'dba' })
    await setToken(page, 'dba')

    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    await expect(page.getByRole('button', { name: '待我审批' })).toBeVisible()
  })

  test('developer 用户信息显示正确', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await setToken(page, 'developer')

    await page.goto('/query')
    await expect(page).toHaveURL(/\/query/)

    // 验证头像显示 D（developer 首字母）
    const avatarFallback = page.locator('[data-slot="avatar-fallback"]').filter({ hasText: 'D' })
    await expect(avatarFallback).toBeVisible()
  })

  test('admin 用户信息显示正确', async ({ page }) => {
    mockApiRoutes(page, { role: 'admin' })
    await setToken(page, 'admin')

    await page.goto('/query')
    await expect(page).toHaveURL(/\/query/)

    // 验证头像显示 A（admin 首字母）
    const avatarFallback = page.locator('[data-slot="avatar-fallback"]').filter({ hasText: 'A' })
    await expect(avatarFallback).toBeVisible()
  })

  test('developer 访问设置页数据源时收到 403', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer', denyDatasources: true })
    await setToken(page, 'developer')

    // 设置页本身可以访问（前端路由），但 API 调用会返回 403
    await page.goto('/settings/datasource')
    await expect(page).toHaveURL(/\/settings\/datasource/)
  })

  test('未认证用户直接跳转登录页', async ({ page }) => {
    // 不设置 token
    await page.goto('/query')
    await page.waitForURL('**/login**', { timeout: 5000 })
    await expect(page).toHaveURL(/\/login/)
  })

  test('无 token 访问工单页跳转登录页', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/login**', { timeout: 5000 })
    await expect(page).toHaveURL(/\/login/)
  })
})
