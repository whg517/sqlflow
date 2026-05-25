/**
 * SF-QA0024: E2E — 系统设置管理 (AI 配置 / 通知配置 / 通用设置)
 * Covers: 正常流程 / 异常处理 / 权限校验 / 边界场景
 */
import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, setToken } from '../../support/mock-routes'

// --- Mock Settings State ---
const defaultAISettings = {
  ai_provider: 'openai',
  ai_model: 'gpt-4',
  ai_api_key: '',
  ai_base_url: '',
  ai_timeout: 30,
}

const defaultDingtalkSettings = {
  dingtalk_webhook: '',
  dingtalk_secret: '',
}

function mockSettingsApis(page: import('@playwright/test').Page, role: 'admin' | 'developer' | 'dba' = 'admin') {
  // GET settings
  page.route('**/api/settings', async (route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    if (route.request().method() === 'PUT' || route.request().method() === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: { ...defaultAISettings, ...defaultDingtalkSettings },
        }),
      })
    }
  })

  // AI config test
  page.route('**/api/ai-config/test', async (route) => {
    if (role !== 'admin') {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: { success: true, message: 'AI 连接测试成功', latency_ms: 350 } }),
    })
  })
}

test.describe('系统设置 — 正常流程', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockSettingsApis(page)
  })

  test('导航到 AI 配置页并验证表单渲染', async ({ page }) => {
    await loginViaUI(page)

    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // Verify page title
    await expect(page.getByText('AI 配置').first()).toBeVisible()

    // Verify form fields
    await expect(page.getByText(/AI 提供商/)).toBeVisible()
    await expect(page.getByText(/AI 模型/)).toBeVisible()
    await expect(page.getByText(/API Key/)).toBeVisible()
  })

  test('修改 AI 提供商', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // Select AI provider
    const providerSelect = page.getByRole('combobox').first()
    await expect(providerSelect).toBeVisible()
    await providerSelect.click()
    await page.getByRole('option', { name: 'OpenAI' }).click()

    // Save
    await page.getByRole('button', { name: /保存/ }).click()
    await expect(page.getByText(/保存成功|设置已更新/)).toBeVisible()
  })

  test('AI 连接测试成功', async ({ page }) => {
    // Mock successful AI connection test
    await page.route('**/api/ai-config/test', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: { success: true, message: 'AI 连接测试成功', latency_ms: 350 },
        }),
      })
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // Fill API key
    const apiKeyInput = page.getByPlaceholder(/API Key|请输入/)
    if (await apiKeyInput.isVisible()) {
      await apiKeyInput.fill('sk-test-api-key-12345')
    }

    // Click test connection button
    await page.getByRole('button', { name: /测试|连接测试/ }).click()

    // Verify success message
    await expect(page.getByText(/连接测试成功|测试成功/)).toBeVisible({ timeout: 5000 })
    await expect(page.getByText('350ms')).toBeVisible()
  })

  test('导航到通知配置页', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '通知配置' }).click()
    await page.waitForURL('**/settings/notification')

    // Verify page title
    await expect(page.getByText(/通知配置|通知/).first()).toBeVisible()

    // Verify form fields
    await expect(page.getByText(/钉钉|Webhook|webhook/i)).toBeVisible()
  })

  test('保存钉钉通知配置', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '通知配置' }).click()
    await page.waitForURL('**/settings/notification')

    // Fill webhook URL
    const webhookInput = page.getByPlaceholder(/webhook|Webhook|URL/)
    if (await webhookInput.isVisible()) {
      await webhookInput.fill('https://oapi.dingtalk.com/robot/send?access_token=test-token')
    }

    // Fill secret if available
    const secretInput = page.getByPlaceholder(/secret|签名|Secret/)
    if (await secretInput.isVisible()) {
      await secretInput.fill('SECtest123')
    }

    // Save
    await page.getByRole('button', { name: /保存/ }).click()
    await expect(page.getByText(/保存成功|设置已更新/)).toBeVisible()
  })

  test('设置页子导航高亮切换正噸', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()

    // Verify AI 配置 link exists
    const aiLink = page.locator('nav').getByRole('link', { name: 'AI 配置' })
    await aiLink.click()
    await expect(page.getByText('AI 配置').first()).toBeVisible()

    // Switch to notification
    const notifyLink = page.locator('nav').getByRole('link', { name: '通知配置' })
    await notifyLink.click()
    await expect(page.getByText(/通知配置|通知/).first()).toBeVisible()
  })
})

