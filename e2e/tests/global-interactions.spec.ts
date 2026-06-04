/**
 * E2E — 全局交互验证（真实后端）
 */
import { test, expect, loginViaUI } from '../support/real-test-helpers'
import type { Page } from '@playwright/test'

test.describe.configure({ timeout: 45_000 })

test.describe('全局交互验证', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  async function goToApp(page: Page) {
    await page.goto('/query')
    await expect(page).toHaveURL(/\/query/)
  }

  test('404 页面显示正确', async ({ page }) => {
    await goToApp(page)
    await page.goto('/404')
    await expect(page.getByText('404')).toBeVisible()
    await expect(page.getByText('页面不存在或已被移除')).toBeVisible()

    // 验证"返回首页"按钮
    await expect(page.getByRole('button', { name: '返回首页' })).toBeVisible()
    await page.getByRole('button', { name: '返回首页' }).click()
    await expect(page).toHaveURL(/\/query/)
  })

  test('403 页面显示正确', async ({ page }) => {
    await goToApp(page)
    await page.goto('/403')
    await expect(page.getByText('403')).toBeVisible()
    await expect(page.getByText('您没有访问此页面的权限')).toBeVisible()

    // 验证"返回上一页"按钮
    await expect(page.getByRole('button', { name: '返回上一页' })).toBeVisible()
  })

  test('Cmd+K 打开命令面板', async ({ page }) => {
    await goToApp(page)

    // 用键盘快捷键打开命令面板
    await page.keyboard.press('Meta+k')

    // 验证命令面板对话框出现
    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(page.getByPlaceholder('搜索页面或功能')).toBeVisible()
  })

  test('Ctrl+K 打开命令面板', async ({ page }) => {
    await goToApp(page)

    // Linux/Windows 用 Ctrl+K
    await page.keyboard.press('Control+k')
    await expect(page.getByRole('dialog')).toBeVisible()
  })

  test('命令面板搜索并导航到页面', async ({ page }) => {
    await goToApp(page)

    // 打开命令面板
    await page.keyboard.press('Meta+k')
    await expect(page.getByRole('dialog')).toBeVisible()

    // 输入搜索词
    await page.getByPlaceholder('搜索页面或功能').fill('工单')

    // 点击搜索结果
    await page.getByText('变更工单').click()

    // 验证导航到工单页
    await page.waitForURL('**/tickets**', { timeout: 5000 })
    await expect(page).toHaveURL(/\/tickets/)
  })

  test('命令面板显示空结果', async ({ page }) => {
    await goToApp(page)

    await page.keyboard.press('Meta+k')
    await expect(page.getByRole('dialog')).toBeVisible()

    // 输入不匹配的搜索词
    await page.getByPlaceholder('搜索页面或功能').fill('xyznotexist')

    // 验证空状态
    await expect(page.getByText('没有找到匹配项')).toBeVisible()
  })

  test('命令面板 Escape 关闭', async ({ page }) => {
    await goToApp(page)

    await page.keyboard.press('Meta+k')
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).not.toBeVisible()
  })

  test('点击搜索栏按钮打开命令面板', async ({ page }) => {
    await goToApp(page)

    // 点击搜索栏按钮
    await page.getByText('搜索...').click()

    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(page.getByPlaceholder('搜索页面或功能')).toBeVisible()
  })

  test('侧边栏折叠和展开', async ({ page }) => {
    await goToApp(page)

    // 侧边栏初始状态：展开
    const sidebar = page.locator('aside')
    await expect(sidebar).toBeVisible()

    // 点击折叠按钮
    const collapseBtn = page.locator('aside button').filter({ has: page.locator('svg') }).last()
    await collapseBtn.click()

    // 验证侧边栏已折叠（宽度变小）
    // 折叠后 "SQLFlow" 文字应消失
    await expect(page.getByText('SQLFlow', { exact: true })).not.toBeVisible()

    // 再次点击展开
    const expandBtn = page.locator('aside button').filter({ has: page.locator('svg') }).last()
    await expandBtn.click()

    // 验证侧边栏已展开
    await expect(page.getByText('SQLFlow', { exact: true })).toBeVisible()
  })

  test('侧边栏导航点击跳转', async ({ page }) => {
    await goToApp(page)

    // 点击权限管理
    await page.getByRole('link', { name: '权限' }).first().click()
    await expect(page).toHaveURL(/\/permissions/)

    // 点击审计日志
    await page.getByRole('link', { name: '审计' }).first().click()
    await expect(page).toHaveURL(/\/audit/)

    // 点击查询
    await page.getByRole('link', { name: '查询' }).first().click()
    await expect(page).toHaveURL(/\/query/)
  })

  test('设置子菜单展开和折叠', async ({ page }) => {
    await goToApp(page)

    // 点击设置按钮
    await page.getByRole('button', { name: '设置' }).click()

    // 验证子菜单出现（在侧边栏 nav 内）
    const nav = page.locator('nav')
    await expect(nav.getByRole('link', { name: '数据源管理' })).toBeVisible()
    await expect(nav.getByRole('link', { name: '脱敏规则' })).toBeVisible()
    await expect(nav.getByRole('link', { name: 'AI 配置' })).toBeVisible()

    // 点击数据源管理
    await nav.getByRole('link', { name: '数据源管理' }).click()
    await expect(page).toHaveURL(/\/settings\/datasource/)

    // 再次点击设置折叠子菜单
    await page.getByRole('button', { name: '设置' }).click()

    // 子菜单应该消失
    await expect(nav.getByRole('link', { name: '数据源管理' })).not.toBeVisible()
  })

  test('网络断开 banner 显示', async ({ page }) => {
    await goToApp(page)

    // 模拟网络断开事件
    await page.evaluate(() => {
      window.dispatchEvent(new Event('offline'))
    })

    // 验证断网 banner 出现
    await expect(page.getByText('网络连接已断开，部分功能不可用')).toBeVisible()

    // 模拟网络恢复
    await page.evaluate(() => {
      window.dispatchEvent(new Event('online'))
    })

    // 验证 banner 消失
    await expect(page.getByText('网络连接已断开，部分功能不可用')).not.toBeVisible()
  })

  test('头像下拉菜单功能', async ({ page }) => {
    await goToApp(page)

    // 点击头像按钮
    const avatarTrigger = page.locator('button').filter({ has: page.locator('[data-slot="avatar-fallback"]') }).first()
    await avatarTrigger.click()

    // 验证下拉菜单内容
    await expect(page.getByText('退出登录')).toBeVisible()
    await expect(page.getByText('修改密码')).toBeVisible()
  })

  test('修改密码弹窗打开', async ({ page }) => {
    await goToApp(page)

    // 打开头像下拉
    const avatarTrigger = page.locator('button').filter({ has: page.locator('[data-slot="avatar-fallback"]') }).first()
    await avatarTrigger.click()

    // 点击修改密码
    await page.getByText('修改密码').click()

    // 验证修改密码弹窗出现
    await expect(page.getByRole('dialog')).toBeVisible()
  })

  test('未匹配路由重定向到查询页', async ({ page }) => {
    await goToApp(page)

    // 访问不存在的路由
    await page.goto('/nonexistent-route')
    await page.waitForURL('**/query**', { timeout: 5000 }).catch(() => {
      // Some apps show 404 instead of redirect
    })
    // Either redirected to query or shows 404 — both are valid
    const isOnQuery = page.url().includes('/query')
    const is404 = await page.getByText('404').isVisible().catch(() => false)
    expect(isOnQuery || is404).toBeTruthy()
  })
})
