import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, within, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import ResultTable from "../ResultTable";
import type { QueryResult } from "@/api/query";

const makeResult = (overrides: Partial<QueryResult> = {}): QueryResult => ({
  columns: ["id", "name", "status"],
  rows: [
    { id: 1, name: "Alice", status: "active" },
    { id: 2, name: "Bob", status: "inactive" },
    { id: 3, name: "Charlie", status: "active" },
  ],
  total: 3,
  execution_time_ms: 10,
  affected_rows: 0,
  desensitized: false,
  desensitized_fields: [],
  warnings: [],
  ...overrides,
});

describe("ResultTable column filters", () => {
  beforeEach(() => {
    // Ensure clean slate from previous tests
    cleanup();
    // Remove any Radix portal remnants from previous tests
    document.body
      .querySelectorAll("[data-radix-portal]")
      .forEach((el) => el.remove());
    document.body
      .querySelectorAll("[data-radix-popper-content-wrapper]")
      .forEach((el) => el.remove());
  });

  it("renders filter icon buttons for each column", () => {
    const result = makeResult();
    render(<ResultTable result={result} />);

    // Three columns → at least 3 filter buttons
    const filterButtons = document.querySelectorAll(
      '[data-slot="popover-trigger"]',
    );
    expect(filterButtons.length).toBeGreaterThanOrEqual(3);
  });

  it("opens filter popover when clicking the filter icon", async () => {
    const user = userEvent.setup();
    const result = makeResult();
    render(<ResultTable result={result} />);

    // Find the filter trigger for the "name" column
    const nameHeader = screen.getByText("name").closest("th")!;
    const filterTrigger = nameHeader.querySelector(
      '[data-slot="popover-trigger"]',
    )!;
    await user.click(filterTrigger);

    // Popover content should appear (via Radix Portal)
    const popover = document.querySelector('[data-slot="popover-content"]');
    expect(popover).toBeInTheDocument();
    expect(popover).toHaveTextContent("筛选: name");
    expect(popover).toHaveTextContent("包含");
    expect(
      within(popover!).getByPlaceholderText("输入筛选值..."),
    ).toBeInTheDocument();
  });

  it('filters rows with "contains" operator via keyboard Enter', async () => {
    const user = userEvent.setup();
    const result = makeResult();
    render(<ResultTable result={result} />);

    // Open filter popover for "name" column
    const nameHeader = screen.getByText("name").closest("th")!;
    const filterTrigger = nameHeader.querySelector(
      '[data-slot="popover-trigger"]',
    )!;
    await user.click(filterTrigger);

    const popover = document.querySelector('[data-slot="popover-content"]')!;

    // Type filter value and press Enter
    const input = within(popover).getByPlaceholderText("输入筛选值...");
    await user.type(input, "ali{Enter}");

    // Wait for popover to close and table to update
    await vi.waitFor(() => {
      expect(screen.queryByText("Bob")).not.toBeInTheDocument();
    });

    // Alice should be visible (contains "ali")
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.queryByText("Charlie")).not.toBeInTheDocument();
  });

  it('filters rows with exact match via keyboard Enter (contains "Bob")', async () => {
    const user = userEvent.setup();
    const result = makeResult();
    render(<ResultTable result={result} />);

    const nameHeader = screen.getByText("name").closest("th")!;
    const filterTrigger = nameHeader.querySelector(
      '[data-slot="popover-trigger"]',
    )!;
    await user.click(filterTrigger);

    const popover = document.querySelector('[data-slot="popover-content"]')!;

    // Type "Bob" — "contains" operator matches only "Bob"
    const input = within(popover).getByPlaceholderText("输入筛选值...");
    await user.type(input, "Bob{Enter}");

    await vi.waitFor(() => {
      expect(screen.queryByText("Alice")).not.toBeInTheDocument();
    });

    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.queryByText("Charlie")).not.toBeInTheDocument();
  });

  it("shows active filter indicator when filter is applied", async () => {
    const user = userEvent.setup();
    const result = makeResult();
    render(<ResultTable result={result} />);

    const nameHeader = screen.getByText("name").closest("th")!;
    const filterTrigger = nameHeader.querySelector(
      '[data-slot="popover-trigger"]',
    )!;
    await user.click(filterTrigger);

    const popover = document.querySelector('[data-slot="popover-content"]')!;
    const input = within(popover).getByPlaceholderText("输入筛选值...");
    await user.type(input, "Alice{Enter}");

    // Wait for filter bar to appear
    await vi.waitFor(() => {
      expect(screen.getByText("已筛选:")).toBeInTheDocument();
    });

    // Filter bar should show column name — "name" appears in both header and filter bar
    const nameElements = screen.getAllByText("name");
    expect(nameElements.length).toBeGreaterThanOrEqual(2); // header + filter bar

    // Should show the operator label (may appear multiple times due to Radix portal)
    expect(screen.getAllByText(/包含/).length).toBeGreaterThanOrEqual(1);

    // Should show the filter value (inside quotes)
    expect(screen.getByText(/"Alice"/)).toBeInTheDocument();
  });

  it("shows empty state when filter matches no rows", async () => {
    const user = userEvent.setup();
    const result = makeResult();
    render(<ResultTable result={result} />);

    const nameHeader = screen.getByText("name").closest("th")!;
    const filterTrigger = nameHeader.querySelector(
      '[data-slot="popover-trigger"]',
    )!;
    await user.click(filterTrigger);

    const popover = document.querySelector('[data-slot="popover-content"]')!;
    const input = within(popover).getByPlaceholderText("输入筛选值...");
    await user.type(input, "NONEXISTENT{Enter}");

    await vi.waitFor(() => {
      expect(screen.getByText("无匹配数据")).toBeInTheDocument();
    });

    expect(screen.getByText("清除筛选条件")).toBeInTheDocument();
  });

  it('clears all filters when clicking "清除全部"', async () => {
    const user = userEvent.setup();
    const result = makeResult();
    render(<ResultTable result={result} />);

    // Apply a filter on "name"
    const nameHeader = screen.getByText("name").closest("th")!;
    const filterTrigger = nameHeader.querySelector(
      '[data-slot="popover-trigger"]',
    )!;
    await user.click(filterTrigger);

    const popover = document.querySelector('[data-slot="popover-content"]')!;
    const input = within(popover).getByPlaceholderText("输入筛选值...");
    await user.type(input, "Alice{Enter}");

    await vi.waitFor(() => {
      expect(screen.getByText("已筛选:")).toBeInTheDocument();
    });

    // Click "清除全部"
    await user.click(screen.getByText("清除全部"));

    // Filter bar should be gone
    await vi.waitFor(() => {
      expect(screen.queryByText("已筛选:")).not.toBeInTheDocument();
    });

    // All rows should be visible again
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("Charlie")).toBeInTheDocument();
  });

  it("removes individual filter by clicking the X button in filter bar", async () => {
    const user = userEvent.setup();
    const result = makeResult();
    render(<ResultTable result={result} />);

    // Apply a filter
    const nameHeader = screen.getByText("name").closest("th")!;
    const filterTrigger = nameHeader.querySelector(
      '[data-slot="popover-trigger"]',
    )!;
    await user.click(filterTrigger);

    const popover = document.querySelector('[data-slot="popover-content"]')!;
    const input = within(popover).getByPlaceholderText("输入筛选值...");
    await user.type(input, "Alice{Enter}");

    await vi.waitFor(() => {
      expect(screen.getByText("已筛选:")).toBeInTheDocument();
    });

    // Click the FilterX icon to remove individual filter
    const clearButtons = document.querySelectorAll("button.ml-0\\.5");
    expect(clearButtons.length).toBeGreaterThanOrEqual(1);
    await user.click(clearButtons[0]);

    // Filter bar should be gone (it was the only filter)
    await vi.waitFor(() => {
      expect(screen.queryByText("已筛选:")).not.toBeInTheDocument();
    });

    // All rows visible
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });

  it("column sorting works and sort indicator is visible", async () => {
    const user = userEvent.setup();
    const result = makeResult();
    const { container } = render(<ResultTable result={result} />);

    // Wait for the requestAnimationFrame to fire (it resets state on mount)
    // Use act + waitFor to flush all pending state updates
    await new Promise((r) => requestAnimationFrame(r));
    await new Promise((r) => requestAnimationFrame(r));

    // Now sort by "name" column
    const allThs = container.querySelectorAll("th");
    let nameThFound: HTMLElement | null = null;
    for (const th of allThs) {
      const firstSpan = th.querySelector("span");
      if (firstSpan && firstSpan.textContent === "name") {
        nameThFound = th as HTMLElement;
        break;
      }
    }
    expect(nameThFound).not.toBeNull();

    const sortArea = nameThFound!.querySelector(
      ".cursor-pointer",
    ) as HTMLElement;
    expect(sortArea).toBeTruthy();
    await user.click(sortArea);

    // Wait for state update to propagate
    await vi.waitFor(() => {
      expect(
        nameThFound!.querySelector("svg.lucide-chevron-up"),
      ).toBeInTheDocument();
    });
  });

  it("resets filters when result changes", async () => {
    const user = userEvent.setup();
    const result1 = makeResult();
    const result2 = makeResult({
      rows: [{ id: 10, name: "Dave", status: "active" }],
    });

    const { rerender } = render(<ResultTable result={result1} />);

    // Apply a filter
    const nameHeader = screen.getByText("name").closest("th")!;
    const filterTrigger = nameHeader.querySelector(
      '[data-slot="popover-trigger"]',
    )!;
    await user.click(filterTrigger);

    const popover = document.querySelector('[data-slot="popover-content"]')!;
    const input = within(popover).getByPlaceholderText("输入筛选值...");
    await user.type(input, "Alice{Enter}");

    await vi.waitFor(() => {
      expect(screen.getByText("已筛选:")).toBeInTheDocument();
    });

    // Change the result — filters should reset
    rerender(<ResultTable result={result2} />);

    // Filter bar should be gone
    await vi.waitFor(() => {
      expect(screen.queryByText("已筛选:")).not.toBeInTheDocument();
    });

    // New data should be visible
    expect(screen.getByText("Dave")).toBeInTheDocument();
  });
});
