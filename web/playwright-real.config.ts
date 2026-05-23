import { defineConfig, devices } from '@playwright/test'

/**
 * Playwright config for real E2E tests (no mocks).
 * Requires:
 *   - Frontend dev server at http://localhost:5173
 *   - Backend API at http://localhost:8080
 *   - MySQL at mysql:3306 (user=root, password=123456, database=testdb)
 *
 * Usage:
 *   npx playwright test --config=playwright-real.config.ts
 *   npx playwright test --config=playwright-real.config.ts --grep "数据源管理"
 */
export default defineConfig({
  testDir: '.',
  testMatch: ['e2e-real/**/*.spec.ts'],
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report-real' }]],
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'on-first-retry',
    // No timeout override — defaults are fine
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // Reuse existing servers — don't start them here since they're external
  webServer: [
    {
      command: 'npm run dev',
      url: 'http://localhost:5173',
      reuseExistingServer: true,
      timeout: 30000,
    },
  ],
})
