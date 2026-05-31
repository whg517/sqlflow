import { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import {
  FileText,
  Database,
  Server,
  AlertCircle,
  RefreshCw,
  TrendingUp,
  TrendingDown,
  Minus,
  Activity,
  ArrowRight,
  Clock,
  User,
} from "lucide-react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  Tooltip as ShTooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  getDashboardOverview,
  type DashboardOverview,
  type TimeRange,
} from "@/api/dashboard";
import { listSensitiveTables } from "@/api/maskRule";

/* ---------- constants ---------- */

const TIME_RANGES: { key: TimeRange; label: string }[] = [
  { key: "today", label: "今日" },
  { key: "this_week", label: "本周" },
  { key: "this_month", label: "本月" },
  { key: "last_30d", label: "近 30 天" },
];

const TICKET_COLORS: Record<string, string> = {
  SUBMITTED: "#3b82f6",      // blue
  AI_REVIEWED: "#8b5cf6",    // violet
  PENDING_APPROVAL: "#f59e0b", // amber
  APPROVED: "#10b981",       // emerald
  SCHEDULED: "#06b6d4",      // cyan
  EXECUTING: "#6366f1",      // indigo
  DONE: "#22c55e",           // green
  FAILED: "#ef4444",         // red
  REJECTED: "#f97316",       // orange
  CANCELLED: "#6b7280",      // gray
};

const TICKET_STATUS_LABELS: Record<string, string> = {
  SUBMITTED: "已提交",
  AI_REVIEWED: "AI 已审",
  PENDING_APPROVAL: "待审批",
  APPROVED: "已通过",
  SCHEDULED: "已排期",
  EXECUTING: "执行中",
  DONE: "已完成",
  FAILED: "已失败",
  REJECTED: "已拒绝",
  CANCELLED: "已取消",
};

/* ---------- helpers ---------- */

function calcTrend(data: { count: number }[]): number {
  if (data.length < 2) return 0;
  const half = Math.floor(data.length / 2);
  const firstHalf = data.slice(0, half).reduce((s, d) => s + d.count, 0);
  const secondHalf = data.slice(half).reduce((s, d) => s + d.count, 0);
  if (firstHalf === 0) return secondHalf > 0 ? 100 : 0;
  return Math.round(((secondHalf - firstHalf) / firstHalf) * 100);
}

