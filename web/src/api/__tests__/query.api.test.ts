import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import {
  executeQuery,
  fetchHistory,
  searchQueryHistory,
  deleteHistory,
  clearHistory,
} from "@/api/query";

function mockJson(data: unknown) {
  mockFetch.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => data,
  });
}

describe("Query API Functions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("token", "test-token");
  });

  afterEach(() => localStorage.clear());

  describe("executeQuery", () => {
    it("executes a query and returns result", async () => {
      const result = {
        columns: ["id", "name"],
        rows: [{ id: 1, name: "Alice" }],
        total: 1,
        execution_time_ms: 50,
        affected_rows: 0,
        desensitized: false,
        desensitized_fields: [],
        warnings: [],
      };
      mockJson({ code: 0, data: result });
      const res = await executeQuery({
        datasource_id: 10,
        database: "testdb",
        sql: "SELECT * FROM users",
      });
      expect(res.columns).toEqual(["id", "name"]);
      expect(res.total).toBe(1);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/query/execute",
        expect.objectContaining({ method: "POST" }),
      );
    });

    it("sends correct request body", async () => {
      mockJson({
        code: 0,
        data: {
          columns: [],
          rows: [],
          total: 0,
          execution_time_ms: 0,
          affected_rows: 0,
          desensitized: false,
          desensitized_fields: [],
          warnings: [],
        },
      });
      await executeQuery({
        datasource_id: 5,
        database: "production",
        sql: "SELECT 1",
      });
      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body).toEqual({
        datasource_id: 5,
        database: "production",
        sql: "SELECT 1",
      });
    });
  });

  describe("fetchHistory", () => {
    it("fetches query history with default params", async () => {
      mockJson({ data: [], page: 1, page_size: 50, total: 0 });
      const res = await fetchHistory();
      expect(res.data).toEqual([]);
      expect(res.page).toBe(1);
    });

    it("fetches with custom pagination", async () => {
      mockJson({ data: [], page: 3, page_size: 10, total: 0 });
      const res = await fetchHistory(3, 10);
      expect(res.page).toBe(3);
      const url = mockFetch.mock.calls[0][0];
      expect(url).toContain("page=3");
      expect(url).toContain("page_size=10");
    });
  });

  describe("searchQueryHistory", () => {
    it("searches query history with keyword", async () => {
      mockJson({ data: [], page: 1, page_size: 5, total: 0 });
      const res = await searchQueryHistory("SELECT users");
      expect(res.page_size).toBe(5);
      const url = mockFetch.mock.calls[0][0];
      expect(url).toContain("keyword=");
    });

    it("searches with custom page and size", async () => {
      mockJson({ data: [], page: 2, page_size: 10, total: 0 });
      const res = await searchQueryHistory("test", 2, 10);
      expect(res.page).toBe(2);
      const url = mockFetch.mock.calls[0][0];
      expect(url).toContain("page=2");
      expect(url).toContain("page_size=10");
    });
  });

  describe("deleteHistory", () => {
    it("deletes a history entry", async () => {
      mockJson({ code: 0 });
      const res = await deleteHistory(1);
      expect(res.code).toBe(0);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/query/history/1",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
  });

  describe("clearHistory", () => {
    it("clears all history", async () => {
      mockJson({ code: 0 });
      const res = await clearHistory();
      expect(res.code).toBe(0);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/query/history",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
  });
});
