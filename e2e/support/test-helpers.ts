/**
 * support/test-helpers.ts — Shared helpers for SF-QA0043+ E2E tests
 *
 * Re-exports from real-api.ts and adds UI helper functions.
 */
import { test, expect, type Page, type APIRequestContext, request } from "@playwright/test";

// --- Config ---

const BASE_URL = process.env.E2E_BASE_URL ?? "http://localhost:8080";
const ADMIN_USER = process.env.E2E_USERNAME ?? "e2eadmin";
const ADMIN_PASS = process.env.E2E_PASSWORD ?? "e2e-test-pass-123";

// --- Internal state ---

let _apiContext: APIRequestContext | undefined;
let _token: string | undefined;

async function getApiContext(): Promise<APIRequestContext> {
  if (!_apiContext) {
    _apiContext = await request.newContext({ baseURL: BASE_URL });
  }
  return _apiContext;
}

// --- Login helpers ---

/** Login via API and return access_token */
async function loginViaApi(
  username?: string,
  password?: string,
): Promise<string> {
  const ctx = await getApiContext();
  const resp = await ctx.post("/api/auth/login", {
    data: {
      username: username ?? ADMIN_USER,
      password: password ?? ADMIN_PASS,
    },
  });
  const body = await resp.json();
  const token = body.data?.access_token;
  if (!token) throw new Error(`Login failed: ${JSON.stringify(body)}`);
  _token = token;
  return token;
}

/** Login via the UI (navigates to login page, fills form, submits) */
async function loginViaUI(page: Page): Promise<void> {
  await page.goto(BASE_URL + "/login");
  await page.waitForLoadState("networkidle");

  // Fill login form
  await page.getByPlaceholder("用户名").fill(ADMIN_USER);
  await page.getByPlaceholder("密码").fill(ADMIN_PASS);

  // Submit
  await page.getByRole("button", { name: /登.*录/i }).click();

  // Wait for redirect to main page
  await page.waitForURL(/\/(query|dashboard|tickets|audit)/, { timeout: 10000 });
}

/** Get current auth token (logs in if needed) */
async function getToken(page?: Page): Promise<string> {
  if (_token) return _token;
  return loginViaApi();
}

// --- Cleanup helpers ---

async function cleanup() {
  // Reset state
  _token = undefined;
  if (_apiContext) {
    await _apiContext.dispose();
    _apiContext = undefined;
  }
}

async function cleanupUsers(prefix: string = "e2e_") {
  const token = await loginViaApi();
  const ctx = await getApiContext();
  const resp = await ctx.get("/api/users", {
    headers: { Authorization: `Bearer ${token}` },
  });
  const body = await resp.json();
  const users = body.data?.items ?? body.data ?? [];
  for (const u of users) {
    if (
      u.username?.startsWith(prefix) ||
      u.username?.startsWith("e2e_dba_") ||
      u.username?.startsWith("e2e_dev_") ||
      u.username?.startsWith("e2e_viewer_")
    ) {
      await ctx.delete(`/api/users/${u.id}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
    }
  }
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
