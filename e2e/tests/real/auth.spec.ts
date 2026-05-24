/**
 * Auth flow E2E — tests against the real backend (no API mocks).
 * Migrated from web/e2e-real/auth.spec.ts.
 */
import { test, expect } from '@playwright/test'
import { apiRequest, login, waitForBackend, cleanup, resetToken } from '../../support/real-api'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const DEFAULT_USERNAME = process.env.E2E_USERNAME ?? 'e2e-admin'
const DEFAULT_PASSWORD = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

// Real-backend tests are slower — give them generous timeouts.
test.describe.configure({ timeout: 30_000 })

test.beforeAll(async () => {
  await waitForBackend()
})

test.afterAll(async () => {
  await cleanup()
})

// ── Inline helpers (auth-specific, thin wrappers) ──

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

async function loginViaUI(
  page: import('@playwright/test').Page,
  credentials = { username: DEFAULT_USERNAME, password: DEFAULT_PASSWORD },
) {
  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill(credentials.username)
  await page.getByPlaceholder('密码').fill(credentials.password)
  await page.getByRole('button', { name: '登 录' }).click()
  await page.waitForURL('**/query**', { timeout: 10_000 })
}

async function injectToken(page: import('@playwright/test').Page, token: string, target = '/query') {
  await page.goto(`${BASE_URL}/login`)
  await page.evaluate((t) => {
    localStorage.setItem('token', t)
  }, token)
  await page.goto(`${BASE_URL}${target}`)
}

async function logoutViaUI(page: import('@playwright/test').Page) {
  const avatarTrigger = page
    .locator('button')
    .filter({ has: page.locator('[data-slot="avatar-fallback"]') })
    .first()
  await avatarTrigger.click()
  await page.getByText('退出登录').click()
  await page.waitForURL('**/login**', { timeout: 5_000 })
}

async function openChangePasswordDialog(page: import('@playwright/test').Page) {
  const avatarTrigger = page
    .locator('button')
    .filter({ has: page.locator('[data-slot="avatar-fallback"]') })
    .first()
  await avatarTrigger.click()
  await page.getByText('修改密码').click()
  await page.getByRole('dialog').waitFor({ state: 'visible' })
}

async function changePasswordViaApi(payload: {
  old_password: string
  new_password: string
}): Promise<{ status: number; body: Record<string, unknown> }> {
  const resp = await apiRequest('PUT', '/api/auth/password', payload)
  const body = await resp.json()
  return { status: resp.status(), body }
}

// ── Tests ──

