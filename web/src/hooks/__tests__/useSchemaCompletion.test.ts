import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// --- Mocks ---

vi.mock("@/api/maskRule", () => ({
  fetchDatasourceTables: vi.fn(),
}));

vi.mock("@/api/client", () => ({
  api: {
    get: vi.fn(),
  },
}));

import { fetchDatasourceTables } from "@/api/maskRule";
import { api } from "@/api/client";
import { useSchemaCompletion } from "../useSchemaCompletion";

const mockedFetchTables = vi.mocked(fetchDatasourceTables);
const mockedApiGet = vi.mocked(api.get);

// --- Helpers ---

function fakeNow() {
  return Date.now();
}

function advanceTime(ms: number) {
  vi.useFakeTimers();
  vi.setSystemTime(fakeNow() + ms);
  vi.useRealTimers();
}

// --- Tests ---

describe("useSchemaCompletion", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("初始状态：缓存为空，loading 为 false", () => {
    const { result } = renderHook(() => useSchemaCompletion());

    expect(result.current.loading).toBe(false);
    expect(typeof result.current.fetchTables).toBe("function");
    expect(typeof result.current.fetchColumns).toBe("function");
    expect(typeof result.current.clearCache).toBe("function");
    expect(typeof result.current.clearDatasourceCache).toBe("function");
  });

  it("fetchTables 成功：缓存表名列表", async () => {
    const tableNames = ["users", "orders", "products"];
    mockedFetchTables.mockResolvedValueOnce({
      code: 0,
      message: "ok",
      data: tableNames,
    } as any);

    const { result } = renderHook(() => useSchemaCompletion());

    let schema: any = null;
    await act(async () => {
      schema = await result.current.fetchTables(1);
    });

    expect(schema).not.toBeNull();
    expect(schema!.tables).toEqual(tableNames);
    expect(schema!.columns).toBeInstanceOf(Map);
    expect(schema!.fetchedAt).toBeGreaterThan(0);
    expect(mockedFetchTables).toHaveBeenCalledTimes(1);
    expect(mockedFetchTables).toHaveBeenCalledWith(1);
  });

  it("fetchColumns 成功：缓存字段列表", async () => {
    // First, populate tables cache
    mockedFetchTables.mockResolvedValueOnce({
      code: 0,
      message: "ok",
      data: ["users"],
    } as any);

    const columns = ["id", "name", "email", "created_at"];
    mockedApiGet.mockResolvedValueOnce({
      code: 0,
      data: columns,
    } as any);

    const { result } = renderHook(() => useSchemaCompletion());

    // fetchTables first to populate cache
    await act(async () => {
      await result.current.fetchTables(1);
    });

    // Then fetchColumns
    let cols: string[] = [];
    await act(async () => {
      cols = await result.current.fetchColumns(1, "users");
    });

    expect(cols).toEqual(columns);
    expect(mockedApiGet).toHaveBeenCalledTimes(1);
    expect(mockedApiGet).toHaveBeenCalledWith(
      "/datasources/1/tables/users/columns",
    );
  });

  it("fetchColumns 在没有 schema 缓存时返回空数组", async () => {
    const { result } = renderHook(() => useSchemaCompletion());

    let cols: string[] = [];
    await act(async () => {
      cols = await result.current.fetchColumns(99, "nonexistent");
    });

    expect(cols).toEqual([]);
    expect(mockedApiGet).not.toHaveBeenCalled();
  });

  it("缓存命中：5分钟内不重复请求", async () => {
    mockedFetchTables.mockResolvedValueOnce({
      code: 0,
      message: "ok",
      data: ["users"],
    } as any);

    const { result } = renderHook(() => useSchemaCompletion());

    // First call
    await act(async () => {
      await result.current.fetchTables(1);
    });

    expect(mockedFetchTables).toHaveBeenCalledTimes(1);

    // Second call within 5 minutes — should use cache
    await act(async () => {
      await result.current.fetchTables(1);
    });

    // fetchDatasourceTables should still only be called once (cache hit)
    expect(mockedFetchTables).toHaveBeenCalledTimes(1);
  });

  it("缓存过期：5分钟后重新请求", async () => {
    mockedFetchTables.mockResolvedValue({
      code: 0,
      message: "ok",
      data: ["users"],
    } as any);

    const { result } = renderHook(() => useSchemaCompletion());

    // First call
    await act(async () => {
      await result.current.fetchTables(1);
    });

    expect(mockedFetchTables).toHaveBeenCalledTimes(1);

    // Advance time beyond 5 minutes
    await act(async () => {
      vi.useFakeTimers();
      vi.advanceTimersByTime(5 * 60 * 1000 + 1);
    });

    // Second call after expiry — should fetch again
    await act(async () => {
      await result.current.fetchTables(1);
    });

    expect(mockedFetchTables).toHaveBeenCalledTimes(2);
  });

  it("clearCache：清空所有缓存", async () => {
    mockedFetchTables.mockResolvedValue({
      code: 0,
      message: "ok",
      data: ["users"],
    } as any);

    const { result } = renderHook(() => useSchemaCompletion());

    // Populate cache for two datasources
    await act(async () => {
      await result.current.fetchTables(1);
    });
    await act(async () => {
      await result.current.fetchTables(2);
    });

    expect(mockedFetchTables).toHaveBeenCalledTimes(2);

    // Clear cache
    act(() => {
      result.current.clearCache();
    });

    // Next calls should re-fetch (cache is empty)
    await act(async () => {
      await result.current.fetchTables(1);
    });
    await act(async () => {
      await result.current.fetchTables(2);
    });

    // Each datasource fetched again
    expect(mockedFetchTables).toHaveBeenCalledTimes(4);
  });

  it("clearDatasourceCache：只清空指定数据源的缓存", async () => {
    mockedFetchTables.mockResolvedValue({
      code: 0,
      message: "ok",
      data: ["users"],
    } as any);

    const { result } = renderHook(() => useSchemaCompletion());

    // Populate cache for two datasources
    await act(async () => {
      await result.current.fetchTables(1);
    });
    await act(async () => {
      await result.current.fetchTables(2);
    });

    expect(mockedFetchTables).toHaveBeenCalledTimes(2);

    // Clear only datasource 1's cache
    act(() => {
      result.current.clearDatasourceCache(1);
    });

    // Datasource 1 should re-fetch
    await act(async () => {
      await result.current.fetchTables(1);
    });
    expect(mockedFetchTables).toHaveBeenCalledTimes(3);

    // Datasource 2 should use cache (no additional call)
    await act(async () => {
      await result.current.fetchTables(2);
    });
    expect(mockedFetchTables).toHaveBeenCalledTimes(3);
  });

  it("API 失败：fetchTables 返回 null", async () => {
    mockedFetchTables.mockRejectedValueOnce(new Error("Network error"));

    const { result } = renderHook(() => useSchemaCompletion());

    let schema: any = null;
    await act(async () => {
      schema = await result.current.fetchTables(1);
    });

    expect(schema).toBeNull();
    expect(result.current.loading).toBe(false);
  });

  it("API 失败：fetchColumns 返回空数组并缓存", async () => {
    // First populate tables cache
    mockedFetchTables.mockResolvedValueOnce({
      code: 0,
      message: "ok",
      data: ["users"],
    } as any);

    mockedApiGet.mockRejectedValueOnce(new Error("404 Not Found"));

    const { result } = renderHook(() => useSchemaCompletion());

    // Fetch tables first
    await act(async () => {
      await result.current.fetchTables(1);
    });

    // fetchColumns fails — should return empty and cache the failure
    let cols: string[] = [];
    await act(async () => {
      cols = await result.current.fetchColumns(1, "users");
    });

    expect(cols).toEqual([]);

    // Second call should use cached empty result (no additional API call)
    await act(async () => {
      cols = await result.current.fetchColumns(1, "users");
    });

    expect(cols).toEqual([]);
    expect(mockedApiGet).toHaveBeenCalledTimes(1);
  });

  it("loading 状态：请求成功后恢复为 false", async () => {
    let resolveApi!: () => void;
    mockedFetchTables.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveApi = () =>
          resolve({ code: 0, message: "ok", data: ["users"] } as any);
      }),
    );

    const { result } = renderHook(() => useSchemaCompletion());

    expect(result.current.loading).toBe(false);

    // Start the fetch and immediately resolve
    resolveApi();
    await act(async () => {
      await result.current.fetchTables(1);
    });

    // After completion, loading should be false
    expect(result.current.loading).toBe(false);
  });
});
