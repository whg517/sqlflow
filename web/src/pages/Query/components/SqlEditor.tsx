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
  type CompletionContext,
  type CompletionResult,
  type Completion,
  type CompletionSource,
  completionKeymap,
} from "@codemirror/autocomplete";
import type { SchemaData } from "@/hooks/useSchemaCompletion";

interface SqlEditorProps {
  value: string;
  onChange: (value: string) => void;
  onExecute: () => void;
  readonly?: boolean;
  schemaData?: SchemaData | null;
  onFetchColumns?: (tableName: string) => Promise<string[]>;
}

// --- SQL Keyword Completions ---

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

// --- SQL Function Completions ---

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

// --- Custom completion icon styling ---

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

// --- Completion Sources ---

/**
 * Create a completion source that combines keywords, functions, tables, and columns.
 * Uses a ref to access the latest schema data so the closure always sees current data.
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

    // Regular completions (keywords, functions, tables)
    for (const kw of SQL_KEYWORDS) options.push(kw);
    for (const fn of SQL_FUNCTIONS) options.push(fn);

    // Tables from schema
    if (schemaData) {
      for (const table of schemaData.tables) {
        options.push({
          label: table,
          type: "class",
          detail: "table",
          boost: 3,
        });
      }
    }

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

// --- Theme ---

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

// --- Completion icon renderer ---

const completionIcons: Record<string, { icon: string; label: string }> = {
  keyword: { icon: "💡", label: "keyword" },
  function: { icon: "💡", label: "function" },
  class: { icon: "📋", label: "table" },
  property: { icon: "📊", label: "column" },
};

// --- Component ---

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

  // Keep refs in sync with props
  useEffect(() => {
    onChangeRef.current = onChange;
    onExecuteRef.current = onExecute;
    onFetchColumnsRef.current = onFetchColumns;
    schemaDataRef.current = schemaData;
  });

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
