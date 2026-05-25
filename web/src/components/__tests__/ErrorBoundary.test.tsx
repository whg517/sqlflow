import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import ErrorBoundary from "@/components/ErrorBoundary";

// Silence console.error from React error boundary logging
const originalConsoleError = console.error;
beforeEach(() => {
  console.error = vi.fn();
});
afterAll(() => {
  console.error = originalConsoleError;
});

function ThrowingChild({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) {
    throw new Error("Test error message");
  }
  return <div>Child content</div>;
}

describe("ErrorBoundary", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("normal rendering", () => {
    it("renders children when no error", () => {
      render(
        <ErrorBoundary>
          <div>Content</div>
        </ErrorBoundary>,
      );
      expect(screen.getByText("Content")).toBeInTheDocument();
    });
  });

  describe("error catching", () => {
    it("shows error UI when child throws", () => {
      render(
        <ErrorBoundary>
          <ThrowingChild shouldThrow={true} />
        </ErrorBoundary>,
      );

      expect(screen.getByText("页面出现了问题")).toBeInTheDocument();
      expect(screen.getByText("Test error message")).toBeInTheDocument();
    });

    it("shows retry button", () => {
      render(
        <ErrorBoundary>
          <ThrowingChild shouldThrow={true} />
        </ErrorBoundary>,
      );

      expect(screen.getByText("重试")).toBeInTheDocument();
    });

    it("shows home button by default", () => {
      render(
        <ErrorBoundary>
          <ThrowingChild shouldThrow={true} />
        </ErrorBoundary>,
      );

      expect(screen.getByText("返回首页")).toBeInTheDocument();
    });

    it("hides home button when showHomeButton is false", () => {
      render(
        <ErrorBoundary showHomeButton={false}>
          <ThrowingChild shouldThrow={true} />
        </ErrorBoundary>,
      );

      expect(screen.queryByText("返回首页")).not.toBeInTheDocument();
    });

    it("shows custom title", () => {
      render(
        <ErrorBoundary title="自定义标题">
          <ThrowingChild shouldThrow={true} />
        </ErrorBoundary>,
      );

      expect(screen.getByText("自定义标题")).toBeInTheDocument();
    });
  });

  describe("retry", () => {
    it("recovers when retry is clicked and error is gone", async () => {
      let shouldThrow = true;

      function ControlledChild() {
        if (shouldThrow) throw new Error("boom");
        return <div>Recovered</div>;
      }

      render(
        <ErrorBoundary>
          <ControlledChild />
        </ErrorBoundary>,
      );

      expect(screen.getByText("页面出现了问题")).toBeInTheDocument();

      shouldThrow = false;
      await userEvent.click(screen.getByText("重试"));

      expect(screen.getByText("Recovered")).toBeInTheDocument();
    });
  });

  describe("custom fallback", () => {
    it("renders custom fallback when provided", () => {
      render(
        <ErrorBoundary fallback={<div>Custom fallback</div>}>
          <ThrowingChild shouldThrow={true} />
        </ErrorBoundary>,
      );

      expect(screen.getByText("Custom fallback")).toBeInTheDocument();
      expect(screen.queryByText("页面出现了问题")).not.toBeInTheDocument();
    });
  });

  describe("onError callback", () => {
    it("calls onError with error and errorInfo", () => {
      const onError = vi.fn();

      render(
        <ErrorBoundary onError={onError}>
          <ThrowingChild shouldThrow={true} />
        </ErrorBoundary>,
      );

      expect(onError).toHaveBeenCalledTimes(1);
      expect(onError.mock.calls[0][0].message).toBe("Test error message");
      expect(onError.mock.calls[0][1]).toHaveProperty("componentStack");
    });
  });
});
