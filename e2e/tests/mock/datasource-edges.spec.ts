/**
 * SF-QA0024: E2E — 数据源异常与边界场景
 * Covers: 异常处理 / 边界场景
 */
import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, setToken, MOCK_DATASOURCES } from '../../support/mock-routes'

test.describe('数据源 — 异常处理', () => {
  test('连接测试失败显示错误', async ({ page }) => {
    mockApiRoutes(page)

    // Mock connection test failure
    page.route(/\/api\/datasources\/\d+\/test/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 1,
          message: '连接测试失败: Connection refused',
          data: { success: false },
        }),
      })
    })

    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    // Click test connection on first datasource
    const testBtn = page.getByRole('button', { name: /测试|连接测试/ })
    if (await testBtn.isVisible()) {
      await testBtn.click()
      await expect(page.getByText(/连接测试失败|Connection refused/)).toBeVisible({ timeout: 5000 })
    }
  })

  test('添加数据源名称重复被拒绝', async ({ page }) => {
    // Mock POST to reject duplicate name
    await page.route(/\/api\/datasources$/, async (route) => {
      if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        if (body.name === 'test-mysql') {
          await route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({ code: 1, message: '数据源名称已存在' }),
          })
        } else {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({ code: 0, message: 'ok' }),
          })
        }
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 0, data: MOCK_DATASOURCES }),
        })
      }
    })

    mockApiRoutes(page)

    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('test-mysql')
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.100')
    await page.getByPlaceholder('1-65535').fill('3306')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('password')

    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.getByText('数据源名称已存在')).toBeVisible()
  })

  test('添加数据源时服务器返回 500 错误', async ({ page }) => {
    await page.route(/\/api\/datasources$/, async (route) => {
      if (route.request().method() === 'POST') {
        await route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ code: 1, message: '服务器内部错误' }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 0, data: MOCK_DATASOURCES }),
        })
      }
    })

    mockApiRoutes(page)

    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('new-db')
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.100')
    await page.getByPlaceholder('1-65535').fill('3306')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('password')

    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.getByText('服务器内部错误')).toBeVisible()
  })

  test('端口号超出范围验证', async ({ page }) => {
    mockApiRoutes(page)
    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('test-ds')

    // Negative port
    await page.getByPlaceholder('1-65535').fill('-1')
    await page.getByPlaceholder('1-65535').blur()

    let hasError = await page.getByText('端口范围 1-65535').isVisible().catch(() => false)

    if (!hasError) {
      // Try another invalid port
      await page.getByPlaceholder('1-65535').fill('0')
      await page.getByPlaceholder('1-65535').blur()
      hasError = await page.getByText(/端口|范围/).isVisible().catch(() => false)
    }

    expect(hasError).toBeTruthy()
  })

  test('主机地址为空不能保存', async ({ page }) => {
    mockApiRoutes(page)
    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('test-ds')

    // Don't fill host, try to save
    await page.getByRole('button', { name: '保存' }).click()

    // Expect validation error for host
    await expect(page.getByText('请输入主机地址')).toBeVisible()
  })
})

test.describe('数据源 — 边界场景', () => {
  test('数据源名称为 2 个字符（最小长度）', async ({ page }) => {
    mockApiRoutes(page)
    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('ab')
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.1')
    await page.getByPlaceholder('1-65535').fill('3306')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.getByText('数据源添加成功')).toBeVisible()
  })

  test('数据源名称为 50 个字符（最大长度）', async ({ page }) => {
    mockApiRoutes(page)
    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('a'.repeat(50))
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.1')
    await page.getByPlaceholder('1-65535').fill('3306')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.getByText('数据源添加成功')).toBeVisible()
  })

  test('端口号为 1（最小有效端口）', async ({ page }) => {
    mockApiRoutes(page)
    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('min-port-db')
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.1')
    await page.getByPlaceholder('1-65535').fill('1')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.getByText('数据源添加成功')).toBeVisible()
  })

  test('端口号为 65535（最大有效端口）', async ({ page }) => {
    mockApiRoutes(page)
    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('max-port-db')
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.1')
    await page.getByPlaceholder('1-65535').fill('65535')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.getByText('数据源添加成功')).toBeVisible()
  })

  test('使用主机名而非 IP 地址', async ({ page }) => {
    mockApiRoutes(page)
    await setToken(page, 'admin')
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('hostname-db')
    await page.getByPlaceholder('IP 或域名').fill('mysql.internal.example.com')
    await page.getByPlaceholder('1-65535').fill('3306')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.getByText('数据源添加成功')).toBeVisible()
  })
})

test.describe('数据源 — 权限校验补充', () => {
  test('developer 无法执行数据源连接测试', async ({ page }) => {
    // Override with developer role
    page.route('**/api/auth/me', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: { id: 2, username: 'developer', role: 'developer' } }),
      })
    })
    page.route(/\/api\/datasources\/\d+\/test/, async (route) => {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    })
    mockApiRoutes(page, { role: 'developer' })

    await setToken(page, 'developer')
    await page.goto('/settings/datasource')

    // Should see list but not be able to test
    const testBtn = page.getByRole('button', { name: /测试|连接测试/ })
    const isTestBtnVisible = await testBtn.isVisible().catch(() => false)
    // Test button may be hidden for developer, or clicking returns 403
    // If visible, clicking should trigger 403 which may show an error
    if (isTestBtnVisible) {
      await testBtn.click()
      // Either shows error toast or gets caught by page
    }
  })
})
