/**
 * ticket-approve-execute.spec.ts
 *
 * E2E 工单审批→执行完整链路 UI 交互测试。
 *
 * 聚焦 UI 层交互细节：
 *   - 审批后抽屉内「执行」按钮出现
 *   - 执行确认对话框交互
 *   - 状态 badge 逐步变化（待审批→已通过→已完成）
 *   - 列表页刷新后状态一致
 *
 * 与 ticket-flow.spec.ts 的区别：
 *   - ticket-flow 通过 API 直接审批/执行，验证数据正确性
 *   - 本文件通过 UI 交互触发审批和执行，验证 UI 联动
 *
 * 前置：docker-compose.test.yml 环境，e2eadmin 账号可用
 */
import { test, expect, BASE_URL, loginViaUI, apiHelper, getFirstDatasourceId } from '../support/real-test-helpers'

test.describe.configure({ timeout: 45_000 })

// --- Helpers ---

const UID = Date.now()
const E2E_TABLE = `e2e_appr_exec_${UID}`

/** Create a ticket via API, return ID. */
async function createTicket(
  page: import('@playwright/test').Page,
  datasourceId: number,
  sql: string,
  reason: string,
): Promise<number> {
  const { status, data } = await apiHelper(page, 'POST', '/tickets', {
    datasource_id: datasourceId,
    database: 'testdb',
    sql,
    db_type: 'mysql',
    change_reason: reason,
  })
  expect(status).toBeLessThan(300)
  const body = data as { code: number; data: { id: number } }
  expect(body.code).toBe(0)
  return body.data.id
}

/** Execute ticket and poll until DONE. */
async function executeTicketAndWait(page: import('@playwright/test').Page, ticketId: number) {
  await apiHelper(page, 'POST', `/tickets/${ticketId}/execute`, {})
  const start = Date.now()
  while (Date.now() - start < 25_000) {
    const { data } = await apiHelper(page, 'GET', `/tickets/${ticketId}`)
    const body = data as { code: number; data: { status: string } }
    if (body.data?.status === 'DONE') return
    if (['REJECTED', 'CANCELLED'].includes(body.data?.status)) {
      throw new Error(`Ticket ${ticketId} ended with ${body.data.status}`)
    }
    await page.waitForTimeout(1_000)
  }
  throw new Error(`Ticket ${ticketId} did not reach DONE within 25s`)
}

/** Cleanup: DROP TABLE IF EXISTS (best-effort). */
async function cleanupTable(page: import('@playwright/test').Page, datasourceId: number) {
  try {
    await apiHelper(page, 'POST', '/query/execute', {
      datasource_id: datasourceId,
      database: 'testdb',
      sql: `DROP TABLE IF EXISTS ${E2E_TABLE}`,
    })
  } catch { /* ignore */ }
}

// --- Tests ---

