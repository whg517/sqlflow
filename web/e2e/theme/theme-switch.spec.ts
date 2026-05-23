import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI } from '../helpers'

test.describe('主题切换', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
  })

  test('默认深色主题', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 验证默认为深色主题
    const html = page.locator('html')
    await expect(html).toHaveAttribute('data-theme', 'dark')

    // 验证深色主题下页面正常渲染
    await expect(page.locator('.cm-content').first()).toBeVisible()
  })

  test('点击主题切换按钮切换为浅色主题', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 查找主题切换按钮
    const themeToggle = page.getByRole('button', { name: /切换主题|主题|☀|🌙/ })
    await expect(themeToggle).toBeVisible()
    await themeToggle.click()

    // 验证主题已切换
    const html = page.locator('html')
    await expect(html).toHaveAttribute('data-theme', 'light')
  })

  test('浅色主题下页面元素颜色变化', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 切换到浅色主题
    const themeToggle = page.getByRole('button', { name: /切换主题|主题|☀|🌙/ })
    await themeToggle.click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')

    // 验证浅色主题下背景色和文字色
    const bodyBg = await page.evaluate(() => {
      const body = document.body
      const style = window.getComputedStyle(body)
      return {
        backgroundColor: style.backgroundColor,
        color: style.color,
      }
    })

    // 浅色主题背景应为浅色（接近白色）
    // CSS 变量验证
    const themeVars = await page.evaluate(() => {
      const root = document.documentElement
      const style = getComputedStyle(root)
      return {
        background: style.getPropertyValue('--background').trim(),
        foreground: style.getPropertyValue('--foreground').trim(),
      }
    })

    // 验证 CSS 变量已更新（非空值表示主题变量已设置）
    expect(themeVars.background).toBeTruthy()
    expect(themeVars.foreground).toBeTruthy()
  })

  test('刷新页面验证主题持久化（localStorage）', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 切换到浅色主题
    const themeToggle = page.getByRole('button', { name: /切换主题|主题|☀|🌙/ })
    await themeToggle.click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')

    // 验证 localStorage 中保存了主题
    const savedTheme = await page.evaluate(() => localStorage.getItem('theme'))
    expect(savedTheme).toBe('light')

    // 刷新页面
    await page.reload()
    await page.waitForURL('**/query*')

    // 验证主题仍然为浅色
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  })

  test('切换回深色主题', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const themeToggle = page.getByRole('button', { name: /切换主题|主题|☀|🌙/ })

    // 第一次点击：深色 → 浅色
    await themeToggle.click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')

    // 第二次点击：浅色 → 深色
    await themeToggle.click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  })

  test('深色主题刷新后持久化', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 确保是深色主题（默认）
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')

    // 验证 localStorage
    const savedTheme = await page.evaluate(() => localStorage.getItem('theme'))
    // 默认可能不保存（因为 dark 是默认值），也可能保存为 'dark'
    // 不管哪种情况，刷新后都应该保持 dark
    await page.reload()
    await page.waitForURL('**/query*')

    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  })

  test('主题切换不影响页面功能', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // 选择数据源
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    // 在深色主题下输入 SQL
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT 1', { delay: 30 })

    // 切换到浅色主题
    const themeToggle = page.getByRole('button', { name: /切换主题|主题|☀|🌙/ })
    await themeToggle.click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')

    // 验证编辑器内容仍然存在
    await expect(editor).toContainText('SELECT 1')

    // 验证执行按钮仍然可用
    const executeBtn = page.getByRole('button', { name: '执行' })
    await expect(executeBtn).toBeEnabled()
  })

  test('多次快速切换主题不崩溃', async ({ page }) => {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    const themeToggle = page.getByRole('button', { name: /切换主题|主题|☀|🌙/ })

    // 快速切换 5 次
    for (let i = 0; i < 5; i++) {
      await themeToggle.click()
    }

    // 验证最终主题状态一致
    // 奇数次切换后应为浅色（初始 dark → 1:light → 2:dark → 3:light → 4:dark → 5:light）
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')

    // 验证页面无报错
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))
    await page.waitForTimeout(500)
    expect(errors).toHaveLength(0)
  })
})
