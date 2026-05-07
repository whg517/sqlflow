import { useRef, useEffect, useCallback, useState } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, placeholder as cmPlaceholder } from '@codemirror/view'
import { json, jsonParseLinter } from '@codemirror/lang-json'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { syntaxHighlighting, defaultHighlightStyle, bracketMatching, foldGutter, indentOnInput } from '@codemirror/language'
import { linter } from '@codemirror/lint'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { ChevronDown, ChevronRight, FileText } from 'lucide-react'
import {
  Tooltip, TooltipContent, TooltipTrigger,
} from '@/components/ui/tooltip'
import type { MongoOperation } from '@/store/queryStore'

interface MongoEditorProps {
  collection: string
  operation: MongoOperation
  filter: string
  options: string
  onCollectionChange: (v: string) => void
  onOperationChange: (v: MongoOperation) => void
  onFilterChange: (v: string) => void
  onOptionsChange: (v: string) => void
  onExecute: () => void
}

const jsonTheme = EditorView.theme({
  '&': {
    height: '100%',
    fontSize: '13px',
    backgroundColor: 'var(--bg-surface)',
    color: 'var(--text-primary)',
  },
  '.cm-content': {
    fontFamily: 'var(--font-mono)',
    caretColor: 'var(--accent-primary)',
    padding: '8px 0',
  },
  '.cm-cursor': {
    borderLeftColor: 'var(--accent-primary)',
  },
  '.cm-activeLine': {
    backgroundColor: 'rgba(99, 102, 241, 0.08)',
  },
  '.cm-activeLineGutter': {
    backgroundColor: 'rgba(99, 102, 241, 0.08)',
  },
  '.cm-gutters': {
    backgroundColor: 'var(--bg-surface)',
    color: 'var(--text-muted)',
    border: 'none',
    borderRight: '1px solid var(--border-subtle)',
  },
  '.cm-lineNumbers .cm-gutterElement': {
    minWidth: '3em',
    padding: '0 8px 0 12px',
  },
  '.cm-foldGutter .cm-gutterElement': {
    cursor: 'pointer',
    color: 'var(--text-muted)',
  },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
    backgroundColor: 'rgba(99, 102, 241, 0.25) !important',
  },
  '&.cm-focused': {
    outline: 'none',
  },
  '.cm-scroller': {
    overflow: 'auto',
    fontFamily: 'var(--font-mono)',
  },
  '.cm-lint-marker': {
    cursor: 'pointer',
  },
})

// --- Preset Templates ---

interface PresetTemplate {
  label: string
  description: string
  filter: string
  options: string
  operation: MongoOperation
}

const PRESET_TEMPLATES: PresetTemplate[] = [
  {
    label: '查询全部',
    description: '返回集合中所有文档',
    operation: 'find',
    filter: '{}',
    options: '{}',
  },
  {
    label: '按 ID 查询',
    description: '根据 _id 查找单个文档',
    operation: 'find',
    filter: '{ "_id": "ObjectId(...)" }',
    options: '{}',
  },
  {
    label: '条件过滤',
    description: '按字段条件查询并排序',
    operation: 'find',
    filter: '{ "status": "active" }',
    options: '{ "sort": { "created_at": -1 }, "limit": 100 }',
  },
  {
    label: '聚合统计',
    description: '$match + $group 聚合管道',
    operation: 'aggregate',
    filter: '[\n  { "$match": { "status": "active" } },\n  { "$group": { "_id": "$category", "count": { "$sum": 1 } } }\n]',
    options: '{}',
  },
  {
    label: '字段投影',
    description: '只返回指定字段',
    operation: 'find',
    filter: '{}',
    options: '{ "projection": { "name": 1, "email": 1 }, "limit": 50 }',
  },
]

// --- JSON Editor Sub-component ---

function JsonEditor({
  value,
  onChange,
  onExecute,
  placeholder,
}: {
  value: string
  onChange: (v: string) => void
  onExecute: () => void
  placeholder?: string
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  const onExecuteRef = useRef(onExecute)
  const isExternalUpdate = useRef(false)

  onChangeRef.current = onChange
  onExecuteRef.current = onExecute

  const getExtensions = useCallback(() => [
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
        key: 'Ctrl-Enter',
        run: () => { onExecuteRef.current(); return true },
      },
      {
        key: 'Cmd-Enter',
        run: () => { onExecuteRef.current(); return true },
      },
    ]),
    EditorView.updateListener.of((update) => {
      if (update.docChanged && !isExternalUpdate.current) {
        onChangeRef.current(update.state.doc.toString())
      }
    }),
    EditorState.tabSize.of(2),
    jsonTheme,
    ...(placeholder ? [cmPlaceholder(placeholder)] : []),
  ], [placeholder])

  useEffect(() => {
    if (!containerRef.current) return
    const view = new EditorView({
      state: EditorState.create({ doc: value, extensions: getExtensions() }),
      parent: containerRef.current,
    })
    viewRef.current = view
    return () => {
      view.destroy()
      viewRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    const currentDoc = view.state.doc.toString()
    if (currentDoc !== value) {
      isExternalUpdate.current = true
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: value },
      })
      isExternalUpdate.current = false
    }
  }, [value])

  return (
    <div ref={containerRef} className="h-full w-full overflow-hidden" />
  )
}

