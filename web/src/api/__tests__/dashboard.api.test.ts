import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import { getDashboardStats, getDashboardOverview } from "@/api/dashboard";

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

  describe("getDashboardOverview", () => {
    it("fetches dashboard overview with default time range", async () => {
      const overview = {
        stats: {
          pending_tickets: 5,
          recent_queries_7d: 120,
          active_datasources: 3,
          total_users: 10,
        },
        query_trend: [
          { date: "2026-05-01", count: 10 },
          { date: "2026-05-02", count: 15 },
        ],
        query_sparkline: [
          { date: "2026-05-25", count: 5 },
          { date: "2026-05-26", count: 8 },
        ],
        ticket_sparkline: [
          { date: "2026-05-25", count: 2 },
          { date: "2026-05-26", count: 1 },
        ],
        ticket_status_dist: [
          { status: "PENDING_APPROVAL", count: 5 },
          { status: "DONE", count: 10 },
        ],
        recent_activities: [
          {
            id: 1,
            created_at: "2026-05-31T10:00:00Z",
            username: "admin",
            action: "QUERY",
            summary: "SELECT * FROM users",
          },
        ],
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0, data: overview }),
      });

      const res = await getDashboardOverview();
      expect(res.data.stats.pending_tickets).toBe(5);
      expect(res.data.query_trend).toHaveLength(2);
      expect(res.data.ticket_status_dist).toHaveLength(2);
      expect(res.data.recent_activities).toHaveLength(1);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/dashboard/overview?range=last_30d",
        expect.any(Object),
      );
    });

    it("fetches dashboard overview with custom time range", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          code: 0,
          data: {
            stats: {},
            query_trend: [],
            query_sparkline: [],
            ticket_sparkline: [],
            ticket_status_dist: [],
            recent_activities: [],
          },
        }),
      });

      await getDashboardOverview("today");
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/dashboard/overview?range=today",
        expect.any(Object),
      );
    });
  });
});
