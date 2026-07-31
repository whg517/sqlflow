import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import { listAllTokens, listMyTokens } from "@/api/token";
import {
  listPermReqs,
  myActivePermissions,
  myPermReqs,
} from "@/api/permission-request";
import { getPerformanceStats, getSlowQueries } from "@/api/performance";

function respond(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    json: async () => body,
  });
}

describe("API response normalization", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("token", "test-token");
  });

  it("reads admin token pagination metadata from the top-level envelope", async () => {
    const token = {
      id: 1,
      user_id: 1,
      name: "CI",
      token_prefix: "sqlflow_test",
      scopes: "read:query",
      expires_at: "2027-01-01T00:00:00Z",
      use_count: 0,
      is_active: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    mockFetch.mockImplementationOnce(() =>
      respond({
        code: 0,
        data: [token],
        page: 2,
        page_size: 20,
        total: 21,
      }),
    );

    const result = await listAllTokens(2, 20);

    expect(result.items).toEqual([token]);
    expect(result).toMatchObject({ page: 2, page_size: 20, total: 21 });
  });

  it("normalizes nullable personal token data", async () => {
    mockFetch.mockImplementationOnce(() => respond({ code: 0, data: null }));

    await expect(listMyTokens()).resolves.toEqual([]);
  });

  it("normalizes nullable permission request envelopes", async () => {
    mockFetch
      .mockImplementationOnce(() =>
        respond({
          code: 0,
          data: null,
          page: 1,
          page_size: 20,
          total: 0,
        }),
      )
      .mockImplementationOnce(() =>
        respond({ code: 0, data: { items: null, total: 0 } }),
      )
      .mockImplementationOnce(() => respond({ code: 0, data: null }));

    await expect(listPermReqs()).resolves.toMatchObject({
      items: [],
      total: 0,
      page: 1,
      pageSize: 20,
    });
    await expect(myPermReqs()).resolves.toEqual({ items: [], total: 0 });
    await expect(myActivePermissions()).resolves.toEqual([]);
  });

  it("normalizes nullable performance collections", async () => {
    mockFetch
      .mockImplementationOnce(() =>
        respond({
          code: 0,
          data: {
            total_queries: 0,
            slow_queries: 0,
            avg_time: 0,
            slow_query_rate: 0,
            daily_trend: null,
            datasource_stats: null,
            top_slow_queries: null,
          },
        }),
      )
      .mockImplementationOnce(() =>
        respond({
          code: 0,
          data: null,
          page: 1,
          page_size: 20,
          total: 0,
        }),
      );

    await expect(getPerformanceStats()).resolves.toMatchObject({
      daily_trend: [],
      datasource_stats: [],
      top_slow_queries: [],
    });
    await expect(getSlowQueries()).resolves.toMatchObject({
      data: [],
      total: 0,
    });
  });
});
