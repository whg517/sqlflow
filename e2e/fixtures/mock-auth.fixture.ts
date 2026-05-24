/**
 * fixtures/mock-auth.fixture.ts — Mock test fixtures
 *
 * Uses Playwright route mocks to intercept all /api/* calls.
 * No backend required — pages served by Go embed on 8080, API calls mocked.
 */
import { test as base, type Page, expect } from '@playwright/test'
import { mockApiRoutes, loginViaUI as mockLoginViaUI } from '../support/mock-routes'

type MockAuthFixture = {
  authenticatedPage: Page
  login: (page: Page, role?: 'admin' | 'developer' | 'dba') => Promise<void>
}

export const test = base.extend<MockAuthFixture>({
  authenticatedPage: async ({ page }, use) => {
    // Setup: apply all mock routes and login as default admin
    mockApiRoutes(page)
    await mockLoginViaUI(page)
    await use(page)
  },

  login: async ({}, use) => {
    const loginFn = async (
      page: Page,
      role: 'admin' | 'developer' | 'dba' = 'admin',
    ) => {
      mockApiRoutes(page, { role })
      await mockLoginViaUI(page)
    }
    await use(loginFn)
  },
})
