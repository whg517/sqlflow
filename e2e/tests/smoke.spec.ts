/**
 * Smoke tests — real E2E against the backend.
 * Migrated from web/e2e-real/smoke.spec.ts.
 */
import { test, expect } from '@playwright/test'
import { login, waitForBackend, cleanup } from '../support/real-api'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:8080'
const USERNAME = process.env.E2E_USERNAME ?? 'e2eadmin'
const PASSWORD = process.env.E2E_PASSWORD ?? 'e2e-test-pass-123'

test.describe('Real E2E Smoke Tests', () => {
  test.beforeAll(async () => {
    await waitForBackend()
  })

  test.afterAll(async () => {
    await cleanup()
  })

  test('backend health endpoint is reachable', async () => {
    const resp = await fetch(`${BASE_URL}/health`)
    expect(resp.ok).toBeTruthy()
    const body = await resp.json()
    expect(body).toBeDefined()
  })

  test('real login returns a valid JWT', async () => {
    const resp = await fetch(`${BASE_URL}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: USERNAME, password: PASSWORD }),
    })
    expect(resp.ok).toBeTruthy()
    const body = await resp.json()
    expect(body.code).toBe(0)
    expect(body.data.access_token).toBeDefined()
    expect(typeof body.data.access_token).toBe('string')
    expect(body.data.access_token.length).toBeGreaterThan(0)
  })

  test('login and navigate to query page', async ({ page }) => {
    await login(page)

    await page.goto(`${BASE_URL}/query`)
    await page.waitForLoadState('networkidle')

    await expect(page).toHaveTitle(/SQLFlow/i)
  })
})
