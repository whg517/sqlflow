import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import { getCoverageAvailability } from "@/api/coverage";

describe("Coverage API", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("token", "test-token");
  });

  afterEach(() => localStorage.clear());

  it("loads the runtime feature status", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        enabled: false,
        reason: "未配置独立的 Coverage PostgreSQL 数据库",
      }),
    });

    const status = await getCoverageAvailability();

    expect(status.enabled).toBe(false);
    expect(status.reason).toContain("PostgreSQL");
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v1/coverage/status",
      expect.objectContaining({ method: "GET" }),
    );
  });
});
