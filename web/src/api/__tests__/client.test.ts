import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// --- Test api/client.ts 401/403/500 error handling ---

// We need to test the actual module, but mock fetch and dependencies
const mockToastError = vi.fn();
vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: (...args: unknown[]) => mockToastError(...args),
  },
}));

// Mock window.location
const originalLocation = window.location;
const mockHref = vi.fn();
Object.defineProperty(window, "location", {
  value: { href: "" },
  writable: true,
});

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value;
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key];
    }),
    clear: () => {
      store = {};
    },
  };
})();
Object.defineProperty(window, "localStorage", { value: localStorageMock });

// Mock fetch
const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

// Import after mocks are set up
import { api } from "@/api/client";

describe("API Client - Error Handling", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorageMock.clear();
  });

  // --- Token handling ---

  describe("token handling", () => {
    it("includes Authorization header when token exists", async () => {
      localStorageMock.getItem.mockReturnValue("test-jwt-token");
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ code: 0, data: {} }),
      });

      await api.get("/test");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/test",
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: "Bearer test-jwt-token",
          }),
        }),
      );
    });

    it("omits Authorization header when no token", async () => {
      localStorageMock.getItem.mockReturnValue(null);
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ code: 0, data: {} }),
      });

      await api.get("/test");

      const call = mockFetch.mock.calls[0];
      const headers = call[1].headers as Record<string, string>;
      expect(headers.Authorization).toBeUndefined();
    });
  });

  // --- 401 Unauthorized ---

  describe("401 unauthorized", () => {
    it("removes token from localStorage on 401", async () => {
      localStorageMock.getItem.mockReturnValue("expired-token");
      mockFetch.mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({}),
      });

      await expect(api.get("/protected")).rejects.toThrow("Unauthorized");

      expect(localStorageMock.removeItem).toHaveBeenCalledWith("token");
    });

    it('shows "登录已过期" toast on 401', async () => {
      localStorageMock.getItem.mockReturnValue("expired-token");
      mockFetch.mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({}),
      });

      await expect(api.get("/protected")).rejects.toThrow();

      expect(mockToastError).toHaveBeenCalledWith("登录已过期，请重新登录");
    });

    it("redirects to /login on 401", async () => {
      localStorageMock.getItem.mockReturnValue("expired-token");
      mockFetch.mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({}),
      });

      await expect(api.get("/protected")).rejects.toThrow();

      expect(window.location.href).toBe("/login");
    });
  });

  // --- 403 Forbidden ---

  describe("403 forbidden", () => {
    it("redirects to /403 on 403", async () => {
      localStorageMock.getItem.mockReturnValue("valid-token");
      mockFetch.mockResolvedValue({
        ok: false,
        status: 403,
        json: () => Promise.resolve({}),
      });

      await expect(api.get("/admin-only")).rejects.toThrow("Forbidden");

      expect(window.location.href).toBe("/403");
    });
  });

  // --- 500 Server Error ---

  describe("500 server error", () => {
    it("shows server error toast on 500", async () => {
      localStorageMock.getItem.mockReturnValue("valid-token");
      mockFetch.mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({}),
      });

      await expect(api.get("/data")).rejects.toThrow("Server error: 500");

      expect(mockToastError).toHaveBeenCalledWith("服务器错误，请稍后重试");
    });

    it("shows server error toast on 502", async () => {
      localStorageMock.getItem.mockReturnValue("valid-token");
      mockFetch.mockResolvedValue({
        ok: false,
        status: 502,
        json: () => Promise.resolve({}),
      });

      await expect(api.get("/data")).rejects.toThrow();

      expect(mockToastError).toHaveBeenCalledWith("服务器错误，请稍后重试");
    });
  });

  // --- 4xx Client Errors (not 401/403) ---

  describe("4xx client errors", () => {
    it("throws with server message on 400", async () => {
      localStorageMock.getItem.mockReturnValue("valid-token");
      mockFetch.mockResolvedValue({
        ok: false,
        status: 400,
        json: () =>
          Promise.resolve({ message: "参数错误: datasource_id is required" }),
      });

      await expect(api.post("/tickets", {})).rejects.toThrow(
        "参数错误: datasource_id is required",
      );
    });

    it("throws generic message when server has no message", async () => {
      localStorageMock.getItem.mockReturnValue("valid-token");
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({}),
      });

      await expect(api.get("/tickets/999")).rejects.toThrow(
        "Request failed: 404",
      );
    });
  });

  // --- Successful requests ---

  describe("successful requests", () => {
    it("returns parsed JSON on 200", async () => {
      localStorageMock.getItem.mockReturnValue("valid-token");
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ code: 0, data: { id: 1, name: "test" } }),
      });

      const result = await api.get("/tickets/1");
      expect(result).toEqual({ code: 0, data: { id: 1, name: "test" } });
    });

    it("sends POST with JSON body", async () => {
      localStorageMock.getItem.mockReturnValue("valid-token");
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ code: 0, data: {} }),
      });

      await api.post("/tickets", { sql: "SELECT 1" });

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/tickets",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ sql: "SELECT 1" }),
        }),
      );
    });
  });

  // --- Method variants ---

  describe("HTTP methods", () => {
    beforeEach(() => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ code: 0 }),
      });
    });

    it("api.get sends GET request", async () => {
      await api.get("/test");
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/test",
        expect.objectContaining({ method: "GET" }),
      );
    });

    it("api.post sends POST request", async () => {
      await api.post("/test", { data: 1 });
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/test",
        expect.objectContaining({ method: "POST" }),
      );
    });

    it("api.put sends PUT request", async () => {
      await api.put("/test", { data: 1 });
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/test",
        expect.objectContaining({ method: "PUT" }),
      );
    });

    it("api.del sends DELETE request", async () => {
      await api.del("/test");
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/test",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
  });
});
