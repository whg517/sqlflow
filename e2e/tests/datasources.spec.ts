import { test, expect, type Page } from '@playwright/test'

// --- Configuration ---
const API_BASE = process.env.E2E_BASE_URL ? process.env.E2E_BASE_URL + '/api' : 'http://localhost:8080/api'
const TEST_DS = {
  name: 'e2e-test-mysql',
  type: 'mysql',
  host: 'mysql',
  port: 3306,
  username: 'root',
  password: '123456',
  database: 'testdb',
  max_open: 10,
}

const ADMIN_USERNAME = process.env.E2E_USERNAME ?? 'e2eadmin'
const ADMIN_PASSWORD = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'
const ADMIN_CREDS = { username: ADMIN_USERNAME, password: ADMIN_PASSWORD }

// --- Helpers ---

/** Login via the real backend and get a JWT token */
async function realLogin(): Promise<{ token: string; refreshToken: string }> {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(ADMIN_CREDS),
  })
  const json = await res.json()
  if (json.code !== 0) {
    throw new Error(`Login failed: ${json.message || JSON.stringify(json)}`)
  }
  return { token: json.data.access_token, refreshToken: json.data.refresh_token }
}

/** Inject token into page localStorage and navigate (bypass UI login) */
async function authSetup(page: Page) {
  const { token } = await realLogin()
  await page.goto('/')
  await page.evaluate((t) => {
    localStorage.setItem('token', t)
  }, token)
}

/** Find and delete a datasource by name via API (cleanup helper) */
async function deleteDatasourceByName(name: string, token: string) {
  // First find it
  const listRes = await fetch(`${API_BASE}/datasources`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const listJson = await listRes.json()
  const ds = (listJson.data ?? []).find(
    (d: { name: string; id: number; status: string }) => d.name === name && d.status !== 'disabled',
  )
  if (ds) {
    await fetch(`${API_BASE}/datasources/${ds.id}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${token}` },
    })
  }
}

/** Fill and submit the datasource creation form */
async function fillDatasourceForm(
  page: Page,
  ds: {
    name: string
    type?: string
    host: string
    port: number
    username: string
    password: string
    database?: string
  },
) {
  await page.getByPlaceholder('2-50 个字符').fill(ds.name)
  if (ds.type) {
    await page.getByRole('combobox').click()
    await page.getByRole('option', { name: ds.type === 'mysql' ? 'MySQL' : 'MongoDB' }).click()
  }
  await page.getByPlaceholder('IP 或域名').fill(ds.host)
  await page.getByPlaceholder('1-65535').fill(String(ds.port))
  await page.getByPlaceholder('数据库用户名').fill(ds.username)
  await page.getByPlaceholder('数据库密码').fill(ds.password)
  if (ds.database) {
    await page.getByPlaceholder('数据库名（可选）').fill(ds.database)
  }
  await page.getByRole('button', { name: '保存' }).click()
}

// --- Tests ---