test.describe('工单审批→执行链路 UI 交互', () => {
  let datasourceId: number

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext()
    const page = await ctx.newPage()
    await loginViaUI(page)
    datasourceId = (await getFirstDatasourceId(page)).id
    await cleanupTable(page, datasourceId)
    await ctx.close()
  })

  test.afterAll(async ({ browser }) => {
    try {
      const ctx = await browser.newContext()
      const page = await ctx.newPage()
      await loginViaUI(page)
      await cleanupTable(page, datasourceId)
      await ctx.close()
    } catch { /* ignore */ }
  })

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page)
  })

  // ── 全流程：创建工单 → UI 审批 → UI 执行 → 列表验证 ────────────────────

  test('完整 UI 流程：审批通过 → 执行 → 状态逐步变化', async ({ page }) => {
    // 1. API 创建 DDL 工单（CREATE TABLE）
    const createSql = `CREATE TABLE ${E2E_TABLE} (id INT PRIMARY KEY, val VARCHAR(50))`
    const ticketId = await createTicket(page, datasourceId, createSql, 'E2E: approve→execute UI chain')

    // 2. 导航到工单列表
    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    const ticketRow = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
    await ticketRow.waitFor({ state: 'visible', timeout: 10_000 })

    // 验证列表状态为「待审批」
    await expect(ticketRow.getByText(/待审批|SUBMITTED/)).toBeVisible()

    // 3. 打开详情抽屉
    await ticketRow.click()

    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })
    await expect(sheet.getByText(`工单 #${ticketId}`)).toBeVisible()

    // ─── UI 审批 ───
    await sheet.getByRole('button', { name: '通过' }).click()

    const approveDialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await approveDialog.waitFor({ state: 'visible', timeout: 5_000 })
    await expect(approveDialog.getByText(/确认通过|审批通过/)).toBeVisible()

    // 填写备注
    await page.getByPlaceholder(/审批备注|填写/).first().fill('SQL 安全，已确认')

    // 确认审批
    await approveDialog.getByRole('button', { name: '确认通过' }).click()

    // 验证 toast
    await expect(page.getByText(/审批通过|已通过/)).toBeVisible({ timeout: 10_000 })

    // 验证抽屉内状态 badge 变为「已通过」
    await expect(sheet.getByText(/已通过|APPROVED/)).toBeVisible({ timeout: 5_000 })

    // ─── UI 执行 ───

    // 审批后「执行」按钮应可见
    await expect(sheet.getByRole('button', { name: '执行' })).toBeVisible({ timeout: 5_000 })

    // 点击执行
    await sheet.getByRole('button', { name: '执行' }).click()

    // 验证执行确认对话框
    const execDialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await execDialog.waitFor({ state: 'visible', timeout: 5_000 })
    await expect(execDialog.getByText(/执行工单|确认执行/)).toBeVisible()

    // 确认执行
    await execDialog.getByRole('button', { name: '确认执行' }).click()

    // 验证执行成功 toast
    await expect(page.getByText(/工单已执行|执行成功|已完成/)).toBeVisible({ timeout: 15_000 })

    // 验证状态 badge 变为「已完成」
    await expect(sheet.getByText(/已完成|DONE/)).toBeVisible({ timeout: 10_000 })

    // ─── 列表验证 ───

    // 关闭抽屉
    await page.locator('button').filter({ has: page.locator('svg.lucide-x') }).first().click()
    await expect(sheet).not.toBeVisible({ timeout: 5_000 })

    // 刷新列表，验证状态同步
    await page.reload()
    await page.waitForURL('**/tickets**')
    await expect(page.getByRole('table').getByText(/已完成|DONE/)).toBeVisible({ timeout: 10_000 })
  })

  // ── 执行确认对话框可以取消 ───────────────────────────────────────────────

  test('执行确认对话框可以取消', async ({ page }) => {
    // 创建工单并 API 审批
    const tid = await createTicket(
      page, datasourceId,
      `SELECT 1 AS e2e_exec_cancel`,
      'E2E: execute dialog cancel test',
    )
    // API 审批以便工单变为 APPROVED
    await apiHelper(page, 'POST', `/tickets/${tid}/approve`, { comment: 'auto approve' })

    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 打开详情
    await page.getByRole('row', { name: new RegExp(`#${tid}`) }).click()
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // 等待审批状态生效
    await expect(sheet.getByText(/已通过|APPROVED/)).toBeVisible({ timeout: 5_000 })
    await expect(sheet.getByRole('button', { name: '执行' })).toBeVisible({ timeout: 5_000 })

    // 打开执行确认对话框
    await sheet.getByRole('button', { name: '执行' }).click()
    const dialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await dialog.waitFor({ state: 'visible', timeout: 5_000 })

    // 取消
    await page.getByRole('button', { name: '取消' }).first().click()
    await expect(dialog).not.toBeVisible({ timeout: 5_000 })

    // 工单状态不变
    await expect(sheet.getByText(/已通过|APPROVED/)).toBeVisible()
  })

  // ── DDL 工单：CREATE TABLE → DROP TABLE 全链路 ─────────────────────────

  test('DDL 全链路：CREATE TABLE → 审批 → 执行 → DROP TABLE → 审批 → 执行', async ({ page }) => {
    // ─── CREATE TABLE ───
    const createSql = `CREATE TABLE ${E2E_TABLE}_ddl (id INT PRIMARY KEY, data TEXT)`
    const createTid = await createTicket(page, datasourceId, createSql, 'E2E: DDL CREATE TABLE chain')

    await page.goto(`${BASE_URL}/tickets`)
    await page.waitForURL('**/tickets**')

    // 打开详情 → 审批 → 执行
    await page.getByRole('row', { name: new RegExp(`#${createTid}`) }).click()
    const sheet = page.locator('[data-slot="sheet-content"]')
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // UI 审批
    await sheet.getByRole('button', { name: '通过' }).click()
    const dialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await dialog.waitFor({ state: 'visible', timeout: 5_000 })
    await page.getByPlaceholder(/审批备注|填写/).first().fill('CREATE approved')
    await dialog.getByRole('button', { name: '确认通过' }).click()
    await expect(page.getByText(/审批通过|已通过/)).toBeVisible({ timeout: 10_000 })

    // UI 执行
    await sheet.getByRole('button', { name: '执行' }).click()
    const execDialog = page.getByRole('dialog').or(page.getByRole('alertdialog'))
    await execDialog.waitFor({ state: 'visible', timeout: 5_000 })
    await execDialog.getByRole('button', { name: '确认执行' }).click()
    await expect(sheet.getByText(/已完成|DONE/)).toBeVisible({ timeout: 15_000 })

    // 验证表已创建
    const { data: descData } = await apiHelper(page, 'POST', '/query/execute', {
      datasource_id: datasourceId,
      database: 'testdb',
      sql: `DESCRIBE ${E2E_TABLE}_ddl`,
    })
    const descResult = descData as { code: number; data: { rows: Array<Record<string, unknown>> } }
    expect(descResult.code).toBe(0)
    const fields = descResult.data.rows.map((r) => String(r['Field'] ?? r['field'] ?? ''))
    expect(fields).toContain('id')
    expect(fields).toContain('data')

    // ─── DROP TABLE ───
    const dropSql = `DROP TABLE ${E2E_TABLE}_ddl`
    const dropTid = await createTicket(page, datasourceId, dropSql, 'E2E: DDL DROP TABLE cleanup')

    // 关闭当前抽屉，重新打开 DROP 工单
    await page.locator('button').filter({ has: page.locator('svg.lucide-x') }).first().click()
    await sheet.waitFor({ state: 'hidden', timeout: 5_000 })

    await page.reload()
    await page.waitForURL('**/tickets**')

    // 打开 DROP 工单 → 审批 → 执行
    await page.getByRole('row', { name: new RegExp(`#${dropTid}`) }).click()
    await sheet.waitFor({ state: 'visible', timeout: 10_000 })

    // UI 审批
    await sheet.getByRole('button', { name: '通过' }).click()
    await dialog.waitFor({ state: 'visible', timeout: 5_000 })
    await page.getByPlaceholder(/审批备注|填写/).first().fill('DROP approved')
    await dialog.getByRole('button', { name: '确认通过' }).click()
    await expect(page.getByText(/审批通过|已通过/)).toBeVisible({ timeout: 10_000 })

    // UI 执行
    await sheet.getByRole('button', { name: '执行' }).click()
    await execDialog.waitFor({ state: 'visible', timeout: 5_000 })
    await execDialog.getByRole('button', { name: '确认执行' }).click()
    await expect(sheet.getByText(/已完成|DONE/)).toBeVisible({ timeout: 15_000 })

    // 验证表已删除
    let tableGone = false
    try {
      await apiHelper(page, 'POST', '/query/execute', {
        datasource_id: datasourceId,
        database: 'testdb',
        sql: `SELECT * FROM ${E2E_TABLE}_ddl`,
      })
    } catch {
      tableGone = true
    }
    expect(tableGone).toBe(true)
  })
})
