import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import {
  getDashboardStats,
  getDashboardOverview,
  getTimeRanges,
} from "@/api/dashboard";

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

  describe("getTimeRanges", () => {
    it("returns predefined time ranges", () => {
      const ranges = getTimeRanges();
      expect(ranges).toHaveLength(4);
      expect(ranges[0].key).toBe("today");
      expect(ranges[3].key).toBe("30d");
      // Each range should have required fields
      for (const r of ranges) {
        expect(r).toHaveProperty("key");
        expect(r).toHaveProperty("label");
        expect(r).toHaveProperty("startDate");
        expect(r).toHaveProperty("endDate");
      }
    });
  });

  describe("getDashboardOverview", () => {
    const mockBackendResponse = {
      pending_tickets: 5,
      recent_queries_7d: 120,
      active_datasources: 3,
      pending_ticket_sparkline: [2, 3, 4, 5, 4, 3, 5],
      query_sparkline: [10, 15, 12, 18, 20, 17, 22],
      datasource_sparkline: [2, 2, 3, 3, 3, 3, 3],
      ticket_status_distribution: {
        PENDING_APPROVAL: 5,
        DONE: 10,
        APPROVED: 3,
      },
      query_trend: [
        { date: "2026-05-25", count: 10 },
        { date: "2026-05-26", count: 15 },
      ],
      recent_activity: [
        {
          id: 1,
          user_id: 1,
          action: "QUERY",
          ip_address: "192.168.1.1",
          created_at: "2026-05-31T10:00:00Z",
        },
      ],
    };

    it("fetches and transforms dashboard overview with time range", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0, data: mockBackendResponse }),
      });

      const timeRange = getTimeRanges()[3]; // "30d"
      const res = await getDashboardOverview(timeRange);

      // Verify data transformation
      expect(res.data.pending_tickets.value).toBe(5);
      expect(res.data.query_count.value).toBe(120);
      expect(res.data.active_datasources.value).toBe(3);

      // Verify sparkline transformation
      expect(res.data.pending_tickets.sparkline).toHaveLength(7);
      expect(res.data.pending_tickets.sparkline[0].value).toBe(2);

      // Verify trend computation
      expect(res.data.pending_tickets.trend).toBe(150); // (5-2)/2 * 100

      // Verify ticket distribution transformation
      expect(res.data.ticket_distribution).toHaveLength(3);
      expect(res.data.ticket_distribution[0].status).toBe("待审批");

      // Verify recent activity transformation
      expect(res.data.recent_activity).toHaveLength(1);
      expect(res.data.recent_activity[0].user).toBe("用户#1");

      // Verify URL uses start_date/end_date
      expect(mockFetch).toHaveBeenCalledWith(
        `/api/dashboard/overview?start_date=${timeRange.startDate}&end_date=${timeRange.endDate}`,
        expect.any(Object),
      );
    });

    it("handles empty sparkline data gracefully", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          code: 0,
          data: {
            ...mockBackendResponse,
            pending_ticket_sparkline: [],
            query_sparkline: [],
            datasource_sparkline: [],
            ticket_status_distribution: {},
            query_trend: [],
            recent_activity: [],
          },
        }),
      });

      const timeRange = getTimeRanges()[0]; // "today"
      const res = await getDashboardOverview(timeRange);

      expect(res.data.pending_tickets.sparkline).toHaveLength(0);
      expect(res.data.pending_tickets.trend).toBe(0);
      expect(res.data.ticket_distribution).toHaveLength(0);
      expect(res.data.recent_activity).toHaveLength(0);
    });
  });
});
