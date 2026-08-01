import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import AggregationView from "../AggregationView";
import type { QueryResult } from "@/features/query/api/query";

function result(aggregations: unknown): QueryResult {
  return {
    shape: "aggregation",
    columns: [],
    rows: [],
    total: 0,
    execution_time_ms: 1,
    affected_rows: 0,
    desensitized: false,
    desensitized_fields: [],
    warnings: [],
    aggregations,
  };
}

describe("AggregationView", () => {
  it("renders bucket keys and counts as a table", () => {
    render(
      <AggregationView
        result={result({
          by_status: {
            buckets: [
              { key: "open", doc_count: 7 },
              { key: "closed", doc_count: 35 },
            ],
          },
        })}
      />,
    );

    expect(screen.getAllByText("open").length).toBeGreaterThan(0);
    expect(screen.getAllByText("7").length).toBeGreaterThan(0);
    expect(screen.getAllByText("closed").length).toBeGreaterThan(0);
    expect(screen.getAllByText("35").length).toBeGreaterThan(0);
  });

  it("surfaces nested bucket groups, not just the top level", () => {
    render(
      <AggregationView
        result={result({
          outer: {
            buckets: [{ key: "a", doc_count: 1 }],
            inner: { buckets: [{ key: "deep", doc_count: 2 }] },
          },
        })}
      />,
    );

    expect(screen.getAllByText("deep").length).toBeGreaterThan(0);
  });

  it("shows an empty state instead of a blank pane", () => {
    render(<AggregationView result={result(undefined)} />);
    expect(screen.getByText("无聚合结果")).toBeInTheDocument();
  });

  it("still renders payloads that contain no buckets", () => {
    render(
      <AggregationView result={result({ avg_price: { value: 42.5 } })} />,
    );
    expect(screen.getByText("原始聚合结果")).toBeInTheDocument();
  });
});
