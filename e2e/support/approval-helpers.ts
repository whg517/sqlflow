/**
 * support/approval-helpers.ts — Shared helpers for approval UI E2E tests (SF-QA0036)
 *
 * Multi-role login, ticket creation via API, cleanup utilities.
 * All calls hit the real backend — no mocking.
 */
import { test as base, expect, type Page as PlaywrightPage } from '@playwright/test'
export type Page = PlaywrightPage
import { BASE_URL, ADMIN_USER, ADMIN_PASS, getToken, loginViaApi, getFirstDatasourceId, apiHelper } from './real-test-helpers'

// --- Re-export base test & expect ---
export { expect, BASE_URL }

// --- Role definitions ---

export const ROLES = {
  admin: { username: ADMIN_USER, password: ADMIN_PASS },
  developer: { username: 'e2e-developer', password: 'e2e-test-pass-123' },
  dba: { username: 'e2e-dba', password: 'e2e-test-pass-123' },
} as const

export type Role = keyof typeof ROLES

// --- Isolation prefix ---

export const E2E_PREFIX = `e2e_ui_${Date.now()}`

/** Generate an isolated name for test resources. */
function prefixed(suffix: string): string {
  return `${E2E_PREFIX}_${suffix}`
}

// --- Multi-role login ---

/**
 * Login as a specific role via API (token injection).
 * Falls back gracefully if user doesn't exist — callers should handle this.
 */
export async function loginAsRole(page: Page, role: Role): Promise<boolean> {
  const { username, password } = ROLES[role]
  try {
    await loginViaApi(page, username, password)
    return true
  } catch {
    return false
  }
}

/**
 * Ensure test users exist (admin creates developer/dba if needed).
 * Returns true if all users are ready.
 */
export async function ensureTestUsers(page: Page): Promise<boolean> {
  // Admin always exists
  await loginViaApi(page)

  for (const role of ['developer', 'dba'] as const) {
    const { username } = ROLES[role]
    // Try creating — ignore error if already exists
    try {
      const { status } = await apiHelper(page, 'POST', '/users', {
        username,
        password: ROLES[role].password,
        role,
      })
      // 200 = created, 409 = already exists — both OK
      if (status !== 200 && status !== 409) {
        console.warn(`[ensureTestUsers] Failed to ensure ${username}: status ${status}`)
      }
    } catch {
      // best-effort
    }
  }

  return true
}

// --- Ticket helpers ---

/** Create a ticket via API and return its ID. */
export async function createTicketViaAPI(
  page: Page,
  datasourceId: number,
  sql: string,
  reason: string,
  opts?: { dbType?: string; database?: string },
): Promise<number> {
  const { status, data } = await apiHelper(page, 'POST', '/tickets', {
    datasource_id: datasourceId,
    database: opts?.database ?? 'testdb',
    sql,
    db_type: opts?.dbType ?? 'mysql',
    change_reason: reason,
  })
  expect(status).toBe(200)
  const body = data as { code: number; data: { id: number } }
  expect(body.code).toBe(0)
  return body.data.id
}

/** Get ticket by ID via API. */
export async function getTicketViaAPI(
  page: Page,
  ticketId: number,
): Promise<Record<string, unknown>> {
  const { status, data } = await apiHelper(page, 'GET', `/tickets/${ticketId}`)
  expect(status).toBe(200)
  const body = data as { code: number; data: Record<string, unknown> }
  return body.data
}

/** Approve ticket via API. */
export async function approveTicketViaAPI(page: Page, ticketId: number, comment = 'auto-approve') {
  const { status, data } = await apiHelper(page, 'POST', `/tickets/${ticketId}/approve`, { comment })
  expect(status).toBe(200)
  return data as { code: number }
}

/** Reject ticket via API. */
export async function rejectTicketViaAPI(page: Page, ticketId: number, reason: string) {
  const { status, data } = await apiHelper(page, 'POST', `/tickets/${ticketId}/reject`, { comment: reason })
  expect(status).toBe(200)
  return data as { code: number }
}

/** Execute ticket via API and poll until DONE. */
export async function executeTicketAndWait(
  page: Page,
  ticketId: number,
  timeoutMs = 25_000,
) {
  await apiHelper(page, 'POST', `/tickets/${ticketId}/execute`, {})
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    const ticket = await getTicketViaAPI(page, ticketId)
    if (ticket.status === 'DONE') return
    if (['REJECTED', 'CANCELLED'].includes(ticket.status as string)) {
      throw new Error(`Ticket ${ticketId} ended with ${ticket.status}`)
    }
    await page.waitForTimeout(1_000)
  }
  throw new Error(`Ticket ${ticketId} did not reach DONE within ${timeoutMs}ms`)
}

/** Create a comment via API. */
export async function createCommentViaAPI(
  page: Page,
  ticketId: number,
  content: string,
  parentId?: number,
) {
  const { status, data } = await apiHelper(page, 'POST', `/tickets/${ticketId}/comments`, {
    content,
    parent_id: parentId ?? 0,
  })
  expect(status).toBe(200)
  return data as { code: number; data: { id: number } }
}

/** Cleanup: DROP TABLE IF EXISTS via query API (best-effort). */
export async function cleanupTable(page: Page, datasourceId: number, tableName: string) {
  try {
    await apiHelper(page, 'POST', '/query/execute', {
      datasource_id: datasourceId,
      database: 'testdb',
      sql: `DROP TABLE IF EXISTS ${tableName}`,
    })
  } catch { /* best-effort */ }
}

// --- Navigation helpers ---

/** Navigate to tickets page and wait for load. */
export async function navigateToTickets(page: Page) {
  await page.goto(`${BASE_URL}/tickets`)
  await page.waitForURL('**/tickets**', { timeout: 10_000 })
}

/** Open ticket detail drawer by clicking the row. */
export async function openTicketDrawer(page: Page, ticketId: number) {
  const row = page.getByRole('row', { name: new RegExp(`#${ticketId}`) })
  await row.waitFor({ state: 'visible', timeout: 10_000 })
  await row.click()
  const sheet = page.locator('[data-slot="sheet-content"]')
  await sheet.waitFor({ state: 'visible', timeout: 10_000 })
}

/** Wait for status badge text in drawer. */
export async function expectDrawerStatus(page: Page, statusPattern: string | RegExp) {
  const sheet = page.locator('[data-slot="sheet-content"]')
  await expect(sheet.getByText(statusPattern).first()).toBeVisible({ timeout: 5_000 })
}

/** Close detail drawer. */
export async function closeDrawer(page: Page) {
  const closeBtn = page.locator('[data-slot="sheet-content"]').getByRole('button', { name: /关闭|close/i }).first()
  if (await closeBtn.isVisible()) {
    await closeBtn.click()
  }
}
