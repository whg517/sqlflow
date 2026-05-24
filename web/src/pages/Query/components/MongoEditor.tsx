import { useRef, useEffect, useCallback, useState, useMemo } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, placeholder as cmPlaceholder } from '@codemirror/view'
import { json, jsonParseLinter } from '@codemirror/lang-json'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { syntaxHighlighting, defaultHighlightStyle, bracketMatching, foldGutter, indentOnInput } from '@codemirror/language'
import { linter } from '@codemirror/lint'
import {
  autocompletion,
  type CompletionContext,
  type CompletionResult,
  type Completion,
  type CompletionSource,
  completionKeymap,
} from '@codemirror/autocomplete'
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
  /** Collection names from schema for autocomplete */
  collectionNames?: string[]
}

// --- MongoDB Operator Completions ---

const MONGO_PIPELINE_OPERATORS: Completion[] = [
  { label: '$match', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$group', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$project', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$sort', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$limit', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$skip', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$count', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$unwind', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$addFields', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$set', type: 'keyword', detail: 'pipeline', boost: 5 },
  { label: '$unset', type: 'keyword', detail: 'pipeline' },
  { label: '$lookup', type: 'keyword', detail: 'pipeline' },
  { label: '$facet', type: 'keyword', detail: 'pipeline' },
  { label: '$bucket', type: 'keyword', detail: 'pipeline' },
  { label: '$bucketAuto', type: 'keyword', detail: 'pipeline' },
  { label: '$out', type: 'keyword', detail: 'pipeline' },
  { label: '$merge', type: 'keyword', detail: 'pipeline' },
  { label: '$replaceRoot', type: 'keyword', detail: 'pipeline' },
  { label: '$sample', type: 'keyword', detail: 'pipeline' },
  { label: '$redact', type: 'keyword', detail: 'pipeline' },
  { label: '$sortByCount', type: 'keyword', detail: 'pipeline' },
]

const MONGO_QUERY_OPERATORS: Completion[] = [
  { label: '$eq', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$ne', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$gt', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$gte', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$lt', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$lte', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$in', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$nin', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$and', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$or', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$not', type: 'keyword', detail: 'query', boost: 4 },
  { label: '$nor', type: 'keyword', detail: 'query' },
  { label: '$exists', type: 'keyword', detail: 'query' },
  { label: '$type', type: 'keyword', detail: 'query' },
  { label: '$regex', type: 'keyword', detail: 'query' },
  { label: '$text', type: 'keyword', detail: 'query' },
  { label: '$where', type: 'keyword', detail: 'query' },
  { label: '$all', type: 'keyword', detail: 'query' },
  { label: '$elemMatch', type: 'keyword', detail: 'query' },
  { label: '$size', type: 'keyword', detail: 'query' },
]

const MONGO_UPDATE_OPERATORS: Completion[] = [
  { label: '$set', type: 'keyword', detail: 'update', boost: 3 },
  { label: '$unset', type: 'keyword', detail: 'update', boost: 3 },
  { label: '$inc', type: 'keyword', detail: 'update', boost: 3 },
  { label: '$push', type: 'keyword', detail: 'update', boost: 3 },
  { label: '$pull', type: 'keyword', detail: 'update', boost: 3 },
  { label: '$addToSet', type: 'keyword', detail: 'update', boost: 3 },
  { label: '$pop', type: 'keyword', detail: 'update', boost: 3 },
  { label: '$rename', type: 'keyword', detail: 'update' },
  { label: '$mul', type: 'keyword', detail: 'update' },
  { label: '$min', type: 'keyword', detail: 'update' },
  { label: '$max', type: 'keyword', detail: 'update' },
  { label: '$currentDate', type: 'keyword', detail: 'update' },
]

const MONGO_AGGREGATION_EXPRESSIONS: Completion[] = [
  { label: '$sum', type: 'function', detail: 'accumulator', boost: 3 },
  { label: '$avg', type: 'function', detail: 'accumulator', boost: 3 },
  { label: '$min', type: 'function', detail: 'accumulator', boost: 3 },
  { label: '$max', type: 'function', detail: 'accumulator', boost: 3 },
  { label: '$first', type: 'function', detail: 'accumulator' },
  { label: '$last', type: 'function', detail: 'accumulator' },
  { label: '$push', type: 'function', detail: 'accumulator' },
  { label: '$addToSet', type: 'function', detail: 'accumulator' },
  { label: '$concatArrays', type: 'function', detail: 'expression' },
  { label: '$filter', type: 'function', detail: 'expression' },
  { label: '$map', type: 'function', detail: 'expression' },
  { label: '$reduce', type: 'function', detail: 'expression' },
  { label: '$cond', type: 'function', detail: 'expression' },
  { label: '$ifNull', type: 'function', detail: 'expression' },
  { label: '$switch', type: 'function', detail: 'expression' },
  { label: '$toString', type: 'function', detail: 'expression' },
  { label: '$toInt', type: 'function', detail: 'expression' },
  { label: '$toDouble', type: 'function', detail: 'expression' },
  { label: '$toDate', type: 'function', detail: 'expression' },
  { label: '$toBool', type: 'function', detail: 'expression' },
]

const ALL_MONGO_COMPLETIONS: Completion[] = [
  ...MONGO_PIPELINE_OPERATORS,
  ...MONGO_QUERY_OPERATORS,
  ...MONGO_UPDATE_OPERATORS,
  ...MONGO_AGGREGATION_EXPRESSIONS,
]

// --- JSON Theme ---

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

// --- Custom completion icon styling ---

const completionTheme = EditorView.baseTheme({
  '.cm-tooltip.cm-tooltip-autocomplete': {
    '& > ul > li[aria-selected]': {
      backgroundColor: 'rgba(99, 102, 241, 0.15)',
      color: 'var(--text-primary)',
    },
    '& > ul > li': {
      padding: '2px 8px',
    },
    '& > ul > li .cm-completionLabel': {
      fontFamily: 'var(--font-mono)',
      fontSize: '12px',
    },
    '& > ul > li .cm-completionDetail': {
      fontSize: '10px',
      marginLeft: '8px',
      fontStyle: 'normal',
    },
  },
  '.cm-completionIcon': {
    width: '16px',
    height: '16px',
    fontSize: '12px',
    lineHeight: '16px',
    textAlign: 'center',
  },
})

// --- Completion icon renderer ---

const completionIcons: Record<string, { icon: string; label: string }> = {
  keyword: { icon: '💡', label: 'operator' },
  function: { icon: '💡', label: 'expression' },
}

// --- MongoDB Completion Source ---

function createMongoCompletionSource(): CompletionSource {
  return function mongoCompletions(context: CompletionContext): CompletionResult | null {
    // Match $ prefixed tokens (MongoDB operators)
    const dollarWord = context.matchBefore(/\$\w*/)
    if (dollarWord && dollarWord.from < dollarWord.to) {
      const text = dollarWord.text.toLowerCase()
      const filtered = ALL_MONGO_COMPLETIONS.filter((opt) =>
        opt.label.toLowerCase().startsWith(text) || opt.label.toLowerCase().includes(text),
      )
      if (filtered.length > 0) {
        return {
          from: dollarWord.from,
          options: filtered,
          validFor: /^\$[\w]*$/,
        }
      }
    }

    // Also trigger explicitly with Ctrl+Space
    if (context.explicit) {
      const word = context.matchBefore(/\$?\w*/)
      return {
        from: word ? word.from : context.pos,
        options: ALL_MONGO_COMPLETIONS,
        validFor: /^\$?[\w]*$/,
      }
    }

    return null
  }
}

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

  useEffect(() => {
    onChangeRef.current = onChange
    onExecuteRef.current = onExecute
  })

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
    // MongoDB operator autocompletion
    autocompletion({
      override: [createMongoCompletionSource()],
      activateOnTyping: true,
      icons: false,
      addToOptions: [
        {
          render: (completion: Completion) => {
            const icon = document.createElement('span')
            const typeInfo = completionIcons[completion.type ?? '']
            if (typeInfo) {
              icon.textContent = typeInfo.icon
              icon.style.marginRight = '4px'
              icon.style.fontSize = '11px'
            }
            return icon
          },
          position: 20,
        },
      ],
    }),
    completionTheme,
    keymap.of([
      ...defaultKeymap,
      ...historyKeymap,
      ...completionKeymap,
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
  collectionNames = [],
}: MongoEditorProps) {
  const [optionsOpen, setOptionsOpen] = useState(false)
  const [templateOpen, setTemplateOpen] = useState(false)
  const [collectionSuggestionsOpen, setCollectionSuggestionsOpen] = useState(false)
  const [collectionInputFocus, setCollectionInputFocus] = useState(false)

  const filterLabel = operation === 'aggregate' ? 'Pipeline' : 'Filter'

  // Filter collection suggestions based on input
  const filteredCollections = useMemo(() => {
    if (!collection.trim()) return collectionNames
    const lower = collection.toLowerCase()
    return collectionNames.filter((name) => name.toLowerCase().includes(lower))
  }, [collection, collectionNames])

  function applyTemplate(tpl: PresetTemplate) {
    onOperationChange(tpl.operation)
    onFilterChange(tpl.filter)
    onOptionsChange(tpl.options)
    setTemplateOpen(false)
  }

  function handleCollectionSelect(name: string) {
    onCollectionChange(name)
    setCollectionSuggestionsOpen(false)
  }

  return (
    <div className="flex h-full flex-col gap-0 overflow-hidden bg-[var(--bg-surface)]">
      {/* Controls bar */}
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--border-default)] px-4 py-2.5">
        {/* Collection input with autocomplete */}
        <div className="relative">
          <input
            type="text"
            value={collection}
            onChange={(e) => {
              onCollectionChange(e.target.value)
              setCollectionSuggestionsOpen(true)
            }}
            onFocus={() => {
              setCollectionInputFocus(true)
              setCollectionSuggestionsOpen(true)
            }}
            onBlur={() => {
              // Delay to allow click on suggestion
              setTimeout(() => {
                setCollectionInputFocus(false)
                setCollectionSuggestionsOpen(false)
              }, 200)
            }}
            placeholder="集合名 (collection)"
            className="h-8 w-52 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent-primary)]"
          />
          {/* Collection suggestions dropdown */}
          {collectionInputFocus && collectionSuggestionsOpen && filteredCollections.length > 0 && (
            <div className="absolute left-0 top-full z-50 mt-1 max-h-48 w-48 overflow-y-auto rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-lg">
              {filteredCollections.map((name) => (
                <button
                  key={name}
                  className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs transition-colors hover:bg-[var(--bg-elevated)]/50"
                  onMouseDown={(e) => {
                    e.preventDefault()
                    handleCollectionSelect(name)
                  }}
                >
                  <span className="text-[var(--text-primary)]">📋</span>
                  <span className="text-xs">{name}</span>
                  <span className="ml-auto text-[10px] text-[var(--text-muted)]">collection</span>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Operation type */}
        <Select value={operation} onValueChange={(v) => onOperationChange(v as MongoOperation)}>
          <SelectTrigger className="h-8 w-36 border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm">
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
