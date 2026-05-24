import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { MemoryRouter } from "react-router-dom";

// --- Mocks ---

const mockNavigate = vi.fn();
vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return { ...actual, useNavigate: () => mockNavigate };
});

// Mock fetch globally
const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import LoginPage from "@/pages/Login";

function renderLogin() {
  return render(
    <MemoryRouter>
      <LoginPage />
    </MemoryRouter>,
  );
}

// The api client calls fetch with {method, headers, body}
function mockLoginSuccess(token = "jwt-token-123") {
  mockFetch.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({ code: 0, data: { token } }),
  });
}

function mockLoginFailure(message = "用户名或密码错误") {
  mockFetch.mockResolvedValueOnce({
    ok: false,
    status: 400,
    json: async () => ({ message }),
  });
}

function mockLoginNetworkError() {
  mockFetch.mockRejectedValueOnce(new TypeError("Failed to fetch"));
}

describe("LoginPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  afterEach(() => localStorage.clear());

  // --- Rendering ---

  describe("rendering", () => {
    it('renders the login form with title "SQLFlow"', () => {
      renderLogin();
      expect(screen.getByText("SQLFlow")).toBeInTheDocument();
    });

    it("renders the subtitle", () => {
      renderLogin();
      expect(screen.getByText("SQL 审批管理平台")).toBeInTheDocument();
    });

    it("renders username input", () => {
      renderLogin();
      expect(screen.getByPlaceholderText("用户名")).toBeInTheDocument();
    });

    it("renders password input", () => {
      renderLogin();
      expect(screen.getByPlaceholderText("密码")).toBeInTheDocument();
    });

    it('renders submit button with text "登 录"', () => {
      renderLogin();
      expect(screen.getByRole("button", { name: "登 录" })).toBeInTheDocument();
    });

    it("renders copyright text", () => {
      renderLogin();
      expect(screen.getByText(/© 2026 SQLFlow/)).toBeInTheDocument();
    });
  });

  // --- Form input ---

  describe("form input", () => {
    it("allows typing username", async () => {
      renderLogin();
      const input = screen.getByPlaceholderText("用户名");
      await userEvent.type(input, "admin");
      expect(input).toHaveValue("admin");
    });

    it("allows typing password", async () => {
      renderLogin();
      const input = screen.getByPlaceholderText("密码");
      await userEvent.type(input, "secret123");
      expect(input).toHaveValue("secret123");
    });
  });

  // --- Form validation on blur ---

  describe("form validation", () => {
    it("shows error when username is empty on blur", async () => {
      renderLogin();
      const input = screen.getByPlaceholderText("用户名");
      input.focus();
      await userEvent.click(document.body);
      expect(screen.getByText("请输入用户名")).toBeInTheDocument();
    });

    it("shows error when username is too short on blur", async () => {
      renderLogin();
      const input = screen.getByPlaceholderText("用户名");
      await userEvent.type(input, "ab");
      await userEvent.tab();
      expect(screen.getByText("用户名需 3-32 个字符")).toBeInTheDocument();
    });

    it("shows error when username is too long on blur", async () => {
      renderLogin();
      const input = screen.getByPlaceholderText("用户名");
      await userEvent.type(input, "a".repeat(33));
      await userEvent.tab();
      expect(screen.getByText("用户名需 3-32 个字符")).toBeInTheDocument();
    });

    it("does not show error for valid username on blur", async () => {
      renderLogin();
      const input = screen.getByPlaceholderText("用户名");
      await userEvent.type(input, "admin");
      await userEvent.tab();
      expect(screen.queryByText("请输入用户名")).not.toBeInTheDocument();
    });

    it("shows error when password is empty on blur", async () => {
      renderLogin();
      const input = screen.getByPlaceholderText("密码");
      input.focus();
      await userEvent.click(document.body);
      expect(screen.getByText("请输入密码")).toBeInTheDocument();
    });

    it("shows error when password is too short on blur", async () => {
      renderLogin();
      const input = screen.getByPlaceholderText("密码");
      await userEvent.type(input, "short");
      await userEvent.tab();
      expect(screen.getByText("密码需 8-128 个字符")).toBeInTheDocument();
    });

    it("does not show error for valid password on blur", async () => {
      renderLogin();
      const input = screen.getByPlaceholderText("密码");
      await userEvent.type(input, "longpassword");
      await userEvent.tab();
      expect(screen.queryByText("请输入密码")).not.toBeInTheDocument();
    });
  });

  // --- Form submission validation ---

  describe("form submission validation", () => {
    it("prevents submission with empty username", async () => {
      renderLogin();
      await userEvent.type(screen.getByPlaceholderText("密码"), "longpassword");
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));
      expect(screen.getByText("请输入用户名")).toBeInTheDocument();
      expect(mockFetch).not.toHaveBeenCalled();
    });

    it("prevents submission with empty password", async () => {
      renderLogin();
      await userEvent.type(screen.getByPlaceholderText("用户名"), "admin");
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));
      expect(screen.getByText("请输入密码")).toBeInTheDocument();
      expect(mockFetch).not.toHaveBeenCalled();
    });

    it("prevents submission with invalid username", async () => {
      renderLogin();
      await userEvent.type(screen.getByPlaceholderText("用户名"), "ab");
      await userEvent.type(screen.getByPlaceholderText("密码"), "longpassword");
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));
      expect(screen.getByText("用户名需 3-32 个字符")).toBeInTheDocument();
      expect(mockFetch).not.toHaveBeenCalled();
    });

    it("prevents submission with invalid password", async () => {
      renderLogin();
      await userEvent.type(screen.getByPlaceholderText("用户名"), "admin");
      await userEvent.type(screen.getByPlaceholderText("密码"), "short");
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));
      expect(screen.getByText("密码需 8-128 个字符")).toBeInTheDocument();
      expect(mockFetch).not.toHaveBeenCalled();
    });
  });

  // --- Login success ---

  describe("login success", () => {
    it("saves token to localStorage on successful login", async () => {
      mockLoginSuccess();
      renderLogin();

      await userEvent.type(screen.getByPlaceholderText("用户名"), "admin");
      await userEvent.type(screen.getByPlaceholderText("密码"), "longpassword");
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));

      await waitFor(() => {
        expect(localStorage.getItem("token")).toBe("jwt-token-123");
      });
    });

    it("navigates to /query on successful login", async () => {
      mockLoginSuccess();
      renderLogin();

      await userEvent.type(screen.getByPlaceholderText("用户名"), "admin");
      await userEvent.type(screen.getByPlaceholderText("密码"), "longpassword");
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith("/query", { replace: true });
      });
    });

    it("sends POST to /api/auth/login with correct body", async () => {
      mockLoginSuccess();
      renderLogin();

      await userEvent.type(screen.getByPlaceholderText("用户名"), "testuser");
      await userEvent.type(screen.getByPlaceholderText("密码"), "mypassword1");
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));

      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledWith("/api/auth/login", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            username: "testuser",
            password: "mypassword1",
          }),
        });
      });
    });
  });

  // --- Login failure ---

  describe("login failure", () => {
    it("shows server error message on login failure", async () => {
      mockLoginFailure("用户名或密码错误");
      renderLogin();

      await userEvent.type(screen.getByPlaceholderText("用户名"), "admin");
      await userEvent.type(
        screen.getByPlaceholderText("密码"),
        "wrongpassword",
      );
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));

      await waitFor(() => {
        expect(screen.getByText("用户名或密码错误")).toBeInTheDocument();
      });
    });

    it("shows generic error on network failure", async () => {
      mockLoginNetworkError();
      renderLogin();

      await userEvent.type(screen.getByPlaceholderText("用户名"), "admin");
      await userEvent.type(screen.getByPlaceholderText("密码"), "longpassword");
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));

      await waitFor(() => {
        // TypeError.message is 'Failed to fetch'
        expect(screen.getByText("Failed to fetch")).toBeInTheDocument();
      });
    });

    it("does not save token on login failure", async () => {
      mockLoginFailure();
      renderLogin();

      await userEvent.type(screen.getByPlaceholderText("用户名"), "admin");
      await userEvent.type(
        screen.getByPlaceholderText("密码"),
        "wrongpassword",
      );
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));

      await waitFor(() => {
        // Wait for the error to appear
        expect(screen.getByText("用户名或密码错误")).toBeInTheDocument();
      });

      expect(localStorage.getItem("token")).toBeNull();
    });
  });

  // --- Loading state ---

  describe("loading state", () => {
    it("disables submit button and shows loading text during login", async () => {
      mockFetch.mockReturnValue(new Promise(() => {}));
      renderLogin();

      await userEvent.type(screen.getByPlaceholderText("用户名"), "admin");
      await userEvent.type(screen.getByPlaceholderText("密码"), "longpassword");
      await userEvent.click(screen.getByRole("button", { name: "登 录" }));

      await waitFor(() => {
        expect(
          screen.getByRole("button", { name: "登录中..." }),
        ).toBeDisabled();
      });
    });
  });

  // --- Server error display ---

  describe("server error display", () => {
    it("does not show error message initially", () => {
      renderLogin();
      expect(screen.queryByText(/错误/)).not.toBeInTheDocument();
    });
  });
});