test('登录成功：获取 token → 存储 → 跳转 /query', async ({ page }) => {
  await loginViaUI(page)
  await expect(page).toHaveURL(/\/query/)

  const token = await page.evaluate(() => localStorage.getItem('token'))
  expect(token).toBeTruthy()

  const meRes = await page.request.get(`${BASE_URL}/api/auth/me`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(meRes.status()).toBe(200)
  const meBody = await meRes.json()
  expect(meBody.code).toBe(0)
  expect(meBody.data.username).toBe(DEFAULT_USERNAME)
})

test('登录失败：错误密码 → 401 → 显示错误提示', async ({ page }) => {
  const { status } = await loginViaApi({
    username: 'e2e-admin',
    password: 'wrong-password-123',
  })
  expect(status).toBe(401)

  await page.goto(`${BASE_URL}/login`)
  await page.getByPlaceholder('用户名').fill('e2e-admin')
  await page.getByPlaceholder('密码').fill('wrong-password-123')
  await page.getByRole('button', { name: '登 录' }).click()

  await expect(page).toHaveURL(/\/login/)

  const errorVisible = await page
    .getByText(/用户名或密码错误|密码错误|认证失败|Unauthorized|登录失败|Error/i)
    .first()
    .isVisible()
  expect(errorVisible).toBeTruthy()

  const token = await page.evaluate(() => localStorage.getItem('token'))
  expect(token).toBeNull()
})

test('未登录访问受保护页面 → 跳转 /login', async ({ page }) => {
  await page.goto(`${BASE_URL}/login`)
  await page.evaluate(() => localStorage.clear())
  await page.goto(`${BASE_URL}/query`)
  await page.waitForURL('**/login**', { timeout: 10_000 })
  await expect(page).toHaveURL(/\/login/)
})

test('未登录访问工单页 → 跳转 /login', async ({ page }) => {
  await page.goto(`${BASE_URL}/login`)
  await page.evaluate(() => localStorage.clear())
  await page.goto(`${BASE_URL}/tickets`)
  await page.waitForURL('**/login**', { timeout: 10_000 })
  await expect(page).toHaveURL(/\/login/)
})

test('登录后登出：token 清除 → 跳转 /login', async ({ page }) => {
  await loginViaUI(page)
  await expect(page).toHaveURL(/\/query/)

  const tokenBefore = await page.evaluate(() => localStorage.getItem('token'))
  expect(tokenBefore).toBeTruthy()

  await logoutViaUI(page)
  await expect(page).toHaveURL(/\/login/)

  const tokenAfter = await page.evaluate(() => localStorage.getItem('token'))
  expect(tokenAfter).toBeNull()
})

test('密码修改：用新密码登录成功', async ({ page }) => {
  const NEW_PASSWORD = 'e2e-new-pass-' + Date.now()
  const ORIGINAL_PASSWORD = DEFAULT_PASSWORD

  // Step 1: Login with original
  await loginViaUI(page)
  await expect(page).toHaveURL(/\/query/)

  // Step 2: Change password via UI
  await openChangePasswordDialog(page)
  await page.getByPlaceholder('请输入当前密码').fill(ORIGINAL_PASSWORD)
  await page.getByPlaceholder('8-128 字符，需包含字母和数字').fill(NEW_PASSWORD)
  await page.getByPlaceholder('再次输入新密码').fill(NEW_PASSWORD)
  await page.getByRole('button', { name: '保存修改' }).click()

  await Promise.race([
    page.waitForURL('**/login**', { timeout: 10_000 }),
    page.getByText('密码修改成功').waitFor({ state: 'visible', timeout: 10_000 }),
  ])

  // Step 3: Login with new password
  await loginViaUI(page, { username: DEFAULT_USERNAME, password: NEW_PASSWORD })
  await expect(page).toHaveURL(/\/query/)

  // Step 4: Revert (best-effort)
  try {
    await changePasswordViaApi({
      old_password: NEW_PASSWORD,
      new_password: ORIGINAL_PASSWORD,
    })
  } catch {
    const token = await getRealToken({ username: DEFAULT_USERNAME, password: NEW_PASSWORD })
    await fetch(`${BASE_URL}/api/auth/password`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ old_password: NEW_PASSWORD, new_password: ORIGINAL_PASSWORD }),
    })
  }

  // Step 5: Verify original still works
  resetToken()
  await loginViaUI(page)
  await expect(page).toHaveURL(/\/query/)
})

test('token 过期/无效 → 访问 API 返回 401 → 自动跳转登录页', async ({ page }) => {
  await injectToken(page, 'expired-fake-token-12345', '/query')
  await page.waitForURL('**/login**', { timeout: 15_000 })
  await expect(page).toHaveURL(/\/login/)

  const token = await page.evaluate(() => localStorage.getItem('token'))
  expect(token).toBeNull()
})

test('真实 API 登录响应格式正确', async ({ request: ctx }) => {
  const res = await ctx.post(`${BASE_URL}/api/auth/login`, {
    data: { username: DEFAULT_USERNAME, password: DEFAULT_PASSWORD },
  })
  expect(res.status()).toBe(200)

  const body = await res.json()
  expect(body.code).toBe(0)
  expect(body.data).toBeDefined()
  expect(typeof body.data.token).toBe('string')
  expect(body.data.token.length).toBeGreaterThan(0)
})
