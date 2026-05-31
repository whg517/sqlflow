import { useState, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import {
  FileText,
  Database,
  Server,
  RefreshCw,
  Loader2,
  AlertCircle,
  Clock,
  ChevronRight,
  TrendingUp,
  TrendingDown,
  Minus,
} from "lucide-react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from "recharts";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import {
  getDashboardOverview,
  getTimeRanges,
  type DashboardOverview,
  type TimeRange,
  type SparklinePoint,
} from "@/api/dashboard";

// --- Config ---

const TIME_RANGES = getTimeRanges();

const PIE_COLORS = ["#3b82f6", "#22c55e", "#ef4444", "#6b7280"];

// STATUS_COLORS reserved for future activity feed badges

// --- Sparkline Mini Chart ---

function MiniSparkline({ data, color }: { data: SparklinePoint[]; color: string }) {
  if (!data || data.length === 0) return null;
  const points = data.map((p, i) => {
    const max = Math.max(...data.map((d) => d.value), 1);
    const x = (i / (data.length - 1)) * 100;
    const y = 40 - (p.value / max) * 32; // 32px usable height in 40px viewBox
    return `${x},${y}`;
  });

  return (
    <svg viewBox="0 0 100 40" className="h-8 w-20" preserveAspectRatio="none">
      <polyline
        points={points.join(" ")}
        fill="none"
        stroke={color}
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}

// --- Trend Indicator ---

function TrendBadge({ trend }: { trend: number }) {
  if (trend > 0) {
    return (
      <span className="inline-flex items-center gap-0.5 text-[10px] text-emerald-400">
        <TrendingUp size={10} /> +{trend}%
      </span>
    );
  }
  if (trend < 0) {
    return (
      <span className="inline-flex items-center gap-0.5 text-[10px] text-red-400">
        <TrendingDown size={10} /> {trend}%
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-0.5 text-[10px] text-zinc-400">
      <Minus size={10} /> 0%
    </span>
  );
}

// --- Stat Card ---

interface StatCardConfig {
  label: string;
  icon: typeof FileText;
  color: string;
  bg: string;
  sparkColor: string;
  link?: string;
}

const STAT_CARDS: StatCardConfig[] = [
  {
    label: "待审批工单",
    icon: FileText,
    color: "text-blue-500",
    bg: "bg-blue-500/10",
    sparkColor: "#3b82f6",
    link: "/tickets?status=PENDING_APPROVAL",
  },
  {
    label: "查询次数",
    icon: Database,
    color: "text-emerald-500",
    bg: "bg-emerald-500/10",
    sparkColor: "#22c55e",
  },
  {
    label: "活跃数据源",
    icon: Server,
    color: "text-purple-500",
    bg: "bg-purple-500/10",
    sparkColor: "#a855f7",
    link: "/settings/datasource",
  },
];

function StatCard({
  config,
  value,
  sparkline,
  trend,
  onClick,
}: {
  config: StatCardConfig;
  value: number;
  sparkline: SparklinePoint[];
  trend: number;
  onClick?: () => void;
}) {
  const Icon = config.icon;

  const content = (
    <Card
      className={cn(
        "transition-all duration-200",
        onClick && "cursor-pointer hover:shadow-lg hover:shadow-[var(--shadow-md)]",
      )}
    >
      <CardContent className="p-4">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div
              className={cn(
                "flex h-9 w-9 shrink-0 items-center justify-center rounded-lg",
                config.bg,
              )}
            >
              <Icon size={18} className={config.color} />
            </div>
            <div>
              <div className="text-2xl font-bold text-[var(--text-primary)] tabular-nums">
                {value}
              </div>
              <div className="text-xs text-[var(--text-secondary)]">
                {config.label}
              </div>
            </div>
          </div>
          <div className="flex flex-col items-end gap-1">
            <TrendBadge trend={trend} />
            <MiniSparkline data={sparkline} color={config.sparkColor} />
          </div>
        </div>
      </CardContent>
    </Card>
  );

  if (onClick) {
    return (
      <div onClick={onClick} className="block">
        {content}
      </div>
    );
  }
  return content;
}

// --- Relative Time ---

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);
  if (days > 0) return `${days} 天前`;
  if (hours > 0) return `${hours} 小时前`;
  if (minutes > 0) return `${minutes} 分钟前`;
  return "刚刚";
}

