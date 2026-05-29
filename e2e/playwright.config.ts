import { defineConfig, devices } from '@playwright/test'

/**
 * SQLFlow E2E Test Configuration
 *
 * SF-QA0027: Unified real-backend E2E tests.
 *
 * Projects:
 *   - real:   All E2E tests against the real backend (docker-compose.test.yml stack)
 *   - mock:   Legacy UI-only tests using route mocks (retained for pure frontend interaction tests)
 *
 * Environment:
 *   E2E_BASE_URL  — backend URL (default http://localhost:8080)
 *   E2E_USERNAME  — admin username (default e2e-admin)
 *   E2E_PASSWORD  — admin password (default e2e-test-pass-123)
 *
 * Run:
 *   npm run test:e2e           # Real E2E (default)
 *   npm run test:e2e:mock      # Legacy mock tests
 *   npm run test:e2e:all       # All projects
 */
export default defineConfig({
  testDir: './tests',

  // Timeouts — real backend tests need more time
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
  globalSetup: require.resolve('./globalSetup'),
  globalTeardown: require.resolve('./globalTeardown'),

  // Projects
  projects: [
    {
      name: 'real',
      testDir: './tests/real',
      testMatch: '**/*.spec.ts',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8080',
      },
    },
    {
      name: 'mock',
      testDir: './tests/mock',
      testMatch: '**/*.spec.ts',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8080',
      },
    },
  ],
})
