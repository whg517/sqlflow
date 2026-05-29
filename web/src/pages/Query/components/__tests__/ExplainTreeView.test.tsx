import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import ExplainTreeView from "../ExplainTreeView";
import type { ExplainRow } from "@/api/explain";

// Mock tooltip components to avoid radix portal issues in tests
vi.mock("@/components/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children, ...props }: { children: React.ReactNode } & Record<string, unknown>) => <span {...props}>{children}</span>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import React from "react";

const makeRow = (overrides: Partial<ExplainRow> = {}): ExplainRow => ({
  id: 1,
  select_type: "SIMPLE",
  table: "users",
  partitions: null,
  type: "ALL",
  possible_keys: null,
  key: null,
  key_len: null,
  ref: null,
  rows: 50000,
  filtered: 100,
  extra: "Using where; Using filesort",
  ...overrides,
});

describe("ExplainTreeView", () => {
  it("renders a single node tree", () => {
    const plan = [makeRow()];
    render(<ExplainTreeView plan={plan} />);
    expect(screen.getByText("users")).toBeInTheDocument();
    expect(screen.getByText("ALL")).toBeInTheDocument();
  });

  it("displays issue badges for full table scan", () => {
    const plan = [makeRow({ type: "ALL", key: null, extra: "Using where; Using filesort" })];
    render(<ExplainTreeView plan={plan} />);
    // Should show danger badge for full table scan (use getAllByText since legend also has this text)
    expect(screen.getAllByText(/严重问题/).length).toBeGreaterThanOrEqual(1);
  });

  it("displays warning badge for filesort", () => {
    const plan = [makeRow({ type: "range", key: "idx_name", extra: "Using filesort" })];
    render(<ExplainTreeView plan={plan} />);
    expect(screen.getAllByText(/警告/).length).toBeGreaterThanOrEqual(1);
  });

  it("renders tree with parent-child relationships", () => {
    const plan = [
      makeRow({ id: 1, table: "orders", type: "ref", key: "idx_user_id" }),
      makeRow({ id: 2, table: "users", type: "eq_ref", key: "PRIMARY" }),
    ];
    render(<ExplainTreeView plan={plan} />);
    expect(screen.getByText("orders")).toBeInTheDocument();
    expect(screen.getByText("users")).toBeInTheDocument();
  });

  it("shows rows count in tree node", () => {
    const plan = [makeRow({ rows: 12345 })];
    render(<ExplainTreeView plan={plan} />);
    // rows count is split across elements, use getAllByText
    const matches = screen.getAllByText(/12,345/);
    expect(matches.length).toBeGreaterThanOrEqual(1);
  });

  it("shows NULL key indicator for missing index", () => {
    const plan = [makeRow({ type: "ALL", key: null })];
    render(<ExplainTreeView plan={plan} />);
    expect(screen.getByText("key: NULL")).toBeInTheDocument();
  });

  it("shows the used key name", () => {
    const plan = [makeRow({ type: "ref", key: "idx_email" })];
    render(<ExplainTreeView plan={plan} />);
    expect(screen.getByText("key: idx_email")).toBeInTheDocument();
  });

  it("renders empty state when plan is empty", () => {
    render(<ExplainTreeView plan={[]} />);
    expect(screen.getByText("无执行计划数据")).toBeInTheDocument();
  });
});
