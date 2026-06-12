import { useRef, useEffect, useCallback } from "react";
import { EditorState, Compartment } from "@codemirror/state";
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
} from "@codemirror/view";
import { sql, MySQL } from "@codemirror/lang-sql";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import {
  syntaxHighlighting,
  defaultHighlightStyle,
  bracketMatching,
  foldGutter,
  indentOnInput,
} from "@codemirror/language";
import {
  autocompletion,
  startCompletion,
  type CompletionContext,
  type CompletionResult,
  type Completion,
  type CompletionSource,
  completionKeymap,
} from "@codemirror/autocomplete";
import type { SchemaData } from "@/hooks/useSchemaCompletion";
import {
  getFrequentQueries,
  type FrequentQuery,
} from "@/api/queryHistory";

interface SqlEditorProps {
  value: string;
  onChange: (value: string) => void;
  onExecute: () => void;
  readonly?: boolean;
  schemaData?: SchemaData | null;
  onFetchColumns?: (tableName: string) => Promise<string[]>;
}

// ==========================================
// SQL Context Detection (SF-FEAT0051)
// ==========================================

type SqlClause =
  | "select"
  | "from"
  | "where"
  | "join"
  | "groupby"
  | "orderby"
  | "having"
  | "limit"
  | "unknown";

/**
 * Detect the current SQL clause by scanning backwards from the cursor position.
 * Uses a lightweight regex-based approach — no full AST parsing.
 */
