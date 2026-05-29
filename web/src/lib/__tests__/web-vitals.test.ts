import { describe, it, expect, vi, beforeEach } from "vitest";

// vi.mock is hoisted; must use factory that doesn't reference top-level variables.
vi.mock("web-vitals", () => ({
  onLCP: vi.fn(),
  onINP: vi.fn(),
  onCLS: vi.fn(),
}));

// Import mocked functions
import { onLCP, onINP, onCLS } from "web-vitals";

// Helper to get fresh initWebVitals (to respect env changes)
async function getFreshInit() {
  vi.resetModules();
  return await import("@/lib/web-vitals");
}

describe("initWebVitals", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not register callbacks in development mode", async () => {
    vi.stubEnv("PROD", false);
    const mod = await getFreshInit();
    mod.initWebVitals();
    expect(onLCP).not.toHaveBeenCalled();
    expect(onINP).not.toHaveBeenCalled();
    expect(onCLS).not.toHaveBeenCalled();
  });

  it("registers LCP, INP, CLS callbacks in production mode", async () => {
    vi.stubEnv("PROD", true);
    const mod = await getFreshInit();
    mod.initWebVitals();
    expect(onLCP).toHaveBeenCalledTimes(1);
    expect(onINP).toHaveBeenCalledTimes(1);
    expect(onCLS).toHaveBeenCalledTimes(1);
  });

  it("reports metrics via sendBeacon when available", async () => {
    const mockSendBeacon = vi.fn().mockReturnValue(true);
    Object.defineProperty(navigator, "sendBeacon", {
      value: mockSendBeacon,
      configurable: true,
    });

    vi.stubEnv("PROD", true);
    const mod = await getFreshInit();
    mod.initWebVitals();

    // Simulate LCP callback invocation
    const lcpCallback = vi.mocked(onLCP).mock.calls[0][0];
    lcpCallback({
      name: "LCP",
      value: 1234,
      rating: "needs-improvement",
      startTime: 1234,
      delta: 1234,
      id: "v1-123",
      navigationType: "navigate",
      entries: [],
    });

    expect(mockSendBeacon).toHaveBeenCalledWith(
      "/api/metrics/web-vitals",
      expect.stringContaining('"name":"LCP"'),
    );
  });

  it("falls back to fetch when sendBeacon is not available", async () => {
    // Remove sendBeacon
    Object.defineProperty(navigator, "sendBeacon", {
      value: undefined,
      configurable: true,
    });

    const mockFetch = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", mockFetch);

    vi.stubEnv("PROD", true);
    const mod = await getFreshInit();
    mod.initWebVitals();

    const clsCallback = vi.mocked(onCLS).mock.calls[0][0];
    clsCallback({
      name: "CLS",
      value: 0.05,
      rating: "good",
      startTime: 0,
      delta: 0.05,
      id: "v1-456",
      navigationType: "navigate",
      entries: [],
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/metrics/web-vitals",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"name":"CLS"'),
      }),
    );
  });
});