// --- Main Component ---

export default function MongoEditor({
  collection,
  operation,
  filter,
  options,
  onCollectionChange,
  onOperationChange,
  onFilterChange,
  onOptionsChange,
  onExecute,
}: MongoEditorProps) {
  const [optionsOpen, setOptionsOpen] = useState(false)
  const [templateOpen, setTemplateOpen] = useState(false)

  const filterLabel = operation === 'aggregate' ? 'Pipeline' : 'Filter'

  function applyTemplate(tpl: PresetTemplate) {
    onOperationChange(tpl.operation)
    onFilterChange(tpl.filter)
    onOptionsChange(tpl.options)
    setTemplateOpen(false)
  }

  return (
    <div className="flex h-full flex-col gap-0 overflow-hidden bg-[var(--bg-surface)]">
      {/* Controls bar */}
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--border-default)] px-3 py-2">
        {/* Collection */}
        <input
          type="text"
          value={collection}
          onChange={(e) => onCollectionChange(e.target.value)}
          placeholder="集合名 (collection)"
          className="h-7 w-48 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-2 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent-primary)]"
        />

        {/* Operation type */}
        <Select value={operation} onValueChange={(v) => onOperationChange(v as MongoOperation)}>
          <SelectTrigger className="h-7 w-32 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="find">find</SelectItem>
            <SelectItem value="aggregate">aggregate</SelectItem>
            <SelectItem value="update">update (工单)</SelectItem>
          </SelectContent>
        </Select>

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
              <div className="fixed inset-0 z-40" onClick={() => setTemplateOpen(false)} />
              <div className="absolute left-0 top-full z-50 mt-1 w-64 rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-lg">
                <div className="px-3 py-2 text-xs font-medium text-[var(--text-secondary)]">预设模板</div>
                <div className="border-t border-[var(--border-default)] py-1">
                  {PRESET_TEMPLATES.map((tpl) => (
                    <button
                      key={tpl.label}
                      className="flex w-full flex-col gap-0.5 px-3 py-2 text-left transition-colors hover:bg-[var(--bg-elevated)]/50"
                      onClick={() => applyTemplate(tpl)}
                    >
                      <span className="text-xs font-medium text-[var(--text-primary)]">{tpl.label}</span>
                      <span className="text-[10px] text-[var(--text-muted)]">{tpl.description}</span>
                    </button>
                  ))}
                </div>
              </div>
            </>
          )}
        </div>

        {/* Options toggle */}
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1 px-2 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
          onClick={() => setOptionsOpen(!optionsOpen)}
        >
          {optionsOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          Options
        </Button>

        {/* Update warning */}
        {operation === 'update' && (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="rounded bg-[var(--risk-medium)]/20 px-2 py-0.5 text-[10px] font-medium text-[var(--risk-medium)]">
                UPDATE 需提交工单
              </span>
            </TooltipTrigger>
            <TooltipContent>
              MongoDB update 操作需要通过变更工单流程执行
            </TooltipContent>
          </Tooltip>
        )}
      </div>

      {/* Filter / Pipeline editor */}
      <div className="flex-1 overflow-hidden">
        <div className="flex h-full flex-col">
          <div className="shrink-0 px-3 pt-2 pb-1">
            <span className="text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">
              {filterLabel}
            </span>
          </div>
          <div className="flex-1 overflow-hidden">
            <JsonEditor
              value={filter}
              onChange={onFilterChange}
              onExecute={onExecute}
              placeholder={operation === 'aggregate'
                ? '[\n  { "$match": { ... } },\n  { "$group": { ... } }\n]'
                : '{ "field": "value" }'}
            />
          </div>
        </div>
      </div>

      {/* Options editor (collapsible) */}
      {optionsOpen && (
        <div className="flex shrink-0 flex-col border-t border-[var(--border-default)]" style={{ height: '160px' }}>
          <div className="shrink-0 px-3 pt-2 pb-1">
            <span className="text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">
              Options (projection, sort, limit, skip)
            </span>
          </div>
          <div className="flex-1 overflow-hidden">
            <JsonEditor
              value={options}
              onChange={onOptionsChange}
              onExecute={onExecute}
              placeholder='{ "projection": { "name": 1 }, "sort": { "_id": -1 }, "limit": 100 }'
            />
          </div>
        </div>
      )}
    </div>
  )
}
