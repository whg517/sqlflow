/**
 * fixtures/index.ts — Unified test fixture
 *
 * All test files import from here:
 *   import { test, expect } from '../fixtures'
 *
 * Every test runs against the real backend via docker-compose.
 */
import { test as base, expect, type Page } from '@playwright/test'
import { loginViaUI, loginViaApi } from '../support/real-test-helpers'

type AuthenticatedFixture = {
  authenticatedPage: Page
}

export const test = base.extend<AuthenticatedFixture>({
  authenticatedPage: async ({ page }, use) => {
    await loginViaUI(page)
    await use(page)
  },
})

export { expect, loginViaUI, loginViaApi }
