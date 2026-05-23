import { useState, useEffect, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Database,
  FileText,
  ShieldCheck,
  ScrollText,
  Server,
  EyeOff,
  Bot,
  Clock,
  Loader2,
  AlertCircle,
  History,
} from 'lucide-react'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import HighlightText from '@/components/HighlightText'
import { searchQueryHistory, type QueryHistoryItem } from '@/api/query'
import { listTickets, getStatusLabel, getStatusColor, type Ticket, type TicketStatus } from '@/api/ticket'
import { searchAuditLogs, getActionLabel, type AuditLog } from '@/api/audit'
import { useQueryStore } from '@/store/queryStore'

// --- Types ---

interface CommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface SearchState {
  queries: QueryHistoryItem[]
  tickets: Ticket[]
  auditLogs: AuditLog[]
  loading: boolean
  errors: string[]
}

const EMPTY_STATE: SearchState = {
  queries: [],
  tickets: [],
  auditLogs: [],
  loading: false,
  errors: [],
}

// --- Static Pages ---

const pages = [
  { to: '/query', label: '查询', icon: Database, group: '页面跳转' },
  { to: '/tickets', label: '变更工单', icon: FileText, group: '页面跳转' },
  { to: '/permissions', label: '权限管理', icon: ShieldCheck, group: '页面跳转' },
  { to: '/audit', label: '审计日志', icon: ScrollText, group: '页面跳转' },
  { to: '/settings/datasource', label: '数据源管理', icon: Server, group: '设置' },
  { to: '/settings/mask-rules', label: '脱敏规则', icon: EyeOff, group: '设置' },
  { to: '/settings/ai-config', label: 'AI 配置', icon: Bot, group: '设置' },
]

// --- Helpers ---

function formatRelativeTime(dateStr: string): string {
  const now = Date.now()
  const then = new Date(dateStr).getTime()
  const diffMs = now - then
  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return '刚刚'
  if (diffMin < 60) return `${diffMin}分钟前`
  const diffHour = Math.floor(diffMin / 60)
  if (diffHour < 24) return `${diffHour}小时前`
  const diffDay = Math.floor(diffHour / 24)
  if (diffDay < 30) return `${diffDay}天前`
  return new Date(dateStr).toLocaleDateString('zh-CN')
}

// --- Component ---

