/**
 * SF-QA0048 — E2E 前端交互：数据导出
 *
 * 覆盖审计日志导出、工单导出、水印验证、权限校验等场景。
 * Playwright 模拟真实用户操作，不使用任何 mock。
 *
 * SF-FEAT0054-FE 更新：导出流程改为对话框模式
 * - 点击"导出"按钮打开 ExportDialog
 * - 在对话框中选择格式（CSV/Excel）和字段
 * - 点击"导出 CSV"或"导出 Excel"确认下载
 */
import { test, expect, request as pwRequest } from "@playwright/test";
import {
  BASE_URL,
  loginViaUI,
  loginViaApi,
  getToken,
} from "../support/test-helpers";

test.describe.configure({ timeout: 45_000 });

// Helper: go to audit page
async function gotoAuditPage(page: import("@playwright/test").Page) {
  await loginViaUI(page);
  await page.goto(BASE_URL + "/audit");
  await page.waitForLoadState("networkidle");
}

// Helper: go to tickets page
async function gotoTicketsPage(page: import("@playwright/test").Page) {
  await loginViaUI(page);
  await page.goto(BASE_URL + "/tickets");
  await page.waitForLoadState("networkidle");
}

/** Open export dialog and trigger CSV download. */
async function triggerExport(page: import("@playwright/test").Page, format: "csv" | "xlsx" = "csv") {
  const formatLabel = format === "xlsx" ? "Excel" : "CSV";
  // Click the main export button to open dialog
  await page.getByRole("button", { name: /^导出$/ }).click();
  // Wait for dialog to appear
  await page.waitForSelector("[data-slot='dialog-content']");
  // Click the export confirm button in dialog
  await page.getByRole("button", { name: new RegExp(`导出 ${formatLabel}`) }).click();
}

