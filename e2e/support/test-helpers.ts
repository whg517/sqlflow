/**
 * support/test-helpers.ts — Shared E2E helpers (compatibility shim).
 *
 * Historically this file duplicated loginViaUI / loginViaApi / getToken /
 * cleanup logic that also lives in real-test-helpers.ts (and real-api.ts),
 * leading to behaviour drift and the spec-cleanup races documented in
 * SF-QA0041 (the reason the suite still runs serially with workers:1).
 *
 * This module now re-exports the canonical helpers from real-test-helpers.ts
 * so existing imports keep working against a single implementation. New specs
 * should import from './real-test-helpers' directly.
 *
 * @deprecated import from './real-test-helpers' instead.
 */
import type { Page } from '@playwright/test';
import {
  test,
  expect,
  BASE_URL,
  ADMIN_USER,
  ADMIN_PASS,
  loginViaUI,
  loginViaApi,
  getToken,
  cleanupDatasources,
  cleanupUsers,
} from './real-test-helpers';

/**
 * Aggregate cleanup used by older specs that imported a bare `cleanup`.
 * Runs the canonical per-resource cleanup helpers in sequence.
 */
async function cleanup(_page?: Page): Promise<void> {
  await cleanupDatasources(_page);
  await cleanupUsers();
}

export {
  test,
  expect,
  BASE_URL,
  ADMIN_USER,
  ADMIN_PASS,
  loginViaUI,
  loginViaApi,
  getToken,
  cleanup,
  cleanupUsers,
};
