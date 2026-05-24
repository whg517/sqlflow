import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import { getDashboardStats } from "@/api/dashboard";

describe("Dashboard API Functions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("token", "test-token");
  });

  afterEach(() => localStorage.clear());

  describe("getDashboardStats", () => {
    it("fetches dashboard stats", async () => {
      const stats = {
        pending_tickets: 5,
        recent_queries_7d: 120,
        active_datasources: 3,
        total_users: 10,
        sensitive_tables: 8,
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0, data: stats }),
      });

      const res = await getDashboardStats();
      expect(res.data.pending_tickets).toBe(5);
      expect(res.data.recent_queries_7d).toBe(120);
      expect(res.data.active_datasources).toBe(3);
      expect(res.data.total_users).toBe(10);
      expect(res.data.sensitive_tables).toBe(8);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/dashboard/stats",
        expect.any(Object),
      );
    });
  });
});
