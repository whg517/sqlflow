/**
 * Auth flow E2E — tests against the real backend (no API mocks).
 *
 * Prerequisites
 * ──────────────
 *  1. Backend running on http://localhost:8080  (or set E2E_BASE_URL)
 *  2. Vite dev server proxing /api → backend   (already in vite.config.ts)
 *  3. Default admin account exists: admin / admin123
 *
 * Run:
 *   npx playwright test e2e-real/auth.spec.ts
 */

import { test, expect, request } from '@playwright/test'
import { apiRequest, login, waitForBackend } from './helpers'

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const DEFAULT_USERNAME = process.env.E2E_USERNAME ?? 'admin'
const DEFAULT_PASSWORD = process.env.E2E_PASSWORD ?? 'admin123'

// ---------------------------------------------------------------------------
// Inline helpers (auth-specific, thin wrappers)
// ---------------------------------------------------------------------------

/**
 * Direct HTTP login — returns { status, body } without touching the browser.
 */
async function loginViaApi(credentials: {
  username: string
  password: string
}): Promise<{ status: number; body: Record<string, unknown> }> {
  const resp = await fetch(`${BASE_URL}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(credentials),
  })
  const body = await resp.json()
  return { status: resp.status, body }
}

/**
 * Obtain a real JWT token via the backend API.
 */
async function getRealToken(credentials = {
  username: DEFAULT_USERNAME,
  password: DEFAULT_PASSWORD,
}): Promise<string> {
  const { status, body } = await loginViaApi(credentials)
  if (status !== 200 || (body as { code: number }).code !== 0) {
    throw new Error(`loginViaApi failed: status=${status}`)
  }
  return (body.data as { token: string }).token
}

/**
 * Log in through the browser UI against the real backend.
 */
async function loginViaUI(
  page: import('@playwright/test').Page,
  credentials = { username: DEFAULT_USERNAME, password: DEFAULT_PASSWORD },
) {
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill(credentials.username)
  await page.getByPlaceholder('密码').fill(credentials.password)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 10_000 })
}

/**
 * Inject a token into localStorage and navigate to a target page.
 */
async function injectToken(
  page: import('@playwright/test').Page,
  token: string,
  target = '/query',
) {
  await page.goto('/login')
  await page.evaluate((t) => {
    localStorage.setItem('token', t)
  }, token)
  await page.goto(target)
}

/**
 * Open the avatar dropdown and click "退出登录".
 */
async function logoutViaUI(page: import('@playwright/test').Page) {
  const avatarTrigger = page
    .locator('button')
    .filter({ has: page.locator('[data-slot="avatar-fallback"]') })
    .first()
  await avatarTrigger.click()
  await page.getByText('退出登录').click()
  await page.waitForURL('**/login**', { timeout: 5_000 })
}

/**
 * Open the avatar dropdown and click "修改密码".
 */
async function openChangePasswordDialog(page: import('@playwright/test').Page) {
  const avatarTrigger = page
    .locator('button')
    .filter({ has: page.locator('[data-slot="avatar-fallback"]') })
    .first()
  await avatarTrigger.click()
  await page.getByText('修改密码').click()
  await page.getByRole('dialog').waitFor({ state: 'visible' })
}

/**
 * Change password via the real API (PUT /api/auth/password).
 */
async function changePasswordViaApi(payload: {
  old_password: string
  new_password: string
}): Promise<{ status: number; body: Record<string, unknown> }> {
  const resp = await apiRequest('PUT', '/api/auth/password', payload)
  const body = await resp.json()
  return { status: resp.status(), body }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// Real-backend tests are slower — give them generous timeouts.
test.describe.configure({ timeout: 30_000 })

test.beforeAll(async () => {
  await waitForBackend()
})

// ──────────────────────────────────────────────────────────────────────
// 1. Login success — real API returns token → stored in localStorage → redirected to /query
// ──────────────────────────────────────────────────────────────────────
test('登录成功：获取 token → 存储 → 跳转 /query', async ({ page }) => {
  await loginViaUI(page)

  // Should have been redirected to /query
  await expect(page).toHaveURL(/\/query/)

  // Token must exist in localStorage
  const token = await page.evaluate(() => localStorage.getItem('token'))
  expect(token).toBeTruthy()

  // Verify the token is valid by calling /api/auth/me through the proxy
  const meRes = await page.request.get('/api/auth/me', {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(meRes.status()).toBe(200)
  const meBody = await meRes.json()
  expect(meBody.code).toBe(0)
  expect(meBody.data.username).toBe(DEFAULT_USERNAME)
})

// ──────────────────────────────────────────────────────────────────────
// 2. Login failure — wrong password → 401 → error message displayed
// ──────────────────────────────────────────────────────────────────────
test('登录失败：错误密码 → 401 → 显示错误提示', async ({ page }) => {
  // First verify the backend actually returns 401 for wrong credentials
  const { status } = await loginViaApi({
    username: 'admin',
    password: 'wrong-password-123',
  })
  expect(status).toBe(401)

  // Now test the UI flow
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill('admin')
  await page.getByPlaceholder('密码').fill('wrong-password-123')
  await page.getByRole('button', { name: '登 录' }).click()

  // Should still be on /login
  await expect(page).toHaveURL(/\/login/)

  // An error message should be visible (either server error or generic message)
  const errorVisible = await page
    .getByText(/用户名或密码错误|密码错误|认证失败|Unauthorized|登录失败|Error/i)
    .first()
    .isVisible()
  expect(errorVisible).toBeTruthy()

  // Token should NOT be stored
  const token = await page.evaluate(() => localStorage.getItem('token'))
  expect(token).toBeNull()
})

// ──────────────────────────────────────────────────────────────────────
// 3. Unauthenticated access to protected page → redirect to /login
// ──────────────────────────────────────────────────────────────────────
test('未登录访问受保护页面 → 跳转 /login', async ({ page }) => {
  await page.goto('/login')
  await page.evaluate(() => localStorage.clear())

  await page.goto('/query')

  // AuthGuard should redirect to /login
  await page.waitForURL('**/login**', { timeout: 10_000 })
  await expect(page).toHaveURL(/\/login/)
})

test('未登录访问工单页 → 跳转 /login', async ({ page }) => {
  await page.goto('/login')
  await page.evaluate(() => localStorage.clear())
  await page.goto('/tickets')
  await page.waitForURL('**/login**', { timeout: 10_000 })
  await expect(page).toHaveURL(/\/login/)
})

// ──────────────────────────────────────────────────────────────────────
// 4. Login → logout → token cleared → redirected to /login
// ──────────────────────────────────────────────────────────────────────
test('登录后登出：token 清除 → 跳转 /login', async ({ page }) => {
  await loginViaUI(page)
  await expect(page).toHaveURL(/\/query/)

  // Verify token exists before logout
  const tokenBefore = await page.evaluate(() => localStorage.getItem('token'))
  expect(tokenBefore).toBeTruthy()

  // Perform logout via the UI
  await logoutViaUI(page)

  // Should be on /login now
  await expect(page).toHaveURL(/\/login/)

  // Token must be gone
  const tokenAfter = await page.evaluate(() => localStorage.getItem('token'))
  expect(tokenAfter).toBeNull()
})

// ──────────────────────────────────────────────────────────────────────
// 5. Password change → login with new password succeeds
// ──────────────────────────────────────────────────────────────────────
test('密码修改：用新密码登录成功', async ({ page }) => {
  const NEW_PASSWORD = 'newTestPass99!'
  const ORIGINAL_PASSWORD = DEFAULT_PASSWORD

  // ── Step 1: Verify we can log in with the original password ──
  await loginViaUI(page)
  await expect(page).toHaveURL(/\/query/)

  // ── Step 2: Change password via UI ──
  await openChangePasswordDialog(page)

  await page.getByPlaceholder('请输入当前密码').fill(ORIGINAL_PASSWORD)
  await page.getByPlaceholder('8-128 字符，需包含字母和数字').fill(NEW_PASSWORD)
  await page.getByPlaceholder('再次输入新密码').fill(NEW_PASSWORD)

  await page.getByRole('button', { name: '保存修改' }).click()

  // Wait for success toast or redirect (token invalidated after password change)
  await Promise.race([
    page.waitForURL('**/login**', { timeout: 10_000 }),
    page.getByText('密码修改成功').waitFor({ state: 'visible', timeout: 10_000 }),
  ])

  // ── Step 3: Log in with the new password ──
  await loginViaUI(page, {
    username: DEFAULT_USERNAME,
    password: NEW_PASSWORD,
  })
  await expect(page).toHaveURL(/\/query/)

  // ── Step 4: Revert password back to original (cleanup) ──
  try {
    await changePasswordViaApi({
      old_password: NEW_PASSWORD,
      new_password: ORIGINAL_PASSWORD,
    })
  } catch {
    // If restoration fails (e.g. token was invalidated), re-authenticate and retry
    const token = await getRealToken({
      username: DEFAULT_USERNAME,
      password: NEW_PASSWORD,
    })
    await fetch(`${BASE_URL}/api/auth/password`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        old_password: NEW_PASSWORD,
        new_password: ORIGINAL_PASSWORD,
      }),
    })
  }

  // ── Step 5: Verify we can still log in with the original password ──
  await page.goto('/login')
  await loginViaUI(page)
  await expect(page).toHaveURL(/\/query/)
})

// ──────────────────────────────────────────────────────────────────────
// 6. Expired / invalid token → 401 from API → auto-redirect to /login
// ──────────────────────────────────────────────────────────────────────
test('token 过期/无效 → 访问 API 返回 401 → 自动跳转登录页', async ({ page }) => {
  // Inject a definitely-invalid token
  await injectToken(page, 'expired-fake-token-12345', '/query')

  // The client calls /api/auth/me → backend returns 401 →
  // client.handleUnauthorized() clears token and redirects to /login.
  await page.waitForURL('**/login**', { timeout: 15_000 })
  await expect(page).toHaveURL(/\/login/)

  // Token should be removed
  const token = await page.evaluate(() => localStorage.getItem('token'))
  expect(token).toBeNull()
})

// ──────────────────────────────────────────────────────────────────────
// 7. Verify the real login API response shape
// ──────────────────────────────────────────────────────────────────────
test('真实 API 登录响应格式正确', async ({ request: ctx }) => {
  const res = await ctx.post(`${BASE_URL}/api/auth/login`, {
    data: {
      username: DEFAULT_USERNAME,
      password: DEFAULT_PASSWORD,
    },
  })
  expect(res.status()).toBe(200)

  const body = await res.json()
  expect(body.code).toBe(0)
  expect(body.data).toBeDefined()
  expect(typeof body.data.token).toBe('string')
  expect(body.data.token.length).toBeGreaterThan(0)
})
