import { useState, useEffect, useCallback } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  Plus,
  Search,
  FileText,
  ChevronLeft,
  ChevronRight,
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
import { api } from "@/api/client";
import {
  listTickets,
  getStatusLabel,
  getStatusColor,
  getRiskLabel,
  getRiskColor,
  getRiskDot,
  formatTime,
  type Ticket,
  type TicketStatus,
} from "@/api/ticket";
import TicketDetailDrawer from "./components/TicketDetailDrawer";

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

  const totalPages = Math.ceil(total / pageSize);

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-3">
        <h1 className="text-base font-semibold text-[var(--text-primary)]">
          变更工单
        </h1>
        <Button
          size="sm"
          className="h-8 gap-1.5 bg-[var(--accent-primary)] px-3 text-xs text-white hover:bg-[var(--accent-hover)]"
          onClick={() => navigate("/tickets/new")}
        >
          <Plus size={14} />
          提交新工单
        </Button>
      </div>

      {/* Tabs */}
      <div className="border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6">
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
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-2.5">
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
      <div className="flex-1 overflow-auto bg-[var(--bg-base)] table-responsive">
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
                <TableHead className="w-24 text-xs text-[var(--text-secondary)]">
                  状态
                </TableHead>
                <TableHead className="w-28 text-xs text-[var(--text-secondary)]">
                  提交时间
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tickets.map((t) => (
                <TableRow
                  key={t.id}
                  className="cursor-pointer border-[var(--border-subtle)] hover:bg-[var(--bg-elevated)]"
                  onClick={() => handleRowClick(t.id)}
                >
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
                    <Badge
                      className={`${getStatusColor(t.status as TicketStatus)} border-0 text-[10px]`}
                    >
                      {getStatusLabel(t.status as TicketStatus)}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs text-[var(--text-muted)]">
                    {formatTime(t.created_at)}
                  </TableCell>
                </TableRow>
              ))}
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

      {/* Detail Drawer */}
      <TicketDetailDrawer
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        ticketId={selectedTicketId}
        userRole={user.role}
        userId={user.id}
        onActionComplete={handleActionComplete}
      />
    </div>
  );
}
