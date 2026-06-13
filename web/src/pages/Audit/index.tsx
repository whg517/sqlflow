import { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  Search,
  Download,
  BarChart3,
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  Loader2,
  Copy,
  Check,
  Link2,
  FileText,
  Filter,
  ShieldCheck,
  Shield,
  ShieldAlert,
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
  exportAuditLogs,
  createAsyncAuditExport,
  getExportTask,
  downloadExportFile,
} from "@/api/export";
import { ExportDialog } from "@/components/ExportDialog";
import type { ExportColumn, ExportFormat } from "@/lib/export-utils";
import { cn } from "@/lib/utils";
import UserAnalyticsTab from "./UserAnalyticsTab";
import {
  getStatusLabel,
  getStatusColor,
  getTicket,
  type Ticket,
} from "@/api/ticket";
import {
  listGitLinks,
  shortenHash,
  type GitLink,
} from "@/api/git";
import { GitBranch, GitPullRequest, ExternalLink } from "lucide-react";

interface DataSourceOption {
  id: number;
  name: string;
  type: string;
}

interface UserOption {
  id: number;
  username: string;
}

// --- Export CSV (server-side with watermark) ---

// downloadBlob and formatFileSize extracted to @/lib/export-utils

/** Audit log columns available for export — must match backend whitelist. */
const AUDIT_EXPORT_COLUMNS: ExportColumn[] = [
  { key: "id", label: "ID" },
  { key: "username", label: "用户" },
  { key: "action", label: "操作" },
  { key: "datasource_id", label: "数据源 ID" },
  { key: "database", label: "数据库" },
  { key: "sql_content", label: "SQL 内容" },
  { key: "sql_summary", label: "SQL 摘要" },
  { key: "result_rows", label: "结果行数" },
  { key: "affected_rows", label: "影响行数" },
  { key: "execution_time_ms", label: "执行时间(ms)" },
  { key: "error_message", label: "错误信息" },
  { key: "ip_address", label: "IP 地址" },
  { key: "ticket_id", label: "关联工单" },
  { key: "created_at", label: "时间" },
];

// --- AI Review Block ---

interface ParsedAIReview {
  risk_level: "low" | "medium" | "high";
  summary: string;
  suggestions: string[];
}

const riskConfig: Record<
  string,
  {
    label: string;
    icon: typeof ShieldCheck;
    badgeClass: string;
  }
> = {
  low: {
    label: "低风险",
    icon: ShieldCheck,
    badgeClass: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
  },
  medium: {
    label: "中风险",
    icon: Shield,
    badgeClass: "bg-amber-500/20 text-amber-400 border-amber-500/30",
  },
  high: {
    label: "高风险",
    icon: ShieldAlert,
    badgeClass: "bg-red-500/20 text-red-400 border-red-500/30",
  },
};

function parseAIReviewResult(raw: string): ParsedAIReview | null {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    if (!parsed.risk_level && !parsed.summary && !parsed.suggestions) return null;
    return {
      risk_level: parsed.risk_level || "medium",
      summary: parsed.summary || "",
      suggestions: Array.isArray(parsed.suggestions)
        ? parsed.suggestions.filter(Boolean)
        : [],
    };
  } catch {
    return null;
  }
}

