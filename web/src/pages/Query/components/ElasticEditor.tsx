import { useRef, useEffect, useCallback, useState, useMemo } from "react";
import { EditorState } from "@codemirror/state";
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
  placeholder as cmPlaceholder,
} from "@codemirror/view";
import { json, jsonParseLinter } from "@codemirror/lang-json";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import {
  syntaxHighlighting,
  defaultHighlightStyle,
  bracketMatching,
  foldGutter,
  indentOnInput,
} from "@codemirror/language";
import { linter } from "@codemirror/lint";
import { Button } from "@/components/ui/button";
import { ChevronDown, FileText } from "lucide-react";

interface ElasticEditorProps {
  indexPattern: string;
  queryBody: string;
  onIndexPatternChange: (v: string) => void;
  onQueryBodyChange: (v: string) => void;
  onExecute: () => void;
  /** Default index patterns from datasource config */
  defaultIndexPatterns?: string[];
}

// --- ES Preset Templates ---

interface ESTemplate {
  label: string;
  description: string;
  body: string;
}

const ES_TEMPLATES: ESTemplate[] = [
  {
    label: "Match All",
    description: "查询所有文档",
    body: JSON.stringify({ query: { match_all: {} }, size: 100 }, null, 2),
  },
  {
    label: "Match 查询",
    description: "按字段值匹配",
    body: JSON.stringify(
      {
        query: {
          match: {
            message: "search text",
          },
        },
        size: 100,
      },
      null,
      2,
    ),
  },
  {
    label: "Term 精确查询",
    description: "按字段精确匹配",
    body: JSON.stringify(
      {
        query: {
          term: {
            status: "active",
          },
        },
        size: 100,
      },
      null,
      2,
    ),
  },
  {
    label: "Bool 复合查询",
    description: "组合 must/should/filter",
    body: JSON.stringify(
      {
        query: {
          bool: {
            must: [{ match: { title: "search" } }],
            filter: [{ range: { timestamp: { gte: "now-7d/d" } } }],
          },
        },
        size: 100,
      },
      null,
      2,
    ),
  },
  {
    label: "聚合统计",
    description: "Terms aggregation + count",
    body: JSON.stringify(
      {
        size: 0,
        aggs: {
          group_by_status: {
            terms: { field: "status.keyword", size: 10 },
          },
        },
      },
      null,
      2,
    ),
  },
  {
    label: "日期范围查询",
    description: "按时间范围过滤",
    body: JSON.stringify(
      {
        query: {
          range: {
            "@timestamp": {
              gte: "now-1d/d",
              lt: "now/d",
            },
          },
        },
        sort: [{ "@timestamp": { order: "desc" } }],
        size: 100,
      },
      null,
      2,
    ),
  },
];

// --- JSON Theme (same as MongoEditor) ---

const jsonTheme = EditorView.theme({
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
  ".cm-lint-marker": {
    cursor: "pointer",
  },
});

// --- JSON Editor Sub-component ---

