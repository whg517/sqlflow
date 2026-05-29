import { useState, useEffect } from "react";
import {
  BarChart3,
  AlertTriangle,
  Gauge,
  FileText,
  TrendingUp,
  Users,
  Database,
  Clock,
  ShieldCheck,
  Shield,
  ArrowUp,
  ArrowDown,
  Loader2,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
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
  TableCell,
  TableRow,
} from "@/components/ui/table";
import {
  getUsageStats,
  getErrorStats,
  getPerformanceReport,
  getTicketReport,
  formatMs,
  formatPercent,
  formatNumber,
  type UsageStatsResponse,
  type ErrorStatsResponse,
  type PerformanceReportResponse,
  type TicketReportResponse,
} from "@/api/report";

// --- Shared Components ---

function StatCard({
  icon: Icon,
  label,
  value,
  sub,
  color,
  bg,
}: {
  icon: React.ElementType;
  label: string;
  value: string | number;
  sub?: React.ReactNode;
  color: string;
  bg: string;
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 py-5">
        <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${bg}`}>
          <Icon size={20} className={color} />
        </div>
        <div className="min-w-0">
          <div className="truncate text-2xl font-bold text-[var(--text-primary)]">
            {value}
          </div>
          <div className="text-sm text-[var(--text-secondary)]">{label}</div>
          {sub && (
            <div className="text-xs text-[var(--text-muted)]">{sub}</div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function TableSection({
  title,
  loading,
  emptyText,
  className,
  children,
}: {
  title: string;
  loading: boolean;
  emptyText: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <Card className={className}>
      <CardContent className="space-y-3 p-4">
        <h3 className="text-sm font-medium text-[var(--text-primary)]">
          {title}
        </h3>
        {loading ? (
          <div className="flex h-32 items-center justify-center">
            <Loader2 size={16} className="animate-spin text-[var(--text-muted)]" />
          </div>
        ) : children ? (
          <div className="overflow-x-auto">{children}</div>
        ) : (
          <div className="flex h-24 items-center justify-center text-sm text-[var(--text-muted)]">
            {emptyText}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// --- Usage Tab ---

function UsageTab({ days }: { days: number }) {
  const [stats, setStats] = useState<UsageStatsResponse["data"] | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    getUsageStats(days)
      .then(setStats)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [days]);

  const latestTrend = stats?.daily_trend?.[stats.daily_trend.length - 1];
  const prevTrend = stats?.daily_trend?.[stats.daily_trend.length - 2];
  const trendUp = latestTrend && prevTrend ? latestTrend.count > prevTrend.count : null;

  return (
    <div className="space-y-5">
      {/* Summary cards */}
      <div className={`grid grid-cols-2 gap-5 md:grid-cols-4${loading ? " animate-pulse" : ""}`}>
        <StatCard icon={BarChart3} label="总操作数" value={stats?.total_actions ?? 0} color="text-blue-500" bg="bg-blue-500/10" />
        <StatCard icon={Users} label="活跃用户" value={stats?.unique_users ?? 0} color="text-emerald-500" bg="bg-emerald-500/10" />
        <StatCard icon={ShieldCheck} label="独立 IP" value={stats?.unique_ips ?? 0} color="text-violet-500" bg="bg-violet-500/10"
          sub={trendUp !== null ? (
            <span className={`flex items-center gap-1 ${trendUp ? "text-emerald-400" : "text-red-400"}`}>
              {trendUp ? <ArrowUp size={10} /> : <ArrowDown size={10} />}
              {latestTrend?.count} 次/今日
            </span>
          ) : undefined}
        />
        <StatCard icon={Clock} label="统计天数" value={days} color="text-amber-500" bg="bg-amber-500/10" />
      </div>

      <div className="grid gap-5 md:grid-cols-2">
        {/* Top Users */}
        <TableSection title="活跃用户 TOP 10" loading={loading} emptyText="暂无数据">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
                <th className="px-3 py-2 font-medium">#</th>
                <th className="px-3 py-2 font-medium">用户</th>
                <th className="px-3 py-2 font-medium text-right">操作数</th>
              </tr>
            </thead>
            <tbody>
              {stats?.top_users?.map((u, i) => (
                <TableRow key={u.user_id}>
                  <TableCell className="text-[var(--text-muted)]">{i + 1}</TableCell>
                  <TableCell className="text-[var(--text-primary)]">{u.username || `#${u.user_id}`}</TableCell>
                  <TableCell className="text-right font-medium text-[var(--text-primary)]">{formatNumber(u.count)}</TableCell>
                </TableRow>
              ))}
            </tbody>
          </table>
        </TableSection>

        {/* Top Actions */}
        <TableSection title="操作类型 TOP 10" loading={loading} emptyText="暂无数据">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
                <th className="px-3 py-2 font-medium">#</th>
                <th className="px-3 py-2 font-medium">操作类型</th>
                <th className="px-3 py-2 font-medium text-right">次数</th>
              </tr>
            </thead>
            <tbody>
              {stats?.top_actions?.map((a, i) => (
                <TableRow key={a.action}>
                  <TableCell className="text-[var(--text-muted)]">{i + 1}</TableCell>
                  <TableCell><Badge variant="outline" className="border-[var(--border-default)] text-[var(--text-primary)]">{a.action}</Badge></TableCell>
                  <TableCell className="text-right font-medium text-[var(--text-primary)]">{formatNumber(a.count)}</TableCell>
                </TableRow>
              ))}
            </tbody>
          </table>
        </TableSection>

        {/* Top Databases */}
        <TableSection title="数据库热度 TOP 10" loading={loading} emptyText="暂无数据">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
                <th className="px-3 py-2 font-medium">#</th>
                <th className="px-3 py-2 font-medium">数据库</th>
                <th className="px-3 py-2 font-medium text-right">操作数</th>
              </tr>
            </thead>
            <tbody>
              {stats?.top_databases?.map((d, i) => (
                <TableRow key={d.database}>
                  <TableCell className="text-[var(--text-muted)]">{i + 1}</TableCell>
                  <TableCell className="flex items-center gap-2 text-[var(--text-primary)]">
                    <Database size={12} className="text-[var(--text-muted)]" />
                    {d.database}
                  </TableCell>
                  <TableCell className="text-right font-medium text-[var(--text-primary)]">{formatNumber(d.count)}</TableCell>
                </TableRow>
              ))}
            </tbody>
          </table>
        </TableSection>

        {/* Daily Trend */}
        <TableSection title="每日操作趋势" loading={loading} emptyText="暂无数据">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
                <th className="px-3 py-2 font-medium">日期</th>
                <th className="px-3 py-2 font-medium text-right">操作数</th>
                <th className="px-3 py-2 font-medium">趋势</th>
              </tr>
            </thead>
            <tbody>
              {stats?.daily_trend?.map((d, i) => {
                const prev = i > 0 ? stats.daily_trend[i - 1].count : d.count;
                const diff = d.count - prev;
                return (
                  <TableRow key={d.date}>
                    <TableCell className="text-[var(--text-primary)]">{d.date}</TableCell>
                    <TableCell className="text-right font-medium text-[var(--text-primary)]">{formatNumber(d.count)}</TableCell>
                    <TableCell>
                      {diff !== 0 ? (
                        <span className={`flex items-center gap-1 text-xs ${diff > 0 ? "text-emerald-400" : "text-red-400"}`}>
                          {diff > 0 ? <ArrowUp size={10} /> : <ArrowDown size={10} />}
                          {Math.abs(diff)}
                        </span>
                      ) : (
                        <span className="text-xs text-[var(--text-muted)]">—</span>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </tbody>
          </table>
        </TableSection>
      </div>
    </div>
  );
}

// --- Error Tab ---

function ErrorTab({ days }: { days: number }) {
  const [stats, setStats] = useState<ErrorStatsResponse["data"] | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    getErrorStats(days)
      .then(setStats)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [days]);

  return (
    <div className="space-y-5">
      <div className={`grid grid-cols-2 gap-5 md:grid-cols-2${loading ? " animate-pulse" : ""}`}>
        <StatCard icon={AlertTriangle} label="总错误数" value={stats?.total_errors ?? 0} color="text-red-500" bg="bg-red-500/10" />
        <StatCard icon={TrendingUp} label="错误率" value={stats ? formatPercent(stats.error_rate) : "0%"} color="text-amber-500" bg="bg-amber-500/10" />
      </div>

      <div className="grid gap-5 md:grid-cols-2">
        {/* Error Types */}
        <TableSection title="错误类型分布" loading={loading} emptyText="暂无错误">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
                <th className="px-3 py-2 font-medium">操作类型</th>
                <th className="px-3 py-2 font-medium text-right">错误次数</th>
              </tr>
            </thead>
            <tbody>
              {stats?.top_error_types?.map((e) => (
                <TableRow key={e.action}>
                  <TableCell className="text-[var(--text-primary)]">{e.action}</TableCell>
                  <TableCell className="text-right">
                    <Badge variant="outline" className="border-red-500/30 bg-red-500/10 text-red-400">{formatNumber(e.count)}</Badge>
                  </TableCell>
                </TableRow>
              ))}
            </tbody>
          </table>
        </TableSection>

        {/* Daily Error Trend */}
        <TableSection title="每日错误趋势" loading={loading} emptyText="暂无数据">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
                <th className="px-3 py-2 font-medium">日期</th>
                <th className="px-3 py-2 font-medium text-right">错误数</th>
              </tr>
            </thead>
            <tbody>
              {stats?.daily_error_trend?.map((d) => (
                <TableRow key={d.date}>
                  <TableCell className="text-[var(--text-primary)]">{d.date}</TableCell>
                  <TableCell className="text-right">
                    <span className={d.count > 0 ? "rounded bg-red-500/10 px-1.5 py-0.5 text-xs font-medium text-red-400" : "text-[var(--text-muted)]"}>
                      {d.count}
                    </span>
                  </TableCell>
                </TableRow>
              ))}
            </tbody>
          </table>
        </TableSection>

        {/* Recent Errors - full width */}
        <TableSection title="最近错误 (最近 20 条)" loading={loading} emptyText="暂无错误" className="md:col-span-2">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
                <th className="px-3 py-2 font-medium">时间</th>
                <th className="px-3 py-2 font-medium">用户</th>
                <th className="px-3 py-2 font-medium">操作</th>
                <th className="px-3 py-2 font-medium">数据库</th>
                <th className="px-3 py-2 font-medium">错误信息</th>
              </tr>
            </thead>
            <tbody>
              {stats?.recent_errors?.map((e) => (
                <TableRow key={e.id}>
                  <TableCell className="whitespace-nowrap text-xs text-[var(--text-muted)]">{e.created_at}</TableCell>
                  <TableCell className="text-[var(--text-primary)]">{e.username || `#${e.id}`}</TableCell>
                  <TableCell><Badge variant="outline" className="border-[var(--border-default)] text-[var(--text-primary)]">{e.action}</Badge></TableCell>
                  <TableCell className="text-[var(--text-secondary)]">{e.database || "—"}</TableCell>
                  <TableCell className="max-w-[400px] truncate text-xs text-red-400">{e.error_message}</TableCell>
                </TableRow>
              ))}
            </tbody>
          </table>
        </TableSection>
      </div>
    </div>
  );
}

// --- Performance Tab ---

function PerformanceTab({ days }: { days: number }) {
  const [stats, setStats] = useState<PerformanceReportResponse["data"] | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    getPerformanceReport(days)
      .then(setStats)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [days]);

  return (
    <div className="space-y-5">
      <div className={`grid grid-cols-2 gap-5 md:grid-cols-4${loading ? " animate-pulse" : ""}`}>
        <StatCard icon={Gauge} label="平均耗时" value={stats ? formatMs(stats.avg_execution_ms) : "0ms"} color="text-blue-500" bg="bg-blue-500/10" />
        <StatCard icon={TrendingUp} label="最大耗时" value={stats ? formatMs(stats.max_execution_ms) : "0ms"} color="text-red-500" bg="bg-red-500/10" />
        <StatCard icon={Shield} label="P95 耗时" value={stats ? formatMs(stats.p95_execution_ms) : "0ms"} color="text-amber-500" bg="bg-amber-500/10" />
        <StatCard icon={BarChart3} label="总返回行数" value={stats ? formatNumber(stats.total_result_rows) : "0"} color="text-emerald-500" bg="bg-emerald-500/10" />
      </div>

      <TableSection title="每日性能趋势" loading={loading} emptyText="暂无数据">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
              <th className="px-3 py-2 font-medium">日期</th>
              <th className="px-3 py-2 font-medium text-right">查询数</th>
              <th className="px-3 py-2 font-medium text-right">平均耗时</th>
              <th className="px-3 py-2 font-medium text-right">最大耗时</th>
              <th className="px-3 py-2 font-medium text-right">返回行数</th>
            </tr>
          </thead>
          <tbody>
            {stats?.daily_perf_trend?.map((d) => (
              <TableRow key={d.date}>
                <TableCell className="text-[var(--text-primary)]">{d.date}</TableCell>
                <TableCell className="text-right text-[var(--text-primary)]">{formatNumber(d.query_count)}</TableCell>
                <TableCell className="text-right text-[var(--text-primary)]">{formatMs(d.avg_time_ms)}</TableCell>
                <TableCell className="text-right">
                  <span className={d.max_time_ms >= 1000 ? "rounded bg-red-500/10 px-1.5 py-0.5 text-xs font-medium text-red-400" : "text-[var(--text-secondary)]"}>
                    {formatMs(d.max_time_ms)}
                  </span>
                </TableCell>
                <TableCell className="text-right text-[var(--text-secondary)]">{formatNumber(d.result_rows)}</TableCell>
              </TableRow>
            ))}
          </tbody>
        </table>
      </TableSection>
    </div>
  );
}

// --- Ticket Tab ---

function TicketTab({ days }: { days: number }) {
  const [stats, setStats] = useState<TicketReportResponse["data"] | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    getTicketReport(days)
      .then(setStats)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [days]);

  const riskLabel: Record<string, string> = { low: "低风险", medium: "中风险", high: "高风险" };
  const riskColor: Record<string, string> = {
    low: "bg-emerald-500/20 text-emerald-400",
    medium: "bg-amber-500/20 text-amber-400",
    high: "bg-red-500/20 text-red-400",
  };

  return (
    <div className="space-y-5">
      <div className={`grid grid-cols-2 gap-5 md:grid-cols-4${loading ? " animate-pulse" : ""}`}>
        <StatCard icon={FileText} label="总工单数" value={stats?.total_tickets ?? 0} color="text-blue-500" bg="bg-blue-500/10" />
        <StatCard icon={Clock} label="待审批" value={stats?.pending_count ?? 0} color="text-amber-500" bg="bg-amber-500/10" />
        <StatCard icon={ShieldCheck} label="平均审批时间" value={stats ? `${stats.avg_approval_time_h.toFixed(1)}h` : "0h"} color="text-emerald-500" bg="bg-emerald-500/10" />
        <StatCard icon={AlertTriangle} label="拒绝率" value={
          stats && stats.total_tickets > 0
            ? formatPercent((stats.rejected_count / stats.total_tickets) * 100)
            : "0%"
        } color="text-red-500" bg="bg-red-500/10" />
      </div>

      {/* Status breakdown */}
      <div className="grid grid-cols-5 gap-3">
        {[
          { label: "已审批", count: stats?.approved_count ?? 0, color: "text-emerald-400", bg: "bg-emerald-500/10" },
          { label: "已完成", count: stats?.done_count ?? 0, color: "text-blue-400", bg: "bg-blue-500/10" },
          { label: "已拒绝", count: stats?.rejected_count ?? 0, color: "text-red-400", bg: "bg-red-500/10" },
          { label: "已取消", count: stats?.cancelled_count ?? 0, color: "text-[var(--text-tertiary)]", bg: "bg-[var(--text-tertiary)]/10" },
          { label: "待审批", count: stats?.pending_count ?? 0, color: "text-amber-400", bg: "bg-amber-500/10" },
        ].map((item) => (
          <Card key={item.label}>
            <CardContent className="py-4 text-center">
              <div className={`text-xl font-bold ${item.color}`}>{formatNumber(item.count)}</div>
              <div className="text-xs text-[var(--text-muted)]">{item.label}</div>
            </CardContent>
          </Card>
        ))}
      </div>

      <div className="grid gap-5 md:grid-cols-2">
        {/* Daily Ticket Trend */}
        <TableSection title="工单趋势" loading={loading} emptyText="暂无数据">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
                <th className="px-3 py-2 font-medium">日期</th>
                <th className="px-3 py-2 font-medium text-right">创建</th>
                <th className="px-3 py-2 font-medium text-right">通过</th>
                <th className="px-3 py-2 font-medium text-right">拒绝</th>
              </tr>
            </thead>
            <tbody>
              {stats?.daily_ticket_trend?.map((d) => (
                <TableRow key={d.date}>
                  <TableCell className="text-[var(--text-primary)]">{d.date}</TableCell>
                  <TableCell className="text-right text-[var(--text-primary)]">{d.created}</TableCell>
                  <TableCell className="text-right text-emerald-400">{d.approved}</TableCell>
                  <TableCell className="text-right">
                    <span className={d.rejected > 0 ? "text-red-400" : "text-[var(--text-muted)]"}>{d.rejected}</span>
                  </TableCell>
                </TableRow>
              ))}
            </tbody>
          </table>
        </TableSection>

        {/* Risk Distribution */}
        <TableSection title="风险分布" loading={loading} emptyText="暂无数据">
          <div className="space-y-3 py-2">
            {stats?.risk_distribution?.map((r) => {
              const total = stats?.total_tickets || 1;
              const pct = (r.count / total) * 100;
              return (
                <div key={r.risk_level} className="space-y-1.5">
                  <div className="flex items-center justify-between">
                    <Badge className={`${riskColor[r.risk_level] || "bg-[var(--text-tertiary)]/15 text-[var(--text-tertiary)]"} border-0`}>
                      {riskLabel[r.risk_level] || r.risk_level}
                    </Badge>
                    <span className="text-xs text-[var(--text-secondary)]">{formatNumber(r.count)} ({formatPercent(pct)})</span>
                  </div>
                  <div className="h-2 overflow-hidden rounded-full bg-[var(--bg-base)]">
                    <div
                      className={`h-full rounded-full transition-all ${
                        r.risk_level === "high" ? "bg-red-500" :
                        r.risk_level === "medium" ? "bg-amber-500" : "bg-emerald-500"
                      }`}
                      style={{ width: `${Math.max(pct, 2)}%` }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </TableSection>
      </div>
    </div>
  );
}

// --- Main Page ---

export default function ReportsPage() {
  const [activeTab, setActiveTab] = useState("usage");
  const [days, setDays] = useState(7);

  const tabConfig = [
    { value: "usage", label: "使用统计" },
    { value: "errors", label: "错误分析" },
    { value: "performance", label: "性能趋势" },
    { value: "tickets", label: "工单统计" },
  ];

  return (
    <div className="mx-auto max-w-[1200px] space-y-6 page-transition">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          审计报表
        </h1>
        <div className="flex items-center gap-3">
          <span className="text-sm text-[var(--text-secondary)]">统计范围</span>
          <Select value={String(days)} onValueChange={(v) => setDays(Number(v))}>
            <SelectTrigger className="h-8 w-28 border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="7">近 7 天</SelectItem>
              <SelectItem value="14">近 14 天</SelectItem>
              <SelectItem value="30">近 30 天</SelectItem>
              <SelectItem value="90">近 90 天</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Tabs */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] px-5 pt-4 pb-1">
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList variant="line" className="h-9">
            {tabConfig.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value} className="text-xs">
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>

      {/* Tab Content */}
      <div className="pb-6">
        {activeTab === "usage" && <UsageTab days={days} />}
        {activeTab === "errors" && <ErrorTab days={days} />}
        {activeTab === "performance" && <PerformanceTab days={days} />}
        {activeTab === "tickets" && <TicketTab days={days} />}
      </div>
    </div>
  );
}