function TrendIndicator({ value }: { value: number }) {
  if (value > 0) {
    return (
      <span className="flex items-center gap-0.5 text-xs font-medium text-emerald-500">
        <TrendingUp size={14} />
        {value}%
      </span>
    );
  }
  if (value < 0) {
    return (
      <span className="flex items-center gap-0.5 text-xs font-medium text-red-500">
        <TrendingDown size={14} />
        {Math.abs(value)}%
      </span>
    );
  }
  return (
    <span className="flex items-center gap-0.5 text-xs font-medium text-[var(--text-muted)]">
      <Minus size={14} />
      0%
    </span>
  );
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString("zh-CN", {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

/* ---------- Stat card with sparkline ---------- */

interface StatCardProps {
  label: string;
  value: number;
  icon: React.ReactNode;
  colorClass: string;
  bgClass: string;
  sparkline: { date: string; count: number }[];
  link?: string;
}

function StatCard({
  label,
  value,
  icon,
  colorClass,
  bgClass,
  sparkline,
  link,
}: StatCardProps) {
  const navigate = useNavigate();
  const trend = calcTrend(sparkline);

  const card = (
    <ShTooltipProvider>
      <ShTooltip>
        <TooltipTrigger asChild>
          <Card
            className={`cursor-pointer transition-all duration-300 hover:shadow-[var(--shadow-md)] ${link ? "" : "cursor-default"}`}
            onClick={() => link && navigate(link)}
          >
            <CardContent className="flex items-start gap-4 py-4">
              <div
                className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${bgClass}`}
              >
                {icon}
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-baseline justify-between gap-2">
                  <span className="text-2xl font-bold text-[var(--text-primary)]">
                    {value}
                  </span>
                  <TrendIndicator value={trend} />
                </div>
                <div className="text-sm text-[var(--text-secondary)]">{label}</div>
                {/* sparkline */}
                <div className="mt-2 h-8 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <LineChart data={sparkline}>
                      <Line
                        type="monotone"
                        dataKey="count"
                        stroke={
                          trend > 0
                            ? "#10b981"
                            : trend < 0
                            ? "#ef4444"
                            : "#6b7280"
                        }
                        strokeWidth={1.5}
                        dot={false}
                      />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              </div>
            </CardContent>
          </Card>
        </TooltipTrigger>
        <TooltipContent side="bottom" className="text-xs">
          <p>
            最近 7 天趋势：{trend > 0 ? "↑" : trend < 0 ? "↓" : "→"} {Math.abs(trend)}%
            对比前半周期
          </p>
        </TooltipContent>
      </ShTooltip>
    </ShTooltipProvider>
  );

  return card;
}

/* ---------- Query trend chart ---------- */

function QueryTrendChart({ data }: { data: { date: string; count: number }[] }) {
  if (!data.length) {
    return (
      <Card className="col-span-1 md:col-span-2">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-[var(--text-secondary)]">
            查询趋势
          </CardTitle>
        </CardHeader>
        <CardContent className="flex h-48 items-center justify-center">
          <p className="text-sm text-[var(--text-muted)]">暂无数据</p>
        </CardContent>
      </Card>
    );
  }

  // Show abbreviated date labels
  const chartData = data.map((d) => ({
    ...d,
    label: d.date.slice(5), // MM-DD
  }));

  return (
    <Card className="col-span-1 md:col-span-2">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-[var(--text-secondary)]">
          <Activity size={16} className="mr-1.5 inline-block" />
          查询趋势
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-56">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={chartData}>
              <XAxis
                dataKey="label"
                tick={{ fontSize: 11, fill: "var(--text-muted)" }}
                axisLine={false}
                tickLine={false}
              />
              <YAxis
                tick={{ fontSize: 11, fill: "var(--text-muted)" }}
                axisLine={false}
                tickLine={false}
                width={32}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: "var(--bg-elevated)",
                  border: "1px solid var(--border-default)",
                  borderRadius: "8px",
                  fontSize: "12px",
                }}
                labelFormatter={(v: string) => `日期: ${v}`}
                formatter={(value: number) => [`${value} 次`, "查询次数"]}
              />
              <Line
                type="monotone"
                dataKey="count"
                stroke="#3b82f6"
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 4, fill: "#3b82f6" }}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

/* ---------- Ticket status pie chart ---------- */

function TicketStatusPie({
  data,
}: {
  data: { status: string; count: number }[];
}) {
  if (!data.length) {
    return (
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-[var(--text-secondary)]">
            工单状态分布
          </CardTitle>
        </CardHeader>
        <CardContent className="flex h-48 items-center justify-center">
          <p className="text-sm text-[var(--text-muted)]">暂无数据</p>
        </CardContent>
      </Card>
    );
  }

  const chartData = data.map((d) => ({
    name: TICKET_STATUS_LABELS[d.status] || d.status,
    value: d.count,
    color: TICKET_COLORS[d.status] || "#6b7280",
  }));

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-[var(--text-secondary)]">
          <FileText size={16} className="mr-1.5 inline-block" />
          工单状态分布
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-56">
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie
                data={chartData}
                dataKey="value"
                nameKey="name"
                cx="50%"
                cy="50%"
                innerRadius={40}
                outerRadius={70}
                paddingAngle={2}
                strokeWidth={0}
              >
                {chartData.map((entry, idx) => (
                  <Cell key={idx} fill={entry.color} />
                ))}
              </Pie>
              <Tooltip
                contentStyle={{
                  backgroundColor: "var(--bg-elevated)",
                  border: "1px solid var(--border-default)",
                  borderRadius: "8px",
                  fontSize: "12px",
                }}
                formatter={(value: number, name: string) => [
                  `${value} 个`,
                  name,
                ]}
              />
            </PieChart>
          </ResponsiveContainer>
          {/* legend */}
          <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1">
            {chartData.map((d) => (
              <div key={d.name} className="flex items-center gap-1 text-xs">
                <span
                  className="h-2.5 w-2.5 rounded-full"
                  style={{ backgroundColor: d.color }}
                />
                <span className="text-[var(--text-secondary)]">{d.name}</span>
                <span className="font-medium text-[var(--text-primary)]">
                  {d.value}
                </span>
              </div>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

/* ---------- Recent activity feed ---------- */

function RecentActivityFeed({
  activities,
}: {
  activities: DashboardOverview["recent_activities"];
}) {
  const navigate = useNavigate();

  if (!activities.length) {
    return (
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-[var(--text-secondary)]">
            最近活动
          </CardTitle>
        </CardHeader>
        <CardContent className="flex h-40 items-center justify-center">
          <p className="text-sm text-[var(--text-muted)]">暂无活动记录</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="col-span-1 md:col-span-3">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-[var(--text-secondary)]">
          <Clock size={16} className="mr-1.5 inline-block" />
          最近活动
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-1">
          {activities.map((item) => (
            <div
              key={item.id}
              className="flex items-center gap-3 rounded-md px-2 py-2 transition-colors hover:bg-[var(--bg-elevated)] cursor-pointer"
              onClick={() => navigate("/audit")}
            >
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-blue-500/10">
                <User size={14} className="text-blue-500" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 text-sm">
                  <span className="font-medium text-[var(--text-primary)] truncate">
                    {item.username || "系统"}
                  </span>
                  <span className="text-xs text-[var(--text-muted)]">
                    {item.action}
                  </span>
                </div>
                <p className="truncate text-xs text-[var(--text-secondary)]">
                  {item.summary}
                </p>
              </div>
              <span className="shrink-0 text-xs text-[var(--text-muted)]">
                {formatTime(item.created_at)}
              </span>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

/* ---------- Main Dashboard Page ---------- */

export default function DashboardPage() {
  const [overview, setOverview] = useState<DashboardOverview | null>(null);
  const [sensitiveCount, setSensitiveCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [timeRange, setTimeRange] = useState<TimeRange>("last_30d");

  const fetchOverview = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await getDashboardOverview(timeRange);
      if (res.code === 0) setOverview(res.data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "获取看板数据失败");
    } finally {
      setLoading(false);
    }
  }, [timeRange]);

  useEffect(() => {
    fetchOverview();
  }, [fetchOverview]);

  useEffect(() => {
    listSensitiveTables({ page_size: 1 })
      .then((res) => setSensitiveCount(res.total ?? 0))
      .catch(() => {});
  }, []);

  // Error state
  if (error && !overview) {
    return (
      <div className="mx-auto max-w-[1200px] page-transition">
        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
          数据看板
        </h1>
        <div className="flex h-48 flex-col items-center justify-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-full bg-red-500/10">
            <AlertCircle size={24} className="text-red-400" />
          </div>
          <div className="text-center">
            <p className="text-sm font-medium text-[var(--text-secondary)]">
              加载失败
            </p>
            <p className="mt-1 text-xs text-[var(--text-muted)]">{error}</p>
          </div>
          <Button variant="outline" size="sm" onClick={fetchOverview}>
            重试
          </Button>
        </div>
      </div>
    );
  }

  // Loading skeleton
  if (loading && !overview) {
    return (
      <div className="mx-auto max-w-[1200px] page-transition">
        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
          数据看板
        </h1>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <Card key={i}>
              <CardContent className="py-5">
                <div className="flex items-center gap-4">
                  <div className="h-10 w-10 rounded-lg bg-[var(--bg-elevated)] animate-pulse" />
                  <div className="flex-1 space-y-2">
                    <div className="h-6 w-16 rounded bg-[var(--bg-elevated)] animate-pulse" />
                    <div className="h-4 w-24 rounded bg-[var(--bg-elevated)] animate-pulse" />
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    );
  }

  const stats = overview?.stats;

  return (
    <div className="mx-auto max-w-[1200px] space-y-6 page-transition">
      {/* Header with time range selector and refresh */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          数据看板
        </h1>
        <div className="flex items-center gap-2">
          <div className="flex items-center rounded-lg border border-[var(--border-default)] bg-[var(--bg-card)] p-0.5">
            {TIME_RANGES.map((tr) => (
              <button
                key={tr.key}
                className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                  timeRange === tr.key
                    ? "bg-[var(--bg-primary)] text-[var(--text-primary)] shadow-sm"
                    : "text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                }`}
                onClick={() => setTimeRange(tr.key)}
              >
                {tr.label}
              </button>
            ))}
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={fetchOverview}
            disabled={loading}
            className="text-[var(--text-secondary)]"
          >
            <RefreshCw
              size={14}
              className={loading ? "animate-spin" : ""}
            />
          </Button>
        </div>
      </div>

      {/* Bento Grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Row 1: 3 stat cards */}
        <StatCard
          label="待审批工单"
          value={stats?.pending_tickets ?? 0}
          icon={<FileText size={20} className="text-blue-500" />}
          colorClass="text-blue-500"
          bgClass="bg-blue-500/10"
          sparkline={overview?.ticket_sparkline ?? []}
          link="/tickets?status=PENDING_APPROVAL"
        />
        <StatCard
          label="查询次数"
          value={stats?.recent_queries_7d ?? 0}
          icon={<Database size={20} className="text-emerald-500" />}
          colorClass="text-emerald-500"
          bgClass="bg-emerald-500/10"
          sparkline={overview?.query_sparkline ?? []}
        />
        <StatCard
          label="敏感表"
          value={sensitiveCount}
          icon={<Server size={20} className="text-red-500" />}
          colorClass="text-red-500"
          bgClass="bg-red-500/10"
          sparkline={[]}
          link="/settings/mask-rules"
        />

        {/* Row 2: query trend (col-span-2) + ticket status pie (col-span-1) */}
        <QueryTrendChart data={overview?.query_trend ?? []} />
        <TicketStatusPie data={overview?.ticket_status_dist ?? []} />

        {/* Row 3: activity feed (full width) */}
        <RecentActivityFeed activities={overview?.recent_activities ?? []} />
      </div>
    </div>
  );
}
