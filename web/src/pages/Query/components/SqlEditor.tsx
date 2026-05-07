import { useRef, useEffect, useCallback } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view'
import { sql, MySQL } from '@codemirror/lang-sql'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { syntaxHighlighting, defaultHighlightStyle, bracketMatching, foldGutter, indentOnInput } from '@codemirror/language'

interface SqlEditorProps {
  value: string
  onChange: (value: string) => void
  onExecute: () => void
  readonly?: boolean
}

const sqlTheme = EditorView.theme({
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
})

export default function SqlEditor({ value, onChange, onExecute, readonly }: SqlEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  const onExecuteRef = useRef(onExecute)
  const isExternalUpdate = useRef(false)

  onChangeRef.current = onChange
  onExecuteRef.current = onExecute

  const getExtensions = useCallback(() => [
    sql({ dialect: MySQL }),
    lineNumbers(),
    highlightActiveLine(),
    highlightActiveLineGutter(),
    history(),
    foldGutter(),
    bracketMatching(),
    indentOnInput(),
    syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
    keymap.of([
      ...defaultKeymap,
      ...historyKeymap,
      {
        key: 'Ctrl-Enter',
        run: () => {
          onExecuteRef.current()
          return true
        },
      },
      {
        key: 'Cmd-Enter',
        run: () => {
          onExecuteRef.current()
          return true
        },
      },
    ]),
    EditorView.updateListener.of((update) => {
      if (update.docChanged && !isExternalUpdate.current) {
        onChangeRef.current(update.state.doc.toString())
      }
    }),
    EditorState.readOnly.of(readonly ?? false),
    sqlTheme,
  ], [readonly])

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

  // Sync external value changes (tab switches) by replacing doc content
  // instead of recreating the entire editor state, preserving undo history per tab
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
    <div
      ref={containerRef}
      className="h-full w-full overflow-hidden"
    />
  )
}