function detectSqlClause(textBeforeCursor: string): SqlClause {
  // Normalize: remove string literals and comments to avoid false matches
  const cleaned = textBeforeCursor
    .replace(/'[^']*'/g, "''") // string literals
    .replace(/--[^\n]*/g, "") // single-line comments
    .replace(/\/\*[\s\S]*?\*\//g, ""); // multi-line comments

  const upper = cleaned.toUpperCase().trimEnd();

  // Match the last major clause keyword (order matters — longer first)
  const clausePatterns: { pattern: RegExp; clause: SqlClause }[] = [
    { pattern: /\bORDER\s+BY\s*$/i, clause: "orderby" },
    { pattern: /\bGROUP\s+BY\s*$/i, clause: "groupby" },
    { pattern: /\bLEFT\s+OUTER\s+JOIN\s*$/i, clause: "join" },
    { pattern: /\bRIGHT\s+OUTER\s+JOIN\s*$/i, clause: "join" },
    { pattern: /\bINNER\s+JOIN\s*$/i, clause: "join" },
    { pattern: /\bLEFT\s+JOIN\s*$/i, clause: "join" },
    { pattern: /\bRIGHT\s+JOIN\s*$/i, clause: "join" },
    { pattern: /\bCROSS\s+JOIN\s*$/i, clause: "join" },
    { pattern: /\bJOIN\s*$/i, clause: "join" },
    { pattern: /\bHAVING\s*$/i, clause: "having" },
    { pattern: /\bWHERE\s*$/i, clause: "where" },
    { pattern: /\bFROM\s*$/i, clause: "from" },
    { pattern: /\bSELECT\s+DISTINCT\s*$/i, clause: "select" },
    { pattern: /\bSELECT\s+ALL\s*$/i, clause: "select" },
    { pattern: /\bSELECT\s*$/i, clause: "select" },
    { pattern: /\bLIMIT\s*$/i, clause: "limit" },
    // ON after JOIN — treat as WHERE-like (column conditions)
    { pattern: /\bON\s*$/i, clause: "where" },
    // SET after UPDATE — treat like WHERE (column assignments)
    { pattern: /\bSET\s*$/i, clause: "where" },
  ];

  for (const { pattern, clause } of clausePatterns) {
    if (pattern.test(upper)) return clause;
  }

  // Fallback: check if inside a clause by looking at the last keyword before cursor
  // Walk backwards through the text looking for top-level clause boundaries
  const lastClauseMatch = upper.match(
    /\b(SELECT|FROM|WHERE|JOIN|GROUP\s+BY|ORDER\s+BY|HAVING|LIMIT|ON|SET)\s[^\s]*$/i,
  );
  if (lastClauseMatch) {
    const kw = lastClauseMatch[1].toUpperCase();
    if (kw === "SELECT") return "select";
    if (kw === "FROM") return "from";
    if (kw === "WHERE") return "where";
    if (kw === "HAVING") return "having";
    if (kw === "LIMIT") return "limit";
    if (kw === "ON") return "where";
    if (kw === "SET") return "where";
    if (kw.includes("JOIN")) return "join";
    if (kw === "GROUP BY" || kw === "ORDER BY") return kw.includes("GROUP") ? "groupby" : "orderby";
  }

  return "unknown";
}

// ==========================================
// SQL Keyword Completions
// ==========================================

const SQL_KEYWORDS: Completion[] = [
  // DML
  { label: "SELECT", type: "keyword", detail: "keyword", boost: 2 },
  { label: "FROM", type: "keyword", detail: "keyword", boost: 2 },
  { label: "WHERE", type: "keyword", detail: "keyword", boost: 2 },
  { label: "AND", type: "keyword", detail: "keyword" },
  { label: "OR", type: "keyword", detail: "keyword" },
  { label: "NOT", type: "keyword", detail: "keyword" },
  { label: "IN", type: "keyword", detail: "keyword" },
  { label: "EXISTS", type: "keyword", detail: "keyword" },
  { label: "BETWEEN", type: "keyword", detail: "keyword" },
  { label: "LIKE", type: "keyword", detail: "keyword" },
  { label: "IS", type: "keyword", detail: "keyword" },
  { label: "NULL", type: "keyword", detail: "keyword" },
  { label: "AS", type: "keyword", detail: "keyword" },
  { label: "ON", type: "keyword", detail: "keyword" },
  { label: "SET", type: "keyword", detail: "keyword" },
  { label: "VALUES", type: "keyword", detail: "keyword" },
  { label: "INTO", type: "keyword", detail: "keyword" },
  // Joins
  { label: "JOIN", type: "keyword", detail: "keyword" },
  { label: "INNER JOIN", type: "keyword", detail: "keyword" },
  { label: "LEFT JOIN", type: "keyword", detail: "keyword" },
  { label: "RIGHT JOIN", type: "keyword", detail: "keyword" },
  { label: "CROSS JOIN", type: "keyword", detail: "keyword" },
  { label: "LEFT OUTER JOIN", type: "keyword", detail: "keyword" },
  { label: "RIGHT OUTER JOIN", type: "keyword", detail: "keyword" },
  // Grouping & ordering
  { label: "GROUP BY", type: "keyword", detail: "keyword" },
  { label: "ORDER BY", type: "keyword", detail: "keyword" },
  { label: "HAVING", type: "keyword", detail: "keyword" },
  { label: "LIMIT", type: "keyword", detail: "keyword" },
  { label: "OFFSET", type: "keyword", detail: "keyword" },
  { label: "ASC", type: "keyword", detail: "keyword" },
  { label: "DESC", type: "keyword", detail: "keyword" },
  { label: "DISTINCT", type: "keyword", detail: "keyword" },
  // DDL
  { label: "INSERT", type: "keyword", detail: "keyword" },
  { label: "UPDATE", type: "keyword", detail: "keyword" },
  { label: "DELETE", type: "keyword", detail: "keyword" },
  { label: "CREATE", type: "keyword", detail: "keyword" },
  { label: "ALTER", type: "keyword", detail: "keyword" },
  { label: "DROP", type: "keyword", detail: "keyword" },
  { label: "TRUNCATE", type: "keyword", detail: "keyword" },
  { label: "TABLE", type: "keyword", detail: "keyword" },
  { label: "INDEX", type: "keyword", detail: "keyword" },
  { label: "VIEW", type: "keyword", detail: "keyword" },
  // Types
  { label: "INT", type: "keyword", detail: "keyword" },
  { label: "BIGINT", type: "keyword", detail: "keyword" },
  { label: "VARCHAR", type: "keyword", detail: "keyword" },
  { label: "TEXT", type: "keyword", detail: "keyword" },
  { label: "DATETIME", type: "keyword", detail: "keyword" },
  { label: "TIMESTAMP", type: "keyword", detail: "keyword" },
  { label: "DECIMAL", type: "keyword", detail: "keyword" },
  { label: "BOOLEAN", type: "keyword", detail: "keyword" },
  // Subquery
  { label: "UNION", type: "keyword", detail: "keyword" },
  { label: "UNION ALL", type: "keyword", detail: "keyword" },
  { label: "ANY", type: "keyword", detail: "keyword" },
  { label: "ALL", type: "keyword", detail: "keyword" },
  // MySQL specific
  { label: "EXPLAIN", type: "keyword", detail: "keyword" },
  { label: "SHOW", type: "keyword", detail: "keyword" },
  { label: "IF", type: "keyword", detail: "keyword" },
  { label: "CASE", type: "keyword", detail: "keyword" },
  { label: "WHEN", type: "keyword", detail: "keyword" },
  { label: "THEN", type: "keyword", detail: "keyword" },
  { label: "ELSE", type: "keyword", detail: "keyword" },
  { label: "END", type: "keyword", detail: "keyword" },
];

// ==========================================
// SQL Function Completions
// ==========================================

const SQL_FUNCTIONS: Completion[] = [
  { label: "COUNT", type: "function", detail: "function", apply: "COUNT(" },
  { label: "SUM", type: "function", detail: "function", apply: "SUM(" },
  { label: "AVG", type: "function", detail: "function", apply: "AVG(" },
  { label: "MAX", type: "function", detail: "function", apply: "MAX(" },
  { label: "MIN", type: "function", detail: "function", apply: "MIN(" },
  { label: "NOW", type: "function", detail: "function", apply: "NOW()" },
  {
    label: "CURDATE",
    type: "function",
    detail: "function",
    apply: "CURDATE()",
  },
  {
    label: "CURTIME",
    type: "function",
    detail: "function",
    apply: "CURTIME()",
  },
  {
    label: "DATE_FORMAT",
    type: "function",
    detail: "function",
    apply: "DATE_FORMAT(",
  },
  {
    label: "STR_TO_DATE",
    type: "function",
    detail: "function",
    apply: "STR_TO_DATE(",
  },
  { label: "IFNULL", type: "function", detail: "function", apply: "IFNULL(" },
  {
    label: "COALESCE",
    type: "function",
    detail: "function",
    apply: "COALESCE(",
  },
  { label: "NULLIF", type: "function", detail: "function", apply: "NULLIF(" },
  { label: "CONCAT", type: "function", detail: "function", apply: "CONCAT(" },
  {
    label: "CONCAT_WS",
    type: "function",
    detail: "function",
    apply: "CONCAT_WS(",
  },
  {
    label: "SUBSTRING",
    type: "function",
    detail: "function",
    apply: "SUBSTRING(",
  },
  { label: "TRIM", type: "function", detail: "function", apply: "TRIM(" },
  { label: "UPPER", type: "function", detail: "function", apply: "UPPER(" },
  { label: "LOWER", type: "function", detail: "function", apply: "LOWER(" },
  { label: "LENGTH", type: "function", detail: "function", apply: "LENGTH(" },
  { label: "REPLACE", type: "function", detail: "function", apply: "REPLACE(" },
  { label: "CAST", type: "function", detail: "function", apply: "CAST(" },
  { label: "CONVERT", type: "function", detail: "function", apply: "CONVERT(" },
  { label: "ROUND", type: "function", detail: "function", apply: "ROUND(" },
  { label: "FLOOR", type: "function", detail: "function", apply: "FLOOR(" },
  { label: "CEIL", type: "function", detail: "function", apply: "CEIL(" },
  { label: "ABS", type: "function", detail: "function", apply: "ABS(" },
  { label: "MOD", type: "function", detail: "function", apply: "MOD(" },
  {
    label: "GROUP_CONCAT",
    type: "function",
    detail: "function",
    apply: "GROUP_CONCAT(",
  },
  {
    label: "FIND_IN_SET",
    type: "function",
    detail: "function",
    apply: "FIND_IN_SET(",
  },
  {
    label: "INET_ATON",
    type: "function",
    detail: "function",
    apply: "INET_ATON(",
  },
  {
    label: "INET_NTOA",
    type: "function",
    detail: "function",
    apply: "INET_NTOA(",
  },
];

// ==========================================
// Context-Aware Completion Filtering (SF-FEAT0051)
// ==========================================

/**
 * Keywords relevant in WHERE-like contexts (WHERE, ON, SET, HAVING)
 */
const WHERE_KEYWORDS = new Set([
  "AND", "OR", "NOT", "IN", "EXISTS", "BETWEEN", "LIKE",
  "IS", "NULL", "AS", "CASE", "WHEN", "THEN", "ELSE", "END",
]);

/**
 * Keywords relevant in SELECT context
 */
const SELECT_KEYWORDS = new Set([
  "AS", "DISTINCT", "CASE", "WHEN", "THEN", "ELSE", "END",
  "FROM", "WHERE", "GROUP BY", "ORDER BY", "HAVING", "LIMIT",
  "JOIN", "INNER JOIN", "LEFT JOIN", "RIGHT JOIN",
  "CROSS JOIN", "LEFT OUTER JOIN", "RIGHT OUTER JOIN",
]);

/**
 * Keywords relevant in ORDER BY context
 */
const ORDERBY_KEYWORDS = new Set(["ASC", "DESC", "LIMIT", "NULL"]);

/**
 * Keywords relevant in GROUP BY context
 */
const GROUPBY_KEYWORDS = new Set(["HAVING", "LIMIT", "ORDER BY"]);

/**
 * Filter completions based on the detected SQL clause context.
 */
function filterByContext(
  clause: SqlClause,
  keywords: Completion[],
  functions: Completion[],
  tableCompletions: Completion[],
  columnCompletions: Completion[],
): Completion[] {
  const result: Completion[] = [];

  switch (clause) {
    case "select":
      // Columns + functions + SELECT-relevant keywords
      result.push(...columnCompletions);
      result.push(...functions);
      for (const kw of keywords) {
        if (SELECT_KEYWORDS.has(kw.label)) result.push(kw);
      }
      break;

    case "from":
      // Tables + aliases (AS)
      result.push(...tableCompletions);
      for (const kw of keywords) {
        if (kw.label === "AS" || kw.label === "JOIN" || kw.label === "INNER JOIN" ||
            kw.label === "LEFT JOIN" || kw.label === "RIGHT JOIN" ||
            kw.label === "WHERE" || kw.label === "GROUP BY" || kw.label === "ORDER BY" ||
            kw.label === "HAVING" || kw.label === "LIMIT") {
          result.push(kw);
        }
      }
      break;

    case "where":
      // Columns + WHERE-relevant keywords + functions
      result.push(...columnCompletions);
      result.push(...functions);
      for (const kw of keywords) {
        if (WHERE_KEYWORDS.has(kw.label)) result.push(kw);
      }
      break;

    case "join":
      // Tables + ON
      result.push(...tableCompletions);
      for (const kw of keywords) {
        if (kw.label === "ON" || kw.label === "AS") result.push(kw);
      }
      break;

    case "groupby":
      // Columns + GROUP BY-relevant keywords
      result.push(...columnCompletions);
      for (const kw of keywords) {
        if (GROUPBY_KEYWORDS.has(kw.label)) result.push(kw);
      }
      break;

    case "orderby":
      // Columns + ASC/DESC
      result.push(...columnCompletions);
      for (const kw of keywords) {
        if (ORDERBY_KEYWORDS.has(kw.label)) result.push(kw);
      }
      break;

    case "having":
      // Columns + functions + condition keywords
      result.push(...columnCompletions);
      result.push(...functions);
      for (const kw of keywords) {
        if (WHERE_KEYWORDS.has(kw.label)) result.push(kw);
      }
      break;

    case "limit":
      // After LIMIT, just number-like keywords
      result.push(...columnCompletions);
      for (const kw of keywords) {
        if (kw.label === "OFFSET") result.push(kw);
      }
      break;

    case "unknown":
    default:
      // Fallback: show everything (original behavior)
      result.push(...keywords);
      result.push(...functions);
      result.push(...tableCompletions);
      result.push(...columnCompletions);
      break;
  }

  return result;
}

// ==========================================
// Custom Completion Icon Styling
// ==========================================

const completionTheme = EditorView.baseTheme({
  ".cm-tooltip.cm-tooltip-autocomplete": {
    "& > ul > li[aria-selected]": {
      backgroundColor: "rgba(99, 102, 241, 0.15)",
      color: "var(--text-primary)",
    },
    "& > ul > li": {
      padding: "2px 8px",
    },
    "& > ul > li .cm-completionLabel": {
      fontFamily: "var(--font-mono)",
      fontSize: "12px",
    },
    "& > ul > li .cm-completionDetail": {
      fontSize: "10px",
      marginLeft: "8px",
      fontStyle: "normal",
    },
  },
  ".cm-completionIcon": {
    width: "16px",
    height: "16px",
    fontSize: "12px",
    lineHeight: "16px",
    textAlign: "center",
  },
});

const completionIcons: Record<string, { icon: string; label: string }> = {
  keyword: { icon: "💡", label: "keyword" },
  function: { icon: "💡", label: "function" },
  class: { icon: "📋", label: "table" },
  property: { icon: "📊", label: "column" },
  text: { icon: "🕐", label: "history" },
};

// ==========================================
// Completion Sources
// ==========================================

/**
 * Context-aware SQL completion source (SF-FEAT0051).
 * Detects current SQL clause and filters completions accordingly.
 */
function createSqlCompletionSource(
  schemaRef: React.MutableRefObject<SchemaData | null | undefined>,
  fetchColumnsFn: () => ((tableName: string) => Promise<string[]>) | undefined,
): CompletionSource {
  return function sqlCompletions(
    context: CompletionContext,
  ): CompletionResult | null {
    const schemaData = schemaRef.current;

    // Build word boundary including dots for table.column
    const word = context.matchBefore(/[\w.]+/);
    if (!word || (word.from === word.to && !context.explicit)) return null;

    const text = word.text;
    const options: Completion[] = [];

    // Check if we're typing after a dot (table.column)
    const dotIndex = text.lastIndexOf(".");
    if (dotIndex > 0) {
      const tableName = text.slice(0, dotIndex);

      // Column completions for the referenced table
      if (schemaData) {
        const columns = schemaData.columns.get(tableName.toLowerCase());
        if (columns && columns.length > 0) {
          for (const col of columns) {
            options.push({
              label: col,
              type: "property",
              detail: "column",
              boost: 5,
            });
          }
        } else {
          // Trigger async column fetch — the next keystroke will have cached results
          const fn = fetchColumnsFn();
          fn?.(tableName);
        }
      }

      if (options.length === 0 && !context.explicit) return null;

      return {
        from: word.from + dotIndex + 1,
        options: options.length > 0 ? options : [],
        validFor: /^[\w]*$/,
      };
    }

    // --- Context-aware filtering (SF-FEAT0051) ---

    // Get text before cursor for clause detection
    const fullText = context.state.doc.toString();
    const cursorPos = context.pos;
    const textBeforeCursor = fullText.slice(0, cursorPos - text.length);

    const clause = detectSqlClause(textBeforeCursor);

    // Build completions by category
    const tableCompletions: Completion[] = [];
    const columnCompletions: Completion[] = [];

    if (schemaData) {
      for (const table of schemaData.tables) {
        tableCompletions.push({
          label: table,
          type: "class",
          detail: "table",
          boost: 3,
        });
      }
      // Add all columns from all tables for column context
      for (const [table, cols] of schemaData.columns.entries()) {
        for (const col of cols) {
          columnCompletions.push({
            label: col,
            type: "property",
            detail: `${table}`,
            boost: 4,
          });
        }
      }
    }

    const contextOptions = filterByContext(
      clause,
      SQL_KEYWORDS,
      SQL_FUNCTIONS,
      tableCompletions,
      columnCompletions,
    );

    options.push(...contextOptions);

    // Filter based on prefix match
    const lowerText = text.toLowerCase();
    const filtered = options.filter((opt) => {
      const lower = opt.label.toLowerCase();
      return lower.startsWith(lowerText) || lower.includes(lowerText);
    });

    if (filtered.length === 0 && !context.explicit) return null;

    return {
      from: word.from,
      options: filtered.length > 0 ? filtered : options,
      validFor: /^[\w.]*$/,
    };
  };
}

/**
 * History-based completion source (SF-FEAT0052-FE).
 * Shows top 5 frequent queries from user history.
 * Triggered by Ctrl+Shift+H or explicit invocation.
 */
function createHistoryCompletionSource(
  historyRef: React.MutableRefObject<FrequentQuery[]>,
): CompletionSource {
  return function historyCompletions(
    context: CompletionContext,
  ): CompletionResult | null {
    // Only show on explicit trigger or when typing with a special prefix
    if (!context.explicit) return null;

    const queries = historyRef.current;
    if (queries.length === 0) return null;

    const options: Completion[] = queries.slice(0, 5).map((q, idx) => {
      const snippet = q.snippet || q.sql_content.slice(0, 80);
      const countLabel = q.execution_count > 0 ? `${q.execution_count}次` : "";
      const timeLabel = q.last_executed_at
        ? new Date(q.last_executed_at).toLocaleDateString("zh-CN")
        : "";

      return {
        label: snippet.length > 60 ? snippet.slice(0, 57) + "..." : snippet,
        type: "text",
        detail: `常用SQL #${idx + 1}`,
        info: `${countLabel}${countLabel && timeLabel ? " · " : ""}${timeLabel}`,
        apply: q.sql_content,
        boost: 10 - idx, // Higher ranked = higher boost
      };
    });

    return {
      from: context.pos,
      options,
      filter: false, // Don't filter — show all 5 results
    };
  };
}

// ==========================================
// Theme
// ==========================================

const sqlTheme = EditorView.theme({
  "&": {
    height: "100%",
    fontSize: "13px",
    backgroundColor: "var(--bg-surface)",
    color: "var(--text-primary)",
  },
  ".cm-content": {
    fontFamily: "var(--font-mono)",
    caretColor: "var(--accent-primary)",
    padding: "8px 0",
  },
  ".cm-cursor": {
    borderLeftColor: "var(--accent-primary)",
  },
  ".cm-activeLine": {
    backgroundColor: "rgba(99, 102, 241, 0.08)",
  },
  ".cm-activeLineGutter": {
    backgroundColor: "rgba(99, 102, 241, 0.08)",
  },
  ".cm-gutters": {
    backgroundColor: "var(--bg-surface)",
    color: "var(--text-muted)",
    border: "none",
    borderRight: "1px solid var(--border-subtle)",
  },
  ".cm-lineNumbers .cm-gutterElement": {
    minWidth: "3em",
    padding: "0 8px 0 12px",
  },
  ".cm-foldGutter .cm-gutterElement": {
    cursor: "pointer",
    color: "var(--text-muted)",
  },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground": {
    backgroundColor: "rgba(99, 102, 241, 0.25) !important",
  },
  "&.cm-focused": {
    outline: "none",
  },
  ".cm-scroller": {
    overflow: "auto",
    fontFamily: "var(--font-mono)",
  },
});

