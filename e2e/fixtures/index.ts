/**
 * fixtures/index.ts — Unified test entry point (SF-QA0027)
 *
 * All test files import from here:
 *   import { test } from '../fixtures'
 *
 * Default project is 'real' (real backend).
 * Set PLAYWRIGHT_PROJECT=mock for legacy route-mocked tests.
 */
import { test as realTest } from './auth.fixture'
import { test as mockTest } from './mock-auth.fixture'

const project = process.env.PLAYWRIGHT_PROJECT ?? 'real'

export const test = project === 'mock' ? mockTest : realTest

// Re-export expect for convenience
export { expect } from '@playwright/test'
