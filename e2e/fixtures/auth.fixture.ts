/**
 * fixtures/auth.fixture.ts — Real test fixtures (SF-QA0027)
 *
 * Authenticates against the real backend API.
 * Requires docker-compose.test.yml stack running.
 */
import { test as base, type Page, expect, request } from '@playwright/test'
import { login, loginViaUI, logoutViaUI, apiRequest, waitForBackend, cleanup, resetToken } from '../support/real-api'

type RealAuthFixture = {
  authenticatedPage: Page
  login: (page: Page, username?: string, password?: string) => Promise<string>
  loginViaUI: typeof loginViaUI
  logoutViaUI: typeof logoutViaUI
  apiRequest: typeof apiRequest
}

export const test = base.extend<RealAuthFixture>({
  authenticatedPage: async ({ page }, use) => {
    await loginViaUI(page)
    await use(page)
  },

  login: async ({}, use) => {
    await use(login)
  },

  loginViaUI: async ({}, use) => {
    await use(loginViaUI)
  },

  logoutViaUI: async ({}, use) => {
    await use(logoutViaUI)
  },

  apiRequest: async ({}, use) => {
    await use(apiRequest)
  },
})

export { expect, waitForBackend, cleanup, resetToken }
