import type { QueryForm } from "@/features/query/api/capabilities";
import { buildESQuerySql, buildMongoSql } from "@/features/query/api/query";
import type { MongoQueryBody } from "@/features/query/api/query";
import type { QueryTab } from "@/store/queryStore";

// A query mode owns everything that differs between data source families:
// how a tab's fields become a request, what makes a tab valid, and how the
// workbench labels it.
//
// The workbench used to answer these with `if (isMongo) … else if (isES) … else`
// repeated at every call site, which meant a fourth data source had to be
// remembered in several places at once. Modes keep those answers in one object
// per family, keyed by the driver-declared query form.

export type BuildOutcome =
  | { ok: true; sql: string }
  | { ok: false; error: string };

export interface QueryMode {
  form: QueryForm;
  /** Placeholder for the database/index input in the toolbar. */
  databasePlaceholder: string;
  /**
   * Turn the tab's editor state into the `sql` field of an execute request.
   * Every payload still travels as a string because that is the wire contract;
   * the mode owns the encoding.
   */
  build(tab: QueryTab): BuildOutcome;
}

const sqlMode: QueryMode = {
  form: "sql",
  databasePlaceholder: "数据库名",
  build(tab) {
    const sql = tab.sql.trim();
    if (!sql) {
      return { ok: false, error: "请输入 SQL" };
    }
    return { ok: true, sql };
  },
};

const documentMode: QueryMode = {
  form: "document",
  databasePlaceholder: "Database",
  build(tab) {
    if (tab.mongoOperation === "update") {
      return { ok: false, error: "MongoDB update 操作需要通过变更工单提交" };
    }
    if (!tab.mongoCollection.trim()) {
      return { ok: false, error: "请输入集合名" };
    }

    let filter: unknown;
    try {
      filter = JSON.parse(tab.mongoFilter || "{}");
    } catch {
      return { ok: false, error: "Filter JSON 格式错误" };
    }

    let options: Record<string, unknown>;
    try {
      options = JSON.parse(tab.mongoOptions || "{}");
    } catch {
      return { ok: false, error: "Options JSON 格式错误" };
    }

    const body: MongoQueryBody = {
      collection: tab.mongoCollection.trim(),
      operation: tab.mongoOperation,
    };
    if (tab.mongoOperation === "aggregate") {
      body.pipeline = Array.isArray(filter) ? filter : [];
    } else {
      body.filter = (filter ?? {}) as Record<string, unknown>;
    }
    if (options && Object.keys(options).length > 0) {
      body.options = options;
    }

    return { ok: true, sql: buildMongoSql(body) };
  },
};

const dslMode: QueryMode = {
  form: "dsl",
  databasePlaceholder: "Index Pattern",
  build(tab) {
    const index = tab.esIndexPattern.trim();
    if (!index) {
      return { ok: false, error: "请输入 Index Pattern" };
    }
    return { ok: true, sql: buildESQuerySql(index, tab.esQueryBody) };
  },
};

const MODES: Record<QueryForm, QueryMode> = {
  sql: sqlMode,
  document: documentMode,
  dsl: dslMode,
};

// Fallback used before the capabilities response arrives, and for datasource
// types whose form the server does not report. SQL is the safe default: its
// editor is plain text and its build step only trims.
const DEFAULT_FORM: QueryForm = "sql";

/** queryModeFor resolves the mode for a driver-declared query form. */
export function queryModeFor(form: QueryForm | undefined): QueryMode {
  return MODES[form ?? DEFAULT_FORM] ?? MODES[DEFAULT_FORM];
}
