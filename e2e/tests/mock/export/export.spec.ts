import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI } from '../../support/mock-routes'

test.describe('审计日志导出', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    await loginViaUI(page)
  })

  test('管理员可以看到导出按钮', async ({ page }) => {
    await page.goto('/audit')
    await page.waitForTimeout(1000)

    // 导出 CSV 按钮应该存在
    await expect(page.getByRole('button', { name: /导出 CSV/ })).toBeVisible()
  })

  test('点击导出按钮触发后端导出API', async ({ page }) => {
    await page.goto('/audit')
    await page.waitForTimeout(1000)

    // 监听导出 API 请求
    const exportPromise = page.waitForRequest(/\/api\/export\/audit/)

    await page.getByRole('button', { name: /导出 CSV/ }).click()

    const request = await exportPromise
    expect(request.method()).toBe('GET')
  })

  test('非管理员无导出权限', async ({ page }) => {
    // 用 developer 角色登录
    mockApiRoutes(page, { role: 'developer' })
    await loginViaUI(page, 'developer', 'developer123')

    await page.goto('/audit')
    await page.waitForTimeout(1000)

    // Developer 不应该能访问审计页面（403）
    // 或者在页面上看到无权限提示
    await expect(page.getByRole('button', { name: /导出 CSV/ })).not.toBeVisible()
  })

  test('导出中显示loading状态', async ({ page }) => {
    // 让导出 API 延迟返回
    await page.route(/\/api\/export\/audit/, async (route) => {
      await new Promise(resolve => setTimeout(resolve, 1000))
      await route.fulfill({
        status: 200,
        contentType: 'text/csv',
        headers: { 'Content-Disposition': 'attachment; filename="test.csv"' },
        body: 'test',
      })
    })

    await page.goto('/audit')
    await page.waitForTimeout(1000)

    const exportBtn = page.getByRole('button', { name: /导出 CSV/ })
    await exportBtn.click()

    // 按钮应显示 loading 状态
    await expect(page.getByText('导出中...')).toBeVisible()
  })
})

test.describe('工单导出', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    await loginViaUI(page)
  })

  test('工单页面显示导出按钮', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForTimeout(1000)

    // 导出 CSV 按钮应该存在
    await expect(page.getByRole('button', { name: /导出 CSV/ })).toBeVisible()
  })

  test('点击导出按钮触发后端导出API', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForTimeout(1000)

    const exportPromise = page.waitForRequest(/\/api\/export\/tickets/)

    await page.getByRole('button', { name: /导出 CSV/ }).click()

    const request = await exportPromise
    expect(request.method()).toBe('GET')
  })

  test('developer可以导出工单', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await loginViaUI(page, 'developer', 'developer123')

    await page.goto('/tickets')
    await page.waitForTimeout(1000)

    // Developer 应该可以看到工单导出按钮
    await expect(page.getByRole('button', { name: /导出 CSV/ })).toBeVisible()
  })

  test('导出中显示loading状态', async ({ page }) => {
    await page.route(/\/api\/export\/tickets/, async (route) => {
      await new Promise(resolve => setTimeout(resolve, 1000))
      await route.fulfill({
        status: 200,
        contentType: 'text/csv',
        headers: { 'Content-Disposition': 'attachment; filename="test.csv"' },
        body: 'test',
      })
    })

    await page.goto('/tickets')
    await page.waitForTimeout(1000)

    const exportBtn = page.getByRole('button', { name: /导出 CSV/ })
    await exportBtn.click()

    await expect(page.getByText('导出中...')).toBeVisible()
  })
})

test.describe('导出权限控制', () => {
  test('审计导出API - developer被拒绝 (403)', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await loginViaUI(page, 'developer', 'developer123')

    // 直接请求导出 API 应该返回 403
    const response = await page.request.get('/api/export/audit')
    expect(response.status()).toBe(403)
  })

  test('工单导出API - 所有认证用户可用', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await loginViaUI(page, 'developer', 'developer123')

    const response = await page.request.get('/api/export/tickets')
    expect(response.status()).toBe(200)
  })
})

test.describe('导出错误处理', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    await loginViaUI(page)
  })

  test('超过10000行限制时显示错误', async ({ page }) => {
    await page.route(/\/api\/export\/audit/, async (route) => {
      await route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ code: 400, message: '导出数据超过10000行上限，请添加筛选条件缩小范围' }),
      })
    })

    await page.goto('/audit')
    await page.waitForTimeout(1000)

    await page.getByRole('button', { name: /导出 CSV/ }).click()
    await page.waitForTimeout(500)

    // 应显示错误 toast
    await expect(page.getByText('导出数据超过10000行上限')).toBeVisible()
  })

  test('服务器错误时显示错误', async ({ page }) => {
    await page.route(/\/api\/export\/tickets/, async (route) => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ code: 500, message: '导出失败' }),
      })
    })

    await page.goto('/tickets')
    await page.waitForTimeout(1000)

    await page.getByRole('button', { name: /导出 CSV/ }).click()
    await page.waitForTimeout(500)

    // 应显示错误信息
    await expect(page.getByText('导出失败')).toBeVisible()
  })
})
