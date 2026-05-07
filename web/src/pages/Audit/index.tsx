import { useState, useEffect, useCallback } from 'react'
import {
  Search, Download, ChevronRight, ChevronDown, Loader2, Copy, Check,
} from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { api } from '@/api/client'
import {
  listAuditLogs,
  getActionLabel, getActionColor, formatAuditTime, formatExecutionTime,
  actionOptions,
  type AuditLog,
} from '@/api/audit'

// --- Types ---

interface DataSourceOption {
  id: number
  name: string
}

interface UserOption {
  id: number
  username: string
}

// --- Export CSV ---

function exportToCsv(logs: AuditLog[]) {
  const headers = ['时间', '用户', '操作', '数据源ID', '数据库', 'SQL内容', '返回行数', '影响行数', '耗时(ms)', '错误信息', '脱敏字段', 'IP地址']
  const rows = logs.map((l) => [
    l.created_at,
    l.username,
    l.action,
    String(l.datasource_id),
    l.database,
    `"${(l.sql_content || '').replace(/"/g, '""')}"`,
    String(l.result_rows),
    String(l.affected_rows),
    String(l.execution_time_ms),
    l.error_message || '',
    l.desensitized_fields || '',
    l.ip_address || '',
  ])
  const csv = [headers.join(','), ...rows.map((r) => r.join(','))].join('\n')
  const BOM = '\uFEFF'
  const blob = new Blob([BOM + csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `audit_logs_${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  toast.success(`已导出 ${logs.length} 条审计日志`)
}

// --- Expandable Row ---

function ExpandedRow({ log }: { log: AuditLog }) {
  const [copied, setCopied] = useState(false)

  function handleCopy() {
    navigator.clipboard.writeText(log.sql_content)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <tr className="border-b border-[var(--border-subtle)] bg-[var(--bg-elevated)]/30">
      <td colSpan={6} className="p-4">
        <div className="grid grid-cols-2 gap-x-8 gap-y-3 lg:grid-cols-4">
          {/* Full SQL */}
          <div className="col-span-full">
            <div className="mb-1 flex items-center justify-between">
              <span className="text-xs font-medium text-[var(--text-secondary)]">完整 SQL</span>
              <button
                onClick={handleCopy}
                className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] text-[var(--text-tertiary)] transition-colors hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]"
              >
                {copied ? <Check size={12} /> : <Copy size={12} />}
                {copied ? '已复制' : '复制'}
              </button>
            </div>
            <pre className="max-h-[200px] overflow-auto rounded-md border border-[var(--border-default)] bg-[var(--bg-base)] p-3 font-mono text-xs leading-relaxed text-[var(--text-primary)]">
              {log.sql_content}
            </pre>
          </div>

          {/* Execution time */}
          <div>
            <span className="text-xs text-[var(--text-muted)]">执行耗时</span>
            <p className="mt-0.5 text-sm text-[var(--text-primary)]">
              {log.execution_time_ms > 0 ? formatExecutionTime(log.execution_time_ms) : '—'}
            </p>
          </div>

          {/* Result rows */}
          <div>
            <span className="text-xs text-[var(--text-muted)]">返回行数</span>
            <p className="mt-0.5 text-sm text-[var(--text-primary)]">
              {log.result_rows >= 0 ? log.result_rows.toLocaleString() : '—'}
            </p>
          </div>

          {/* Affected rows */}
          <div>
            <span className="text-xs text-[var(--text-muted)]">影响行数</span>
            <p className="mt-0.5 text-sm text-[var(--text-primary)]">
              {log.affected_rows >= 0 ? log.affected_rows.toLocaleString() : '—'}
            </p>
          </div>

          {/* IP address */}
          <div>
            <span className="text-xs text-[var(--text-muted)]">IP 地址</span>
            <p className="mt-0.5 text-sm text-[var(--text-primary)]">
              {log.ip_address || '—'}
            </p>
          </div>

          {/* Desensitization info */}
          {log.desensitized_fields && (
            <div className="col-span-full">
              <span className="text-xs text-[var(--text-muted)]">脱敏字段</span>
              <div className="mt-1 flex flex-wrap gap-1">
                {log.desensitized_fields.split(',').map((f) => (
                  <Badge key={f} className="border-0 bg-amber-500/15 text-[10px] text-amber-400">
                    {f.trim()}
                  </Badge>
                ))}
              </div>
            </div>
          )}

          {/* Error message */}
          {log.error_message && (
            <div className="col-span-full">
              <span className="text-xs text-[var(--text-muted)]">错误信息</span>
              <p className="mt-0.5 text-sm text-red-400">{log.error_message}</p>
            </div>
          )}
        </div>
      </td>
    </tr>
  )
}

// --- Main Page ---

export default function AuditPage() {
  // Data
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(50)
  const [loading, setLoading] = useState(false)
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set())

  // Filters
  const [userFilter, setUserFilter] = useState<string>('')
  const [actionFilter, setActionFilter] = useState<string>('')
  const [datasourceFilter, setDatasourceFilter] = useState<string>('')
  const [startDate, setStartDate] = useState<string>('')
  const [endDate, setEndDate] = useState<string>('')
  const [keyword, setKeyword] = useState('')
  const [searchInput, setSearchInput] = useState('')

  // Options
  const [datasources, setDatasources] = useState<DataSourceOption[]>([])
  const [users, setUsers] = useState<UserOption[]>([])

  // Export state
  const [exporting, setExporting] = useState(false)

  // Load filter options
  useEffect(() => {
    api.get<{ code: number; data: DataSourceOption[] }>('/datasources').then((res) => {
      setDatasources(res.data ?? [])
    }).catch(() => {})
    api.get<{ code: number; data: UserOption[] }>('/users').then((res) => {
      setUsers(res.data ?? [])
    }).catch(() => {})
  }, [])

  // Fetch audit logs
  const fetchLogs = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listAuditLogs({
        page,
        page_size: pageSize,
        user_id: userFilter || undefined,
        action: actionFilter || undefined,
        datasource_id: datasourceFilter || undefined,
        start: startDate || undefined,
        end: endDate || undefined,
        keyword: keyword || undefined,
      })
      setLogs(res.data ?? [])
      setTotal(res.total)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '获取审计日志失败')
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, userFilter, actionFilter, datasourceFilter, startDate, endDate, keyword])

  useEffect(() => {
    fetchLogs()
  }, [fetchLogs])

  // Search handler
  function handleSearch() {
    setKeyword(searchInput.trim())
    setPage(1)
  }

  function handleSearchKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter') handleSearch()
  }

  // Toggle row expansion
  function toggleExpand(id: number) {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  // Export handler — fetch all matching logs (up to 10000) for export
  async function handleExport() {
    setExporting(true)
    try {
      const res = await listAuditLogs({
        page: 1,
        page_size: 10000,
        user_id: userFilter || undefined,
        action: actionFilter || undefined,
        datasource_id: datasourceFilter || undefined,
        start: startDate || undefined,
        end: endDate || undefined,
        keyword: keyword || undefined,
      })
      const data = res.data ?? []
      if (data.length === 0) {
        toast.info('没有可导出的数据')
        return
      }
      exportToCsv(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '导出失败')
    } finally {
      setExporting(false)
    }
  }

  const totalPages = Math.ceil(total / pageSize)

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-3">
        <h1 className="text-base font-semibold text-[var(--text-primary)]">审计日志</h1>
        <Button
          size="sm"
          variant="outline"
          className="h-8 gap-1.5 border-[var(--border-default)] px-3 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)]"
          onClick={handleExport}
          disabled={exporting || loading}
        >
          {exporting ? (
            <Loader2 size={14} className="animate-spin" />
          ) : (
            <Download size={14} />
          )}
          {exporting ? '导出中...' : '导出 CSV'}
        </Button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-2.5">
        {/* User filter */}
        <Select value={userFilter} onValueChange={(v) => { setUserFilter(v === '__all__' ? '' : v); setPage(1) }}>
          <SelectTrigger className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
            <SelectValue placeholder="全部用户" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">全部用户</SelectItem>
            {users.map((u) => (
              <SelectItem key={u.id} value={String(u.id)}>{u.username}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Action filter */}
        <Select value={actionFilter} onValueChange={(v) => { setActionFilter(v === '__all__' ? '' : v); setPage(1) }}>
          <SelectTrigger className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
            <SelectValue placeholder="操作类型" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">全部类型</SelectItem>
            {actionOptions.map((a) => (
              <SelectItem key={a} value={a}>{getActionLabel(a)}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Datasource filter */}
        <Select value={datasourceFilter} onValueChange={(v) => { setDatasourceFilter(v === '__all__' ? '' : v); setPage(1) }}>
          <SelectTrigger className="h-7 w-32 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
            <SelectValue placeholder="数据源" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">全部数据源</SelectItem>
            {datasources.map((ds) => (
              <SelectItem key={ds.id} value={String(ds.id)}>{ds.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Date range */}
        <div className="flex items-center gap-1">
          <Input
            type="date"
            value={startDate}
            onChange={(e) => { setStartDate(e.target.value); setPage(1) }}
            className="h-7 w-[124px] border-[var(--border-default)] bg-[var(--bg-elevated)] px-2 text-xs text-[var(--text-primary)]"
          />
          <span className="text-xs text-[var(--text-muted)]">~</span>
          <Input
            type="date"
            value={endDate}
            onChange={(e) => { setEndDate(e.target.value); setPage(1) }}
            className="h-7 w-[124px] border-[var(--border-default)] bg-[var(--bg-elevated)] px-2 text-xs text-[var(--text-primary)]"
          />
        </div>

        {/* Search */}
        <div className="relative ml-auto">
          <Search size={14} className="absolute left-2 top-1/2 -translate-y-1/2 text-[var(--text-muted)]" />
          <Input
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onKeyDown={handleSearchKeyDown}
            placeholder="搜索 SQL / 表名..."
            className="h-7 w-48 rounded-md border-[var(--border-default)] bg-[var(--bg-elevated)] pl-7 pr-2 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
          />
        </div>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto bg-[var(--bg-base)]">
        {loading ? (
          <div className="flex h-32 items-center justify-center">
            <Loader2 className="h-5 w-5 animate-spin text-[var(--text-muted)]" />
          </div>
        ) : logs.length === 0 ? (
          <div className="flex h-32 flex-col items-center justify-center gap-2">
            <p className="text-sm text-[var(--text-muted)]">暂无审计日志</p>
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="border-[var(--border-default)] bg-[var(--bg-surface)] hover:bg-[var(--bg-surface)]">
                <TableHead className="w-8" />
                <TableHead className="w-[140px] text-xs text-[var(--text-secondary)]">时间</TableHead>
                <TableHead className="w-24 text-xs text-[var(--text-secondary)]">用户</TableHead>
                <TableHead className="w-24 text-xs text-[var(--text-secondary)]">操作</TableHead>
                <TableHead className="w-28 text-xs text-[var(--text-secondary)]">数据库</TableHead>
                <TableHead className="text-xs text-[var(--text-secondary)]">SQL 摘要</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((log) => {
                const isExpanded = expandedIds.has(log.id)
                return (
                  <>
                    <TableRow
                      key={log.id}
                      className="cursor-pointer border-[var(--border-subtle)] hover:bg-[var(--bg-elevated)]"
                      onClick={() => toggleExpand(log.id)}
                    >
                      <TableCell className="w-8 px-2">
                        {isExpanded ? (
                          <ChevronDown size={14} className="text-[var(--text-muted)]" />
                        ) : (
                          <ChevronRight size={14} className="text-[var(--text-muted)]" />
                        )}
                      </TableCell>
                      <TableCell className="text-xs text-[var(--text-muted)]">
                        {formatAuditTime(log.created_at)}
                      </TableCell>
                      <TableCell className="text-xs text-[var(--text-primary)]">
                        {log.username || `#${log.user_id}`}
                      </TableCell>
                      <TableCell>
                        <Badge className={`${getActionColor(log.action)} border-0 text-[10px]`}>
                          {getActionLabel(log.action)}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-[var(--text-secondary)]">
                        {log.database || '—'}
                      </TableCell>
                      <TableCell className="max-w-[400px] text-xs text-[var(--text-primary)]">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className="block truncate">
                              {log.action === 'EXPORT'
                                ? `导出 ${log.result_rows.toLocaleString()} 行`
                                : (log.sql_summary || log.sql_content)}
                            </span>
                          </TooltipTrigger>
                          <TooltipContent
                            side="bottom"
                            align="start"
                            className="max-w-[400px] whitespace-pre-wrap break-all text-xs"
                          >
                            {log.sql_content}
                          </TooltipContent>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                    {isExpanded && <ExpandedRow key={`expanded-${log.id}`} log={log} />}
                  </>
                )
              })}
            </TableBody>
          </Table>
        )}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between border-t border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-2">
          <span className="text-xs text-[var(--text-muted)]">
            共 {total} 条，第 {page}/{totalPages} 页
          </span>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              className="h-7 w-7 p-0 text-xs"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
            >
              &lt;
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 w-7 p-0 text-xs"
              disabled={page >= totalPages}
              onClick={() => setPage(page + 1)}
            >
              &gt;
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
