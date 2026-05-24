/**
 * fixtures/auth.fixture.ts — Real test fixtures
 *
 * Authenticates against the real backend API.
 * Requires docker-compose stack running (sqlflow + mysql-test).
 */
import { test as base, type Page, expect, request } from '@playwright/test'
import { login as realLogin, apiRequest } from '../support/real-api'

type RealAuthFixture = {
  authenticatedPage: Page
  login: (page: Page, username?: string, password?: string) => Promise<void>
  apiRequest: typeof apiRequest
}

export const test = base.extend<RealAuthFixture>({
  authenticatedPage: async ({ page }, use) => {
    await realLogin(page)
    await use(page)
  },

  login: async ({}, use) => {
    const loginFn = async (
      page: Page,
      username?: string,
      password?: string,
    ) => {
      await realLogin(page, username, password)
    }
    await use(loginFn)
  },

  apiRequest: async ({}, use) => {
    await use(apiRequest)
  },
})