// ==========================================
// Component
// ==========================================

export default function SqlEditor({
  value,
  onChange,
  onExecute,
  readonly,
  schemaData,
  onFetchColumns,
}: SqlEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  const onExecuteRef = useRef(onExecute);
  const onFetchColumnsRef = useRef(onFetchColumns);
  const schemaDataRef = useRef(schemaData);
  const autocompleteCompartment = useRef(new Compartment());
  const isExternalUpdate = useRef(false);

  // History state (SF-FEAT0052-FE)
  const historyRef = useRef<FrequentQuery[]>([]);
  const historyLoadingRef = useRef(false);

  // Keep refs in sync with props
  useEffect(() => {
    onChangeRef.current = onChange;
    onExecuteRef.current = onExecute;
    onFetchColumnsRef.current = onFetchColumns;
    schemaDataRef.current = schemaData;
  });

  // Fetch frequent queries on mount (SF-FEAT0052-FE)
  useEffect(() => {
    if (historyLoadingRef.current) return;
    historyLoadingRef.current = true;
    getFrequentQueries()
      .then((res) => {
        historyRef.current = res.data ?? [];
      })
      .catch(() => {
        // Silently fail — history is supplementary, not critical
        historyRef.current = [];
      })
      .finally(() => {
        historyLoadingRef.current = false;
      });
  }, []);

  // Reconfigure autocomplete when schemaData changes
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const compartment = autocompleteCompartment.current;
    if (!compartment.get(view.state)) return; // not initialized yet
    view.dispatch({
      effects: compartment.reconfigure(
        autocompletion({
          override: [
            createSqlCompletionSource(
              schemaDataRef,
              () => onFetchColumnsRef.current,
            ),
            createHistoryCompletionSource(historyRef),
          ],
          activateOnTyping: true,
          icons: false,
          addToOptions: [
            {
              render: (completion: Completion) => {
                const icon = document.createElement("span");
                const typeInfo = completionIcons[completion.type ?? ""];
                if (typeInfo) {
                  icon.textContent = typeInfo.icon;
                  icon.style.marginRight = "4px";
                  icon.style.fontSize = "11px";
                }
                return icon;
              },
              position: 20,
            },
          ],
        }),
      ),
    });
  }, [schemaData]);

  const getExtensions = useCallback(
    () => [
      sql({ dialect: MySQL }),
      lineNumbers(),
      highlightActiveLine(),
      highlightActiveLineGutter(),
      history(),
      foldGutter(),
      bracketMatching(),
      indentOnInput(),
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      // Autocompletion in a compartment for dynamic reconfiguration
      autocompleteCompartment.current.of(
        autocompletion({
          override: [
            createSqlCompletionSource(
              schemaDataRef,
              () => onFetchColumnsRef.current,
            ),
            createHistoryCompletionSource(historyRef),
          ],
          activateOnTyping: true,
          icons: false,
          addToOptions: [
            {
              render: (completion: Completion) => {
                const icon = document.createElement("span");
                const typeInfo = completionIcons[completion.type ?? ""];
                if (typeInfo) {
                  icon.textContent = typeInfo.icon;
                  icon.style.marginRight = "4px";
                  icon.style.fontSize = "11px";
                }
                return icon;
              },
              position: 20,
            },
          ],
        }),
      ),
      completionTheme,
      keymap.of([
        ...defaultKeymap,
        ...historyKeymap,
        ...completionKeymap,
        {
          key: "Ctrl-Enter",
          run: () => {
            onExecuteRef.current();
            return true;
          },
        },
        {
          key: "Cmd-Enter",
          run: () => {
            onExecuteRef.current();
            return true;
          },
        },
        // SF-FEAT0052-FE: Ctrl+Shift+H triggers history completion
        {
          key: "Ctrl-Shift-h",
          run: (view) => {
            startCompletion(view);
            return true;
          },
        },
      ]),
      EditorView.updateListener.of((update) => {
        if (update.docChanged && !isExternalUpdate.current) {
          onChangeRef.current(update.state.doc.toString());
        }
      }),
      EditorState.readOnly.of(readonly ?? false),
      sqlTheme,
    ],
    [readonly],
  );

  useEffect(() => {
    if (!containerRef.current) return;
    const view = new EditorView({
      state: EditorState.create({ doc: value, extensions: getExtensions() }),
      parent: containerRef.current,
    });
    viewRef.current = view;
    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Sync external value changes (tab switches) by replacing doc content
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const currentDoc = view.state.doc.toString();
    if (currentDoc !== value) {
      isExternalUpdate.current = true;
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: value },
      });
      isExternalUpdate.current = false;
    }
  }, [value]);

  return <div ref={containerRef} className="h-full w-full overflow-hidden" />;
}
