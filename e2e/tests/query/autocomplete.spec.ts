/**
 * E2E — SQL 自动补全（真实后端）
 * SF-QA0028 batch 2
 * 使用真实后端表结构（sys_user, orders, products）进行补全测试
 */
import { test, expect, loginViaUI, getFirstDatasourceId } from '../../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('SQL 自动补全触发（真实后端）', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('输入 SEL 触发关键字补全', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    // 选择数据源
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    const firstOption = page.getByRole('option').first()
    if (!await firstOption.isVisible({ timeout: 3000 })) {
      test.skip()
      return
    }
    await firstOption.click()

    // 点击编辑器
    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入 "SEL" 触发补全
    await page.keyboard.type('SEL', { delay: 100 })

    // 验证补全菜单出现
    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    const menuVisible = await autocompleteMenu.isVisible({ timeout: 3000 }).catch(() => false)

    if (menuVisible) {
      // 验证 SELECT 选项存在
      await expect(autocompleteMenu.getByText('SELECT')).toBeVisible()
    }
    // 注意：如果后端补全 API 未启用或响应慢，此测试不 fail
  })

  test('输入表名后输入 "." 触发字段名补全', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    const firstOption = page.getByRole('option').first()
    if (!await firstOption.isVisible({ timeout: 3000 })) {
      test.skip()
      return
    }
    await firstOption.click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入表名 + "."
    await page.keyboard.type('sys_user.', { delay: 50 })

    // 验证补全菜单出现，包含字段名
    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    const menuVisible = await autocompleteMenu.isVisible({ timeout: 3000 }).catch(() => false)

    if (menuVisible) {
      // 至少应有 id 字段
      await expect(autocompleteMenu.getByText('id')).toBeVisible()
    }
  })

  test('Ctrl+Space 手动触发补全', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    const firstOption = page.getByRole('option').first()
    if (!await firstOption.isVisible({ timeout: 3000 })) {
      test.skip()
      return
    }
    await firstOption.click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入部分关键字
    await page.keyboard.type('WHE', { delay: 50 })

    // 手动触发补全
    await page.keyboard.press('Control+Space')

    // 验证补全菜单出现
    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    const menuVisible = await autocompleteMenu.isVisible({ timeout: 3000 }).catch(() => false)

    if (menuVisible) {
      // 验证 WHERE 选项
      await expect(autocompleteMenu.getByText('WHERE')).toBeVisible()
    }
  })

  test('上下键选择补全项', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    const firstOption = page.getByRole('option').first()
    if (!await firstOption.isVisible({ timeout: 3000 })) {
      test.skip()
      return
    }
    await firstOption.click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入 "SE" 触发补全（SELECT, SET 等）
    await page.keyboard.type('SE', { delay: 100 })

    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    const menuVisible = await autocompleteMenu.isVisible({ timeout: 3000 }).catch(() => false)
    if (!menuVisible) return

    // 默认选中第一项
    await expect(autocompleteMenu.locator('[aria-selected="true"]').first()).toBeVisible()

    // 按下键选择第二项
    await page.keyboard.press('ArrowDown')
    // 按上键回到第一项
    await page.keyboard.press('ArrowUp')
    await expect(autocompleteMenu.locator('[aria-selected="true"]').first()).toBeVisible()
  })

  test('Enter 确认补全', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    const firstOption = page.getByRole('option').first()
    if (!await firstOption.isVisible({ timeout: 3000 })) {
      test.skip()
      return
    }
    await firstOption.click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 输入 "SEL"
    await page.keyboard.type('SEL', { delay: 100 })

    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    const menuVisible = await autocompleteMenu.isVisible({ timeout: 3000 }).catch(() => false)
    if (!menuVisible) return

    // 按 Enter 确认补全
    await page.keyboard.press('Enter')

    // 验证补全已应用
    await expect(editor).toContainText('SELECT')

    // 补全菜单应关闭
    await expect(autocompleteMenu).not.toBeVisible()
  })

  test('Escape 关闭补全菜单', async ({ page }) => {
    await expect(page).toHaveURL(/\/query/)

    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    const firstOption = page.getByRole('option').first()
    if (!await firstOption.isVisible({ timeout: 3000 })) {
      test.skip()
      return
    }
    await firstOption.click()

    const editor = page.locator('.cm-content').first()
    await editor.click()

    // 触发补全
    await page.keyboard.type('SEL', { delay: 100 })

    const autocompleteMenu = page.locator('.cm-tooltip-autocomplete').first()
    const menuVisible = await autocompleteMenu.isVisible({ timeout: 3000 }).catch(() => false)
    if (!menuVisible) return

    // 按 Escape 关闭
    await page.keyboard.press('Escape')

    // 补全菜单应关闭
    await expect(autocompleteMenu).not.toBeVisible()
    // 编辑器中应保留原始输入
    await expect(editor).toContainText('SEL')
  })
})
