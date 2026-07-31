import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import CoverageSummaryPage from "@/pages/Coverage/CoverageSummaryPage";

describe("CoverageSummaryPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("token", "test-token");
  });

  it("shows an unavailable state without requesting coverage data", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        enabled: false,
        reason: "未配置独立的 Coverage PostgreSQL 数据库",
      }),
    });

    render(
      <MemoryRouter>
        <CoverageSummaryPage />
      </MemoryRouter>,
    );

    expect(
      await screen.findByText("覆盖度审计尚未启用"),
    ).toBeInTheDocument();
    expect(screen.getByText(/Coverage PostgreSQL/)).toBeInTheDocument();

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v1/coverage/status",
      expect.any(Object),
    );
  });
});
