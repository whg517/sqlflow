import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import { streamAIReview, exportQuery } from "@/api/query";

describe("Query API - streamAIReview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("token", "test-token");
  });

  afterEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("calls onEvent for each SSE event", async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();

    // Simulate SSE stream
    const encoder = new TextEncoder();
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(
          encoder.encode("event: content\ndata: Analyzing...\n\n"),
        );
        controller.enqueue(
          encoder.encode('event: result\ndata: {\"risk_level\": \"low\"}\n\n'),
        );
        controller.close();
      },
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      body: stream,
    });

    const cancel = streamAIReview(
      { datasource_id: 1, database: "db", sql: "SELECT 1" },
      onEvent,
      onError,
    );

    // Wait for stream to process
    await new Promise((r) => setTimeout(r, 100));

    expect(onEvent).toHaveBeenCalledTimes(2);
    expect(onEvent).toHaveBeenCalledWith({
      type: "content",
      data: "Analyzing...",
    });
    expect(onEvent).toHaveBeenCalledWith({
      type: "result",
      data: { risk_level: "low" },
    });
  });

  it("sends correct request headers", async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      body: new ReadableStream({
        start(c) {
          c.close();
        },
      }),
    });

    streamAIReview(
      { datasource_id: 1, database: "db", sql: "SELECT 1" },
      onEvent,
      onError,
    );

    await new Promise((r) => setTimeout(r, 50));

    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/query/review");
    expect(opts.method).toBe("POST");
    expect(opts.headers.Authorization).toBe("Bearer test-token");
    expect(opts.headers["Content-Type"]).toBe("application/json");
  });

  it("handles 401 response", async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
    });

    streamAIReview(
      { datasource_id: 1, database: "db", sql: "SELECT 1" },
      onEvent,
      onError,
    );

    await new Promise((r) => setTimeout(r, 50));

    expect(localStorage.getItem("token")).toBeNull();
    // window.location.href = '/login' is called but jsdom doesn't navigate
  });

  it("handles non-ok response with error message", async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 400,
      json: async () => ({ message: "SQL too complex" }),
    });

    streamAIReview(
      { datasource_id: 1, database: "db", sql: "SELECT 1" },
      onEvent,
      onError,
    );

    await new Promise((r) => setTimeout(r, 50));

    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ message: "SQL too complex" }),
    );
  });

  it("handles network error", async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();

    mockFetch.mockRejectedValueOnce(new Error("Network error"));

    streamAIReview(
      { datasource_id: 1, database: "db", sql: "SELECT 1" },
      onEvent,
      onError,
    );

    await new Promise((r) => setTimeout(r, 50));

    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ message: "Network error" }),
    );
  });

  it("cancel function aborts the request", () => {
    const onEvent = vi.fn();
    const onError = vi.fn();

    // Create a promise that never resolves
    mockFetch.mockReturnValueOnce(new Promise(() => {}));

    const cancel = streamAIReview(
      { datasource_id: 1, database: "db", sql: "SELECT 1" },
      onEvent,
      onError,
    );

    // Cancel immediately
    cancel();

    expect(mockFetch).toHaveBeenCalledTimes(1);
    // The AbortSignal should be set
    expect(mockFetch.mock.calls[0][1].signal).toBeDefined();
  });

  it("handles empty data lines", async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();

    const encoder = new TextEncoder();
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode("\n"));
        controller.enqueue(encoder.encode("event: content\ndata: \n\n"));
        controller.enqueue(encoder.encode("event: done\ndata: [DONE]\n\n"));
        controller.close();
      },
    });

    mockFetch.mockResolvedValueOnce({ ok: true, status: 200, body: stream });

    streamAIReview(
      { datasource_id: 1, database: "db", sql: "SELECT 1" },
      onEvent,
      onError,
    );

    await new Promise((r) => setTimeout(r, 100));

    // Should have called for content and done events
    expect(onEvent).toHaveBeenCalledWith({ type: "content", data: "" });
    expect(onEvent).toHaveBeenCalledWith({ type: "done", data: "[DONE]" });
  });

  it("ignores abort errors", async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();

    const error = new Error("Aborted");
    error.name = "AbortError";
    mockFetch.mockRejectedValueOnce(error);

    streamAIReview(
      { datasource_id: 1, database: "db", sql: "SELECT 1" },
      onEvent,
      onError,
    );

    await new Promise((r) => setTimeout(r, 50));

    expect(onError).not.toHaveBeenCalled();
  });
});

describe("Query API - exportQuery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("token", "test-token");
  });

  afterEach(() => {
    localStorage.clear();
    // Clean up any dynamically created elements
    document.querySelectorAll("a").forEach((a) => {
      if (a.href && a.href.startsWith("blob:")) a.remove();
    });
  });

  it("downloads CSV file", async () => {
    const csvContent = "id,name\n1,Alice";
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ "Content-Disposition": "filename=export.csv" }),
      blob: async () => new Blob([csvContent], { type: "text/csv" }),
    });

    // Mock URL.createObjectURL and revokeObjectURL
    const mockUrl = "blob:http://localhost/test";
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL: vi.fn(() => mockUrl),
      revokeObjectURL: vi.fn(),
    });

    await exportQuery({
      datasource_id: 1,
      database: "db",
      sql: "SELECT * FROM users",
      format: "csv",
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/query/export",
      expect.objectContaining({
        method: "POST",
      }),
    );
    expect(URL.createObjectURL).toHaveBeenCalled();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(mockUrl);
  });

  it("downloads JSON file", async () => {
    const jsonContent = '[{"id":1}]';
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ "Content-Disposition": "filename=data.json" }),
      blob: async () => new Blob([jsonContent], { type: "application/json" }),
    });

    const mockUrl = "blob:http://localhost/test";
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL: vi.fn(() => mockUrl),
      revokeObjectURL: vi.fn(),
    });

    await exportQuery({
      datasource_id: 1,
      database: "db",
      sql: "SELECT 1",
      format: "json",
    });

    expect(mockFetch).toHaveBeenCalled();
  });

  it("handles missing Content-Disposition header", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers(),
      blob: async () => new Blob(["data"], { type: "text/csv" }),
    });

    const mockUrl = "blob:http://localhost/test";
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL: vi.fn(() => mockUrl),
      revokeObjectURL: vi.fn(),
    });

    await exportQuery({
      datasource_id: 1,
      database: "db",
      sql: "SELECT 1",
      format: "csv",
    });

    // Should not throw
  });

  it("handles 401 response", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
    });

    await expect(
      exportQuery({
        datasource_id: 1,
        database: "db",
        sql: "SELECT 1",
        format: "csv",
      }),
    ).rejects.toThrow("Unauthorized");

    expect(localStorage.getItem("token")).toBeNull();
    // window.location.href = '/login' is called but jsdom doesn't navigate
  });

  it("handles non-ok response", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 400,
      json: async () => ({ message: "Export failed" }),
    });

    await expect(
      exportQuery({
        datasource_id: 1,
        database: "db",
        sql: "SELECT 1",
        format: "csv",
      }),
    ).rejects.toThrow("Export failed");
  });
});
