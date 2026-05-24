import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";

// --- Mocks ---

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

const mockApiGet = vi.fn();
vi.mock("@/api/client", () => ({
  api: {
    get: (...args: unknown[]) => mockApiGet(...args),
    post: vi.fn(),
    put: vi.fn(),
    del: vi.fn(),
  },
}));

const mockListAuditLogs = vi.fn();
vi.mock("@/api/audit", () => ({
  listAuditLogs: (...args: unknown[]) => mockListAuditLogs(...args),
  getActionLabel: (a: string) => (a === "EXPORT" ? "导出" : a),
  getActionBadgeStyle: (a: string) => {
    const map: Record<string, string> = {
      SELECT: "bg-blue-500/20 text-blue-400",
      UPDATE: "bg-yellow-500/20 text-yellow-400",
      DELETE: "bg-red-500/20 text-red-400",
      DDL: "bg-violet-500/20 text-violet-400",
    };
    return map[a] ?? "bg-slate-500/20 text-slate-400";
  },
  formatAuditTime: () => "05-24 10:00:00",
  formatExecutionTime: (ms: number) =>
    ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(2)}s`,
  actionOptions: ["SELECT", "UPDATE", "DELETE", "DDL", "EXPORT"],
}));

const mockListTickets = vi.fn();
vi.mock("@/api/ticket", () => ({
  listTickets: (...args: unknown[]) => mockListTickets(...args),
  getStatusLabel: (s: string) => s,
  getStatusColor: () => "bg-blue-500/20 text-blue-400",
}));

// Mock Radix Select for JSDOM
vi.mock("@/components/ui/select", () => ({
  Select: ({
    children,
    value,
    onValueChange,
  }: {
    children: React.ReactNode;
    value: string;
    onValueChange: (v: string) => void;
  }) => {
    (globalThis as Record<string, unknown>).__auditSelectOnValueChange =
      onValueChange;
    return <div data-testid="select">{children}</div>;
  },
  SelectTrigger: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-trigger">{children}</div>
  ),
  SelectValue: ({ placeholder }: { placeholder: string }) => (
    <span>{placeholder}</span>
  ),
  SelectContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-content">{children}</div>
  ),
  SelectItem: ({
    children,
    value,
  }: {
    children: React.ReactNode;
    value: string;
  }) => (
    <div
      data-testid="select-item"
      data-value={value}
      onClick={() => {
        const cb = (globalThis as Record<string, unknown>)
          .__auditSelectOnValueChange as ((v: string) => void) | undefined;
        cb?.(value);
      }}
    >
      {children}
    </div>
  ),
}));

vi.mock("@/components/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Tooltip: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}));

import AuditPage from "@/pages/Audit";

// --- Fixtures ---

const mockLogs = [
  {
    id: 1,
    user_id: 1,
    username: "admin",
    action: "SELECT",
    datasource_id: 10,
    database: "testdb",
    sql_content: "SELECT * FROM users WHERE id = 1",
    sql_summary: "SELECT * FROM users...",
    result_rows: 1,
    affected_rows: 0,
    execution_time_ms: 45,
    error_message: "",
    desensitized_fields: "",
    ip_address: "192.168.1.1",
    created_at: "2026-05-24T10:00:00Z",
  },
  {
    id: 2,
    user_id: 2,
    username: "developer",
    action: "UPDATE",
    datasource_id: 11,
    database: "proddb",
    sql_content: 'UPDATE orders SET status = "done" WHERE id = 5',
    sql_summary: "UPDATE orders...",
    result_rows: 0,
    affected_rows: 1,
    execution_time_ms: 120,
    error_message: "",
    desensitized_fields: "email,phone",
    ip_address: "10.0.0.5",
    created_at: "2026-05-24T11:00:00Z",
  },
];

function setupMocks() {
  mockApiGet.mockImplementation((url: string) => {
    if (url.includes("/datasources")) {
      return Promise.resolve({
        code: 0,
        data: [
          { id: 10, name: "MySQL Test", type: "mysql" },
          { id: 11, name: "MySQL Prod", type: "mysql" },
        ],
      });
    }
    if (url.includes("/users")) {
      return Promise.resolve({
        code: 0,
        data: [
          { id: 1, username: "admin" },
          { id: 2, username: "developer" },
        ],
      });
    }
    return Promise.resolve({});
  });
  mockListAuditLogs.mockResolvedValue({
    data: mockLogs,
    page: 1,
    page_size: 50,
    total: 2,
  });
  mockListTickets.mockResolvedValue({ data: [], total: 0 });
}

function renderAuditPage() {
  return render(
    <MemoryRouter initialEntries={["/audit"]}>
      <Routes>
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/tickets" element={<div>Tickets Page</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AuditPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMocks();
    delete (globalThis as Record<string, unknown>).__auditSelectOnValueChange;
  });

  // --- Rendering ---

  describe("rendering", () => {
    it('renders page header "审计日志"', () => {
      renderAuditPage();
      expect(screen.getByText("审计日志")).toBeInTheDocument();
    });

    it("renders total count badge", async () => {
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("2 条")).toBeInTheDocument();
      });
    });

    it("renders filter bar elements", () => {
      renderAuditPage();
      expect(screen.getByPlaceholderText(/搜索 SQL/)).toBeInTheDocument();
      expect(screen.getByText("导出 CSV")).toBeInTheDocument();
    });

    it("renders search input", () => {
      renderAuditPage();
      expect(screen.getByPlaceholderText(/搜索 SQL/)).toBeInTheDocument();
    });

    it("renders export CSV button", () => {
      renderAuditPage();
      expect(screen.getByText("导出 CSV")).toBeInTheDocument();
    });
  });

  // --- Data display ---

  describe("data display", () => {
    it("displays audit log rows", async () => {
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("testdb")).toBeInTheDocument();
        expect(screen.getByText("proddb")).toBeInTheDocument();
      });
    });

    it("displays action badges", async () => {
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("SELECT")).toBeInTheDocument();
        expect(screen.getByText("UPDATE")).toBeInTheDocument();
      });
    });

    it("displays database names", async () => {
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("testdb")).toBeInTheDocument();
        expect(screen.getByText("proddb")).toBeInTheDocument();
      });
    });

    it("displays SQL summary", async () => {
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("SELECT * FROM users...")).toBeInTheDocument();
      });
    });
  });

  // --- Empty state ---

  describe("empty state", () => {
    it('shows "暂无审计日志" when no logs', async () => {
      mockListAuditLogs.mockResolvedValue({
        data: [],
        page: 1,
        page_size: 50,
        total: 0,
      });
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("暂无审计日志")).toBeInTheDocument();
      });
    });
  });

  // --- Error state ---

  describe("error state", () => {
    it("shows error message with retry button on fetch failure", async () => {
      mockListAuditLogs.mockRejectedValue(new Error("Network error"));
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("加载失败")).toBeInTheDocument();
        expect(screen.getByText("Network error")).toBeInTheDocument();
        expect(screen.getByText("重试")).toBeInTheDocument();
      });
    });
  });

  // --- Loading state ---

  describe("loading state", () => {
    it("shows skeleton while loading", () => {
      mockListAuditLogs.mockReturnValue(new Promise(() => {}));
      renderAuditPage();
      expect(
        document.querySelectorAll(".animate-pulse").length,
      ).toBeGreaterThan(0);
    });
  });

  // --- Search ---

  describe("keyword search", () => {
    it("updates search input and triggers search on Enter", async () => {
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("proddb")).toBeInTheDocument();
      });

      const searchInput = screen.getByPlaceholderText(/搜索 SQL/);
      await userEvent.type(searchInput, "SELECT{Enter}");

      await waitFor(() => {
        expect(mockListAuditLogs).toHaveBeenCalledWith(
          expect.objectContaining({ keyword: "SELECT" }),
        );
      });
    });
  });

  // --- Row expand ---

  describe("row expand", () => {
    it("toggles expand on row click", async () => {
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("proddb")).toBeInTheDocument();
      });

      const row = screen.getByText("proddb").closest("tr")!;
      await userEvent.click(row);

      // Expanded row should show full SQL and copy button
      await waitFor(() => {
        expect(screen.getByText("完整 SQL")).toBeInTheDocument();
        expect(screen.getByText("复制")).toBeInTheDocument();
      });
    });

    it("shows execution time and affected rows in expanded row", async () => {
      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText("proddb")).toBeInTheDocument();
      });

      const row2 = screen.getByText("proddb").closest("tr")!;
      await userEvent.click(row2);

      await waitFor(() => {
        expect(screen.getByText("执行耗时")).toBeInTheDocument();
        expect(screen.getByText("影响行数")).toBeInTheDocument();
        expect(screen.getByText("返回行数")).toBeInTheDocument();
        expect(screen.getByText("IP 地址")).toBeInTheDocument();
      });
    });
  });

  // --- Export CSV ---

  describe("export CSV", () => {
    it("calls listAuditLogs with page_size 10000 on export click", async () => {
      renderAuditPage();
      await waitFor(() => screen.getByText("导出 CSV"));

      await userEvent.click(screen.getByText("导出 CSV"));

      await waitFor(() => {
        expect(mockListAuditLogs).toHaveBeenCalledWith(
          expect.objectContaining({ page_size: 10000 }),
        );
      });
    });
  });

  // --- Pagination ---

  describe("pagination", () => {
    it("shows pagination when multiple pages", async () => {
      const manyLogs = Array.from({ length: 55 }, (_, i) => ({
        ...mockLogs[0],
        id: i + 1,
      }));
      mockListAuditLogs.mockResolvedValue({
        data: manyLogs.slice(0, 50),
        page: 1,
        page_size: 50,
        total: 55,
      });

      renderAuditPage();
      await waitFor(() => {
        expect(screen.getByText(/共 55 条/)).toBeInTheDocument();
      });
    });
  });
});
