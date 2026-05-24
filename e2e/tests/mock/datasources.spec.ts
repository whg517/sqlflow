import { test, expect } from '@playwright/test'
import { mockApiRoutes, setToken, MOCK_DATASOURCES } from '../../support/mock-routes'

test.describe('数据源管理', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    await setToken(page, 'admin')
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

    // 验证数据源数据（mock 返回了 2 条）
    await expect(page.getByText('test-mysql')).toBeVisible()
    await expect(page.getByText('test-mongo')).toBeVisible()

    // 验证类型 badge（使用 exact 避免匹配到数据库名中的 mysql）
    await expect(page.getByText('MySQL', { exact: true })).toBeVisible()
    await expect(page.getByText('MongoDB', { exact: true })).toBeVisible()
  })

  test('数据源状态 badge 显示正确', async ({ page }) => {
    await page.goto('/settings/datasource')

    // mock 数据源状态为 active，应显示"正常"
    await expect(page.getByText('正常').first()).toBeVisible()
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
    // Mock POST 创建数据源
    await page.route('**/api/datasources', async (route) => {
      if (route.request().method() === 'POST') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 0, message: 'ok' }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 0, data: MOCK_DATASOURCES }),
        })
      }
    })

    await page.goto('/settings/datasource')
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 填写完整表单
    await page.getByPlaceholder('2-50 个字符').fill('new-postgres')
    await page.getByPlaceholder('IP 或域名').fill('192.168.1.100')
    await page.getByPlaceholder('1-65535').fill('5432')
    await page.getByPlaceholder('数据库用户名').fill('pguser')
    await page.getByPlaceholder('数据库密码').fill('pgpassword123')

    await page.getByRole('button', { name: '保存' }).click()

    // 验证成功 toast
    await expect(page.getByText('数据源添加成功')).toBeVisible()

    // 验证对话框关闭
    await expect(page.getByRole('dialog')).not.toBeVisible()
  })

  test('编辑数据源对话框可以打开', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 点击编辑按钮
    await page.getByRole('button', { name: '编辑' }).first().click()

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
    await aiBtn.click()
    await expect(aiBtn).toHaveClass(/accent-primary/)
  })

  test('禁用数据源确认对话框', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 点击禁用按钮
    await page.getByRole('button', { name: '禁用' }).first().click()

    // 验证确认对话框
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await expect(page.getByText('确认禁用数据源')).toBeVisible()
    await expect(page.getByText('确定要禁用数据源「test-mysql」吗？')).toBeVisible()

    // 取消操作
    await page.getByRole('button', { name: '取消' }).click()
    await expect(page.getByRole('alertdialog')).not.toBeVisible()
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
    await page.getByRole('option', { name: 'MongoDB' }).click()
    await expect(page.getByRole('combobox')).toContainText('MongoDB')
  })

  test('编辑数据源密码字段标注"留空不修改"', async ({ page }) => {
    await page.goto('/settings/datasource')

    // 点击编辑按钮
    await page.getByRole('button', { name: '编辑' }).first().click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 编辑模式下密码字段应显示"留空不修改"提示
    await expect(page.getByText('留空不修改')).toBeVisible()
  })
})
