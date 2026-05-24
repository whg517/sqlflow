import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, setToken, MOCK_TICKET, MOCK_TOKEN } from '../helpers'

test.describe('工单审批与执行完整流程', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    // Mock comments API（TicketDetailDrawer 内的 CommentSection 会调用）
    page.route(/\/api\/tickets\/\d+\/comments/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: [] }),
      })
    })
  })

  test('完整流程：创建工单 → 审批通过 → 执行工单', async ({ page }) => {
    // ========== 1. 登录并导航到工单页 ==========
    await loginViaUI(page)
    await page.getByRole('link', { name: '工单' }).click()
    await page.waitForURL('**/tickets**')

    // ========== 2. 创建工单 ==========
    await page.getByRole('button', { name: '提交新工单' }).click()
    await page.waitForURL('**/tickets/new**')

    // 填写工单表单
    await page.getByText('选择数据源').click()
    await page.getByText('test-mysql').click()
    await page.getByPlaceholder('输入数据库名（可选）').fill('testdb')
    await page.getByPlaceholder('输入要执行的 SQL 语句').fill('ALTER TABLE users ADD COLUMN phone VARCHAR(20)')
    await page.getByPlaceholder(/请说明此次变更的原因/).fill('Add phone column for user profile enhancement')

    // 提交工单
    await page.getByRole('button', { name: '提交工单' }).click()

    // 验证提交成功
    await expect(page.getByText('工单提交成功')).toBeVisible()
    await page.waitForURL('**/tickets**', { timeout: 5000 })

    // ========== 3. 打开工单详情（审批） ==========
    // Mock 工单详情返回 PENDING_APPROVAL 状态
    await page.route('**/api/tickets/*', async (route) => {
      const url = route.request().url()
      if (url.includes('/approve')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: { ...MOCK_TICKET, status: 'APPROVED', reviewer_id: 1, reviewer_name: 'admin' },
          }),
        })
      } else if (url.includes('/execute')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: { ...MOCK_TICKET, status: 'DONE', executed_at: new Date().toISOString() },
          }),
        })
      } else {
        // GET ticket detail → return PENDING_APPROVAL status
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: { ...MOCK_TICKET, status: 'PENDING_APPROVAL' },
          }),
        })
      }
    })

    // 点击工单行打开详情
    await page.getByRole('row', { name: /#1/ }).click()
    await expect(page.getByText('工单 #1')).toBeVisible()

    // 验证工单状态为待审批
    await expect(page.locator('[data-slot="sheet-content"]').getByText('待审批')).toBeVisible()

    // ========== 4. 审批通过 ==========
    // 点击"通过"按钮
    await page.locator('[data-slot="sheet-content"]').getByRole('button', { name: '通过' }).click()

    // 验证审批确认对话框
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await expect(page.getByText('确认通过工单')).toBeVisible()

    // 填写审批备注
    await page.getByPlaceholder('填写审批备注...').fill('LGTM')

    // 确认审批
    await page.getByRole('button', { name: '确认通过' }).click()

    // 验证审批成功 toast
    await expect(page.getByText('审批通过')).toBeVisible()

    // ========== 5. 执行工单 ==========
    // 审批后工单状态变为 APPROVED，应出现"执行"按钮
    // 重新打开详情（审批后 detail 会刷新）
    await expect(page.locator('[data-slot="sheet-content"]').getByRole('button', { name: '执行' })).toBeVisible({ timeout: 5000 })

    // 点击执行
    await page.locator('[data-slot="sheet-content"]').getByRole('button', { name: '执行' }).click()

    // 验证执行确认对话框
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await expect(page.getByText('确认执行工单')).toBeVisible()
    await expect(page.getByText(/此操作将直接在目标数据库上执行变更/)).toBeVisible()

    // 确认执行
    await page.getByRole('button', { name: '确认执行' }).click()

    // 验证执行成功 toast
    await expect(page.getByText('工单已执行')).toBeVisible()
  })

  test('创建工单 → 驳回流程', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // Mock 工单详情返回 PENDING_APPROVAL 状态
    await page.route('**/api/tickets/*', async (route) => {
      const url = route.request().url()
      if (url.includes('/reject')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: { ...MOCK_TICKET, status: 'REJECTED', reviewer_id: 1, reviewer_name: 'admin' },
          }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: { ...MOCK_TICKET, status: 'PENDING_APPROVAL' },
          }),
        })
      }
    })

    // 打开工单详情
    await page.getByRole('row', { name: /#1/ }).click()
    await expect(page.getByText('工单 #1')).toBeVisible()

    // 点击拒绝按钮
    await page.locator('[data-slot="sheet-content"]').getByRole('button', { name: '拒绝' }).click()

    // 验证驳回对话框
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await expect(page.getByText('驳回工单')).toBeVisible()
    await expect(page.getByText(/请填写驳回原因/)).toBeVisible()

    // 不填原因直接提交，应提示
    await page.getByRole('button', { name: '确认驳回' }).click()
    await expect(page.getByText('请填写驳回原因')).toBeVisible()

    // 填写驳回原因
    await page.getByPlaceholder('请说明驳回原因...').fill('SQL 变更风险过高，需要优化')

    // 确认驳回
    await page.getByRole('button', { name: '确认驳回' }).click()

    // 验证驳回成功
    await expect(page.getByText('已驳回')).toBeVisible()
  })

  test('取消工单流程', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // Mock 工单状态为 PENDING_APPROVAL（可取消）
    await page.route('**/api/tickets/*', async (route) => {
      const url = route.request().url()
      if (url.includes('/cancel')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: { ...MOCK_TICKET, status: 'CANCELLED' },
          }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 0,
            message: 'ok',
            data: { ...MOCK_TICKET, status: 'PENDING_APPROVAL' },
          }),
        })
      }
    })

    // 打开工单详情
    await page.getByRole('row', { name: /#1/ }).click()
    await expect(page.getByText('工单 #1')).toBeVisible()

    // 点击取消工单按钮
    await page.locator('[data-slot="sheet-content"]').getByRole('button', { name: '取消工单' }).click()

    // 验证取消确认对话框
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await expect(page.getByText('取消工单')).toBeVisible()
    await expect(page.getByText(/此操作不可恢复/)).toBeVisible()

    // 填写取消原因
    await page.getByPlaceholder('请说明取消原因...').fill('需求变更，不再需要此修改')

    // 确认取消
    await page.getByRole('button', { name: '确认取消' }).click()

    // 验证取消成功
    await expect(page.getByText('工单已取消')).toBeVisible()
  })

  test('审批通过对话框可以取消', async ({ page }) => {
    await loginViaUI(page)
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 打开工单详情
    await page.getByRole('row', { name: /#1/ }).click()
    await expect(page.getByText('工单 #1')).toBeVisible()

    // 点击通过按钮打开对话框
    await page.locator('[data-slot="sheet-content"]').getByRole('button', { name: '通过' }).click()
    await expect(page.getByRole('alertdialog')).toBeVisible()

    // 取消操作
    await page.getByRole('button', { name: '取消' }).first().click()
    await expect(page.getByRole('alertdialog')).not.toBeVisible()
  })

  test('执行确认对话框可以取消', async ({ page }) => {
    // Mock 工单为 APPROVED 状态（可执行）
    await page.route('**/api/tickets/*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: { ...MOCK_TICKET, status: 'APPROVED', reviewer_id: 1, reviewer_name: 'admin' },
        }),
      })
    })

    await loginViaUI(page)
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 打开工单详情
    await page.getByRole('row', { name: /#1/ }).click()
    await expect(page.getByText('工单 #1')).toBeVisible()

    // 点击执行按钮打开确认对话框
    await page.locator('[data-slot="sheet-content"]').getByRole('button', { name: '执行' }).click()
    await expect(page.getByRole('alertdialog')).toBeVisible()

    // 取消
    await page.getByRole('button', { name: '取消' }).first().click()
    await expect(page.getByRole('alertdialog')).not.toBeVisible()
  })

  test('工单详情显示 AI 评审结果', async ({ page }) => {
    // Mock 带有 AI 评审结果的工单
    const ticketWithAIReview = {
      ...MOCK_TICKET,
      ai_review_result: JSON.stringify({
        risk_level: 'high',
        risk_score: 85,
        decision: 'ticket',
        summary: 'ALTER TABLE 操作会修改表结构，建议在低峰期执行',
        suggestions: ['建议先备份表数据', '建议在业务低峰期执行'],
        impact_analysis: '添加列操作对现有数据无影响，但会增加存储空间',
      }),
    }

    await page.route('**/api/tickets/*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: ticketWithAIReview,
        }),
      })
    })

    await loginViaUI(page)
    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // 打开工单详情
    await page.getByRole('row', { name: /#1/ }).click()
    await expect(page.getByText('工单 #1')).toBeVisible()

    // 验证 AI 评审内容
    await expect(page.locator('[data-slot="sheet-content"]').getByText('ALTER TABLE 操作会修改表结构')).toBeVisible()
    await expect(page.locator('[data-slot="sheet-content"]').getByText('建议先备份表数据')).toBeVisible()
    await expect(page.locator('[data-slot="sheet-content"]').getByText('添加列操作对现有数据无影响')).toBeVisible()
  })

  test('非 admin/dba 用户看不到审批按钮', async ({ page }) => {
    // 以 developer 身份登录
    mockApiRoutes(page, { role: 'developer' })
    await setToken(page, 'developer')

    await page.goto('/tickets')
    await page.waitForURL('**/tickets**')

    // Mock 工单详情
    await page.route('**/api/tickets/*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          message: 'ok',
          data: MOCK_TICKET,
        }),
      })
    })

    // 打开工单详情
    await page.getByRole('row', { name: /#1/ }).click()
    await expect(page.getByText('工单 #1')).toBeVisible()

    // developer 不是 DBA，不应该看到通过/拒绝按钮
    await expect(page.locator('[data-slot="sheet-content"]').getByRole('button', { name: '通过' })).not.toBeVisible()
    await expect(page.locator('[data-slot="sheet-content"]').getByRole('button', { name: '拒绝' })).not.toBeVisible()
  })
})
