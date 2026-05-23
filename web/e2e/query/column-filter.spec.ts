import { test, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI, MOCK_QUERY_RESULT } from '../helpers'

// --- Column Filter Mock Data ---

const FILTER_TEST_DATA = {
  code: 0,
  message: 'ok',
  data: {
    columns: ['id', 'name', 'department', 'status', 'salary'],
    rows: [
      { id: 1, name: 'Alice', department: 'Engineering', status: 'active', salary: 15000 },
      { id: 2, name: 'Bob', department: 'Marketing', status: 'active', salary: 12000 },
      { id: 3, name: 'Carol', department: 'Engineering', status: 'inactive', salary: 18000 },
      { id: 4, name: 'Dave', department: 'HR', status: 'active', salary: 11000 },
      { id: 5, name: 'Eve', department: 'Engineering', status: 'active', salary: 16000 },
      { id: 6, name: 'Frank', department: 'Marketing', status: 'inactive', salary: 13000 },
      { id: 7, name: 'Grace', department: 'HR', status: 'active', salary: 10000 },
      { id: 8, name: 'Henry', department: 'Engineering', status: 'inactive', salary: 17000 },
    ],
    total: 8,
    execution_time_ms: 20,
    affected_rows: 0,
    desensitized: false,
    desensitized_fields: [],
    warnings: [],
  },
}

function mockFilterApis(page: import('@playwright/test').Page) {
  page.route('**/api/query/execute', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(FILTER_TEST_DATA),
    })
  })
}

