import { defineConfig, devices } from '@playwright/test'

/**
 * Real E2E test configuration — connects to a live backend.
 *
 * Prerequisites:
 *   1. Backend running (docker-compose up or docker-compose -f docker-compose.e2e.yaml up)
 *   2. Frontend built & served at the baseURL, or backend serving static files
 *   3. No mock — all API calls hit the real backend
 */
export default defineConfig({
  testDir: './e2e-real',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI
    ? [['list'], ['html', { open: 'never', outputFolder: 'playwright-report-e2e' }]]
    : [['list'], ['html', { open: 'never', outputFolder: 'playwright-report-e2e' }]],

  timeout: 30_000, // 30s per test

  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8080',
    trace: 'on-first-retry',
    navigationTimeout: 15_000,
    actionTimeout: 10_000,
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],

  // No webServer — backend is provided by docker-compose.
  // The user is responsible for starting the stack before running tests.
})
