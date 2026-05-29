/**
 * E2E — 工单列表（真实后端）
 * Migrated from mock/tests/mock/tickets.spec.ts
 */
import { test, expect, loginViaUI, getFirstDatasourceId, apiRequest, apiHelper, BASE_URL } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('工单操作', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  test('工单列表页面正确渲染', async ({ page }) => {
    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')

    // 验证页面标题
    await expect(page.getByText('变更工单')).toBeVisible()

    // 验证提交新工单按钮
    await expect(page.getByRole('button', { name: '提交新工单' })).toBeVisible()
  })

  test('工单列表显示数据', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 验证有数据行
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    const count = await dataRows.count()
    expect(count).toBeGreaterThan(0)
  })

  test('工单列表表头完整', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 验证表头列
    await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'SQL 摘要' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '数据库' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: 'AI 风险' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '状态' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '提交时间' })).toBeVisible()
  })

  test('工单状态 Tab 切换', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 验证 Tab 列表
    const tabs = ['全部', '待审批', '已通过', '已拒绝', '已取消', '已执行']
    for (const tab of tabs) {
      await expect(page.getByRole('tab', { name: tab })).toBeVisible()
    }

    // 点击"待审批"tab
    await page.getByRole('tab', { name: '待审批' }).click()

    // 验证 tab 选中状态
    await expect(page.getByRole('tab', { name: '待审批' })).toHaveAttribute('data-state', 'active')
  })

  test('新建工单页面导航', async ({ page }) => {
    // 从工单列表点击"提交新工单"按钮
    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')
    await page.getByRole('button', { name: '提交新工单' }).click()

    // 验证跳转到新工单页
    await page.waitForURL('**/tickets/new**')
    await expect(page).toHaveURL(/\/tickets\/new/)
    await expect(page.getByText('提交新工单')).toBeVisible()
  })

  test('新建工单表单字段验证', async ({ page }) => {
    await page.goto('/tickets/new')

    // 验证表单字段存在
    await expect(page.getByText('选择数据源')).toBeVisible()
    await expect(page.getByText('数据库名')).toBeVisible()
    await expect(page.getByText('SQL 内容')).toBeVisible()
    await expect(page.getByText('变更原因')).toBeVisible()

    // 验证必填标记
    await expect(page.getByText('数据源').getByText('*')).toBeVisible()
    await expect(page.getByText('SQL 内容').getByText('*')).toBeVisible()
    await expect(page.getByText('变更原因').getByText('*')).toBeVisible()
  })

  test('新建工单空表单提交显示验证错误', async ({ page }) => {
    await page.goto('/tickets/new')

    // 点击提交
    await page.getByRole('button', { name: '提交工单' }).click()

    // 验证错误提示
    await expect(page.getByText('请选择数据源')).toBeVisible()
    await expect(page.getByText('请输入 SQL')).toBeVisible()
    await expect(page.getByText('请填写变更原因')).toBeVisible()
  })

  test('新建工单变更原因长度不足验证', async ({ page }) => {
    await page.goto('/tickets/new')

    // 选择数据源
    await page.getByText('选择数据源').click()
    const firstOption = page.getByRole('option').first()
    await expect(firstOption).toBeVisible({ timeout: 5000 })
    await firstOption.click()

    // 输入 SQL
    await page.getByPlaceholder('输入要执行的 SQL 语句').fill('SELECT 1')

    // 输入过短的变更原因（少于 10 个字符）
    await page.getByPlaceholder(/请说明此次变更的原因/).fill('太短了')

    await page.getByRole('button', { name: '提交工单' }).click()

    await expect(page.getByText('变更原因至少 10 个字符')).toBeVisible()
  })

  test('新建工单成功提交', async ({ page }) => {
    await page.goto('/tickets/new')

    // 选择数据源
    await page.getByText('选择数据源').click()
    const firstOption = page.getByRole('option').first()
    await expect(firstOption).toBeVisible({ timeout: 5000 })
    await firstOption.click()

    // 输入数据库名
    const dbNameInput = page.getByPlaceholder('输入数据库名')
    if (await dbNameInput.isVisible()) {
      await dbNameInput.fill('testdb')
    }

    // 输入 SQL
    await page.getByPlaceholder('输入要执行的 SQL 语句').fill('SELECT 1')

    // 输入变更原因
    await page.getByPlaceholder(/请说明此次变更的原因/).fill('Test ticket for E2E testing flow')

    // 提交
    const [response] = await Promise.all([
      page.waitForResponse('**/api/tickets', { timeout: 10_000 }),
      page.getByRole('button', { name: '提交工单' }).click(),
    ])

    // 验证跳转回工单列表
    await page.waitForURL('**/tickets**', { timeout: 10_000 })
    await expect(page).toHaveURL(/\/tickets/)
  })

  test('新建工单取消返回列表', async ({ page }) => {
    await page.goto('/tickets/new')

    // 点击取消按钮
    await page.getByRole('button', { name: '取消' }).click()

    // 验证返回工单列表
    await page.waitForURL('**/tickets**', { timeout: 5000 })
    await expect(page).toHaveURL(/\/tickets/)
  })

  test('新建工单返回按钮', async ({ page }) => {
    await page.goto('/tickets/new')

    // 点击返回箭头
    await page.getByRole('button').filter({ has: page.locator('svg.lucide-arrow-left') }).click()

    await page.waitForURL('**/tickets**', { timeout: 5000 })
    await expect(page).toHaveURL(/\/tickets/)
  })

  test('工单详情抽屉打开', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 点击工单行打开详情
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    const count = await dataRows.count()
    if (count === 0) {
      test.skip()
      return
    }

    await dataRows.first().click()

    // 验证 Sheet 侧边抽屉打开
    await expect(page.getByText(/工单 #\d+/)).toBeVisible()
  })

  test('工单详情 SQL 内容可复制', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    if ((await dataRows.count()) === 0) {
      test.skip()
      return
    }

    // 点击工单行打开详情
    await dataRows.first().click()
    await expect(page.getByText(/工单 #\d+/)).toBeVisible()

    // 验证 SQL 内容显示
    const sheetContent = page.locator('[data-slot="sheet-content"]')
    await expect(sheetContent).toBeVisible()

    // 验证复制按钮存在
    const copyBtn = page.locator('button').filter({ has: page.locator('svg.lucide-copy') })
    await expect(copyBtn.first()).toBeVisible()
  })

  test('工单筛选 - 我提交的', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 点击"我提交的"按钮
    await page.getByRole('button', { name: '我提交的' }).click()

    // 验证按钮高亮
    const btn = page.getByRole('button', { name: '我提交的' })
    await expect(btn).toHaveClass(/accent-primary/)
  })

  test('工单筛选 - 待我审批（admin）', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // admin 角色应该看到"待我审批"按钮
    await expect(page.getByRole('button', { name: '待我审批' })).toBeVisible()

    // 点击筛选
    await page.getByRole('button', { name: '待我审批' }).click()
    await expect(page.getByRole('button', { name: '待我审批' })).toHaveClass(/accent-primary/)
  })

  test('工单搜索功能', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 输入搜索关键词
    await page.getByPlaceholder('搜索 SQL 内容...').fill('ALTER TABLE')
    await page.keyboard.press('Enter')

    // 搜索触发列表刷新
    await expect(page.getByPlaceholder('搜索 SQL 内容...')).toHaveValue('ALTER TABLE')
  })

  test('变更原因字数统计', async ({ page }) => {
    await page.goto('/tickets/new')

    const reasonTextarea = page.getByPlaceholder(/请说明此次变更的原因/)
    const testReason = '这是一个测试用的变更原因说明'
    await reasonTextarea.fill(testReason)

    // 验证字数统计
    await expect(page.getByText(`${testReason.length}/500`)).toBeVisible()
  })

  test('工单列表从查询页导航到达', async ({ page }) => {
    // 从侧边栏点击工单
    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')
    await expect(page).toHaveURL(/\/tickets/)
  })

  test('工单数据源筛选下拉', async ({ page }) => {
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 验证数据源筛选 Select 存在
    const dsSelect = page.getByRole('combobox').filter({ hasText: '数据源' })
    await expect(dsSelect).toBeVisible()

    // 点击打开下拉
    await dsSelect.click()

    // 验证选项
    await expect(page.getByRole('option', { name: '全部数据源' })).toBeVisible()
  })
})
