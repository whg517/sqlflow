import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import { listAuditLogs, searchAuditLogs } from "@/api/audit";

function mockJson(data: unknown) {
  mockFetch.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => data,
  });
}

describe("Audit API Functions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("token", "test-token");
  });

  afterEach(() => localStorage.clear());

  describe("listAuditLogs", () => {
    it("fetches audit logs with no params", async () => {
      mockJson({ data: [], page: 1, page_size: 50, total: 0 });
      const res = await listAuditLogs();
      expect(res.data).toEqual([]);
      expect(res.page).toBe(1);
    });

    it("fetches audit logs with all filter params", async () => {
      mockJson({ data: [], page: 2, page_size: 20, total: 0 });
      const res = await listAuditLogs({
        page: 2,
        page_size: 20,
        user_id: "5",
        action: "SELECT",
        datasource_id: "10",
        start: "2026-01-01",
        end: "2026-05-23",
        keyword: "users",
      });
      expect(res.page).toBe(2);
      const url = mockFetch.mock.calls[0][0];
      expect(url).toContain("page=2");
      expect(url).toContain("action=SELECT");
      expect(url).toContain("keyword=users");
    });

    it("builds correct query string with partial params", async () => {
      mockJson({ data: [], page: 1, page_size: 50, total: 0 });
      await listAuditLogs({ action: "DELETE", keyword: "temp" });
      const url = mockFetch.mock.calls[0][0];
      expect(url).toContain("action=DELETE");
      expect(url).toContain("keyword=temp");
      expect(url).not.toContain("page=");
    });
  });

  describe("searchAuditLogs", () => {
    it("searches audit logs with keyword", async () => {
      mockJson({ data: [], page: 1, page_size: 5, total: 0 });
      const res = await searchAuditLogs("SELECT * FROM users");
      expect(res.page_size).toBe(5);
      const url = mockFetch.mock.calls[0][0];
      expect(url).toContain("keyword=");
      expect(url).toContain("page_size=5");
    });

    it("searches with custom page size", async () => {
      mockJson({ data: [], page: 1, page_size: 10, total: 0 });
      const res = await searchAuditLogs("test", 10);
      expect(res.page_size).toBe(10);
      const url = mockFetch.mock.calls[0][0];
      expect(url).toContain("page_size=10");
    });
  });
});
