import { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  Plus,
  Search,
  FileText,
  ChevronLeft,
  ChevronRight,
  Download,
  Loader2,
  CheckCircle2,
  XCircle,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
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
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { api } from "@/api/client";
import {
  listTickets,
  getStatusLabel,
  getStatusColor,
  getRiskLabel,
  getRiskColor,
  getRiskDot,
  formatTime,
  batchApproveTickets,
  batchRejectTickets,
  type Ticket,
  type TicketStatus,
} from "@/api/ticket";
import {
  exportTickets,
  createAsyncTicketExport,
  getExportTask,
  downloadExportFile,
} from "@/api/export";
import { ExportDialog } from "@/components/ExportDialog";
import type { ExportColumn, ExportFormat } from "@/lib/export-utils";
import TicketDetailDrawer from "./components/TicketDetailDrawer";
import {
  getTicketSLAStatuses,
  getSLAStatusLabel,
  getSLAStatusColor,
  getSLADot,
  formatSLARemaining,
  type SLATicketStatus,
} from "@/api/sla";

// --- Ticket Export Columns ---

/** Ticket columns available for export — must match backend whitelist. */
const TICKET_EXPORT_COLUMNS: ExportColumn[] = [
  { key: "id", label: "ID" },
  { key: "submitter_name", label: "提交人" },
  { key: "datasource_id", label: "数据源 ID" },
  { key: "database", label: "数据库" },
  { key: "sql_content", label: "SQL 内容" },
  { key: "sql_summary", label: "SQL 摘要" },
  { key: "sql_type", label: "SQL 类型" },
  { key: "status", label: "状态" },
  { key: "risk_level", label: "风险等级" },
  { key: "reviewer_name", label: "审批人" },
  { key: "review_comment", label: "审批意见" },
  { key: "change_reason", label: "变更原因" },
  { key: "revision", label: "版本号" },
  { key: "auto_approved", label: "自动审批" },
  { key: "created_at", label: "创建时间" },
  { key: "updated_at", label: "更新时间" },
];

// --- Status Tab Config ---

interface StatusTab {
  value: string;
  label: string;
  status?: TicketStatus;
}

const statusTabs: StatusTab[] = [
  { value: "all", label: "全部" },
  { value: "PENDING_APPROVAL", label: "待审批", status: "PENDING_APPROVAL" },
  { value: "APPROVED", label: "已通过", status: "APPROVED" },
  { value: "REJECTED", label: "已拒绝", status: "REJECTED" },
  { value: "CANCELLED", label: "已取消", status: "CANCELLED" },
  { value: "DONE", label: "已执行", status: "DONE" },
];

// --- Types ---

interface DataSourceOption {
  id: number;
  name: string;
  type: string;
}

interface CurrentUser {
  id: number;
  username: string;
  role: string;
}

// --- Main Page ---

export default function TicketPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  // State
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(50);
  const [loading, setLoading] = useState(false);

  // Filters
  const [activeTab, setActiveTab] = useState("all");
  const [scopeFilter, setScopeFilter] = useState<string>("");
  const [datasourceFilter, setDatasourceFilter] = useState<string>("");
  const [riskFilter, setRiskFilter] = useState<string>("");
  const [keyword, setKeyword] = useState("");
  const [searchInput, setSearchInput] = useState("");

  // Datasources
  const [datasources, setDatasources] = useState<DataSourceOption[]>([]);

  // User
  const [user, setUser] = useState<CurrentUser>({
    id: 0,
    username: "",
    role: "",
  });

  // Detail drawer
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [selectedTicketId, setSelectedTicketId] = useState<number | null>(null);

  // Export state
  const [exportDialogOpen, setExportDialogOpen] = useState(false);

  // SLA status
  const [slaStatuses, setSlaStatuses] = useState<Record<number, SLATicketStatus>>({});

  // Batch selection
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [batchDialogOpen, setBatchDialogOpen] = useState<false | "approve" | "reject">(false);
  const [batchReason, setBatchReason] = useState("");
  const [batchLoading, setBatchLoading] = useState(false);

  // Only PENDING_APPROVAL tickets can be selected for batch operations (approvers only)
  const isApprover = user.role === "admin" || user.role === "dba";
  const selectableIds = useMemo(
    () => tickets.filter((t) => t.status === "PENDING_APPROVAL").map((t) => t.id),
    [tickets],
  );

  // Open detail drawer if `id` param present in URL (from global search)
  useEffect(() => {
    const idParam = searchParams.get("id");
    if (idParam) {
      const id = Number(idParam);
      if (id > 0) {
        queueMicrotask(() => {
          setSelectedTicketId(id);
          setDrawerOpen(true);
        });
      }
    }
  }, [searchParams]);

  // Load user
  useEffect(() => {
    api
      .get<{ code: number; data: CurrentUser }>("/auth/me")
      .then((res) => {
        if (res.code === 0) setUser(res.data);
      })
      .catch(() => {});
  }, []);

  // Load datasources
  useEffect(() => {
    api
      .get<{ code: number; data: DataSourceOption[] }>("/datasources")
      .then((res) => {
        setDatasources(res.data ?? []);
      })
      .catch(() => {});
  }, []);

  // Fetch tickets
  const fetchTickets = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listTickets({
        page,
        page_size: pageSize,
        status: activeTab !== "all" ? activeTab : undefined,
        datasource_id: datasourceFilter || undefined,
        risk_level: riskFilter || undefined,
        keyword: keyword || undefined,
        scope: scopeFilter as "mine" | "pending" | undefined,
      });
      setTickets(res.data ?? []);
      setTotal(res.total);
      setSelectedIds(new Set());
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "获取工单列表失败");
    } finally {
      setLoading(false);
    }
  }, [
    page,
    pageSize,
    activeTab,
    datasourceFilter,
    riskFilter,
    keyword,
    scopeFilter,
  ]);

  useEffect(() => {
    const id = requestAnimationFrame(() => {
      fetchTickets();
    });
    return () => cancelAnimationFrame(id);
  }, [fetchTickets]);

  // Fetch SLA statuses for PENDING_APPROVAL tickets
  useEffect(() => {
    const pendingIds = tickets
      .filter((t) => t.status === "PENDING_APPROVAL")
      .map((t) => t.id);
    if (pendingIds.length === 0) {
      setSlaStatuses({});
      return;
    }
    getTicketSLAStatuses(pendingIds)
      .then((res) => {
        setSlaStatuses(res.data ?? {});
      })
      .catch(() => {
        // ignore SLA fetch errors
      });
  }, [tickets]);

  // Tab change
  function handleTabChange(value: string) {
    setActiveTab(value);
    setPage(1);
  }

  // Search
  function handleSearch() {
    setKeyword(searchInput.trim());
    setPage(1);
  }

  function handleSearchKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter") handleSearch();
  }

  // Row click
  function handleRowClick(id: number) {
    setSelectedTicketId(id);
    setDrawerOpen(true);
  }

  // Action complete -> refresh list
  function handleActionComplete() {
    fetchTickets();
  }

  // --- Batch selection handlers ---
  function toggleSelect(id: number) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleSelectAll() {
    if (selectedIds.size === selectableIds.length && selectableIds.length > 0) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(selectableIds));
    }
  }

  function clearSelection() {
    setSelectedIds(new Set());
  }

  async function handleBatchAction() {
    if (selectedIds.size === 0 || !batchDialogOpen) return;
    if (selectedIds.size > 50) {
      toast.error("单次批量操作最多 50 条");
      return;
    }
    setBatchLoading(true);
    try {
      const ids = Array.from(selectedIds);
      const fn = batchDialogOpen === "approve" ? batchApproveTickets : batchRejectTickets;
      const res = await fn({ ticket_ids: ids, reason: batchReason || undefined });
      if (res.data.failed > 0) {
        toast.warning(
          `${batchDialogOpen === "approve" ? "批量通过" : "批量拒绝"}完成：${res.data.succeeded} 成功，${res.data.failed} 失败`,
        );
      } else {
        toast.success(
          `${batchDialogOpen === "approve" ? "批量通过" : "批量拒绝"}成功，共 ${res.data.succeeded} 条`,
        );
      }
      setBatchDialogOpen(false);
      setBatchReason("");
      setSelectedIds(new Set());
      fetchTickets();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "批量操作失败");
    } finally {
      setBatchLoading(false);
    }
  }

  // Export handler — tries sync first, falls back to async for large datasets
  // ExportDialog callbacks — build params from current filters
  const buildTicketExportParams = useCallback(
    (format: ExportFormat, columns: string[]) => ({
      status: activeTab !== "all" ? activeTab : undefined,
      datasource_id: datasourceFilter || undefined,
      risk_level: riskFilter || undefined,
      keyword: keyword || undefined,
      format,
      columns,
    }),
    [activeTab, datasourceFilter, riskFilter, keyword],
  );

  const handleSyncExport = useCallback(
    async (format: ExportFormat, columns: string[]) => {
      const params = buildTicketExportParams(format, columns);
      return exportTickets(params);
    },
    [buildTicketExportParams],
  );

  const handleAsyncExport = useCallback(
    async (format: ExportFormat, columns: string[]) => {
      const params = buildTicketExportParams(format, columns);
      return createAsyncTicketExport(params);
    },
    [buildTicketExportParams],
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
      <div className="flex items-center justify-between mb-5">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          变更工单
        </h1>
        <div className="flex items-center gap-2">
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
          <Button
            size="sm"
            className="h-8 gap-1.5 bg-[var(--accent-primary)] px-3 text-xs text-white hover:bg-[var(--accent-hover)]"
            onClick={() => navigate("/tickets/new")}
          >
            <Plus size={14} />
            提交新工单
          </Button>
        </div>
      </div>

      {/* Export Dialog */}
      <ExportDialog
        open={exportDialogOpen}
        onOpenChange={setExportDialogOpen}
        exportType="ticket"
        columns={TICKET_EXPORT_COLUMNS}
        filenamePrefix="tickets"
        syncExport={handleSyncExport}
        asyncExport={handleAsyncExport}
        getTask={handleGetTask}
        downloadTask={handleDownloadTask}
        disabled={loading}
      />

      {/* Tabs + Filters + Table — all inside a card */}
      <div className="flex-1 overflow-hidden rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] flex flex-col">
      {/* Tabs */}
      <div className="border-b border-[var(--border-default)] px-5 pt-4">
        <Tabs value={activeTab} onValueChange={handleTabChange}>
          <TabsList variant="line" className="h-9">
            {statusTabs.map((tab) => (
              <TabsTrigger
                key={tab.value}
                value={tab.value}
                className="text-xs"
              >
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--border-default)] px-5 py-3">
        {/* Quick scope */}
        <Button
          variant="ghost"
          size="sm"
          className={`h-7 px-2 text-xs ${scopeFilter === "mine" ? "text-[var(--accent-primary)]" : "text-[var(--text-secondary)]"}`}
          onClick={() => {
            setScopeFilter(scopeFilter === "mine" ? "" : "mine");
            setPage(1);
          }}
        >
          我提交的
        </Button>
        {(user.role === "admin" || user.role === "dba") && (
          <Button
            variant="ghost"
            size="sm"
            className={`h-7 px-2 text-xs ${scopeFilter === "pending" ? "text-[var(--accent-primary)]" : "text-[var(--text-secondary)]"}`}
            onClick={() => {
              setScopeFilter(scopeFilter === "pending" ? "" : "pending");
              setPage(1);
            }}
          >
            待我审批
          </Button>
        )}

        <div className="mx-1 h-4 w-px bg-[var(--border-default)]" />

        {/* Datasource filter */}
        <Select
          value={datasourceFilter}
          onValueChange={(v) => {
            setDatasourceFilter(v === "__all__" ? "" : v);
            setPage(1);
          }}
        >
          <SelectTrigger className="h-7 w-32 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
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

        {/* Risk filter */}
        <Select
          value={riskFilter}
          onValueChange={(v) => {
            setRiskFilter(v === "__all__" ? "" : v);
            setPage(1);
          }}
        >
          <SelectTrigger className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
            <SelectValue placeholder="AI 风险" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">全部风险</SelectItem>
            <SelectItem value="low">低风险</SelectItem>
            <SelectItem value="medium">中风险</SelectItem>
            <SelectItem value="high">高风险</SelectItem>
          </SelectContent>
        </Select>

        {/* Search */}
        <div className="relative ml-auto">
          <Search
            size={14}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]"
          />
          <Input
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onKeyDown={handleSearchKeyDown}
            placeholder="搜索 SQL 内容..."
            className="h-7 w-48 rounded-md border-[var(--border-default)] bg-[var(--bg-elevated)] pl-7 pr-2 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
          />
        </div>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto table-responsive">
        {loading && !tickets.length ? (
          <div className="flex h-32 items-center justify-center">
            <div className="flex flex-col items-center gap-3">
              <div className="grid grid-cols-4 gap-3 w-full max-w-[800px] px-6">
                {[1, 2, 3, 4, 5, 6].map((i) => (
                  <div key={i} className="space-y-2">
                    <div className="h-3 w-12 rounded bg-[var(--bg-elevated)] animate-pulse" />
                    <div className="h-4 w-full rounded bg-[var(--bg-elevated)] animate-pulse" />
                  </div>
                ))}
              </div>
            </div>
          </div>
        ) : tickets.length === 0 ? (
          <div className="flex h-48 flex-col items-center justify-center gap-3 py-12 page-transition">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-[var(--bg-elevated)] empty-state-icon">
              <FileText size={24} className="text-[var(--text-muted)]" />
            </div>
            <div className="text-center">
              <p className="text-sm font-medium text-[var(--text-secondary)]">
                {activeTab !== "all" ||
                scopeFilter ||
                datasourceFilter ||
                keyword
                  ? "没有匹配的工单"
                  : "暂无变更工单"}
              </p>
              <p className="mt-1 text-xs text-[var(--text-muted)]">
                {activeTab !== "all" ||
                scopeFilter ||
                datasourceFilter ||
                keyword
                  ? "尝试切换 Tab 或清空筛选条件"
                  : "提交 SQL 变更申请后，工单将在此展示"}
              </p>
            </div>
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="border-[var(--border-default)] bg-[var(--bg-surface)] hover:bg-[var(--bg-surface)]">
                {isApprover && (
                  <TableHead className="w-10 px-3">
                    <Checkbox
                      checked={selectableIds.length > 0 && selectedIds.size === selectableIds.length}
                      onCheckedChange={toggleSelectAll}
                      disabled={selectableIds.length === 0}
                    />
                  </TableHead>
                )}
                <TableHead className="w-16 text-xs text-[var(--text-secondary)]">
                  ID
                </TableHead>
                <TableHead className="text-xs text-[var(--text-secondary)]">
                  SQL 摘要
                </TableHead>
                <TableHead className="w-24 text-xs text-[var(--text-secondary)]">
                  数据库
                </TableHead>
                <TableHead className="w-24 text-xs text-[var(--text-secondary)]">
                  AI 风险
                </TableHead>
                <TableHead className="w-32 text-xs text-[var(--text-secondary)]">
                  状态
                </TableHead>
                <TableHead className="w-24 text-xs text-[var(--text-secondary)]">
                  SLA
                </TableHead>
                <TableHead className="w-28 text-xs text-[var(--text-secondary)]">
                  提交时间
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tickets.map((t) => {
                const selectable = t.status === "PENDING_APPROVAL" && isApprover;
                const selected = selectedIds.has(t.id);
                return (
                <TableRow
                  key={t.id}
                  className={`cursor-pointer border-[var(--border-subtle)] hover:bg-[var(--bg-elevated)] ${selected ? "bg-[var(--accent-muted)]" : ""}`}
                  onClick={() => handleRowClick(t.id)}
                >
                  {isApprover && (
                    <TableCell className="w-10 px-3" onClick={(e) => e.stopPropagation()}>
                      <Checkbox
                        checked={selected}
                        onCheckedChange={() => toggleSelect(t.id)}
                        disabled={!selectable}
                      />
                    </TableCell>
                  )}
                  <TableCell className="text-xs font-medium text-[var(--accent-primary)]">
                    #{t.id}
                  </TableCell>
                  <TableCell className="max-w-[300px] text-xs text-[var(--text-primary)]">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="block truncate">
                          {t.sql_summary || t.sql_content}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent
                        side="bottom"
                        align="start"
                        className="max-w-[400px] whitespace-pre-wrap break-all text-xs"
                      >
                        {t.sql_content}
                      </TooltipContent>
                    </Tooltip>
                  </TableCell>
                  <TableCell className="text-xs text-[var(--text-secondary)]">
                    {t.database || "—"}
                  </TableCell>
                  <TableCell>
                    {t.risk_level ? (
                      <span
                        className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-medium ${getRiskColor(t.risk_level)}`}
                      >
                        <span
                          className={`inline-block h-1.5 w-1.5 rounded-full ${getRiskDot(t.risk_level)}`}
                        />
                        {getRiskLabel(t.risk_level)}
                      </span>
                    ) : (
                      <span className="text-xs text-[var(--text-muted)]">
                        —
                      </span>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-0.5">
                      <Badge
                        className={`${getStatusColor(t.status as TicketStatus)} border-0 text-[10px] w-fit`}
                      >
                        {getStatusLabel(t.status as TicketStatus)}
                      </Badge>
                      {t.status === "PENDING_APPROVAL" && t.total_stages > 1 && (
                        <span className="text-[10px] text-zinc-500">
                          阶段 {t.current_stage + 1}/{t.total_stages}
                        </span>
                      )}
                      {t.auto_approved && (
                        <span className="inline-flex items-center gap-0.5 text-[10px] text-blue-400">
                          🤖 自动通过
                        </span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    {t.status === "PENDING_APPROVAL" && slaStatuses[t.id] ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="inline-flex items-center gap-1.5">
                            <span
                              className={`inline-block h-1.5 w-1.5 rounded-full ${getSLADot(slaStatuses[t.id].sla_status)}`}
                            />
                            <span
                              className={`rounded-full px-1.5 py-0.5 text-[10px] font-medium ${getSLAStatusColor(slaStatuses[t.id].sla_status)}`}
                            >
                              {formatSLARemaining(slaStatuses[t.id].time_remaining_seconds)}
                            </span>
                          </span>
                        </TooltipTrigger>
                        <TooltipContent className="text-xs">
                          {getSLAStatusLabel(slaStatuses[t.id].sla_status)}
                          {slaStatuses[t.id].time_remaining_seconds <= 0
                            ? " (已超时)"
                            : ` (剩余 ${formatSLARemaining(slaStatuses[t.id].time_remaining_seconds)})`}
                        </TooltipContent>
                      </Tooltip>
                    ) : (
                      <span className="text-xs text-[var(--text-muted)]">
                        —
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="text-xs text-[var(--text-muted)]">
                    {formatTime(t.created_at)}
                  </TableCell>
                </TableRow>
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
      {/* Batch action bar */}
      {isApprover && selectedIds.size > 0 && (
        <div className="flex items-center gap-3 border-t border-[var(--border-default)] bg-[var(--bg-elevated)] px-5 py-2">
          <span className="text-xs text-[var(--text-secondary)]">
            已选 <span className="font-medium text-[var(--accent-primary)]">{selectedIds.size}</span> 条
          </span>
          <Button
            size="sm"
            className="h-7 gap-1 bg-[var(--success)] px-3 text-xs text-white hover:bg-[var(--success)]/80"
            onClick={() => setBatchDialogOpen("approve")}
          >
            <CheckCircle2 size={14} />
            批量通过
          </Button>
          <Button
            size="sm"
            variant="destructive"
            className="h-7 gap-1 px-3 text-xs"
            onClick={() => setBatchDialogOpen("reject")}
          >
            <XCircle size={14} />
            批量拒绝
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-xs text-[var(--text-muted)]"
            onClick={clearSelection}
          >
            取消选择
          </Button>
        </div>
      )}
      </div>{/* end card container */}

      {/* Detail Drawer */}
      <TicketDetailDrawer
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        ticketId={selectedTicketId}
        userRole={user.role}
        userId={user.id}
        onActionComplete={handleActionComplete}
      />

      {/* Batch confirm dialog */}
      <AlertDialog open={batchDialogOpen !== false} onOpenChange={(open) => { if (!open) { setBatchDialogOpen(false); setBatchReason(""); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {batchDialogOpen === "approve" ? "批量通过" : "批量拒绝"}确认
            </AlertDialogTitle>
            <AlertDialogDescription>
              确认要{batchDialogOpen === "approve" ? "通过" : "拒绝"}
              <span className="font-semibold text-[var(--text-primary)]"> {selectedIds.size} </span>
              条工单吗？{batchDialogOpen === "reject" && "请填写拒绝理由。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div>
            <label className="text-xs text-[var(--text-secondary)] mb-1 block">
              {batchDialogOpen === "approve" ? "审批意见（可选）" : "拒绝理由"}
            </label>
            <textarea
              className="w-full rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 py-2 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-primary)]/30 resize-none"
              rows={3}
              value={batchReason}
              onChange={(e) => setBatchReason(e.target.value)}
              placeholder={batchDialogOpen === "approve" ? "可选填写审批意见..." : "请填写拒绝理由..."}
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={batchLoading}>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleBatchAction}
              disabled={batchLoading || (batchDialogOpen === "reject" && !batchReason.trim())}
              className={batchDialogOpen === "approve" ? "bg-[var(--success)] hover:bg-[var(--success)]/80" : undefined}
            >
              {batchLoading && <Loader2 size={14} className="animate-spin" />}
              确认{batchDialogOpen === "approve" ? "通过" : "拒绝"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
