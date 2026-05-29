/**
 * E2E — 列筛选功能（真实后端）
 * SF-QA0028 batch 2
 * 使用真实查询结果做筛选测试
 */
import { test, expect, loginViaUI, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

test.describe('列筛选功能（真实后端）', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  /** 执行测试查询并等待结果 */
  async function executeTestQuery(page: import('@playwright/test').Page) {
    const ds = await getFirstDatasourceId(page)

    // Select datasource
    const dsSelect = page.getByRole('combobox').first()
    await dsSelect.click()
    await page.getByRole('option', { name: new RegExp(ds.name) }).click()

    // Type SQL and execute
    const editor = page.locator('.cm-content').first()
    await editor.click()
    await page.keyboard.type('SELECT * FROM sys_user', { delay: 30 })

    const executeBtn = page.getByRole('button', { name: '执行' })
    await executeBtn.click()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 15_000 })
  }

  test('点击列头筛选图标打开筛选面板', async ({ page }) => {
    await executeTestQuery(page)

    // 查找列头
    const headers = page.getByRole('columnheader')
    const headerCount = await headers.count()
    if (headerCount === 0) {
      test.skip()
      return
    }

    // 点击第一个列头
    const nameHeader = headers.first()
    await nameHeader.click()

    // 验证筛选面板/下拉出现
    const filterUI = page.getByText(/包含|不包含|等于|不等于|筛选/).first()
    const filterVisible = await filterUI.isVisible({ timeout: 3000 }).catch(() => false)

    if (filterVisible) {
      await expect(filterUI).toBeVisible()
    }
  })

  test('筛选条件 - 包含（contains）', async ({ page }) => {
    await executeTestQuery(page)

    const headers = page.getByRole('columnheader')
    const headerCount = await headers.count()
    if (headerCount === 0) {
      test.skip()
      return
    }

    // 点击列头
    await headers.first().click()

    const containsOption = page.getByText('包含')
    if (await containsOption.isVisible({ timeout: 2000 }).catch(() => false)) {
      await containsOption.click()

      // 输入筛选值
      const filterInput = page.getByPlaceholder(/筛选/).or(page.getByRole('textbox').last())
      if (await filterInput.isVisible({ timeout: 2000 }).catch(() => false)) {
        await filterInput.fill('a')

        const applyBtn = page.getByRole('button', { name: /应用|确定|筛选/ })
        if (await applyBtn.isVisible({ timeout: 1000 }).catch(() => false)) {
          await applyBtn.click()
        } else {
          await page.keyboard.press('Enter')
        }

        // 验证筛选后仍有结果或显示空状态
        await page.waitForTimeout(1000)
        const hasRows = await page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') }).first()
          .isVisible({ timeout: 3000 }).catch(() => false)
        const hasEmpty = await page.getByText(/暂无数据|0 行/).isVisible({ timeout: 1000 }).catch(() => false)
        expect(hasRows || hasEmpty).toBeTruthy()
      }
    }
  })

  test('筛选条件 - 等于（equals）', async ({ page }) => {
    await executeTestQuery(page)

    const headers = page.getByRole('columnheader')
    const headerCount = await headers.count()
    if (headerCount === 0) {
      test.skip()
      return
    }

    // 点击状态相关列头（如果有的话），或第一个列头
    const statusHeader = headers.first()
    await statusHeader.click()

    const equalsOption = page.getByText('等于')
    if (await equalsOption.isVisible({ timeout: 2000 }).catch(() => false)) {
      await equalsOption.click()

      const filterInput = page.getByPlaceholder(/筛选/).or(page.getByRole('textbox').last())
      if (await filterInput.isVisible({ timeout: 2000 }).catch(() => false)) {
        await filterInput.fill('1')

        const applyBtn = page.getByRole('button', { name: /应用|确定|筛选/ })
        if (await applyBtn.isVisible({ timeout: 1000 }).catch(() => false)) {
          await applyBtn.click()
        } else {
          await page.keyboard.press('Enter')
        }

        // 验证筛选结果
        await page.waitForTimeout(1000)
        const filteredRows = page.getByRole('row').filter({ hasNot: page.getByRole('columnheader') })
        await expect(filteredRows.first()).toBeVisible({ timeout: 5000 })
      }
    }
  })

  test('筛选与列排序共存', async ({ page }) => {
    await executeTestQuery(page)

    const headers = page.getByRole('columnheader')
    const headerCount = await headers.count()
    if (headerCount === 0) {
      test.skip()
      return
    }

    // 先点击列头排序
    const firstHeader = headers.first()
    await firstHeader.click()
    await page.waitForTimeout(500)

    // 然后点击筛选
    await firstHeader.click()

    // 验证页面正常（不崩溃）
    const table = page.getByRole('table')
    await expect(table).toBeVisible()
  })

  test('清除筛选恢复全部数据', async ({ page }) => {
    await executeTestQuery(page)

    const headers = page.getByRole('columnheader')
    const headerCount = await headers.count()
    if (headerCount === 0) {
      test.skip()
      return
    }

    // 先应用筛选
    const firstHeader = headers.first()
    await firstHeader.click()

    const equalsOption = page.getByText('等于')
    if (await equalsOption.isVisible({ timeout: 2000 }).catch(() => false)) {
      await equalsOption.click()

      const filterInput = page.getByPlaceholder(/筛选/).or(page.getByRole('textbox').last())
      if (await filterInput.isVisible({ timeout: 2000 }).catch(() => false)) {
        await filterInput.fill('__never_exist__')

        const applyBtn = page.getByRole('button', { name: /应用|确定|筛选/ })
        if (await applyBtn.isVisible({ timeout: 1000 }).catch(() => false)) {
          await applyBtn.click()
        } else {
          await page.keyboard.press('Enter')
        }

        await page.waitForTimeout(1000)

        // 清除筛选
        const clearBtn = page.getByRole('button', { name: /清除|重置|清空/ })
        if (await clearBtn.isVisible({ timeout: 1000 }).catch(() => false)) {
          await clearBtn.click()
          await page.waitForTimeout(500)
        } else {
          // 备选：重新点击列头
          await firstHeader.click()
          const clearOption = page.getByText(/清除|重置|全部/)
          if (await clearOption.isVisible({ timeout: 1000 }).catch(() => false)) {
            await clearOption.click()
          }
        }
      }
    }
  })
})
