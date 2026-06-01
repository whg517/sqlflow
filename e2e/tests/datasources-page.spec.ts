/**
 * E2E — 数据源管理页面（真实后端）
 */
import { test, expect, loginViaUI, apiRequest, getFirstDatasourceId, createTestDatasource, cleanupDatasources, ADMIN_USER, ADMIN_PASS } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('数据源管理', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test.afterAll(async () => {
    await cleanupDatasources()
  })

  test('导航到数据源设置页', async ({ page }) => {
    await page.goto('/settings/datasource')
    await expect(page).toHaveURL(/\/settings\/datasource/)

    // 验证页面标题
    await expect(page.getByText('数据源配置')).toBeVisible()

    // 验证左侧导航高亮（使用 exact 避免匹配到"添加数据源"按钮）
    await expect(page.getByRole('button', { name: '数据源', exact: true })).toHaveClass(/accent-primary/)
  })

  test('数据源列表正确展示', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 验证表头
    await expect(page.getByRole('columnheader', { name: '名称' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '类型' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '地址' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '数据库' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '敏感表' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '状态' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '操作' })).toBeVisible()

    // 验证有数据行
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    const count = await dataRows.count()
    expect(count).toBeGreaterThan(0)
  })

  test('数据源状态 badge 显示正确', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 应至少有一个数据源显示状态 badge
    const statusBadges = page.getByText('正常')
    const count = await statusBadges.count()
    expect(count).toBeGreaterThan(0)
  })

  test('添加数据源对话框可以打开', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 点击添加按钮
    await page.getByRole('button', { name: '添加数据源' }).click()

    // 验证对话框出现
    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(page.getByRole('heading', { name: '添加数据源' })).toBeVisible()

    // 验证表单字段（使用 .first() 避免与表格列名冲突）
    await expect(page.getByLabel('添加数据源').getByText('名称')).toBeVisible()
    await expect(page.getByLabel('添加数据源').getByText('类型')).toBeVisible()
    await expect(page.getByLabel('添加数据源').getByText('主机')).toBeVisible()
    await expect(page.getByLabel('添加数据源').getByText('端口')).toBeVisible()
    await expect(page.getByLabel('添加数据源').getByText('用户名')).toBeVisible()
    await expect(page.getByLabel('添加数据源').getByText('密码')).toBeVisible()
  })

  test('添加数据源表单验证', async ({ page }) => {
    await page.goto('/settings/datasource')
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 不填写任何字段直接提交
    await page.getByRole('button', { name: '保存' }).click()

    // 验证验证错误
    await expect(page.getByText('请输入名称')).toBeVisible()
    await expect(page.getByText('请输入主机地址')).toBeVisible()
    await expect(page.getByText('请输入用户名')).toBeVisible()
    await expect(page.getByText('请输入密码')).toBeVisible()
  })

  test('添加数据源端口号验证', async ({ page }) => {
    await page.goto('/settings/datasource')
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 先填写名称（通过名称验证）
    await page.getByPlaceholder('2-50 个字符').fill('test-ds')

    // 填写无效端口
    await page.getByPlaceholder('1-65535').fill('99999')
    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.getByText('端口范围 1-65535')).toBeVisible()
  })

  test('添加数据源名称长度验证', async ({ page }) => {
    await page.goto('/settings/datasource')
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 填写过短名称
    await page.getByPlaceholder('2-50 个字符').fill('A')
    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.getByText('名称需 2-50 个字符')).toBeVisible()
  })

  test('添加数据源成功', async ({ page }) => {
    await page.goto('/settings/datasource')
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const dsName = `e2e-ds-${Date.now()}`

    // 填写完整表单
    await page.getByPlaceholder('2-50 个字符').fill(dsName)
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.100')
    await page.getByPlaceholder('1-65535').fill('5432')
    await page.getByPlaceholder('数据库用户名').fill('pguser')
    await page.getByPlaceholder('数据库密码').fill('pgpassword123')

    // 等待 API 响应
    const [response] = await Promise.all([
      page.waitForResponse('**/api/datasources', { timeout: 10_000 }),
      page.getByRole('button', { name: '保存' }).click(),
    ])

    // 验证成功 toast 或对话框关闭
    const dialogClosed = await page.getByRole('dialog').isVisible({ timeout: 3000 }).then((v) => !v).catch(() => true)
    expect(dialogClosed).toBeTruthy()
  })

  test('编辑数据源对话框可以打开', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 点击编辑按钮（第一个存在的）
    const editBtn = page.getByRole('button', { name: '编辑' }).first()
    await expect(editBtn).toBeVisible()
    await editBtn.click()

    // 验证对话框出现且标题为"编辑数据源"
    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(page.getByRole('heading', { name: '编辑数据源' })).toBeVisible()
  })

  test('关闭添加对话框', async ({ page }) => {
    await page.goto('/settings/datasource')
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 点击取消按钮关闭
    await page.getByRole('button', { name: '取消' }).click()
    await expect(page.getByRole('dialog')).not.toBeVisible()
  })

  test('设置页子导航切换', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 验证当前选中的是数据源 tab（使用 exact 避免匹配"添加数据源"按钮）
    const dsBtn = page.getByRole('button', { name: '数据源', exact: true })
    await expect(dsBtn).toHaveClass(/accent-primary/)

    // 切换到脱敏规则
    const maskBtn = page.getByRole('button', { name: '脱敏规则', exact: true })
    await maskBtn.click()
    await expect(maskBtn).toHaveClass(/accent-primary/)
    await expect(dsBtn).not.toHaveClass(/accent-primary/)

    // 切换到 AI 配置
    const aiBtn = page.getByRole('button', { name: 'AI 配置', exact: true })
    if (await aiBtn.isVisible()) {
      await aiBtn.click()
      await expect(aiBtn).toHaveClass(/accent-primary/)
    }
  })

  test('禁用数据源确认对话框', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 点击禁用按钮（第一个存在的）
    const disableBtn = page.getByRole('button', { name: '禁用' }).first()
    if (await disableBtn.isVisible()) {
      await disableBtn.click()

      // 验证确认对话框出现
      await expect(page.getByRole('alertdialog')).toBeVisible()
      await expect(page.getByText('确认禁用数据源')).toBeVisible()

      // 取消操作
      await page.getByRole('button', { name: '取消' }).click()
      await expect(page.getByRole('alertdialog')).not.toBeVisible()
    }
  })

  test('数据源类型选择 MySQL 和 MongoDB', async ({ page }) => {
    await page.goto('/settings/datasource')
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 验证默认类型为 MySQL（使用 combobox 定位 Select 值）
    await expect(page.getByRole('combobox')).toContainText('MySQL')

    // 点击选择器打开下拉
    await page.getByRole('combobox').click()

    // 选择 MongoDB
    const mongoOption = page.getByRole('option', { name: 'MongoDB' })
    if (await mongoOption.isVisible()) {
      await mongoOption.click()
      await expect(page.getByRole('combobox')).toContainText('MongoDB')
    }
  })

  test('编辑数据源密码字段标注"留空不修改"', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 点击编辑按钮
    const editBtn = page.getByRole('button', { name: '编辑' }).first()
    await expect(editBtn).toBeVisible()
    await editBtn.click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 编辑模式下密码字段应显示"留空不修改"提示
    await expect(page.getByText('留空不修改')).toBeVisible()
  })
})
