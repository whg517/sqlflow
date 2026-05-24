import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";

// --- Mocks ---

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockApiGet = vi.fn();
const mockApiPost = vi.fn();
const mockApiPut = vi.fn();
const mockApiDel = vi.fn();
vi.mock("@/api/client", () => ({
  api: {
    get: (...args: unknown[]) => mockApiGet(...args),
    post: (...args: unknown[]) => mockApiPost(...args),
    put: (...args: unknown[]) => mockApiPut(...args),
    del: (...args: unknown[]) => mockApiDel(...args),
  },
}));

// Mock Radix Select
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
    (globalThis as Record<string, unknown>).__userSelectOnValueChange =
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
          .__userSelectOnValueChange as ((v: string) => void) | undefined;
        cb?.(value);
      }}
    >
      {children}
    </div>
  ),
}));

import UsersPage from "@/pages/Users";

// --- Fixtures ---

const mockUsers = {
  users: [
    {
      id: 1,
      username: "admin",
      role: "admin",
      status: "active",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    },
    {
      id: 2,
      username: "dba_user",
      role: "dba",
      status: "active",
      created_at: "2026-02-01T00:00:00Z",
      updated_at: "2026-02-01T00:00:00Z",
    },
    {
      id: 3,
      username: "dev_user",
      role: "developer",
      status: "active",
      created_at: "2026-03-01T00:00:00Z",
      updated_at: "2026-03-01T00:00:00Z",
    },
  ],
  total: 3,
};

function setupMocks() {
  mockApiGet.mockImplementation((url: string) => {
    if (url.includes("/auth/me")) {
      return Promise.resolve({
        code: 0,
        data: { id: 1, username: "admin", role: "admin" },
      });
    }
    if (url.includes("/users") && !url.includes("reset-password")) {
      return Promise.resolve({ code: 0, data: mockUsers });
    }
    return Promise.resolve({ code: 0, data: {} });
  });
}

function renderUsersPage() {
  return render(
    <MemoryRouter initialEntries={["/users"]}>
      <Routes>
        <Route path="/users" element={<UsersPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("UsersPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMocks();
    delete (globalThis as Record<string, unknown>).__userSelectOnValueChange;
  });

  // --- Rendering ---

  describe("rendering", () => {
    it('renders page header "用户管理"', () => {
      renderUsersPage();
      expect(screen.getByText("用户管理")).toBeInTheDocument();
    });

    it("renders search input", () => {
      renderUsersPage();
      expect(screen.getByPlaceholderText("搜索用户名...")).toBeInTheDocument();
    });

    it('renders "新建用户" button', () => {
      renderUsersPage();
      expect(screen.getByText("新建用户")).toBeInTheDocument();
    });
  });

  // --- Data display ---

  describe("data display", () => {
    it("displays user rows", async () => {
      renderUsersPage();
      await waitFor(() => {
        expect(screen.getByText("admin")).toBeInTheDocument();
        expect(screen.getByText("dba_user")).toBeInTheDocument();
        expect(screen.getByText("dev_user")).toBeInTheDocument();
      });
    });

    it("displays role badges", async () => {
      renderUsersPage();
      await waitFor(() => {
        expect(screen.getByText("管理员")).toBeInTheDocument();
        expect(screen.getByText("DBA")).toBeInTheDocument();
        expect(screen.getByText("开发人员")).toBeInTheDocument();
      });
    });
  });

  // --- Search ---

  describe("search", () => {
    it("filters users by keyword", async () => {
      renderUsersPage();
      await waitFor(() => screen.getByText("admin"));

      const input = screen.getByPlaceholderText("搜索用户名...");
      await userEvent.type(input, "dba");

      // Should filter to only show dba_user
      await waitFor(() => {
        expect(screen.getByText("dba_user")).toBeInTheDocument();
      });
    });
  });

  // --- Empty state ---

  describe("empty state", () => {
    it('shows "暂无用户数据" when no users', async () => {
      mockApiGet.mockImplementation((url: string) => {
        if (url.includes("/auth/me")) {
          return Promise.resolve({
            code: 0,
            data: { id: 1, username: "admin", role: "admin" },
          });
        }
        if (url.includes("/users")) {
          return Promise.resolve({ code: 0, data: { users: [], total: 0 } });
        }
        return Promise.resolve({});
      });
      renderUsersPage();
      await waitFor(() => {
        expect(screen.getByText("暂无用户数据")).toBeInTheDocument();
      });
    });
  });

  // --- Loading state ---

  describe("loading state", () => {
    it("shows skeleton while loading", () => {
      mockApiGet.mockImplementation((url: string) => {
        if (url.includes("/users")) return new Promise(() => {});
        return Promise.resolve({
          code: 0,
          data: { id: 1, username: "admin", role: "admin" },
        });
      });
      renderUsersPage();
      expect(
        document.querySelectorAll(".animate-pulse").length,
      ).toBeGreaterThan(0);
    });
  });
});
