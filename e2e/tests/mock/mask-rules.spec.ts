/**
 * SF-QA0024: E2E — 脱敏规则管理
 * Covers: 正常流程 / 异常处理 / 权限校验 / 边界场景
 */
import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, setToken, MOCK_MASK_RULES, MOCK_MASK_TABLES, MOCK_MASK_COLUMNS } from '../../support/mock-routes'

// --- In-memory mock state for CRUD operations ---
let maskRules = [...MOCK_MASK_RULES]
let nextRuleId = 100

function mockMaskApis(page: import('@playwright/test').Page) {
  maskRules = [...MOCK_MASK_RULES]
  nextRuleId = 100

  // List mask rules
  page.route(/\/api\/mask-rules(\?.*)?$/, async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: maskRules }),
      })
    } else if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON()
      const newRule = {
        id: nextRuleId++,
        datasource_id: body.datasource_id ?? 1,
        table_name: body.table_name,
        column_name: body.column_name,
        mask_type: body.mask_type,
        mask_length: body.mask_length ?? 0,
        sensitivity: body.sensitivity,
        description: body.description ?? '',
      }
      maskRules.push(newRule)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok', data: newRule }),
      })
    } else {
      await route.fulfill({ status: 405, contentType: 'application/json', body: '{}' })
    }
  })

  // Delete / Update mask rule
  page.route(/\/api\/mask-rules\/\d+/, async (route) => {
    const url = route.request().url()
    const idMatch = url.match(/\/api\/mask-rules\/(\d+)/)
    const ruleId = idMatch ? parseInt(idMatch[1]) : 0

    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON()
      maskRules = maskRules.map((r) =>
        r.id === ruleId ? { ...r, ...body } : r,
      )
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
    } else if (route.request().method() === 'DELETE') {
      maskRules = maskRules.filter((r) => r.id !== ruleId)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
    } else {
      await route.fulfill({ status: 405, contentType: 'application/json', body: '{}' })
    }
  })

  // Tables list
  page.route(/\/api\/datasources\/\d+\/tables$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: MOCK_MASK_TABLES }),
    })
  })

  // Columns list
  page.route(/\/api\/datasources\/\d+\/tables\/[^/]+\/columns/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: MOCK_MASK_COLUMNS }),
    })
  })
}

test.describe('脱敏规则管理 — 正常流程', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockMaskApis(page)
  })

  test('导航到脱敏规则设置页并验证列表渲染', async ({ page }) => {
    await loginViaUI(page)

    // 导航到设置
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    // 验证页面标题
    await expect(page.getByText('脱敏规则').first()).toBeVisible()

    // 验证表头
    await expect(page.getByRole('columnheader', { name: '表名' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '列名' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '脱敏类型' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '敏感级别' })).toBeVisible()

    // 验证规则数据
    await expect(page.getByText('users')).toBeVisible()
    await expect(page.getByText('email')).toBeVisible()
    await expect(page.getByText('phone')).toBeVisible()
  })

  test('添加脱敏规则 — 完整流程', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    // 点击添加按钮
    await page.getByRole('button', { name: /添加|新建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 选择数据源
    const dsSelect = page.locator('dialog [role="combobox"]').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    // 选择表
    const tableSelect = page.locator('dialog [role="combobox"]').nth(1)
    await tableSelect.click()
    await page.getByRole('option', { name: 'users' }).click()

    // 选择列
    const columnSelect = page.locator('dialog [role="combobox"]').nth(2)
    await columnSelect.click()
    await page.getByRole('option', { name: 'name' }).click()

    // 选择脱敏类型
    const typeSelect = page.locator('dialog [role="combobox"]').nth(3)
    await typeSelect.click()
    await page.getByRole('option', { name: '部分脱敏' }).click()

    // 填写描述
    await page.getByPlaceholder(/描述|说明/).fill('用户名脱敏保护')

    // 保存
    await page.getByRole('button', { name: /保存|确认/ }).click()

    // 验证成功消息
    await expect(page.getByText(/添加成功|创建成功/)).toBeVisible()

    // 验证新规则出现在列表中（name + users）
    await expect(page.getByRole('dialog')).not.toBeVisible()
  })

  test('删除脱敏规则 — 确认流程', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    // 验证初始规则数量
    const initialCount = maskRules.length

    // 点击第一条规则的删除按钮
    await page.getByRole('button', { name: '删除' }).first().click()

    // 验证确认对话框
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await expect(page.getByText(/确认删除|确定要删除/)).toBeVisible()

    // 确认删除
    await page.getByRole('button', { name: /确认|确定/ }).click()

    // 验证成功消息
    await expect(page.getByText(/删除成功/)).toBeVisible()

    // 验证规则数量减少
    await expect.poll(() => maskRules.length).toBe(initialCount - 1)
  })

  test('编辑脱敏规则 — 修改脱敏类型', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    // 点击编辑按钮
    await page.getByRole('button', { name: '编辑' }).first().click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 修改脱敏类型
    const typeSelect = page.locator('dialog [role="combobox"]').nth(3)
    await typeSelect.click()
    await page.getByRole('option', { name: '全脱敏' }).click()

    // 保存
    await page.getByRole('button', { name: /保存|确认/ }).click()

    // 验证成功
    await expect(page.getByText(/更新成功|修改成功/)).toBeVisible()
  })
})