function AiReviewBlock({ review }: { review: ParsedAIReview }) {
  const risk = riskConfig[review.risk_level] || riskConfig.medium;
  const RiskIcon = risk.icon;

  return (
    <div className="col-span-full rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-elevated)]/50 p-4">
      <div className="mb-2 flex items-center gap-2">
        <RiskIcon size={14} className={risk.badgeClass.split(" ")[1]} />
        <span className="text-xs font-medium text-[var(--text-secondary)]">
          AI 评审
        </span>
        <Badge className={`${risk.badgeClass} border text-[10px]`}>
          {risk.label}
        </Badge>
      </div>
      {review.summary && (
        <p className="mb-2 text-xs leading-relaxed text-[var(--text-primary)]">
          {review.summary}
        </p>
      )}
      {review.suggestions.length > 0 && (
        <ul className="space-y-1">
          {review.suggestions.map((s, i) => (
            <li
              key={i}
              className="flex items-start gap-1.5 text-xs text-[var(--text-secondary)]"
            >
              <span className="mt-1 h-1 w-1 shrink-0 rounded-full bg-[var(--text-muted)]" />
              {s}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// --- Ticket Block ---

function TicketBlock({ ticketId }: { ticketId: number }) {
  const [ticket, setTicket] = useState<Ticket | null>(null);
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;
    async function fetchTicket() {
      if (!ticketId) return;
      setLoading(true);
      try {
        const res = await getTicket(ticketId);
        if (!cancelled && res.data) {
          setTicket(res.data);
        }
      } catch (err) {
        console.error("Failed to fetch ticket:", err);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    fetchTicket();
    return () => {
      cancelled = true;
    };
  }, [ticketId]);

  if (loading) {
    return (
      <span className="flex items-center gap-1 text-xs text-[var(--text-muted)]">
        <Loader2 size={12} className="animate-spin" />
        查找关联工单…
      </span>
    );
  }

  if (!ticket) return null;

  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        navigate(`/tickets?id=${ticket.id}`);
      }}
      className="inline-flex items-center gap-1.5 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-2.5 py-1 text-xs transition-colors hover:bg-[var(--bg-surface)]"
    >
      <Link2 size={12} className="text-[var(--text-muted)]" />
      <span className="font-medium text-[var(--text-primary)]">
        #{ticket.id}
      </span>
      <span className="text-[var(--text-secondary)]">
        {ticket.sql_summary || ticket.sql_content.slice(0, 40)}
      </span>
      <Badge
        className={`${getStatusColor(ticket.status)} ml-1 border-0 text-[10px]`}
      >
        {getStatusLabel(ticket.status)}
      </Badge>
    </button>
  );
}

// --- Git Links for Audit Log ---

function GitLinksForAudit({ auditLogId }: { auditLogId: number }) {
  const [links, setLinks] = useState<GitLink[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function fetchLinks() {
      setLoading(true);
      try {
        const res = await listGitLinks("audit_log", auditLogId);
        if (!cancelled) setLinks(res.data ?? []);
      } catch (err) {
        console.error("Failed to fetch git links:", err);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    fetchLinks();
    return () => { cancelled = true; };
  }, [auditLogId]);

  if (loading) return null;
  if (links.length === 0) return null;

  return (
    <div className="col-span-full">
      <span className="text-xs text-[var(--text-muted)]">Git 关联</span>
      <div className="mt-1 space-y-1.5">
        {links.map((link) => {
          const shortHash = shortenHash(link.commit_hash);
          const commitURL = link.repo_url && link.commit_hash
            ? `${link.repo_url.replace(/\/$/, "")}/commit/${link.commit_hash}`
            : null;
          return (
            <div key={link.id} className="flex items-center gap-2 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-2.5 py-1.5">
              {link.link_type === "pr" ? (
                <GitPullRequest size={12} className="shrink-0 text-violet-400" />
              ) : (
                <GitBranch size={12} className="shrink-0 text-emerald-400" />
              )}
              <span className="text-xs text-[var(--text-primary)]">
                {link.link_type === "pr" && link.pr_number > 0 ? (
                  <>
                    PR #{link.pr_number}
                    {link.pr_title ? `: ${link.pr_title}` : ""}
                  </>
                ) : (
                  link.commit_message || shortHash
                )}
              </span>
              {shortHash && (
                <span className="text-[11px] font-mono text-[var(--text-muted)]">
                  {commitURL ? (
                    <a href={commitURL} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-0.5 text-emerald-500 hover:underline" onClick={(e) => e.stopPropagation()}>
                      {shortHash}<ExternalLink size={9} />
                    </a>
                  ) : shortHash}
                </span>
              )}
              {link.author_name && (
                <span className="text-[11px] text-[var(--text-muted)]">{link.author_name}</span>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
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

  const aiReview = useMemo(
    () => parseAIReviewResult(log.ai_review_result),
    [log.ai_review_result],
  );

  function handleCopy() {
    navigator.clipboard.writeText(log.sql_content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <tr className="audit-expanded-row border-b border-[var(--border-subtle)] bg-[var(--bg-elevated)]/30">
      <td colSpan={6} className="p-0">
        <div className="overflow-hidden">
          <div className="p-5">
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

              {/* Linked ticket — use ticket_id for precise matching */}
              {log.ticket_id > 0 && (
                <div className="col-span-full">
                  <span className="text-xs text-[var(--text-muted)]">
                    关联工单
                  </span>
                  <div className="mt-1">
                    <TicketBlock ticketId={log.ticket_id} />
                  </div>
                </div>
              )}

              {/* Git links */}
              <GitLinksForAudit auditLogId={log.id} />

              {/* AI Review result */}
              {aiReview && <AiReviewBlock review={aiReview} />}

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

  // Tab
  const [activeTab, setActiveTab] = useState<"logs" | "analytics">("logs");
  const [userRole, setUserRole] = useState<string>("");

  useEffect(() => {
    api
      .get<{ code: number; data: { role: string } }>("/auth/me")
      .then((res) => {
        if (res.code === 0) setUserRole(res.data.role);
      })
      .catch(() => {});
  }, []);

  const isAdmin = userRole === "admin";

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

  // Export state (legacy — retained for type compat)
  const [exportDialogOpen, setExportDialogOpen] = useState(false);

  // Load filter options
  useEffect(() => {
    api
      .get<{ code: number; data: DataSourceOption[] }>("/datasources")
      .then((res) => {
        setDatasources(res.data ?? []);
      })
      .catch((err) => { console.error("Failed to fetch datasources:", err); });
    api
      .get<{ code: number; data: { users: UserOption[] } | UserOption[] }>("/users")
      .then((res) => {
        const d = res.data
        setUsers(Array.isArray(d) ? d : (d as { users: UserOption[] }).users ?? []);
      })
      .catch((err) => { console.error("Failed to fetch users:", err); });
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

  // ExportDialog callbacks — build params from current filters
  const buildAuditExportParams = useCallback(
    (format: ExportFormat, columns: string[]) => ({
      user_id: userFilter || undefined,
      action: actionFilter || undefined,
      datasource_id: datasourceFilter || undefined,
      start: startDate || undefined,
      end: endDate || undefined,
      keyword: keyword || undefined,
      format,
      columns,
    }),
    [userFilter, actionFilter, datasourceFilter, startDate, endDate, keyword],
  );

  const handleSyncExport = useCallback(
    async (format: ExportFormat, columns: string[]) => {
      const params = buildAuditExportParams(format, columns);
      return exportAuditLogs(params);
    },
    [buildAuditExportParams],
  );

  const handleAsyncExport = useCallback(
    async (format: ExportFormat, columns: string[]) => {
      const params = buildAuditExportParams(format, columns);
      return createAsyncAuditExport(params);
    },
    [buildAuditExportParams],
  );

  const handleGetTask = useCallback(
    (taskId: number) => getExportTask(taskId),
    [],
  );

  const handleDownloadTask = useCallback(
    async (taskId: number) => downloadExportFile(taskId),
    [],
  );

  const totalPages = Math.ceil(total / pageSize);

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between mb-3 px-5 py-3">
        <div className="flex items-center gap-2.5">
          <FileText size={18} className="text-[var(--accent-primary)]" />
          <h1 className="text-base font-semibold text-[var(--text-primary)]">
            审计
          </h1>
          {/* Tab Switcher */}
          <div className="ml-3 flex items-center gap-1">
            <button
              type="button"
              className={cn(
                "rounded-md px-3 py-1 text-xs font-medium transition-colors",
                activeTab === "logs"
                  ? "bg-[var(--accent-primary)]/15 text-[var(--accent-primary)]"
                  : "text-[var(--text-muted)] hover:text-[var(--text-primary)]",
              )}
              onClick={() => setActiveTab("logs")}
            >
              操作日志
            </button>
            {isAdmin && (
              <button
                type="button"
                className={cn(
                  "flex items-center gap-1 rounded-md px-3 py-1 text-xs font-medium transition-colors",
                  activeTab === "analytics"
                    ? "bg-[var(--accent-primary)]/15 text-[var(--accent-primary)]"
                    : "text-[var(--text-muted)] hover:text-[var(--text-primary)]",
                )}
                onClick={() => setActiveTab("analytics")}
              >
                <BarChart3 size={12} />
                用户分析
              </button>
            )}
          </div>
          {activeTab === "logs" && total > 0 && (
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
            onClick={() => setExportDialogOpen(true)}
            disabled={loading}
          >
            <Download size={14} />
            导出
          </Button>
        </div>
      </div>

      {activeTab === "logs" && (<>
      {/* Export Dialog */}
      <ExportDialog
        open={exportDialogOpen}
        onOpenChange={setExportDialogOpen}
        exportType="audit"
        columns={AUDIT_EXPORT_COLUMNS}
        filenamePrefix="audit_logs"
        syncExport={handleSyncExport}
        asyncExport={handleAsyncExport}
        getTask={handleGetTask}
        downloadTask={handleDownloadTask}
        disabled={loading}
      />

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-5 py-3">
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
                      className="cursor-pointer border-[var(--border-subtle)] hover:bg-[var(--bg-elevated)] [&>td]:py-3.5"
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
        <div className="flex items-center justify-between border-t border-[var(--border-default)] bg-[var(--bg-surface)] px-5 py-3">
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
      </>)}

      {activeTab === "analytics" && (
        <div className="flex-1 overflow-auto px-5 pb-5">
          <UserAnalyticsTab isAdmin={isAdmin} />
        </div>
      )}
    </div>
  );
}
