import { useState, useEffect, useCallback } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  Search,
  Download,
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  Loader2,
  Copy,
  Check,
  Link2,
  FileText,
  Filter,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { api } from "@/api/client";
import {
  listAuditLogs,
  getActionLabel,
  getActionBadgeStyle,
  formatAuditTime,
  formatExecutionTime,
  actionOptions,
  type AuditLog,
} from "@/api/audit";
import {
  listTickets,
  getStatusLabel,
  getStatusColor,
  type Ticket,
} from "@/api/ticket";

// --- Types ---

interface DataSourceOption {
  id: number;
  name: string;
  type: string;
}

interface UserOption {
  id: number;
  username: string;
}

// --- Export CSV ---

function exportToCsv(logs: AuditLog[]) {
  const headers = [
    "时间",
    "用户",
    "操作",
    "数据源ID",
    "数据库",
    "SQL内容",
    "返回行数",
    "影响行数",
    "耗时(ms)",
    "错误信息",
    "脱敏字段",
    "IP地址",
  ];
  const rows = logs.map((l) => [
    l.created_at,
    l.username,
    l.action,
    String(l.datasource_id),
    l.database,
    `"${(l.sql_content || "").replace(/"/g, '""')}"`,
    String(l.result_rows),
    String(l.affected_rows),
    String(l.execution_time_ms),
    l.error_message || "",
    l.desensitized_fields || "",
    l.ip_address || "",
  ]);
  const csv = [headers.join(","), ...rows.map((r) => r.join(","))].join("\n");
  const BOM = "\uFEFF";
  const blob = new Blob([BOM + csv], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `audit_logs_${new Date().toISOString().slice(0, 10)}.csv`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
  toast.success(`已导出 ${logs.length} 条审计日志`);
}

// --- Expandable Row ---

function ExpandedRow({
  log,
  datasourceName,
  datasourceType,
}: {
  log: AuditLog;
  datasourceName: string;
  datasourceType: string;
}) {
  const [copied, setCopied] = useState(false);
  const [linkedTicket, setLinkedTicket] = useState<Ticket | null>(null);
  const [ticketLoading, setTicketLoading] = useState(false);
  const navigate = useNavigate();

  // Fetch linked ticket by matching datasource_id + sql_content
  useEffect(() => {
    let cancelled = false;
    async function searchTicket() {
      if (!log.datasource_id || !log.sql_content?.trim()) return;
      setTicketLoading(true);
      try {
        const res = await listTickets({
          datasource_id: String(log.datasource_id),
          keyword: log.sql_content.trim().slice(0, 100),
          page_size: 5,
        });
        if (cancelled) return;
        const matched = (res.data ?? []).find(
          (t) =>
            t.datasource_id === log.datasource_id &&
            t.sql_content?.trim() === log.sql_content?.trim(),
        );
        if (matched) setLinkedTicket(matched);
      } catch {
        // Silently ignore ticket lookup failures
      } finally {
        if (!cancelled) setTicketLoading(false);
      }
    }
    searchTicket();
    return () => {
      cancelled = true;
    };
  }, [log.datasource_id, log.sql_content]);

  function handleCopy() {
    navigator.clipboard.writeText(log.sql_content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <tr className="audit-expanded-row border-b border-[var(--border-subtle)] bg-[var(--bg-elevated)]/30">
      <td colSpan={6} className="p-0">
        <div className="overflow-hidden">
          <div className="p-4">
            <div className="grid grid-cols-4 gap-3">
              {/* Full SQL */}
              <div className="col-span-full">
                <div className="mb-1 flex items-center justify-between">
                  <span className="text-xs font-medium text-[var(--text-secondary)]">
                    完整 SQL
                  </span>
                  <button
                    onClick={handleCopy}
                    className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] text-[var(--text-tertiary)] transition-colors hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]"
                  >
                    {copied ? <Check size={12} /> : <Copy size={12} />}
                    {copied ? "已复制" : "复制"}
                  </button>
                </div>
                <pre className="max-h-[200px] overflow-auto rounded-md border border-[var(--border-default)] bg-[var(--bg-base)] p-3 font-mono text-xs leading-relaxed text-[var(--text-primary)]">
                  {log.sql_content}
                </pre>
              </div>

              {/* Execution time */}
              <div>
                <span className="text-xs text-[var(--text-muted)]">
                  执行耗时
                </span>
                <p className="mt-0.5 text-sm font-medium text-[var(--text-primary)]">
                  {log.execution_time_ms > 0
                    ? formatExecutionTime(log.execution_time_ms)
                    : "—"}
                </p>
              </div>

              {/* Affected rows */}
              <div>
                <span className="text-xs text-[var(--text-muted)]">
                  影响行数
                </span>
                <p className="mt-0.5 text-sm font-medium text-[var(--text-primary)]">
                  {log.affected_rows >= 0
                    ? log.affected_rows.toLocaleString()
                    : "—"}
                </p>
              </div>

              {/* Result rows */}
              <div>
                <span className="text-xs text-[var(--text-muted)]">
                  返回行数
                </span>
                <p className="mt-0.5 text-sm font-medium text-[var(--text-primary)]">
                  {log.result_rows >= 0
                    ? log.result_rows.toLocaleString()
                    : "—"}
                </p>
              </div>

              {/* IP address */}
              <div>
                <span className="text-xs text-[var(--text-muted)]">
                  IP 地址
                </span>
                <p className="mt-0.5 text-sm text-[var(--text-primary)]">
                  {log.ip_address || "—"}
                </p>
              </div>

              {/* Database type */}
              <div>
                <span className="text-xs text-[var(--text-muted)]">
                  数据库类型
                </span>
                <p className="mt-0.5 text-sm text-[var(--text-primary)]">
                  {datasourceType ? (
                    <Badge className="border-0 bg-sky-500/15 text-[10px] text-sky-400">
                      {datasourceType}
                    </Badge>
                  ) : (
                    "—"
                  )}
                </p>
              </div>

              {/* Datasource name */}
              <div>
                <span className="text-xs text-[var(--text-muted)]">
                  数据源名称
                </span>
                <p className="mt-0.5 text-sm text-[var(--text-primary)]">
                  {datasourceName || `#${log.datasource_id}`}
                </p>
              </div>

              {/* Operator */}
              <div>
                <span className="text-xs text-[var(--text-muted)]">操作人</span>
                <p className="mt-0.5 text-sm text-[var(--text-primary)]">
                  {log.username || `#${log.user_id}`}
                </p>
              </div>

              {/* Timestamp */}
              <div>
                <span className="text-xs text-[var(--text-muted)]">时间戳</span>
                <p className="mt-0.5 text-sm text-[var(--text-primary)]">
                  {new Date(log.created_at).toLocaleString("zh-CN", {
                    year: "numeric",
                    month: "2-digit",
                    day: "2-digit",
                    hour: "2-digit",
                    minute: "2-digit",
                    second: "2-digit",
                  })}
                </p>
              </div>

              {/* Linked ticket */}
              <div className="col-span-full">
                <span className="text-xs text-[var(--text-muted)]">
                  关联工单
                </span>
                <div className="mt-1">
                  {ticketLoading ? (
                    <span className="flex items-center gap-1 text-xs text-[var(--text-muted)]">
                      <Loader2 size={12} className="animate-spin" />
                      查找中…
                    </span>
                  ) : linkedTicket ? (
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate(`/tickets?id=${linkedTicket.id}`);
                      }}
                      className="inline-flex items-center gap-1.5 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-2.5 py-1 text-xs transition-colors hover:bg-[var(--bg-surface)]"
                    >
                      <Link2 size={12} className="text-[var(--text-muted)]" />
                      <span className="font-medium text-[var(--text-primary)]">
                        #{linkedTicket.id}
                      </span>
                      <span className="text-[var(--text-secondary)]">
                        {linkedTicket.sql_summary ||
                          linkedTicket.sql_content.slice(0, 40)}
                      </span>
                      <Badge
                        className={`${getStatusColor(linkedTicket.status)} ml-1 border-0 text-[10px]`}
                      >
                        {getStatusLabel(linkedTicket.status)}
                      </Badge>
                    </button>
                  ) : (
                    <span className="text-xs text-[var(--text-muted)]">
                      无关联工单
                    </span>
                  )}
                </div>
              </div>

              {/* Desensitization info */}
              {log.desensitized_fields && (
                <div className="col-span-full">
                  <span className="text-xs text-[var(--text-muted)]">
                    脱敏字段
                  </span>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {log.desensitized_fields.split(",").map((f) => (
                      <Badge
                        key={f}
                        className="border-0 bg-amber-500/15 text-[10px] text-amber-400"
                      >
                        {f.trim()}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}

              {/* Error message */}
              {log.error_message && (
                <div className="col-span-full">
                  <span className="text-xs text-[var(--text-muted)]">
                    错误信息
                  </span>
                  <p className="mt-0.5 text-sm text-red-400">
                    {log.error_message}
                  </p>
                </div>
              )}
            </div>
          </div>
        </div>
      </td>
    </tr>
  );
}

// --- Main Page ---

export default function AuditPage() {
  const [searchParams] = useSearchParams();

  // Data
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(50);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set());

  // Filters
  const [userFilter, setUserFilter] = useState<string>("");
  const [actionFilter, setActionFilter] = useState<string>("");
  const [datasourceFilter, setDatasourceFilter] = useState<string>("");
  const [startDate, setStartDate] = useState<string>("");
  const [endDate, setEndDate] = useState<string>("");
  const [keyword, setKeyword] = useState(
    () => searchParams.get("highlight") ?? "",
  );
  const [searchInput, setSearchInput] = useState(
    () => searchParams.get("highlight") ?? "",
  );

  // Sync keyword from URL `highlight` param when navigating from global search
  useEffect(() => {
    const hl = searchParams.get("highlight");
    if (hl) {
      queueMicrotask(() => {
        setKeyword(hl);
        setSearchInput(hl);
      });
    }
  }, [searchParams]);

  // Options
  const [datasources, setDatasources] = useState<DataSourceOption[]>([]);
  const [users, setUsers] = useState<UserOption[]>([]);

  // Export state
  const [exporting, setExporting] = useState(false);

  // Load filter options
  useEffect(() => {
    api
      .get<{ code: number; data: DataSourceOption[] }>("/datasources")
      .then((res) => {
        setDatasources(res.data ?? []);
      })
      .catch(() => {});
    api
      .get<{ code: number; data: UserOption[] }>("/users")
      .then((res) => {
        setUsers(res.data ?? []);
      })
      .catch(() => {});
  }, []);

  // Fetch audit logs
  const fetchLogs = useCallback(async () => {
    setLoading(true);
    setError(null);
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
      });
      setLogs(res.data ?? []);
      setTotal(res.total);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "获取审计日志失败";
      setError(msg);
      toast.error(msg);
    } finally {
      setLoading(false);
    }
  }, [
    page,
    pageSize,
    userFilter,
    actionFilter,
    datasourceFilter,
    startDate,
    endDate,
    keyword,
  ]);

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);
  /* eslint-enable react-hooks/set-state-in-effect */

  // Reset all filters
  function handleResetFilters() {
    setUserFilter("");
    setActionFilter("");
    setDatasourceFilter("");
    setStartDate("");
    setEndDate("");
    setKeyword("");
    setSearchInput("");
    setPage(1);
  }

  const hasActiveFilters =
    userFilter ||
    actionFilter ||
    datasourceFilter ||
    startDate ||
    endDate ||
    keyword;

  // Search handler
  function handleSearch() {
    setKeyword(searchInput.trim());
    setPage(1);
  }

  function handleSearchKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter") handleSearch();
  }

  // Toggle row expansion
  function toggleExpand(id: number) {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  // Export handler — fetch all matching logs (up to 10000) for export
  async function handleExport() {
    setExporting(true);
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
      });
      const data = res.data ?? [];
      if (data.length === 0) {
        toast.info("没有可导出的数据");
        return;
      }
      exportToCsv(data);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "导出失败");
    } finally {
      setExporting(false);
    }
  }

  const totalPages = Math.ceil(total / pageSize);

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-3">
        <div className="flex items-center gap-2.5">
          <FileText size={18} className="text-[var(--accent-primary)]" />
          <h1 className="text-base font-semibold text-[var(--text-primary)]">
            审计日志
          </h1>
          {total > 0 && (
            <span className="rounded-full bg-[var(--bg-elevated)] px-2 py-0.5 text-xs text-[var(--text-muted)]">
              {total} 条
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {hasActiveFilters && (
            <Button
              size="sm"
              variant="ghost"
              className="h-8 gap-1.5 px-3 text-xs text-[var(--text-muted)] hover:text-[var(--text-primary)]"
              onClick={handleResetFilters}
            >
              <Filter size={13} />
              清除筛选
            </Button>
          )}
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
            {exporting ? "导出中..." : "导出 CSV"}
          </Button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-2.5">
        {/* User filter */}
        <Select
          value={userFilter}
          onValueChange={(v) => {
            setUserFilter(v === "__all__" ? "" : v);
            setPage(1);
          }}
        >
          <SelectTrigger className="h-7 w-[120px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
            <SelectValue placeholder="全部用户" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">全部用户</SelectItem>
            {users.map((u) => (
              <SelectItem key={u.id} value={String(u.id)}>
                {u.username}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Action filter */}
        <Select
          value={actionFilter}
          onValueChange={(v) => {
            setActionFilter(v === "__all__" ? "" : v);
            setPage(1);
          }}
        >
          <SelectTrigger className="h-7 w-[120px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
            <SelectValue placeholder="操作类型" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">全部类型</SelectItem>
            {actionOptions.map((a) => (
              <SelectItem key={a} value={a}>
                {getActionLabel(a)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Datasource filter */}
        <Select
          value={datasourceFilter}
          onValueChange={(v) => {
            setDatasourceFilter(v === "__all__" ? "" : v);
            setPage(1);
          }}
        >
          <SelectTrigger className="h-7 w-[132px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
            <SelectValue placeholder="数据源" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">全部数据源</SelectItem>
            {datasources.map((ds) => (
              <SelectItem key={ds.id} value={String(ds.id)}>
                {ds.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Separator */}
        <div className="mx-1 h-5 w-px bg-[var(--border-subtle)]" />

        {/* Date range */}
        <div className="flex items-center gap-1.5">
          <Input
            type="date"
            value={startDate}
            onChange={(e) => {
              setStartDate(e.target.value);
              setPage(1);
            }}
            className="h-7 w-[130px] border-[var(--border-default)] bg-[var(--bg-elevated)] px-2 text-xs text-[var(--text-primary)]"
          />
          <span className="text-xs text-[var(--text-muted)]">~</span>
          <Input
            type="date"
            value={endDate}
            onChange={(e) => {
              setEndDate(e.target.value);
              setPage(1);
            }}
            className="h-7 w-[130px] border-[var(--border-default)] bg-[var(--bg-elevated)] px-2 text-xs text-[var(--text-primary)]"
          />
        </div>

        {/* Search — 7 field full-text */}
        <div className="relative ml-auto">
          <Search
            size={14}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]"
          />
          <Input
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onKeyDown={handleSearchKeyDown}
            placeholder="搜索 SQL/表名/用户/IP/数据库/错误/脱敏..."
            className="h-7 w-[260px] rounded-md border-[var(--border-default)] bg-[var(--bg-elevated)] pl-8 pr-8 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)] transition-[width] duration-200 focus:w-[320px]"
          />
          {searchInput && (
            <button
              onClick={() => {
                setSearchInput("");
                setKeyword("");
              }}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            >
              ×
            </button>
          )}
        </div>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto bg-[var(--bg-base)] table-responsive">
        {error && !logs.length ? (
          <div className="flex h-48 flex-col items-center justify-center gap-3 py-12 page-transition">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-red-500/10">
              <FileText size={24} className="text-red-400" />
            </div>
            <div className="text-center">
              <p className="text-sm font-medium text-[var(--text-secondary)]">
                加载失败
              </p>
              <p className="mt-1 text-xs text-[var(--text-muted)]">{error}</p>
            </div>
            <Button
              size="sm"
              variant="outline"
              className="h-7 gap-1.5 border-[var(--border-default)] text-xs"
              onClick={() => fetchLogs()}
            >
              重试
            </Button>
          </div>
        ) : loading && !logs.length ? (
          <div className="space-y-0 p-0">
            {[1, 2, 3, 4, 5].map((i) => (
              <div
                key={i}
                className="flex items-center gap-4 border-b border-[var(--border-subtle)] px-6 py-3"
              >
                <div className="h-3 w-3 rounded bg-[var(--bg-elevated)] animate-pulse" />
                <div className="h-3 w-20 rounded bg-[var(--bg-elevated)] animate-pulse" />
                <div className="h-3 w-16 rounded bg-[var(--bg-elevated)] animate-pulse" />
                <div className="h-5 w-14 rounded-full bg-[var(--bg-elevated)] animate-pulse" />
                <div className="h-3 w-24 rounded bg-[var(--bg-elevated)] animate-pulse" />
                <div className="h-3 flex-1 rounded bg-[var(--bg-elevated)] animate-pulse" />
              </div>
            ))}
          </div>
        ) : logs.length === 0 ? (
          <div className="flex h-48 flex-col items-center justify-center gap-3 py-12 page-transition">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-[var(--bg-elevated)] empty-state-icon">
              <FileText size={24} className="text-[var(--text-muted)]" />
            </div>
            <div className="text-center">
              <p className="text-sm font-medium text-[var(--text-secondary)]">
                {hasActiveFilters ? "没有匹配的审计日志" : "暂无审计日志"}
              </p>
              <p className="mt-1 text-xs text-[var(--text-muted)]">
                {hasActiveFilters
                  ? "尝试调整筛选条件或清空搜索关键词"
                  : "执行 SQL 查询后，审计记录将在此展示"}
              </p>
            </div>
            {hasActiveFilters && (
              <Button
                size="sm"
                variant="outline"
                className="h-7 gap-1.5 border-[var(--border-default)] text-xs"
                onClick={handleResetFilters}
              >
                清除所有筛选
              </Button>
            )}
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="border-[var(--border-default)] bg-[var(--bg-surface)] hover:bg-[var(--bg-surface)]">
                <TableHead className="w-8" />
                <TableHead className="w-[140px] text-xs text-[var(--text-secondary)]">
                  时间
                </TableHead>
                <TableHead className="w-24 text-xs text-[var(--text-secondary)]">
                  用户
                </TableHead>
                <TableHead className="w-24 text-xs text-[var(--text-secondary)]">
                  操作
                </TableHead>
                <TableHead className="w-28 text-xs text-[var(--text-secondary)]">
                  数据库
                </TableHead>
                <TableHead className="text-xs text-[var(--text-secondary)]">
                  SQL 摘要
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((log) => {
                const isExpanded = expandedIds.has(log.id);
                const ds = datasources.find((d) => d.id === log.datasource_id);
                return (
                  <>
                    <TableRow
                      key={log.id}
                      className="cursor-pointer border-[var(--border-subtle)] hover:bg-[var(--bg-elevated)]"
                      onClick={() => toggleExpand(log.id)}
                    >
                      <TableCell className="w-8 px-2">
                        {isExpanded ? (
                          <ChevronDown
                            size={14}
                            className="text-[var(--text-muted)] transition-transform duration-200"
                          />
                        ) : (
                          <ChevronRight
                            size={14}
                            className="text-[var(--text-muted)] transition-transform duration-200"
                          />
                        )}
                      </TableCell>
                      <TableCell className="text-xs text-[var(--text-muted)]">
                        {formatAuditTime(log.created_at)}
                      </TableCell>
                      <TableCell className="text-xs text-[var(--text-primary)]">
                        {log.username || `#${log.user_id}`}
                      </TableCell>
                      <TableCell>
                        <Badge
                          className={`${getActionBadgeStyle(log.action)} border-0 text-[10px]`}
                        >
                          {getActionLabel(log.action)}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-[var(--text-secondary)]">
                        {log.database || "—"}
                      </TableCell>
                      <TableCell className="max-w-[400px] text-xs text-[var(--text-primary)]">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className="block truncate">
                              {log.action === "EXPORT"
                                ? `导出 ${log.result_rows.toLocaleString()} 行`
                                : log.sql_summary || log.sql_content}
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
                    {isExpanded && (
                      <ExpandedRow
                        key={`expanded-${log.id}`}
                        log={log}
                        datasourceName={ds?.name ?? ""}
                        datasourceType={ds?.type ?? ""}
                      />
                    )}
                  </>
                );
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
              className="h-7 w-7 p-0 text-xs text-[var(--text-secondary)]"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
            >
              <ChevronLeft size={14} />
            </Button>
            <span className="min-w-[60px] text-center text-xs text-[var(--text-secondary)]">
              {page} / {totalPages}
            </span>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 w-7 p-0 text-xs text-[var(--text-secondary)]"
              disabled={page >= totalPages}
              onClick={() => setPage(page + 1)}
            >
              <ChevronRight size={14} />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