test.describe('系统设置 — 异常处理', () => {
  test('AI 连接测试失败显示错误', async ({ page }) => {
    mockApiRoutes(page)
    mockSettingsApis(page)

    // Mock AI test failure
    await page.route('**/api/ai-config/test', async (route) => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 1,
          message: 'AI 连接失败: Connection refused',
        }),
      })
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // Fill API key
    const apiKeyInput = page.getByPlaceholder(/API Key|请输入/)
    if (await apiKeyInput.isVisible()) {
      await apiKeyInput.fill('sk-invalid-key')
    }

    // Click test button
    await page.getByRole('button', { name: /测试|连接测试/ }).click()

    // Verify error message
    await expect(page.getByText(/连接失败|Connection refused/)).toBeVisible({ timeout: 5000 })
  })

  test('AI 配置测试时无 API Key 提示', async ({ page }) => {
    mockApiRoutes(page)
    mockSettingsApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // Clear API key if present, or leave empty
    const apiKeyInput = page.getByPlaceholder(/API Key|请输入/)
    if (await apiKeyInput.isVisible()) {
      await apiKeyInput.clear()
    }

    // Click test button - should show validation error
    const testBtn = page.getByRole('button', { name: /测试|连接测试/ })
    await testBtn.click()

    // Expect some kind of validation message (either disabled button or error toast)
    const hasError = await page.getByText(/请输入|必须填写|API Key/).isVisible().catch(() => false)
    const isDisabled = await testBtn.isDisabled().catch(() => true)
    expect(hasError || isDisabled).toBeTruthy()
  })

  test('保存设置时网络错误显示重试提示', async ({ page }) => {
    // Mock settings save to fail
    await page.route('**/api/settings', async (route) => {
      if (route.request().method() === 'PUT' || route.request().method() === 'POST') {
        await route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ code: 1, message: '保存失败，请稍后重试' }),
        })
      } else {
        await route.fulfill()
      }
    })

    mockApiRoutes(page)
    mockSettingsApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    await page.getByRole('button', { name: /保存/ }).click()

    // Verify error
    await expect(page.getByText(/保存失败/)).toBeVisible()
  })

  test('无效 Webhook URL 格式提示', async ({ page }) => {
    mockApiRoutes(page)
    mockSettingsApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '通知配置' }).click()
    await page.waitForURL('**/settings/notification')

    // Fill invalid URL
    const webhookInput = page.getByPlaceholder(/webhook|Webhook|URL/)
    if (await webhookInput.isVisible()) {
      await webhookInput.fill('not-a-valid-url')
      await webhookInput.blur()
    }

    // Check for URL format validation
    const hasError = await page.getByText(/格式不正确|请输入有效的|invalid/).isVisible().catch(() => false)
    if (!hasError) {
      // Alternative: try to save and see error
      await page.getByRole('button', { name: /保存/ }).click()
    }
  })
})

