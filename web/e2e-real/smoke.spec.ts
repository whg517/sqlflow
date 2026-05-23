import { test, expect } from '@playwright/test'
import { login, waitForBackend, cleanup } from './helpers'

test.describe('Real E2E Smoke Tests', () => {
  test.beforeAll(async () => {
    // Ensure the backend is reachable before any test runs.
    await waitForBackend()
  })

  test.afterAll(async () => {
    // Clean up any resources created during the test run.
    await cleanup()
  })

  test('backend health endpoint is reachable', async () => {
    const resp = await fetch('http://localhost:8080/health')
    expect(resp.ok).toBeTruthy()
    const body = await resp.json()
    // Backend returns { status: 'ok' } or similar
    expect(body).toBeDefined()
  })

  test('real login returns a valid JWT', async () => {
    const resp = await fetch('http://localhost:8080/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: 'admin', password: 'admin123' }),
    })
    expect(resp.ok).toBeTruthy()
    const body = await resp.json()
    expect(body.code).toBe(0)
    expect(body.data.token).toBeDefined()
    expect(typeof body.data.token).toBe('string')
    expect(body.data.token.length).toBeGreaterThan(0)
  })

  test('login and navigate to home page', async ({ page }) => {
    await login(page)

    // After login, the app should redirect to the query page (or home).
    await page.goto('http://localhost:8080/query')
    await page.waitForLoadState('networkidle')

    // Verify the page loaded successfully — check for a key element.
    // The query page should have a SQL editor or similar content.
    await expect(page).toHaveTitle(/SQLFlow/i)
  })
})