// --- Main Page ---

export default function DashboardPage() {
  const navigate = useNavigate();
  const [data, setData] = useState<DashboardOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [range, setRange] = useState<TimeRange>(TIME_RANGES[1]); // default: week
  const [refreshing, setRefreshing] = useState(false);

  const fetchData = useCallback(
    async (showRefresh = false) => {
      if (showRefresh) setRefreshing(true);
      else setLoading(true);
      setError(null);

      try {
        const res = await getDashboardOverview(range);
        if (res.code === 0) {
          setData(res.data);
        } else {
          setError("获取概览数据失败");
        }
      } catch {
        setError("获取概览数据失败");
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [range],
  );

  // Initial fetch (triggers on mount via loading state)
  if (loading && !error && !data) {
    void fetchData();
  }

  // Error state
  if (error && !data) {
    return (
      <div className="mx-auto max-w-[1200px] page-transition">
        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
          概览
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
          <Button
            variant="outline"
            size="sm"
            className="mt-2 gap-1 text-xs"
            onClick={() => fetchData()}
          >
            <RefreshCw size={12} />
            重试
          </Button>
        </div>
      </div>
    );
  }

  // Loading state
  if (loading) {
    return (
      <div className="mx-auto max-w-[1200px]">
        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
          概览
        </h1>
        <div className="grid grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <Card key={i}>
              <CardContent className="p-4">
                <div className="flex items-center gap-3">
                  <div className="h-9 w-9 rounded-lg bg-zinc-800 animate-pulse" />
                  <div className="flex-1 space-y-2">
                    <div className="h-6 w-12 rounded bg-zinc-800 animate-pulse" />
                    <div className="h-3 w-20 rounded bg-zinc-800 animate-pulse" />
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
        <div className="mt-4 grid grid-cols-3 gap-4">
          <div className="col-span-2 h-64 rounded-lg border border-[var(--border-default)] bg-zinc-800/30 animate-pulse" />
          <div className="h-64 rounded-lg border border-[var(--border-default)] bg-zinc-800/30 animate-pulse" />
        </div>
      </div>
    );
  }

  if (!data) return null;

  const statValues = [
    data.pending_tickets,
    data.query_count,
    data.active_datasources,
  ];

  return (
    <div className="mx-auto max-w-[1200px] space-y-5 page-transition dashboard-grid">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          概览
        </h1>
        <div className="flex items-center gap-3">
          {/* Time range selector */}
          <div className="flex items-center gap-0.5 rounded-lg border border-[var(--border-default)] bg-[var(--bg-elevated)] p-0.5">
            {TIME_RANGES.map((r) => (
              <button
                key={r.value}
                className={cn(
                  "rounded-md px-2.5 py-1 text-xs transition-colors",
                  range.key === r.key
                    ? "bg-[var(--accent-primary)] text-white font-medium"
                    : "text-[var(--text-secondary)] hover:text-[var(--text-primary)]",
                )}
                onClick={() => setRange(r)}
              >
                {r.label}
              </button>
            ))}
          </div>

          {/* Refresh button */}
          <Button
            variant="outline"
            size="sm"
            className="h-7 gap-1 text-xs border-[var(--border-default)]"
            onClick={() => fetchData(true)}
            disabled={refreshing}
          >
            {refreshing ? (
              <Loader2 size={12} className="animate-spin" />
            ) : (
              <RefreshCw size={12} />
            )}
            刷新
          </Button>
        </div>
      </div>

      {/* Bento Grid Layout */}
      <div className="grid grid-cols-3 gap-4">
        {/* Row 1: 3 stat cards */}
        {STAT_CARDS.map((config, i) => (
          <StatCard
            key={config.label}
            config={config}
            value={statValues[i]?.value ?? 0}
            sparkline={statValues[i]?.sparkline ?? []}
            trend={statValues[i]?.trend ?? 0}
            onClick={config.link ? () => navigate(config.link!) : undefined}
          />
        ))}

        {/* Row 2: Query trend (col-span-2) + Ticket distribution (col-span-1) */}
        <Card className="col-span-2">
          <CardContent className="p-4">
            <h3 className="mb-3 text-sm font-medium text-[var(--text-primary)]">
              查询趋势
            </h3>
            <div className="h-52">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={data.query_trend}>
                  <XAxis
                    dataKey="date"
                    tick={{ fontSize: 10, fill: "var(--text-muted)" }}
                    axisLine={false}
                    tickLine={false}
                  />
                  <YAxis
                    tick={{ fontSize: 10, fill: "var(--text-muted)" }}
                    axisLine={false}
                    tickLine={false}
                    width={30}
                  />
                  <RechartsTooltip
                    contentStyle={{
                      backgroundColor: "var(--bg-surface)",
                      border: "1px solid var(--border-default)",
                      borderRadius: "8px",
                      fontSize: "12px",
                    }}
                  />
                  <Line
                    type="monotone"
                    dataKey="count"
                    stroke="#3b82f6"
                    strokeWidth={2}
                    dot={false}
                    activeDot={{ r: 4, strokeWidth: 0, fill: "#3b82f6" }}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        <Card className="col-span-1">
          <CardContent className="p-4">
            <h3 className="mb-3 text-sm font-medium text-[var(--text-primary)]">
              工单状态分布
            </h3>
            <div className="h-40">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={data.ticket_distribution}
                    cx="50%"
                    cy="50%"
                    innerRadius={40}
                    outerRadius={60}
                    paddingAngle={3}
                    dataKey="count"
                    nameKey="status"
                  >
                    {data.ticket_distribution.map((_, i) => (
                      <Cell
                        key={i}
                        fill={PIE_COLORS[i % PIE_COLORS.length]}
                        stroke="none"
                      />
                    ))}
                  </Pie>
                  <RechartsTooltip
                    contentStyle={{
                      backgroundColor: "var(--bg-surface)",
                      border: "1px solid var(--border-default)",
                      borderRadius: "8px",
                      fontSize: "12px",
                    }}
                  />
                </PieChart>
              </ResponsiveContainer>
            </div>
            {/* Legend */}
            <div className="mt-2 flex flex-wrap gap-2">
              {data.ticket_distribution.map((item, i) => (
                <div key={item.status} className="flex items-center gap-1">
                  <span
                    className="inline-block h-2 w-2 rounded-full"
                    style={{ backgroundColor: PIE_COLORS[i % PIE_COLORS.length] }}
                  />
                  <span className="text-[10px] text-[var(--text-secondary)]">
                    {item.status} ({item.count})
                  </span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Row 3: Activity feed (full width) */}
        <Card className="col-span-3">
          <CardContent className="p-4">
            <h3 className="mb-3 text-sm font-medium text-[var(--text-primary)]">
              最近活动
            </h3>
            {data.recent_activity.length === 0 ? (
              <div className="flex h-20 items-center justify-center text-xs text-[var(--text-muted)]">
                暂无活动记录
              </div>
            ) : (
              <div className="space-y-0">
                {data.recent_activity.map((item) => (
                  <Tooltip key={item.id}>
                    <TooltipTrigger asChild>
                      <div className="flex items-center gap-3 border-b border-[var(--border-default)] py-2.5 last:border-0 hover:bg-[var(--bg-elevated)] transition-colors cursor-default">
                        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--bg-elevated)]">
                          <Clock size={12} className="text-[var(--text-muted)]" />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="text-xs font-medium text-[var(--text-primary)]">
                              {item.user}
                            </span>
                            <span className="text-xs text-[var(--text-muted)]">
                              {item.action}
                            </span>
                          </div>
                          <p className="mt-0.5 text-xs text-[var(--text-muted)] truncate">
                            {item.target}
                          </p>
                        </div>
                        <span className="shrink-0 text-[10px] text-[var(--text-muted)]">
                          {relativeTime(item.timestamp)}
                        </span>
                        <ChevronRight
                          size={12}
                          className="shrink-0 text-[var(--text-muted)]"
                        />
                      </div>
                    </TooltipTrigger>
                    <TooltipContent side="left" className="text-xs">
                      {new Date(item.timestamp).toLocaleString("zh-CN")}
                    </TooltipContent>
                  </Tooltip>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