function JsonEditor({
  value,
  onChange,
  onExecute,
  placeholder,
}: {
  value: string;
  onChange: (v: string) => void;
  onExecute: () => void;
  placeholder?: string;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  const onExecuteRef = useRef(onExecute);
  const isExternalUpdate = useRef(false);

  useEffect(() => {
    onChangeRef.current = onChange;
    onExecuteRef.current = onExecute;
  });

  const getExtensions = useCallback(
    () => [
      json(),
      lineNumbers(),
      highlightActiveLine(),
      highlightActiveLineGutter(),
      history(),
      foldGutter(),
      bracketMatching(),
      indentOnInput(),
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      linter(jsonParseLinter()),
      keymap.of([
        ...defaultKeymap,
        ...historyKeymap,
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
      EditorState.tabSize.of(2),
      jsonTheme,
      ...(placeholder ? [cmPlaceholder(placeholder)] : []),
    ],
    [placeholder],
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

// --- Main Component ---

export default function ElasticEditor({
  indexPattern,
  queryBody,
  onIndexPatternChange,
  onQueryBodyChange,
  onExecute,
  defaultIndexPatterns = [],
}: ElasticEditorProps) {
  const [templateOpen, setTemplateOpen] = useState(false);
  const [indexListOpen, setIndexListOpen] = useState(false);
  const indexPickerRef = useRef<HTMLDivElement>(null);

  const filteredIndexPatterns = useMemo(() => {
    const keyword = indexPattern.trim().toLowerCase();
    if (!keyword) return defaultIndexPatterns;
    return defaultIndexPatterns.filter((pattern) =>
      pattern.toLowerCase().includes(keyword),
    );
  }, [defaultIndexPatterns, indexPattern]);

  useEffect(() => {
    if (!indexListOpen) return;

    function handlePointerDown(event: PointerEvent) {
      if (
        event.target instanceof Node &&
        !indexPickerRef.current?.contains(event.target)
      ) {
        setIndexListOpen(false);
      }
    }

    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, [indexListOpen]);

  function selectIndexPattern(pattern: string) {
    onIndexPatternChange(pattern);
    setIndexListOpen(false);
  }

  function applyTemplate(tpl: ESTemplate) {
    onQueryBodyChange(tpl.body);
    setTemplateOpen(false);
  }

  const defaultBody = useMemo(
    () =>
      JSON.stringify(
        {
          query: { match_all: {} },
          size: 100,
        },
        null,
        2,
      ),
    [],
  );

  return (
    <div className="flex h-full flex-col gap-0 overflow-hidden bg-[var(--bg-surface)]">
      {/* Controls bar */}
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--border-default)] px-3 py-2">
        {/* Index Pattern input */}
        <div ref={indexPickerRef} className="relative">
          <input
            type="text"
            value={indexPattern}
            role="combobox"
            aria-label="Index Pattern"
            aria-expanded={indexListOpen}
            aria-controls="es-index-pattern-options"
            aria-autocomplete="list"
            onFocus={() =>
              setIndexListOpen(defaultIndexPatterns.length > 0)
            }
            onChange={(e) => {
              onIndexPatternChange(e.target.value);
              setIndexListOpen(defaultIndexPatterns.length > 0);
            }}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                setIndexListOpen(false);
              } else if (
                e.key === "ArrowDown" &&
                filteredIndexPatterns.length > 0
              ) {
                e.preventDefault();
                setIndexListOpen(true);
              }
            }}
            placeholder="Index Pattern (e.g. logs-*, my-index)"
            className="h-8 w-72 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 pr-16 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent-primary)]"
          />
          {defaultIndexPatterns.length > 0 && (
            <button
              type="button"
              aria-label="选择索引"
              className="absolute right-1 top-1/2 flex h-6 -translate-y-1/2 items-center gap-1 rounded px-1.5 text-[10px] text-[var(--text-muted)] hover:bg-[var(--bg-surface)] hover:text-[var(--text-secondary)]"
              onClick={() => setIndexListOpen((open) => !open)}
            >
              {defaultIndexPatterns.length}
              <ChevronDown
                size={12}
                className={`transition-transform ${indexListOpen ? "rotate-180" : ""}`}
              />
            </button>
          )}
          {indexListOpen && (
            <div
              id="es-index-pattern-options"
              role="listbox"
              aria-label="Elasticsearch 索引"
              className="absolute left-0 top-full z-50 mt-1 max-h-64 w-96 overflow-y-auto rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-1 shadow-xl"
            >
              {filteredIndexPatterns.length > 0 ? (
                filteredIndexPatterns.map((pattern) => (
                  <button
                    key={pattern}
                    type="button"
                    role="option"
                    aria-selected={pattern === indexPattern}
                    className="block w-full truncate rounded-md px-3 py-2 text-left text-sm text-[var(--text-primary)] hover:bg-[var(--bg-elevated)]"
                    title={pattern}
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => selectIndexPattern(pattern)}
                  >
                    {pattern}
                  </button>
                ))
              ) : (
                <div className="px-3 py-3 text-sm text-[var(--text-muted)]">
                  没有匹配的索引
                </div>
              )}
            </div>
          )}
        </div>

        {/* Templates dropdown */}
        <div className="relative">
          <Button
            variant="ghost"
            size="sm"
            className="h-7 gap-1 px-2 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
            onClick={() => setTemplateOpen(!templateOpen)}
          >
            <FileText size={12} />
            模板
          </Button>
          {templateOpen && (
            <>
              <div
                className="fixed inset-0 z-40"
                onClick={() => setTemplateOpen(false)}
              />
              <div className="absolute left-0 top-full z-50 mt-1 w-72 rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-lg">
                <div className="px-3 py-2 text-xs font-medium text-[var(--text-secondary)]">
                  查询模板
                </div>
                <div className="border-t border-[var(--border-default)] py-1">
                  {ES_TEMPLATES.map((tpl) => (
                    <button
                      key={tpl.label}
                      className="flex w-full flex-col gap-0.5 px-3 py-2 text-left transition-colors hover:bg-[var(--bg-elevated)]/50"
                      onClick={() => applyTemplate(tpl)}
                    >
                      <span className="text-xs font-medium text-[var(--text-primary)]">
                        {tpl.label}
                      </span>
                      <span className="text-[10px] text-[var(--text-muted)]">
                        {tpl.description}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            </>
          )}
        </div>

        {/* ES indicator */}
        <span className="rounded bg-orange-500/20 px-1.5 py-0.5 text-[10px] font-medium text-orange-400">
          Elasticsearch
        </span>
      </div>

      {/* Query Body editor */}
      <div className="flex-1 overflow-hidden">
        <div className="flex h-full flex-col">
          <div className="shrink-0 px-3 pt-2 pb-1">
            <span className="text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">
              Query DSL
            </span>
          </div>
          <div className="flex-1 overflow-hidden">
            <JsonEditor
              value={queryBody || defaultBody}
              onChange={onQueryBodyChange}
              onExecute={onExecute}
              placeholder={
                '{\n  "query": {\n    "match_all": {}\n  },\n  "size": 100\n}'
              }
            />
          </div>
        </div>
      </div>
    </div>
  );
}