test.describe('数据源管理 - 真实数据库连接', () => {
  let token: string

  test.beforeAll(async () => {
    // Login once for the whole suite
    const loginResult = await realLogin()
    token = loginResult.token
  })

  test.beforeEach(async ({ page }) => {
    // Inject token and navigate
    await authSetup(page)
  })

  test.afterAll(async () => {
    // Cleanup: remove e2e-test-mysql if still exists
    await deleteDatasourceByName(TEST_DS.name, token)
  })

  test('1. 创建 MySQL 数据源并验证列表中出现', async ({ page }) => {
    // Cleanup first
    await deleteDatasourceByName(TEST_DS.name, token)

    await page.goto('/settings/datasource')
    await page.waitForLoadState('networkidle')

    // Open add dialog
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(page.getByRole('heading', { name: '添加数据源' })).toBeVisible()

    // Fill form
    await fillDatasourceForm(page, TEST_DS)

    // Wait for success toast
    await expect(page.getByText('数据源添加成功')).toBeVisible({ timeout: 10000 })

    // Dialog should close
    await expect(page.getByRole('dialog')).not.toBeVisible()

    // Verify in list
    await expect(page.getByText(TEST_DS.name).first()).toBeVisible({ timeout: 10000 })

    // Verify status is "正常" (active)
    const row = page.getByRole('row').filter({ hasText: TEST_DS.name })
    await expect(row.locator('.bg-emerald-500\\/20, .bg-emerald-500\\/15')).toBeVisible()
  })

  test('2. 查看数据源表结构', async ({ page }) => {
    // Ensure datasource exists
    await deleteDatasourceByName(TEST_DS.name, token)
    const createRes = await fetch(`${API_BASE}/datasources`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(TEST_DS),
    })
    const createJson = await createRes.json()
    expect(createJson.code).toBe(0)
    const dsId = createJson.data.id

    await page.goto('/settings/datasource')
    await page.waitForLoadState('networkidle')

    // Click "测试" button to test connection first
    const testBtn = page.getByRole('button', { name: '测试' }).first()
    await testBtn.click()
    await expect(page.getByText('连接成功')).toBeVisible({ timeout: 15000 })

    // Navigate to query page and fetch tables via API
    // Tables API: GET /api/datasources/:id/tables
    const tablesRes = await fetch(`${API_BASE}/datasources/${dsId}/tables`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const tablesJson = await tablesRes.json()
    expect(tablesJson.code).toBe(0)
    const tables: string[] = tablesJson.data ?? []
    expect(tables.length).toBeGreaterThan(0)
    // Known tables from testdb
    expect(tables).toContain('sys_user')
  })

  test('3. 创建重复名称数据源时提示错误', async ({ page }) => {
    // Ensure the datasource exists
    await deleteDatasourceByName(TEST_DS.name, token)
    await fetch(`${API_BASE}/datasources`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(TEST_DS),
    })

    await page.goto('/settings/datasource')
    await page.waitForLoadState('networkidle')

    // Try to create with the same name
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await fillDatasourceForm(page, TEST_DS)

    // Should show error toast (UNIQUE constraint on name)
    await expect(
      page.getByText(/失败|已存在|错误|duplicate|unique/i).first(),
    ).toBeVisible({ timeout: 10000 })

    // Dialog may or may not close depending on implementation — but error is shown
  })

  test('4. 使用错误密码创建数据源时连接失败', async ({ page }) => {
    const badDS = {
      ...TEST_DS,
      name: 'e2e-test-mysql-bad',
      password: 'wrong_password_12345',
    }

    // Cleanup first
    await deleteDatasourceByName(badDS.name, token)

    await page.goto('/settings/datasource')
    await page.waitForLoadState('networkidle')

    // The create API still succeeds (it just stores the config),
    // but test connection should fail
    await page.getByRole('button', { name: '添加数据源' }).click()
    await expect(page.getByRole('dialog')).toBeVisible()

    await fillDatasourceForm(page, badDS)

    // Creation itself should succeed (backend just stores config)
    await expect(page.getByText('数据源添加成功')).toBeVisible({ timeout: 10000 })
    await expect(page.getByRole('dialog')).not.toBeVisible()

    // Now test the connection — it should fail
    await page.waitForLoadState('networkidle')
    const rows = page.getByRole('row').filter({ hasText: badDS.name })
    await expect(rows).toBeVisible({ timeout: 10000 })

    // Click "测试" (test connection) button for this row
    await rows.getByRole('button', { name: '测试' }).click()

    // Should show connection failure toast
    await expect(page.getByText(/连接失败|连接测试失败/i).first()).toBeVisible({ timeout: 15000 })

    // Cleanup
    await deleteDatasourceByName(badDS.name, token)
  })

  test('5. 删除（禁用）数据源后列表中消失', async ({ page }) => {
    // Ensure datasource exists
    await deleteDatasourceByName(TEST_DS.name, token)
    await fetch(`${API_BASE}/datasources`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(TEST_DS),
    })

    await page.goto('/settings/datasource')
    await page.waitForLoadState('networkidle')

    // Verify it's in the list
    await expect(page.getByText(TEST_DS.name).first()).toBeVisible({ timeout: 10000 })

    // Click "禁用" button
    const row = page.getByRole('row').filter({ hasText: TEST_DS.name })
    await row.getByRole('button', { name: '禁用' }).click()

    // Confirm in the alert dialog
    await expect(page.getByRole('alertdialog')).toBeVisible()
    await expect(page.getByText(/确认禁用数据源/)).toBeVisible()
    await page.getByRole('button', { name: '确认禁用' }).click()

    // Wait for success toast
    await expect(page.getByText('数据源已禁用')).toBeVisible({ timeout: 10000 })

    // The row should no longer be visible (disabled items filtered out by backend)
    // Reload to verify
    await page.reload()
    await page.waitForLoadState('networkidle')
    await expect(page.getByText(TEST_DS.name)).not.toBeVisible({ timeout: 10000 })
  })

  test('6. 通过 API 直接测试完整生命周期', async ({ request }) => {
    // This test verifies the backend API directly, ensuring
    // real MySQL connectivity end-to-end

    const uniqueName = `e2e-lifecycle-${Date.now()}`

    // 1. Create
    const createRes = await request.post(`${API_BASE}/datasources`, {
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      data: { ...TEST_DS, name: uniqueName },
    })
    const createJson = await createRes.json()
    expect(createJson.code).toBe(0)
    const dsId = createJson.data.id
    expect(dsId).toBeGreaterThan(0)

    // 2. List — should contain the new datasource
    const listRes = await request.get(`${API_BASE}/datasources`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const listJson = await listRes.json()
    const ds = listJson.data.find((d: { name: string }) => d.name === uniqueName)
    expect(ds).toBeDefined()
    expect(ds.status).toBe('active')
    expect(ds.host).toBe(TEST_DS.host)
    expect(ds.port).toBe(TEST_DS.port)

    // 3. Test connection
    const testRes = await request.post(`${API_BASE}/datasources/${dsId}/test`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {},
    })
    const testJson = await testRes.json()
    expect(testJson.code).toBe(0)
    expect(testJson.data.success).toBe(true)

    // 4. Get tables
    const tablesRes = await request.get(`${API_BASE}/datasources/${dsId}/tables`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const tablesJson = await tablesRes.json()
    expect(tablesJson.code).toBe(0)
    expect(tablesJson.data.length).toBeGreaterThan(0)

    // 5. Disable
    const delRes = await request.delete(`${API_BASE}/datasources/${dsId}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const delJson = await delRes.json()
    expect(delJson.code).toBe(0)

    // 6. Verify disabled — list should not show it as active
    const listRes2 = await request.get(`${API_BASE}/datasources`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const listJson2 = await listRes2.json()
    const ds2 = listJson2.data.find((d: { name: string; status: string }) => d.name === uniqueName)
    // Backend may return disabled items or filter them — check status
    if (ds2) {
      expect(ds2.status).toBe('disabled')
    }
  })
})
