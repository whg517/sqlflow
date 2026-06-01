/**
 * E2E — 数据源异常与边界场景（真实后端）
 * Covers: 异常处理 / 边界场景
 */
import { test, expect, loginViaUI, apiRequest, createTestDatasource, deleteDatasourceByName, cleanupDatasources } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('数据源 — 异常处理', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test.afterAll(async () => {
    await cleanupDatasources()
  })

  test('添加数据源名称重复被拒绝', async ({ page }) => {
    // 先创建一个数据源
    const dsName = `e2e-dup-${Date.now()}`
    await createTestDatasource(page, { name: dsName })

    // 尝试用相同名称再创建
    await page.goto('/settings/datasource')
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill(dsName)
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.100')
    await page.getByPlaceholder('1-65535').fill('3306')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('password')

    await page.getByRole('button', { name: '保存' }).click()

    // 验证重复名称错误
    await expect(page.getByText(/名称已存在|重复/)).toBeVisible({ timeout: 5000 })
  })

  test('端口号超出范围验证', async ({ page }) => {
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
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test.afterAll(async () => {
    await cleanupDatasources()
  })

  test('数据源名称为 2 个字符（最小长度）', async ({ page }) => {
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const dsName = `ds${Date.now().toString().slice(-2)}`
    await page.getByPlaceholder('2-50 个字符').fill(dsName)
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.1')
    await page.getByPlaceholder('1-65535').fill('3306')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    const [response] = await Promise.all([
      page.waitForResponse('**/api/datasources', { timeout: 10_000 }),
      page.getByRole('button', { name: '保存' }).click(),
    ])

    // Verify dialog closes or success toast
    const dialogClosed = await page.getByRole('dialog').isVisible({ timeout: 3000 }).then((v) => !v).catch(() => true)
    expect(dialogClosed).toBeTruthy()
  })

  test('数据源名称为 50 个字符（最大长度）', async ({ page }) => {
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await page.getByPlaceholder('2-50 个字符').fill('a'.repeat(50))
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.1')
    await page.getByPlaceholder('1-65535').fill('3306')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    const [response] = await Promise.all([
      page.waitForResponse('**/api/datasources', { timeout: 10_000 }),
      page.getByRole('button', { name: '保存' }).click(),
    ])

    const dialogClosed = await page.getByRole('dialog').isVisible({ timeout: 3000 }).then((v) => !v).catch(() => true)
    expect(dialogClosed).toBeTruthy()
  })

  test('端口号为 1（最小有效端口）', async ({ page }) => {
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const dsName = `e2e-port1-${Date.now()}`
    await page.getByPlaceholder('2-50 个字符').fill(dsName)
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.1')
    await page.getByPlaceholder('1-65535').fill('1')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    const [response] = await Promise.all([
      page.waitForResponse('**/api/datasources', { timeout: 10_000 }),
      page.getByRole('button', { name: '保存' }).click(),
    ])

    const dialogClosed = await page.getByRole('dialog').isVisible({ timeout: 3000 }).then((v) => !v).catch(() => true)
    expect(dialogClosed).toBeTruthy()
  })

  test('端口号为 65535（最大有效端口）', async ({ page }) => {
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const dsName = `e2e-port65535-${Date.now()}`
    await page.getByPlaceholder('2-50 个字符').fill(dsName)
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.1')
    await page.getByPlaceholder('1-65535').fill('65535')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    const [response] = await Promise.all([
      page.waitForResponse('**/api/datasources', { timeout: 10_000 }),
      page.getByRole('button', { name: '保存' }).click(),
    ])

    const dialogClosed = await page.getByRole('dialog').isVisible({ timeout: 3000 }).then((v) => !v).catch(() => true)
    expect(dialogClosed).toBeTruthy()
  })

  test('使用主机名而非 IP 地址', async ({ page }) => {
    await page.goto('/settings/datasource')

    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const dsName = `e2e-hostname-${Date.now()}`
    await page.getByPlaceholder('2-50 个字符').fill(dsName)
    await page.getByPlaceholder('IP 或域名').fill('mysql.internal.example.com')
    await page.getByPlaceholder('1-65535').fill('3306')
    await page.getByPlaceholder('数据库用户名').fill('root')
    await page.getByPlaceholder('数据库密码').fill('pass')

    const [response] = await Promise.all([
      page.waitForResponse('**/api/datasources', { timeout: 10_000 }),
      page.getByRole('button', { name: '保存' }).click(),
    ])

    const dialogClosed = await page.getByRole('dialog').isVisible({ timeout: 3000 }).then((v) => !v).catch(() => true)
    expect(dialogClosed).toBeTruthy()
  })
})
