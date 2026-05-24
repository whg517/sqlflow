/**
 * fixtures/index.ts — Unified test entry point
 *
 * All test files import from here:
 *   import { test } from '../fixtures'
 *
 * Automatically selects mock or real test base based on PLAYWRIGHT_PROJECT env var.
 * Mock tests use route-mocked auth + API responses.
 * Real tests use real backend login + API calls.
 */
import { test as realTest } from './auth.fixture'
import { test as mockTest } from './mock-auth.fixture'

const project = process.env.PLAYWRIGHT_PROJECT ?? 'mock'

export const test = project === 'real' ? realTest : mockTest

// Re-export expect for convenience
export { expect } from '@playwright/test'
