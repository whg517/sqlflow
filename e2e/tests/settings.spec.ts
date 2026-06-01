/**
 * E2E — 系统设置管理（真实后端）
 *
 * Tests AI config, notification config, settings CRUD, permission checks.
 */
import { test, expect } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const ADMIN_USER = process.env.E2E_USERNAME ?? 'e2eadmin'
const ADMIN_PASS = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

async function loginReal(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill(ADMIN_USER)
  await page.getByPlaceholder('密码').fill(ADMIN_PASS)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 15_000 })
}

async function getSettingsViaApi(page: import('@playwright/test').Page) {
  const token = await page.evaluate(() => localStorage.getItem('token'))
  const res = await page.evaluate(async ({ baseUrl, token }) => {
    const r = await fetch(`${baseUrl}/api/settings`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    return await r.json()
  }, { baseUrl: BASE_URL, token })
  return res as { code: number; data: Record<string, unknown> }
}

// --- Tests ---

test.describe('系统设置 — 真实后端', () => {
  test.beforeEach(async ({ page }) => {
    await loginReal(page)
  })

  test('导航到 AI 配置页并验证表单渲染', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    await expect(page.getByText('AI 配置').first()).toBeVisible()
    await expect(page.getByText(/AI 提供商/)).toBeVisible()
    await expect(page.getByText(/AI 模型/)).toBeVisible()
    await expect(page.getByText(/API Key/)).toBeVisible()
  })

  test('AI 配置页 GET settings 返回有效数据', async ({ page }) => {
    const res = await getSettingsViaApi(page)
    expect(res.code).toBe(0)
    expect(res.data).toBeDefined()
    expect(res.data).toHaveProperty('ai_provider')
    expect(res.data).toHaveProperty('ai_model')
  })

  test('导航到通知配置页', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '通知配置' }).click()
    await page.waitForURL('**/settings/notification')

    await expect(page.getByText(/通知配置|通知/).first()).toBeVisible()
  })

  test('设置子导航正确切换', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()

    const aiLink = page.locator('nav').getByRole('link', { name: 'AI 配置' })
    await aiLink.click()
    await page.waitForURL('**/settings/ai')

    const notifyLink = page.locator('nav').getByRole('link', { name: '通知配置' })
    await notifyLink.click()
    await page.waitForURL('**/settings/notification')
    await expect(page.getByText(/通知配置|通知/).first()).toBeVisible()
  })

  test('admin 可以看到所有设置入口', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()

    const aiLink = page.locator('nav').getByRole('link', { name: 'AI 配置' })
    await expect(aiLink).toBeVisible()

    const notifyLink = page.locator('nav').getByRole('link', { name: '通知配置' })
    await expect(notifyLink).toBeVisible()
  })

  test('修改 AI 提供商并保存', async ({ page }) => {
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    const providerSelect = page.getByRole('combobox').first()
    if (await providerSelect.isVisible()) {
      await providerSelect.click()
      await page.getByRole('option', { name: 'OpenAI' }).click()
      await page.getByRole('button', { name: /保存/ }).click()
      await expect(page.getByText(/保存成功|设置已更新|ok/)).toBeVisible({ timeout: 10_000 })
    }
  })
})
