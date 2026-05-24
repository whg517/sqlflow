import { defineConfig, devices } from '@playwright/test'

/**
 * SQLFlow E2E Test Configuration
 *
 * Projects:
 *   - mock: route-mocked API tests (no backend needed, uses Go embed serve on 8080)
 *   - real: real backend API tests (requires docker-compose stack)
 *
 * Run:
 *   npx playwright test --project=mock    # Mock E2E
 *   npx playwright test --project=real    # Real E2E
 *   npx playwright test                   # All projects
 */
export default defineConfig({
  testDir: './tests',

  // Timeouts
  timeout: 30_000,
  expect: { timeout: 10_000 },

  // Retry on CI
  retries: process.env.CI ? 2 : 0,
  forbidOnly: !!process.env.CI,

  // Serial execution — E2E tests have shared state
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
  globalSetup: require.resolve('./globalSetup'),
  globalTeardown: require.resolve('./globalTeardown'),

  // Projects: mock and real separated by directory
  projects: [
    {
      name: 'mock',
      testDir: './tests/mock',
      testMatch: '**/*.spec.ts',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8080',
      },
    },
    {
      name: 'real',
      testDir: './tests/real',
      testMatch: '**/*.spec.ts',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8080',
      },
    },
  ],
})