test.describe("数据导出", () => {
  // ── 1. 审计日志导出 UI ──────────────────────────────────────

  test("审计日志页面：导出按钮可见", async ({ page }) => {
    await gotoAuditPage(page);
    const exportBtn = page.getByRole("button", { name: /^导出$/ });
    await expect(exportBtn).toBeVisible();
  });

  test("审计日志页面：导出按钮包含下载图标", async ({ page }) => {
    await gotoAuditPage(page);
    const exportBtn = page.getByRole("button", { name: /^导出$/ });
    const icon = exportBtn.locator("svg");
    await expect(icon).toBeVisible();
  });

  test("审计日志导出：点击打开导出对话框", async ({ page }) => {
    await gotoAuditPage(page);
    await page.getByRole("button", { name: /^导出$/ }).click();
    // Dialog should be visible
    await expect(page.getByText("导出数据")).toBeVisible();
    await expect(page.getByText("选择导出格式和需要导出的字段")).toBeVisible();
    // CSV and Excel format options
    await expect(page.getByText("CSV")).toBeVisible();
    await expect(page.getByText("Excel")).toBeVisible();
    // Column checkboxes
    await expect(page.getByText("ID")).toBeVisible();
    await expect(page.getByText("用户")).toBeVisible();
  });

  test("审计日志导出：对话框中切换 Excel 格式", async ({ page }) => {
    await gotoAuditPage(page);
    await page.getByRole("button", { name: /^导出$/ }).click();
    // Click Excel option
    await page.getByDisplayValue("xlsx").click();
    // Export button should show "导出 Excel"
    await expect(page.getByRole("button", { name: /导出 Excel/ })).toBeVisible();
  });

  test("审计日志导出：CSV 下载触发成功", async ({ page }) => {
    await gotoAuditPage(page);

    const downloadPromise = page.waitForEvent("download", { timeout: 15000 });
    await triggerExport(page, "csv");
    const download = await downloadPromise;

    const filename = download.suggestedFilename();
    expect(filename).toMatch(/audit_logs.*\.csv/);

    const filePath = await download.path();
    const fs = await import("fs");
    const csvContent = fs.readFileSync(filePath!, "utf-8");

    // CSV should have BOM header (UTF-8 with BOM for Excel compatibility)
    expect(csvContent).toContain("\u{FEFF}");
    expect(csvContent).toContain("ID");
    expect(csvContent).toContain("时间");
    expect(csvContent).toContain("用户");
  });

  test("审计日志导出：CSV 包含水印行", async ({ page }) => {
    await gotoAuditPage(page);

    const downloadPromise = page.waitForEvent("download", { timeout: 15000 });
    await triggerExport(page, "csv");
    const download = await downloadPromise;

    const filePath = await download.path();
    const fs = await import("fs");
    const csvContent = fs.readFileSync(filePath!, "utf-8");

    // Should contain watermark line (starts with #)
    expect(csvContent).toContain("# 导出水印");
  });

  test("审计日志导出：数据行与 API 数据一致", async ({ page }) => {
    await gotoAuditPage(page);

    // Get page data count from API
    const token = await getToken(page);
    const apiResp = await page.request.get(BASE_URL + "/api/audit/logs", {
      headers: { Authorization: `Bearer ${token}` },
    });
    const apiData = await apiResp.json();
    const apiTotal = apiData.data?.total ?? apiData.data?.items?.length ?? 0;

    // Download and count rows
    const downloadPromise = page.waitForEvent("download", { timeout: 15000 });
    await triggerExport(page, "csv");
    const download = await downloadPromise;

    const filePath = await download.path();
    const fs = await import("fs");
    const csvContent = fs.readFileSync(filePath!, "utf-8");
    const dataLines = csvContent.split("\n").filter((l) => l.trim().length > 0 && !l.startsWith("#"));

    // Header + data rows
    expect(dataLines.length - 1).toBeGreaterThanOrEqual(Math.min(apiTotal, 1));
  });

  test("审计日志导出：按钮加载后可点击", async ({ page }) => {
    await gotoAuditPage(page);
    await page.waitForTimeout(1000);
    const exportBtn = page.getByRole("button", { name: /^导出$/ });
    await expect(exportBtn).toBeEnabled();
  });

  test("审计日志导出：成功后显示 toast 提示", async ({ page }) => {
    await gotoAuditPage(page);

    const downloadPromise = page.waitForEvent("download", { timeout: 15000 });
    await triggerExport(page, "csv");
    await downloadPromise;

    const toast = page.locator("[data-sonner-toast]").filter({ hasText: /导出成功|含水印/ });
    await expect(toast).toBeVisible({ timeout: 5000 });
  });

  // ── 2. 工单导出 UI ──────────────────────────────────────

  test("工单页面：导出按钮可见", async ({ page }) => {
    await gotoTicketsPage(page);
    const exportBtn = page.getByRole("button", { name: /^导出$/ });
    await expect(exportBtn).toBeVisible();
  });

  test("工单导出：CSV 下载触发成功", async ({ page }) => {
    await gotoTicketsPage(page);

    const downloadPromise = page.waitForEvent("download", { timeout: 15000 });
    await triggerExport(page, "csv");
    const download = await downloadPromise;

    const filename = download.suggestedFilename();
    expect(filename).toMatch(/tickets.*\.csv/);

    const filePath = await download.path();
    const fs = await import("fs");
    const csvContent = fs.readFileSync(filePath!, "utf-8");

    expect(csvContent).toContain("\u{FEFF}");
    expect(csvContent).toContain("ID");
    expect(csvContent).toContain("提交人");
  });

  test("工单导出：CSV 包含完整字段", async ({ page }) => {
    await gotoTicketsPage(page);

    const downloadPromise = page.waitForEvent("download", { timeout: 15000 });
    await triggerExport(page, "csv");
    const download = await downloadPromise;

    const filePath = await download.path();
    const fs = await import("fs");
    const csvContent = fs.readFileSync(filePath!, "utf-8");

    const lines = csvContent.split("\n");
    const header = lines[0].replace(/^\u{FEFF}/, "");

    expect(header).toContain("ID");
    expect(header).toContain("SQL内容");
    expect(header).toContain("状态");
    expect(header).toContain("风险等级");
  });

  test("工单导出：CSV 包含水印行", async ({ page }) => {
    await gotoTicketsPage(page);

    const downloadPromise = page.waitForEvent("download", { timeout: 15000 });
    await triggerExport(page, "csv");
    const download = await downloadPromise;

    const filePath = await download.path();
    const fs = await import("fs");
    const csvContent = fs.readFileSync(filePath!, "utf-8");
    expect(csvContent).toContain("# 导出水印");
  });

  test("工单导出：成功后显示 toast 提示", async ({ page }) => {
    await gotoTicketsPage(page);

    const downloadPromise = page.waitForEvent("download", { timeout: 15000 });
    await triggerExport(page, "csv");
    await downloadPromise;

    const toast = page.locator("[data-sonner-toast]").filter({ hasText: /导出成功|含水印/ });
    await expect(toast).toBeVisible({ timeout: 5000 });
  });

  // ── 3. API 层导出验证 ──────────────────────────────────────

  test("API 审计导出：返回 200 + CSV 内容", async ({ page }) => {
    const token = await loginViaApi();
    const resp = await page.request.get(BASE_URL + "/api/export/audit", {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(resp.status()).toBe(200);

    const contentType = resp.headers()["content-type"] ?? "";
    expect(contentType).toMatch(/text\/csv|octet-stream/);

    const body = await resp.text();
    expect(body).toContain("ID");
    expect(body).toContain("时间");
  });

  test("API 工单导出：返回 200 + CSV 内容", async ({ page }) => {
    const token = await loginViaApi();
    const resp = await page.request.get(BASE_URL + "/api/export/tickets", {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(resp.status()).toBe(200);

    const contentType = resp.headers()["content-type"] ?? "";
    expect(contentType).toMatch(/text\/csv|octet-stream/);

    const body = await resp.text();
    expect(body).toContain("ID");
    expect(body).toContain("提交人");
  });

  test("API 审计导出：带筛选参数返回正确数据", async ({ page }) => {
    const token = await loginViaApi();

    const resp = await page.request.get(
      BASE_URL + "/api/export/audit?action=audit_export",
      { headers: { Authorization: `Bearer ${token}` } },
    );
    expect(resp.status()).toBe(200);

    const body = await resp.text();
    const lines = body.split("\n").filter((l) => l.trim().length > 0);
    if (lines.length > 1) {
      // Skip header and watermark comment lines
      const dataLines = lines.slice(1).filter((l) => !l.startsWith("#"));
      for (const line of dataLines) {
        expect(line).toContain("audit_export");
      }
    }
  });

  test("API 工单导出：按状态筛选", async ({ page }) => {
    const token = await loginViaApi();

    const resp = await page.request.get(
      BASE_URL + "/api/export/tickets?status=SUBMITTED",
      { headers: { Authorization: `Bearer ${token}` } },
    );
    expect(resp.status()).toBe(200);

    const body = await resp.text();
    const lines = body.split("\n").filter((l) => l.trim().length > 0);
    if (lines.length > 1) {
      const dataLines = lines.slice(1).filter((l) => !l.startsWith("#"));
      for (const line of dataLines) {
        expect(line).toContain("SUBMITTED");
      }
    }
  });

  test("API 审计导出：无权限用户被拒绝", async ({ page }) => {
    const adminToken = await loginViaApi();
    const username = `e2e_viewer_${Date.now()}`;

    await page.request.post(BASE_URL + "/api/users", {
      headers: { Authorization: `Bearer ${adminToken}` },
      data: {
        username,
        password: "Test-pass-123!",
        role: "viewer",
      },
    });

    const loginResp = await page.request.post(BASE_URL + "/api/auth/login", {
      data: { username, password: "Test-pass-123!" },
    });
    const loginData = await loginResp.json();
    const viewerToken = loginData.data?.access_token;

    if (viewerToken) {
      const resp = await page.request.get(BASE_URL + "/api/export/audit", {
        headers: { Authorization: `Bearer ${viewerToken}` },
      });
      expect([401, 403]).toContain(resp.status());
    }

    // Cleanup
    await page.request.delete(
      BASE_URL + `/api/users/username/${username}`,
      { headers: { Authorization: `Bearer ${adminToken}` } },
    );
  });

  // ── 4. 导出任务管理 ──────────────────────────────────────

  test("API 导出任务列表：返回空或数组", async ({ page }) => {
    const token = await loginViaApi();
    const resp = await page.request.get(BASE_URL + "/api/export/tasks", {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(resp.status()).toBe(200);

    const data = await resp.json();
    expect(data.code).toBe(0);
    if (data.data !== null) {
      expect(Array.isArray(data.data)).toBe(true);
    }
  });

  // ── 5. CSV 格式验证 ──────────────────────────────────────

  test("审计导出 CSV：BOM 头正确（Excel 兼容）", async ({ page }) => {
    const token = await loginViaApi();
    const ctx = await pwRequest.newContext({ baseURL: BASE_URL });
    const resp = await ctx.get("/api/export/audit", {
      headers: { Authorization: `Bearer ${token}` },
    });
    const buffer = await resp.body();
    const bytes = new Uint8Array(buffer);

    // BOM for UTF-8 is EF BB BF
    expect(bytes[0]).toBe(0xef);
    expect(bytes[1]).toBe(0xbb);
    expect(bytes[2]).toBe(0xbf);
    await ctx.dispose();
  });

  test("工单导出 CSV：BOM 头正确（Excel 兼容）", async ({ page }) => {
    const token = await loginViaApi();
    const ctx = await pwRequest.newContext({ baseURL: BASE_URL });
    const resp = await ctx.get("/api/export/tickets", {
      headers: { Authorization: `Bearer ${token}` },
    });
    const buffer = await resp.body();
    const bytes = new Uint8Array(buffer);

    expect(bytes[0]).toBe(0xef);
    expect(bytes[1]).toBe(0xbb);
    expect(bytes[2]).toBe(0xbf);
    await ctx.dispose();
  });

  test("审计导出 CSV：字段完整（header 列数 >= 10）", async ({ page }) => {
    const token = await loginViaApi();
    const resp = await page.request.get(BASE_URL + "/api/export/audit", {
      headers: { Authorization: `Bearer ${token}` },
    });
    const body = await resp.text();
    const headerLine = body.split("\n")[0].replace(/^\u{FEFF}/, "");
    const columns = headerLine.split(",");
    expect(columns.length).toBeGreaterThanOrEqual(10);
  });

  test("工单导出 CSV：字段完整（header 列数 >= 10）", async ({ page }) => {
    const token = await loginViaApi();
    const resp = await page.request.get(BASE_URL + "/api/export/tickets", {
      headers: { Authorization: `Bearer ${token}` },
    });
    const body = await resp.text();
    const headerLine = body.split("\n")[0].replace(/^\u{FEFF}/, "");
    const columns = headerLine.split(",");
    expect(columns.length).toBeGreaterThanOrEqual(10);
  });

  // ── 6. 水印验证 ──────────────────────────────────────

  test("审计导出：CSV 水印包含导出人信息", async ({ page }) => {
    const token = await loginViaApi();
    const resp = await page.request.get(BASE_URL + "/api/export/audit", {
      headers: { Authorization: `Bearer ${token}` },
    });
    const body = await resp.text();
    // Watermark should contain the username
    expect(body).toContain("e2eadmin");
  });

  test("工单导出：CSV 水印包含导出人信息", async ({ page }) => {
    const token = await loginViaApi();
    const resp = await page.request.get(BASE_URL + "/api/export/tickets", {
      headers: { Authorization: `Bearer ${token}` },
    });
    const body = await resp.text();
    expect(body).toContain("e2eadmin");
  });

  // ── 7. 页面刷新稳定性 ──────────────────────────────────────

  test("审计日志页面刷新后导出按钮仍可用", async ({ page }) => {
    await gotoAuditPage(page);
    await page.reload();
    await page.waitForLoadState("networkidle");

    const exportBtn = page.getByRole("button", { name: /^导出$/ });
    await expect(exportBtn).toBeVisible();
    await expect(exportBtn).toBeEnabled();
  });

  test("工单页面刷新后导出按钮仍可用", async ({ page }) => {
    await gotoTicketsPage(page);
    await page.reload();
    await page.waitForLoadState("networkidle");

    const exportBtn = page.getByRole("button", { name: /^导出$/ });
    await expect(exportBtn).toBeVisible();
    await expect(exportBtn).toBeEnabled();
  });

  // ── 8. 错误处理 ──────────────────────────────────────

  test("未登录访问审计页：重定向到登录页", async ({ page }) => {
    await page.goto(BASE_URL + "/audit");
    await page.waitForLoadState("networkidle");
    const url = page.url();
    expect(url).toMatch(/\/login|\/auth/);
  });

  test("未登录访问工单页：重定向到登录页", async ({ page }) => {
    await page.goto(BASE_URL + "/tickets");
    await page.waitForLoadState("networkidle");
    const url = page.url();
    expect(url).toMatch(/\/login|\/auth/);
  });
});
