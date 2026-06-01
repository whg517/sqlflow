import { defineConfig } from '@playwright/test'

/**
 *
 * All tests run against the real backend (docker-compose.test.yml).
 * No mock/route interception.
 *
 * Environment:
 *   E2E_BASE_URL  — backend URL (default http://localhost:8080)
 *   E2E_USERNAME  — admin username (default e2e-admin)
 *   E2E_PASSWORD  — admin password (default e2e-test-pass-123)
 *
 * Run:
 *   npm run test:e2e    # All E2E tests
 *   npx playwright test  # Same
 */
export default defineConfig({
  testDir: './tests',

  // Timeouts — real backend tests need generous timeouts
  timeout: 45_000,
  expect: { timeout: 10_000 },

  // Retry on CI only
  retries: process.env.CI ? 2 : 0,
  forbidOnly: !!process.env.CI,

  // Serial execution — E2E tests share database state
  workers: 1,
  fullyParallel: false,

  // Reporter
  reporter: [
    ['list'],
    ['html', { open: 'never', outputFolder: 'test-results/playwright-report' }],
  ],

  // Global config
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8080',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    navigationTimeout: 15_000,
    actionTimeout: 10_000,
  },

  // Global setup/teardown
  globalSetup: new URL('./globalSetup.ts', import.meta.url).pathname,
  globalTeardown: new URL('./globalTeardown.ts', import.meta.url).pathname,

  // Projects
  projects: [
    {
      name: 'smoke',
      testDir: './tests',
      testMatch: '**/smoke.spec.ts',
    },
    {
      name: 'real',
      testDir: './tests',
      testIgnore: '**/smoke.spec.ts',
    },
  ],
})