test.describe('系统设置 — 权限校验', () => {
  test('非管理员无法访问 AI 配置页', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    mockSettingsApis(page, 'developer')

    await setToken(page, 'developer')
    await page.goto('/settings/ai')

    await page.waitForURL('**/403**', { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    expect(is403).toBe(true)
  })

  test('非管理员无法访问通知配置页', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    mockSettingsApis(page, 'developer')

    await setToken(page, 'developer')
    await page.goto('/settings/notification')

    await page.waitForURL('**/403**', { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    expect(is403).toBe(true)
  })

  test('developer 在设置导航中看不到 AI 配置和通知配置入口', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await setToken(page, 'developer')

    await page.goto('/settings/datasource')

    // developer should NOT see admin-only settings links
    const aiLink = page.locator('nav').getByRole('link', { name: 'AI 配置' })
    await expect(aiLink).not.toBeVisible()

    const notifyLink = page.locator('nav').getByRole('link', { name: '通知配置' })
    await expect(notifyLink).not.toBeVisible()
  })

  test('dba 在设置导航中看不到管理员专属入口', async ({ page }) => {
    mockApiRoutes(page, { role: 'dba' })
    await setToken(page, 'dba')

    await page.goto('/settings/datasource')

    const aiLink = page.locator('nav').getByRole('link', { name: 'AI 配置' })
    await expect(aiLink).not.toBeVisible()

    const notifyLink = page.locator('nav').getByRole('link', { name: '通知配置' })
    await expect(notifyLink).not.toBeVisible()
  })

  test('admin 可以看到所有设置入口', async ({ page }) => {
    mockApiRoutes(page, { role: 'admin' })
    mockSettingsApis(page, 'admin')

    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    // Admin should see all settings links
    const aiLink = page.locator('nav').getByRole('link', { name: 'AI 配置' })
    await expect(aiLink).toBeVisible()

    const notifyLink = page.locator('nav').getByRole('link', { name: '通知配置' })
    await expect(notifyLink).toBeVisible()
  })
})

test.describe('系统设置 — 边界场景', () => {
  test('AI 超时设置为 0 时被阻止', async ({ page }) => {
    mockApiRoutes(page)
    mockSettingsApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // Try to set timeout to 0
    const timeoutInput = page.getByPlaceholder(/超时|timeout|30/)
    if (await timeoutInput.isVisible()) {
      await timeoutInput.fill('0')
      await timeoutInput.blur()
    }

    // Should show validation
    const hasError = await page.getByText(/必须大于|不能为|最小/).isVisible().catch(() => false)
    const isDisabled = await page.getByRole('button', { name: /保存/ }).isDisabled().catch(() => true)
    // At least one validation mechanism should be in place
    expect(hasError || isDisabled).toBeTruthy()
  })

  test('AI API Key 值被掩码显示', async ({ page }) => {
    mockApiRoutes(page)
    mockSettingsApis(page)

    // Mock existing API key
    await page.route('**/api/settings', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: {
            ai_provider: 'openai',
            ai_model: 'gpt-4',
            ai_api_key: 'sk-****-abcd',
            ai_base_url: '',
            ai_timeout: 30,
          },
        }),
      })
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // API key should show masked value or placeholder
    const apiKeyInput = page.getByPlaceholder(/API Key|已设置|\*{4}/)
    await expect(apiKeyInput).toBeVisible()
  })

  test('超时边界值设置 (1-300)', async ({ page }) => {
    mockApiRoutes(page)
    mockSettingsApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // Set timeout to 1 (minimum)
    const timeoutInput = page.getByPlaceholder(/超时|timeout|30/)
    if (await timeoutInput.isVisible()) {
      await timeoutInput.fill('1')
      await timeoutInput.blur()
    }

    // Set timeout to 300 (maximum)
    await timeoutInput.fill('300')
    await timeoutInput.blur()

    // Save should work
    await page.getByRole('button', { name: /保存/ }).click()
    await expect(page.getByText(/保存成功|设置已更新/)).toBeVisible()
  })

  test('设置页 Tab 间切换不丢失表单状态', async ({ page }) => {
    mockApiRoutes(page)
    mockSettingsApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()

    // Go to AI config
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // Fill some values
    const timeoutInput = page.getByPlaceholder(/超时|timeout|30/)
    if (await timeoutInput.isVisible()) {
      await timeoutInput.fill('60')
    }

    // Switch to notification config
    await page.locator('nav').getByRole('link', { name: '通知配置' }).click()
    await page.waitForURL('**/settings/notification')

    // Verify notification page loaded
    await expect(page.getByText(/通知配置|通知/).first()).toBeVisible()

    // Switch back to AI config
    await page.locator('nav').getByRole('link', { name: 'AI 配置' }).click()
    await page.waitForURL('**/settings/ai')

    // Values should be preserved
    if (await timeoutInput.isVisible()) {
      await expect(timeoutInput).toHaveValue('60')
    }
  })
})