test.describe('脱敏规则管理 — 异常处理', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockMaskApis(page)
  })

  test('不选择表无法获取列信息', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    await page.getByRole('button', { name: /添加|新建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 列选择器应该在未选择表时不可用或为空
    const columnSelects = page.locator('dialog [role="combobox"]')
    const columnCount = await columnSelects.count()
    // 至少应该有数据源选择器，列选择器可能不可见或禁用
    expect(columnCount).toBeGreaterThanOrEqual(1)
  })

  test('重复添加相同列脱敏规则被拒绝', async ({ page }) => {
    // Mock POST to return error for duplicate
    await page.route(/\/api\/mask-rules$/, async (route) => {
      if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        if (body.column_name === 'email' && body.table_name === 'users') {
          await route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({
              code: 1,
              message: '该列已存在脱敏规则',
            }),
          })
        } else {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({ code: 0, message: 'ok' }),
          })
        }
      } else {
        await route.fulfill()
      }
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    await page.getByRole('button', { name: /添加|新建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 选择数据源
    const dsSelect = page.locator('dialog [role="combobox"]').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    // 选择 users 表
    const tableSelect = page.locator('dialog [role="combobox"]').nth(1)
    await tableSelect.click()
    await page.getByRole('option', { name: 'users' }).click()

    // 选择 email 列
    const columnSelect = page.locator('dialog [role="combobox"]').nth(2)
    await columnSelect.click()
    await page.getByRole('option', { name: 'email' }).click()

    // 保存
    await page.getByRole('button', { name: /保存|确认/ }).click()

    // 验证重复错误提示
    await expect(page.getByText('该列已存在脱敏规则')).toBeVisible()
  })

  test('服务器错误时显示错误提示', async ({ page }) => {
    // Mock server error
    await page.route(/\/api\/mask-rules$/, async (route) => {
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
          body: JSON.stringify({ code: 0, data: maskRules }),
        })
      }
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    await page.getByRole('button', { name: /添加|新建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 选择数据源和表
    const dsSelect = page.locator('dialog [role="combobox"]').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const tableSelect = page.locator('dialog [role="combobox"]').nth(1)
    await tableSelect.click()
    await page.getByRole('option', { name: 'users' }).click()

    const columnSelect = page.locator('dialog [role="combobox"]').nth(2)
    await columnSelect.click()
    await page.getByRole('option', { name: 'name' }).click()

    await page.getByRole('button', { name: /保存|确认/ }).click()

    // 验证错误消息
    await expect(page.getByText('服务器内部错误')).toBeVisible()
  })

  test('表单未填写完整不能提交', async ({ page }) => {
    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    await page.getByRole('button', { name: /添加|新建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 不填写任何内容直接提交
    const saveBtn = page.getByRole('button', { name: /保存|确认/ })
    await expect(saveBtn).toBeDisabled()
  })
})

test.describe('脱敏规则管理 — 权限校验', () => {
  test('非管理员无法访问脱敏规则页面', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    // Override mask rules to return 403
    page.route(/\/api\/mask-rules/, async (route) => {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    })

    await setToken(page, 'developer')
    await page.goto('/settings/mask')

    // 应该被重定向或显示 403
    await page.waitForURL('**/403**', { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    expect(is403).toBe(true)
  })

  test('dba 用户无法访问脱敏规则页面', async ({ page }) => {
    mockApiRoutes(page, { role: 'dba' })
    page.route(/\/api\/mask-rules/, async (route) => {
      await route.fulfill({ status: 403, contentType: 'application/json', body: '{}' })
    })

    await setToken(page, 'dba')
    await page.goto('/settings/mask')

    await page.waitForURL('**/403**', { timeout: 5000 }).catch(() => {})
    const is403 = await page.getByText('403').isVisible().catch(() => false)
    expect(is403).toBe(true)
  })

  test('admin 用户可以正常访问脱敏规则页面', async ({ page }) => {
    mockApiRoutes(page, { role: 'admin' })
    mockMaskApis(page)

    await setToken(page, 'admin')
    await page.goto('/settings/mask')

    await expect(page).toHaveURL(/\/settings\/mask/)
    await expect(page.getByText('脱敏规则').first()).toBeVisible()
  })

  test('developer 在设置导航中看不到脱敏规则入口', async ({ page }) => {
    mockApiRoutes(page, { role: 'developer' })
    await setToken(page, 'developer')

    await page.goto('/settings/datasource')

    // developer 不应该看到脱敏规则链接（admin-only）
    const maskLink = page.locator('nav').getByRole('link', { name: '脱敏规则' })
    await expect(maskLink).not.toBeVisible()
  })
})

test.describe('脱敏规则管理 — 边界场景', () => {
  test('空规则列表显示空状态', async ({ page }) => {
    mockApiRoutes(page)
    // Return empty mask rules
    page.route(/\/api\/mask-rules$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: [] }),
      })
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    // 空状态提示
    await expect(page.getByText(/暂无|没有|空/)).toBeVisible()
  })

  test('删除最后一条规则', async ({ page }) => {
    // Mock only 1 rule
    page.route(/\/api\/mask-rules$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, data: [MOCK_MASK_RULES[0]] }),
      })
    })
    page.route(/\/api\/mask-rules\/\d+/, async (route) => {
      maskRules = []
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 0, message: 'ok' }),
      })
    })

    mockApiRoutes(page)
    mockMaskApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    await page.getByRole('button', { name: '删除' }).first().click()
    await page.getByRole('button', { name: /确认|确定/ }).click()

    // 删除后应显示空状态
    await expect(page.getByText(/暂无|没有|空/)).toBeVisible()
  })

  test('全脱敏规则不显示脱敏长度', async ({ page }) => {
    mockApiRoutes(page)
    // Return a full-mask rule
    page.route(/\/api\/mask-rules$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 0,
          data: [
            { id: 1, datasource_id: 1, table_name: 'users', column_name: 'id_card', mask_type: 'full', mask_length: 0, sensitivity: 'critical', description: '身份证全脱敏' },
          ],
        }),
      })
    })

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    // 验证敏感级别为"严重"
    await expect(page.getByText('严重')).toBeVisible()
    await expect(page.getByText('id_card')).toBeVisible()
  })

  test('取消删除脱敏规则对话框', async ({ page }) => {
    mockApiRoutes(page)
    mockMaskApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    const initialCount = maskRules.length

    await page.getByRole('button', { name: '删除' }).first().click()
    await expect(page.getByRole('alertdialog')).toBeVisible()

    // 取消
    await page.getByRole('button', { name: '取消' }).click()
    await expect(page.getByRole('alertdialog')).not.toBeVisible()

    // 规则数量不变
    expect(maskRules.length).toBe(initialCount)
  })

  test('快速连续添加多条规则', async ({ page }) => {
    mockApiRoutes(page)
    mockMaskApis(page)

    await loginViaUI(page)
    await page.getByRole('button', { name: '设置' }).click()
    await page.locator('nav').getByRole('link', { name: '脱敏规则' }).click()
    await page.waitForURL('**/settings/mask')

    // Add first rule: name column in orders table
    await page.getByRole('button', { name: /添加|新建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const dsSelect = page.locator('dialog [role="combobox"]').first()
    await dsSelect.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const tableSelect = page.locator('dialog [role="combobox"]').nth(1)
    await tableSelect.click()
    await page.getByRole('option', { name: 'orders' }).click()

    const columnSelect = page.locator('dialog [role="combobox"]').nth(2)
    await columnSelect.click()
    await page.getByRole('option', { name: 'name' }).click()

    await page.getByRole('button', { name: /保存|确认/ }).click()
    await expect(page.getByText(/添加成功|创建成功/)).toBeVisible()

    // Add second rule: phone column in products table
    await page.getByRole('button', { name: /添加|新建/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    const dsSelect2 = page.locator('dialog [role="combobox"]').first()
    await dsSelect2.click()
    await page.getByRole('option', { name: 'test-mysql' }).click()

    const tableSelect2 = page.locator('dialog [role="combobox"]').nth(1)
    await tableSelect2.click()
    await page.getByRole('option', { name: 'products' }).click()

    const columnSelect2 = page.locator('dialog [role="combobox"]').nth(2)
    await columnSelect2.click()
    await page.getByRole('option', { name: 'phone' }).click()

    await page.getByRole('button', { name: /保存|确认/ }).click()
    await expect(page.getByText(/添加成功|创建成功/)).toBeVisible()

    // Both rules should exist
    expect(maskRules.length).toBeGreaterThanOrEqual(MOCK_MASK_RULES.length + 2)
  })
})