test.describe('列筛选功能', () => {
  test.beforeEach(async ({ page }) => {
    mockApiRoutes(page)
    mockFilterApis(page)
  })

  async function executeTestQuery(page: import('@playwright/test').Page) {
    await loginViaUI(page)
    await expect(page).toHaveURL(/\/query/)

    // Select datasource
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: /test-mysql/ }).click()

    // Type SQL and execute
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM employees', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 10000 })
  }

  test('点击列头筛选图标打开筛选面板', async ({ page }) => {
    await executeTestQuery(page)

    // 悬停列头显示筛选图标（或直接点击列头筛选按钮）
    const nameHeader = page.getByRole('columnheader', { name: 'name' })
    await expect(nameHeader).toBeVisible()

    // 点击列头上的筛选图标
    const filterIcon = nameHeader.locator('button, [data-testid="column-filter"]').first()
    if (await filterIcon.isVisible()) {
      await filterIcon.click()
    } else {
      // 备选：右键点击列头打开筛选菜单
      await nameHeader.click({ button: 'right' })
    }

    // 验证筛选面板/下拉出现
    await expect(page.getByText(/包含|不包含|等于|不等于/).first()).toBeVisible({ timeout: 3000 })
  })

  test('筛选条件 - 包含（contains）', async ({ page }) => {
    await executeTestQuery(page)

    // 打开 name 列筛选
    const nameHeader = page.getByRole('columnheader', { name: 'name' })
    await nameHeader.click()

    // 选择"包含"条件
    const containsOption = page.getByText('包含')
    if (await containsOption.isVisible()) {
      await containsOption.click()
    }

    // 输入筛选值
    const filterInput = page.getByPlaceholder(/筛选/).or(page.getByRole('textbox').last())
    await filterInput.fill('li')

    // 应用筛选
    const applyBtn = page.getByRole('button', { name: /应用|确定|筛选/ })
    if (await applyBtn.isVisible()) {
      await applyBtn.click()
    } else {
      await page.keyboard.press('Enter')
    }

    // 验证筛选结果 — 包含 "li" 的名字：Alice
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows).toHaveCount(1)
    await expect(page.getByText('Alice')).toBeVisible()
    // Bob, Carol 等不包含 "li" 的不应出现
    await expect(page.getByText('Bob')).not.toBeVisible()
    await expect(page.getByText('Carol')).not.toBeVisible()
  })

  test('筛选条件 - 不包含（not contains）', async ({ page }) => {
    await executeTestQuery(page)

    const nameHeader = page.getByRole('columnheader', { name: 'name' })
    await nameHeader.click()

    // 选择"不包含"
    const notContainsOption = page.getByText('不包含')
    if (await notContainsOption.isVisible()) {
      await notContainsOption.click()
    }

    const filterInput = page.getByPlaceholder(/筛选/).or(page.getByRole('textbox').last())
    await filterInput.fill('li')

    const applyBtn = page.getByRole('button', { name: /应用|确定|筛选/ })
    if (await applyBtn.isVisible()) {
      await applyBtn.click()
    } else {
      await page.keyboard.press('Enter')
    }

    // 不包含 "li" 的行应该保留
    await expect(page.getByText('Bob')).toBeVisible()
    await expect(page.getByText('Carol')).toBeVisible()
    await expect(page.getByText('Dave')).toBeVisible()
    // Alice 包含 "li" 不应出现
    await expect(page.getByText('Alice')).not.toBeVisible()
  })

  test('筛选条件 - 等于（equals）', async ({ page }) => {
    await executeTestQuery(page)

    const statusHeader = page.getByRole('columnheader', { name: 'status' })
    await statusHeader.click()

    const equalsOption = page.getByText('等于')
    if (await equalsOption.isVisible()) {
      await equalsOption.click()
    }

    const filterInput = page.getByPlaceholder(/筛选/).or(page.getByRole('textbox').last())
    await filterInput.fill('active')

    const applyBtn = page.getByRole('button', { name: /应用|确定|筛选/ })
    if (await applyBtn.isVisible()) {
      await applyBtn.click()
    } else {
      await page.keyboard.press('Enter')
    }

    // 验证只有 status=active 的行
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows).toHaveCount(5) // Alice, Bob, Dave, Eve, Grace

    // inactive 不应出现
    await expect(page.getByRole('cell', { name: 'inactive' })).not.toBeVisible()
  })

  test('筛选条件 - 不等于（not equals）', async ({ page }) => {
    await executeTestQuery(page)

    const statusHeader = page.getByRole('columnheader', { name: 'status' })
    await statusHeader.click()

    const notEqualsOption = page.getByText('不等于')
    if (await notEqualsOption.isVisible()) {
      await notEqualsOption.click()
    }

    const filterInput = page.getByPlaceholder(/筛选/).or(page.getByRole('textbox').last())
    await filterInput.fill('active')

    const applyBtn = page.getByRole('button', { name: /应用|确定|筛选/ })
    if (await applyBtn.isVisible()) {
      await applyBtn.click()
    } else {
      await page.keyboard.press('Enter')
    }

    // 只有 inactive 的行
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows).toHaveCount(3) // Carol, Frank, Henry
  })

  test('筛选与列排序共存', async ({ page }) => {
    await executeTestQuery(page)

    // 先按 salary 降序排列
    const salaryHeader = page.getByRole('columnheader', { name: 'salary' })
    await salaryHeader.click() // 第一次点击 → 升序
    await salaryHeader.click() // 第二次点击 → 降序

    // 验证排序：Henry (17000) 应该在 Alice (15000) 之前
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    const firstRowText = await dataRows.first().textContent()
    expect(firstRowText).toContain('Henry')

    // 打开 department 列筛选
    const deptHeader = page.getByRole('columnheader', { name: 'department' })
    await deptHeader.click()

    const equalsOption = page.getByText('等于')
    if (await equalsOption.isVisible()) {
      await equalsOption.click()
    }

    const filterInput = page.getByPlaceholder(/筛选/).or(page.getByRole('textbox').last())
    await filterInput.fill('Engineering')

    const applyBtn = page.getByRole('button', { name: /应用|确定|筛选/ })
    if (await applyBtn.isVisible()) {
      await applyBtn.click()
    } else {
      await page.keyboard.press('Enter')
    }

    // 验证筛选后排序仍然保持：Engineering 中 salary 降序
    const filteredRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(filteredRows).toHaveCount(4) // Alice, Carol, Eve, Henry (Engineering)
    const firstFilteredText = await filteredRows.first().textContent()
    expect(firstFilteredText).toContain('Henry') // 18000 > 17000 > 16000 > 15000
  })

  test('清除筛选恢复全部数据', async ({ page }) => {
    await executeTestQuery(page)

    // 先应用筛选
    const statusHeader = page.getByRole('columnheader', { name: 'status' })
    await statusHeader.click()

    const equalsOption = page.getByText('等于')
    if (await equalsOption.isVisible()) {
      await equalsOption.click()
    }

    const filterInput = page.getByPlaceholder(/筛选/).or(page.getByRole('textbox').last())
    await filterInput.fill('active')

    const applyBtn = page.getByRole('button', { name: /应用|确定|筛选/ })
    if (await applyBtn.isVisible()) {
      await applyBtn.click()
    } else {
      await page.keyboard.press('Enter')
    }

    // 验证筛选生效
    const dataRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(dataRows).toHaveCount(5)

    // 清除筛选
    const clearBtn = page.getByRole('button', { name: /清除|重置|清空/ })
    if (await clearBtn.isVisible()) {
      await clearBtn.click()
    } else {
      // 备选：重新点击列头清除筛选
      await statusHeader.click()
      const clearOption = page.getByText(/清除|重置|全部/)
      if (await clearOption.isVisible()) {
        await clearOption.click()
      }
    }

    // 验证全部数据恢复
    const allRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
    await expect(allRows).toHaveCount(8)
    await expect(page.getByText('Alice')).toBeVisible()
    await expect(page.getByText('Henry')).toBeVisible()
    await expect(page.getByRole('cell', { name: 'active' })).toBeVisible()
    await expect(page.getByRole('cell', { name: 'inactive' })).toBeVisible()
  })
})
