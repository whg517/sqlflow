import { describe, it, expect } from "vitest";
import { queryModeFor } from "@/features/query/pages/Query/queryModes";
import type { QueryTab } from "@/store/queryStore";

// The workbench used to repeat `if (isMongo) … else if (isES) … else` at every
// place a request was built. These tests pin the single replacement so a fourth
// data source cannot be half-added.

function tab(overrides: Partial<QueryTab> = {}): QueryTab {
  return {
    id: "t1",
    title: "Query",
    sql: "",
    datasourceId: 1,
    datasourceType: "",
    database: "",
    queryParams: [],
    sourceTemplateId: null,
    sourceTemplateName: "",
    result: null,
    executing: false,
    error: null,
    dirty: false,
    mongoCollection: "",
    mongoOperation: "find",
    mongoFilter: "",
    mongoOptions: "",
    aiReviewStatus: "idle",
    aiReviewResult: null,
    aiReviewContent: "",
    aiReviewError: null,
    esIndexPattern: "",
    esQueryBody: "",
    ...overrides,
  } as QueryTab;
}

describe("queryModeFor", () => {
  it("falls back to sql for an unknown or missing form", () => {
    expect(queryModeFor(undefined).form).toBe("sql");
  });

  it("selects a mode per driver-declared form", () => {
    expect(queryModeFor("sql").form).toBe("sql");
    expect(queryModeFor("document").form).toBe("document");
    expect(queryModeFor("dsl").form).toBe("dsl");
  });
});

describe("sql mode", () => {
  const mode = queryModeFor("sql");

  it("trims the statement", () => {
    const out = mode.build(tab({ sql: "  SELECT 1  " }));
    expect(out).toEqual({ ok: true, sql: "SELECT 1" });
  });

  it("rejects an empty statement", () => {
    expect(mode.build(tab({ sql: "   " })).ok).toBe(false);
  });
});

describe("document mode", () => {
  const mode = queryModeFor("document");

  it("builds a find body", () => {
    const out = mode.build(
      tab({
        mongoCollection: "users",
        mongoOperation: "find",
        mongoFilter: '{"a":1}',
      }),
    );
    expect(out.ok).toBe(true);
    if (!out.ok) return;
    expect(JSON.parse(out.sql)).toEqual({
      collection: "users",
      operation: "find",
      filter: { a: 1 },
    });
  });

  it("maps aggregate filters onto pipeline", () => {
    const out = mode.build(
      tab({
        mongoCollection: "orders",
        mongoOperation: "aggregate",
        mongoFilter: '[{"$match":{"x":1}}]',
      }),
    );
    expect(out.ok).toBe(true);
    if (!out.ok) return;
    expect(JSON.parse(out.sql).pipeline).toEqual([{ $match: { x: 1 } }]);
  });

  it("routes update to the ticket workflow instead of executing", () => {
    const out = mode.build(
      tab({ mongoCollection: "users", mongoOperation: "update" }),
    );
    expect(out.ok).toBe(false);
    if (out.ok) return;
    expect(out.error).toContain("工单");
  });

  it("reports malformed filter JSON", () => {
    const out = mode.build(
      tab({ mongoCollection: "users", mongoFilter: "{not json" }),
    );
    expect(out.ok).toBe(false);
    if (out.ok) return;
    expect(out.error).toContain("Filter");
  });

  it("requires a collection", () => {
    expect(mode.build(tab({ mongoCollection: "  " })).ok).toBe(false);
  });
});

describe("dsl mode", () => {
  const mode = queryModeFor("dsl");

  it("packs index and body together", () => {
    const out = mode.build(
      tab({
        esIndexPattern: "logs-*",
        esQueryBody: '{"query":{"match_all":{}}}',
      }),
    );
    expect(out.ok).toBe(true);
    if (!out.ok) return;
    const parsed = JSON.parse(out.sql);
    expect(parsed.index).toBe("logs-*");
    expect(parsed.body).toEqual({ query: { match_all: {} } });
  });

  it("requires an index pattern", () => {
    const out = mode.build(tab({ esIndexPattern: "   " }));
    expect(out.ok).toBe(false);
    if (out.ok) return;
    expect(out.error).toContain("Index Pattern");
  });
});