export default function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const navigate = useNavigate()
  const restoreHistoryAsTab = useQueryStore((s) => s.restoreHistoryAsTab)

  const [keyword, setKeyword] = useState('')
  const [search, setSearch] = useState<SearchState>(EMPTY_STATE)
  const [recentQueries, setRecentQueries] = useState<QueryHistoryItem[]>([])
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Track previous open state to detect dialog-open transitions
  const prevOpenRef = useRef(open)
  useEffect(() => {
    if (open && !prevOpenRef.current) {
      // Dialog just opened — reset form state.
      setKeyword('')
      setSearch(EMPTY_STATE)
      // Fetch recent 5 queries for empty-state recommendations
      searchQueryHistory('', 1, 5)
        .then((res) => setRecentQueries(res.data ?? []))
        .catch(() => { /* silently skip */ })
    }
    prevOpenRef.current = open
  }, [open])

  // Debounced search
  const performSearch = useCallback(async (kw: string) => {
    if (!kw.trim()) {
      setSearch(EMPTY_STATE)
      return
    }

    setSearch((prev) => ({ ...prev, loading: true, errors: [] }))

    const results: SearchState = {
      queries: [],
      tickets: [],
      auditLogs: [],
      loading: false,
      errors: [],
    }

    // Three concurrent API requests
    const promises = [
      searchQueryHistory(kw, 1, 5)
        .then((res) => {
          results.queries = res.data ?? []
        })
        .catch((err) => {
          results.errors.push(`查询历史: ${err instanceof Error ? err.message : '搜索失败'}`)
        }),

      listTickets({ keyword: kw, page: 1, page_size: 5 })
        .then((res) => {
          results.tickets = res.data ?? []
        })
        .catch((err) => {
          results.errors.push(`工单: ${err instanceof Error ? err.message : '搜索失败'}`)
        }),

      searchAuditLogs(kw, 5)
        .then((res) => {
          results.auditLogs = res.data ?? []
        })
        .catch((err) => {
          results.errors.push(`审计日志: ${err instanceof Error ? err.message : '搜索失败'}`)
        }),
    ]

    await Promise.allSettled(promises)
    setSearch(results)
  }, [])

  // Handle input value change with 300ms debounce
  const handleValueChange = useCallback(
    (value: string) => {
      setKeyword(value)
      if (debounceRef.current) {
        clearTimeout(debounceRef.current)
      }
      debounceRef.current = setTimeout(() => {
        performSearch(value)
      }, 300)
    },
    [performSearch],
  )

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current)
      }
    }
  }, [])

  // Keyboard shortcut
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        onOpenChange(!open)
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onOpenChange])

  // Execute a command and close dialog
  const runCommand = (command: () => void) => {
    onOpenChange(false)
    command()
  }

  // --- Navigation handlers ---

  function openQueryInNewTab(item: QueryHistoryItem) {
    runCommand(() => {
      navigate('/query')
      // Use setTimeout to ensure navigation completes before modifying store
      setTimeout(() => {
        restoreHistoryAsTab(item.sql_content, item.datasource_id, item.database)
      }, 50)
    })
  }

  function openTicketDetail(ticket: Ticket) {
    runCommand(() => {
      navigate(`/tickets?id=${ticket.id}`)
    })
  }

  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- parameter required by callback signature
  function openAuditHighlight(_log: AuditLog) {
    runCommand(() => {
      navigate(`/audit?highlight=${encodeURIComponent(keyword)}`)
    })
  }

  // --- Determine which mode to show ---

  const hasKeyword = keyword.trim().length > 0
  const hasResults =
    search.queries.length > 0 || search.tickets.length > 0 || search.auditLogs.length > 0

  const pageGroup = pages.filter((p) => p.group === '页面跳转')
  const settingsGroup = pages.filter((p) => p.group === '设置')

  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title="全局搜索"
      description="搜索页面、查询历史、工单或审计日志..."
    >
      <CommandInput
        placeholder="搜索页面、查询历史、工单或审计日志..."
        value={keyword}
        onValueChange={handleValueChange}
      />
      <CommandList>
        {/* No keyword: show page shortcuts */}
        {!hasKeyword && (
          <>
            <CommandEmpty>没有找到匹配项</CommandEmpty>
            <CommandGroup heading="页面">
              {pageGroup.map((page) => (
                <CommandItem
                  key={page.to}
                  onSelect={() => runCommand(() => navigate(page.to))}
                >
                  <page.icon size={16} />
                  <span>{page.label}</span>
                </CommandItem>
              ))}
            </CommandGroup>
            <CommandSeparator />
            {/* Recent query history recommendations */}
            {recentQueries.length > 0 && (
              <>
                <CommandGroup heading="最近查询">
                  {recentQueries.map((item) => (
                    <CommandItem
                      key={`rq-${item.id}`}
                      value={`recent-${item.id}-${item.sql_summary}`}
                      onSelect={() => openQueryInNewTab(item)}
                    >
                      <History size={16} className="shrink-0 text-muted-foreground" />
                      <span className="flex-1 truncate">
                        {item.sql_summary || item.sql_content.slice(0, 60)}
                      </span>
                      <span className="ml-2 shrink-0 text-[10px] text-muted-foreground">
                        {item.database}
                      </span>
                      <span className="ml-1.5 shrink-0 text-[10px] text-muted-foreground">
                        {formatRelativeTime(item.created_at)}
                      </span>
                    </CommandItem>
                  ))}
                </CommandGroup>
                <CommandSeparator />
              </>
            )}
            <CommandGroup heading="设置">
              {settingsGroup.map((page) => (
                <CommandItem
                  key={page.to}
                  onSelect={() => runCommand(() => navigate(page.to))}
                >
                  <page.icon size={16} />
                  <span>{page.label}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        )}

        {/* With keyword: show search results */}
        {hasKeyword && (
          <>
            {/* Loading state */}
            {search.loading && (
              <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
                <Loader2 size={16} className="animate-spin" />
                搜索中...
              </div>
            )}

            {/* Not loading — show results or empty */}
            {!search.loading && (
              <>
                {/* Errors (partial failures) */}
                {search.errors.length > 0 && search.errors.length < 3 && (
                  <div className="flex items-center gap-2 px-2 py-1.5 text-xs text-amber-500">
                    <AlertCircle size={12} />
                    <span>{search.errors.join('；')}</span>
                  </div>
                )}

                {/* All three APIs failed */}
                {search.errors.length >= 3 && (
                  <CommandEmpty>搜索失败，请稍后重试</CommandEmpty>
                )}

                {/* No results at all */}
                {search.errors.length < 3 && !hasResults && (
                  <CommandEmpty>未找到结果</CommandEmpty>
                )}

                {/* Query History Results */}
                {search.queries.length > 0 && (
                  <CommandGroup heading="查询历史">
                    {search.queries.map((item) => (
                      <CommandItem
                        key={`q-${item.id}`}
                        value={`query-${item.id}-${item.sql_summary}`}
                        onSelect={() => openQueryInNewTab(item)}
                      >
                        <Clock size={16} className="shrink-0 text-muted-foreground" />
                        <span className="flex-1 truncate">
                          <HighlightText
                            text={item.sql_summary || item.sql_content}
                            keyword={keyword}
                            maxLen={60}
                          />
                        </span>
                        <span className="ml-2 shrink-0 text-[10px] text-muted-foreground">
                          {item.database}
                        </span>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}

                {/* Ticket Results */}
                {search.tickets.length > 0 && (
                  <CommandGroup heading="工单">
                    {search.tickets.map((ticket) => (
                      <CommandItem
                        key={`t-${ticket.id}`}
                        value={`ticket-${ticket.id}-${ticket.sql_summary}`}
                        onSelect={() => openTicketDetail(ticket)}
                      >
                        <FileText size={16} className="shrink-0 text-muted-foreground" />
                        <span className="flex-1 truncate">
                          <HighlightText
                            text={ticket.sql_summary || ticket.sql_content}
                            keyword={keyword}
                            maxLen={60}
                          />
                        </span>
                        <span
                          className={`ml-2 shrink-0 rounded-full px-1.5 py-0.5 text-[10px] font-medium ${getStatusColor(ticket.status as TicketStatus)}`}
                        >
                          {getStatusLabel(ticket.status as TicketStatus)}
                        </span>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}

                {/* Audit Log Results */}
                {search.auditLogs.length > 0 && (
                  <CommandGroup heading="审计日志">
                    {search.auditLogs.map((log) => (
                      <CommandItem
                        key={`a-${log.id}`}
                        value={`audit-${log.id}-${log.sql_summary}`}
                        onSelect={() => openAuditHighlight(log)}
                      >
                        <ScrollText size={16} className="shrink-0 text-muted-foreground" />
                        <span className="flex-1 truncate">
                          <HighlightText
                            text={log.sql_summary || log.sql_content}
                            keyword={keyword}
                            maxLen={60}
                          />
                        </span>
                        <span className="ml-2 shrink-0 text-[10px] text-muted-foreground">
                          {getActionLabel(log.action)}
                        </span>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}
              </>
            )}
          </>
        )}
      </CommandList>
    </CommandDialog>
  )
}
