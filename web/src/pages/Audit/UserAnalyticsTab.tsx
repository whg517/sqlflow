import { useState, useCallback, useMemo } from "react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  LineChart,
  Line,
  CartesianGrid,
} from "recharts";
import {
  Activity,
  AlertTriangle,
  Loader2,
  TrendingUp,
  ChevronDown,
  ChevronRight,
  RefreshCw,
  ShieldAlert,
  Clock3,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  getUserAnalytics,
  getActionLabel,
  getAnomalyLabel,
  TIME_RANGE_OPTIONS,
  ACTION_COLORS,
  type UserAnalytics,
  type ActiveUserEntry,
} from "@/api/userAnalytics";

// --- Config ---

const FREQUENCY_DIMENSIONS = [
  { value: "day", label: "按天" },
  { value: "week", label: "按周" },
  { value: "month", label: "按月" },
] as const;

type FrequencyDimension = (typeof FREQUENCY_DIMENSIONS)[number]["value"];

// ==========================================
// User Analytics Tab
// ==========================================

interface UserAnalyticsTabProps {
  isAdmin: boolean;
}

export default function UserAnalyticsTab({ isAdmin }: UserAnalyticsTabProps) {
  if (!isAdmin) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="text-center">
          <ShieldAlert size={32} className="mx-auto mb-2 text-[var(--text-muted)]" />
          <p className="text-sm text-[var(--text-muted)]">
            仅管理员可查看用户行为分析
          </p>
        </div>
      </div>
    );
  }

  return <AnalyticsContent />;
}

// ==========================================
// Analytics Content
// ==========================================

function AnalyticsContent() {
  const [data, setData] = useState<UserAnalytics | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [timeRange, setTimeRange] = useState<string>("7d");
  const [freqDimension, setFreqDimension] = useState<FrequencyDimension>("day");
  const [drillUser, setDrillUser] = useState<ActiveUserEntry | null>(null);
  const [expandedAnomalies, setExpandedAnomalies] = useState<Set<number>>(new Set());

  const fetchData = useCallback(async (range: string) => {
    setLoading(true);
    setError(null);
    try {
      const res = await getUserAnalytics({ time_range: range });
      setData(res.data);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "获取分析数据失败";
      setError(msg);
      toast.error(msg);
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial load: loading-guarded fetch (avoids StrictMode double render)
  if (loading && !data && !error) {
    void fetchData(timeRange);
  }

  // Re-fetch when timeRange changes (user interaction)
  const [prevRange, setPrevRange] = useState(timeRange);
  if (prevRange !== timeRange) {
    setPrevRange(timeRange);
    void fetchData(timeRange);
  }

  function handleRefresh() {
    void fetchData(timeRange);
  }

  function toggleAnomaly(idx: number) {
    setExpandedAnomalies((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) next.delete(idx);
      else next.add(idx);
      return next;
    });
  }

  // --- Render ---

  if (loading && !data && !error) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-[var(--text-muted)]" />
      </div>
    );
  }

  if (error && !data) {
    return (
      <div className="flex h-64 flex-col items-center justify-center gap-3">
        <AlertTriangle size={24} className="text-[var(--text-muted)]" />
        <p className="text-sm text-[var(--text-muted)]">数据加载失败</p>
        <Button variant="outline" size="sm" onClick={handleRefresh} className="gap-1">
          <RefreshCw size={14} />
          重试
        </Button>
      </div>
    );
  }

  if (!data) return null;

  const hasData =
    data.top_active_users.length > 0 ||
    data.query_frequency.length > 0 ||
    data.action_type_breakdown.length > 0;

  if (!hasData) {
    return (
      <div className="space-y-4">
        <AnalyticsToolbar
          timeRange={timeRange}
          onTimeRangeChange={setTimeRange}
          onRefresh={handleRefresh}
          loading={loading}
        />
        <div className="flex h-64 flex-col items-center justify-center gap-2">
          <Activity size={24} className="text-[var(--text-muted)]" />
          <p className="text-sm text-[var(--text-muted)]">
            当前时间范围内暂无审计数据
          </p>
          <p className="text-xs text-[var(--text-muted)]">
            用户操作后，分析数据将在 10 分钟内更新
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* Toolbar */}
      <AnalyticsToolbar
        timeRange={timeRange}
        onTimeRangeChange={(v) => {
          setDrillUser(null);
          setTimeRange(v);
        }}
        onRefresh={handleRefresh}
        loading={loading}
      />

      {/* Drill user banner */}
      {drillUser && (
        <div className="flex items-center gap-2 rounded-lg border border-[var(--accent-primary)]/30 bg-[var(--accent-primary)]/5 px-4 py-2">
          <TrendingUp size={14} className="text-[var(--accent-primary)]" />
          <span className="text-sm text-[var(--text-secondary)]">
            下钻用户：<span className="font-medium text-[var(--text-primary)]">{drillUser.username}</span>
          </span>
          <span className="text-xs text-[var(--text-muted)]">
            总操作 {drillUser.total_actions} 次 / 查询 {drillUser.query_count} 次 / 活跃 {drillUser.active_days} 天
          </span>
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto h-7 text-xs"
            onClick={() => setDrillUser(null)}
          >
            退出下钻
          </Button>
        </div>
      )}

      {/* Top row: Bar chart + Pie chart */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* TOP 10 Active Users */}
        <ChartCard title="TOP 10 活跃用户" icon={<BarChart className="h-4 w-4" />}>
          <TopUsersChart
            users={data.top_active_users}
            onUserClick={(user) => setDrillUser(user)}
          />
        </ChartCard>

        {/* Action Type Breakdown */}
        <ChartCard title="操作类型占比" icon={<Activity className="h-4 w-4" />}>
          <ActionTypeChart breakdown={data.action_type_breakdown} />
        </ChartCard>
      </div>

      {/* Middle: Query Frequency Trend */}
      <ChartCard
        title="查询频率趋势"
        icon={<TrendingUp className="h-4 w-4" />}
        extra={
          <Select
            value={freqDimension}
            onValueChange={(v) => setFreqDimension(v as FrequencyDimension)}
          >
            <SelectTrigger className="h-7 w-24 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FREQUENCY_DIMENSIONS.map((dim) => (
                <SelectItem key={dim.value} value={dim.value} className="text-xs">
                  {dim.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      >
        <QueryFrequencyChart
          frequency={data.query_frequency}
          dimension={freqDimension}
        />
      </ChartCard>

      {/* Bottom: Anomalous Behaviors */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-4">
        <div className="mb-3 flex items-center gap-2">
          <AlertTriangle size={16} className="text-amber-400" />
          <h3 className="text-sm font-medium text-[var(--text-primary)]">
            异常行为检测
          </h3>
          {data.anomalous_behaviors.length > 0 ? (
            <Badge className="border-0 bg-red-500/20 text-red-400 text-[10px]">
              {data.anomalous_behaviors.length} 项异常
            </Badge>
          ) : (
            <Badge className="border-0 bg-emerald-500/20 text-emerald-400 text-[10px]">
              无异常
            </Badge>
          )}
        </div>

        {data.anomalous_behaviors.length === 0 ? (
          <div className="flex h-20 items-center justify-center">
            <p className="text-sm text-[var(--text-muted)]">
              ✅ 当前时间范围内未检测到异常行为
            </p>
          </div>
        ) : (
          <div className="space-y-2">
            {data.anomalous_behaviors.map((anomaly, idx) => {
              const expanded = expandedAnomalies.has(idx);
              return (
                <div
                  key={`${anomaly.user_id}-${anomaly.anomaly_type}-${idx}`}
                  className="rounded-lg border border-red-500/20 bg-red-500/5"
                >
                  <button
                    type="button"
                    className="flex w-full items-center gap-2 px-3 py-2 text-left"
                    onClick={() => toggleAnomaly(idx)}
                  >
                    {expanded ? (
                      <ChevronDown size={14} className="shrink-0 text-[var(--text-muted)]" />
                    ) : (
                      <ChevronRight size={14} className="shrink-0 text-[var(--text-muted)]" />
                    )}
                    <AlertTriangle size={14} className="shrink-0 text-red-400" />
                    <span className="text-sm font-medium text-[var(--text-primary)]">
                      {getAnomalyLabel(anomaly.anomaly_type)}
                    </span>
                    <span className="text-xs text-[var(--text-secondary)]">
                      {anomaly.username}
                    </span>
                    <Badge className="ml-auto border-0 bg-red-500/20 text-red-400 text-[10px]">
                      {anomaly.count} 次
                    </Badge>
                  </button>
                  {expanded && (
                    <div className="border-t border-red-500/10 px-3 py-2">
                      <p className="text-xs text-[var(--text-secondary)]">
                        {anomaly.description}
                      </p>
                      <p className="mt-1 text-[10px] text-[var(--text-muted)]">
                        时间窗口：{anomaly.time_window}
                      </p>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Data freshness */}
      <div className="flex items-center justify-end gap-2 text-xs text-[var(--text-muted)]">
        <Clock3 size={12} />
        <span>
          数据生成于 {new Date(data.generated_at).toLocaleString("zh-CN")}
        </span>
      </div>
    </div>
  );
}

// ==========================================
// Toolbar
// ==========================================

interface ToolbarProps {
  timeRange: string;
  onTimeRangeChange: (v: string) => void;
  onRefresh: () => void;
  loading: boolean;
}

function AnalyticsToolbar({ timeRange, onTimeRangeChange, onRefresh, loading }: ToolbarProps) {
  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-2">
        <span className="text-sm text-[var(--text-secondary)]">时间范围</span>
        <Select value={timeRange} onValueChange={onTimeRangeChange}>
          <SelectTrigger className="h-8 w-32 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIME_RANGE_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value} className="text-xs">
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Button
        variant="ghost"
        size="sm"
        className="gap-1 text-xs"
        onClick={onRefresh}
        disabled={loading}
      >
        <RefreshCw size={13} className={loading ? "animate-spin" : ""} />
        刷新
      </Button>
    </div>
  );
}

// ==========================================
// Chart Card Wrapper
// ==========================================

interface ChartCardProps {
  title: string;
  icon?: React.ReactNode;
  extra?: React.ReactNode;
  children: React.ReactNode;
}

function ChartCard({ title, icon, extra, children }: ChartCardProps) {
  return (
    <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          {icon && <span className="text-[var(--accent-primary)]">{icon}</span>}
          <h3 className="text-sm font-medium text-[var(--text-primary)]">{title}</h3>
        </div>
        {extra}
      </div>
      {children}
    </div>
  );
}

// ==========================================
// TOP 10 Active Users Bar Chart
// ==========================================

interface TopUsersChartProps {
  users: ActiveUserEntry[];
  onUserClick: (user: ActiveUserEntry) => void;
}

function TopUsersChart({ users, onUserClick }: TopUsersChartProps) {
  const chartData = users.map((u) => ({
    name: u.username,
    total: u.total_actions,
    queries: u.query_count,
    approvals: u.approval_count,
    raw: u,
  }));

  if (chartData.length === 0) {
    return <EmptyChart />;
  }

  return (
    <ResponsiveContainer width="100%" height={280}>
      <BarChart
        data={chartData}
        margin={{ top: 5, right: 10, left: 0, bottom: 5 }}
        onClick={(e) => {
          if (e && e.activePayload && e.activePayload[0]) {
            const raw = e.activePayload[0].payload?.raw as ActiveUserEntry | undefined;
            if (raw) onUserClick(raw);
          }
        }}
      >
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border-default)" vertical={false} />
        <XAxis
          dataKey="name"
          tick={{ fontSize: 11, fill: "var(--text-muted)" }}
          angle={-30}
          textAnchor="end"
          height={60}
        />
        <YAxis tick={{ fontSize: 11, fill: "var(--text-muted)" }} />
        <RechartsTooltip
          cursor={{ fill: "var(--bg-elevated)", opacity: 0.5 }}
          content={({ active, payload }) => {
            if (!active || !payload || payload.length === 0) return null;
            const d = payload[0].payload;
            return (
              <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-2 text-xs shadow-lg">
                <p className="font-medium text-[var(--text-primary)]">{d.name}</p>
                <p className="text-[var(--text-secondary)]">总操作: {d.total}</p>
                <p className="text-[var(--text-secondary)]">查询: {d.queries}</p>
                <p className="text-[var(--text-secondary)]">审批: {d.approvals}</p>
                <p className="mt-1 text-[10px] text-[var(--text-muted)]">点击下钻 →</p>
              </div>
            );
          }}
        />
        <Bar dataKey="total" radius={[4, 4, 0, 0]} fill="#3b82f6" />
      </BarChart>
    </ResponsiveContainer>
  );
}

// ==========================================
// Action Type Pie Chart
// ==========================================

interface ActionTypeChartProps {
  breakdown: { action: string; count: number; ratio: number }[];
}

function ActionTypeChart({ breakdown }: ActionTypeChartProps) {
  const chartData = breakdown.map((item) => ({
    name: getActionLabel(item.action),
    value: item.count,
    ratio: item.ratio,
  }));

  if (chartData.length === 0) {
    return <EmptyChart />;
  }

  return (
    <div className="flex items-center gap-4">
      <ResponsiveContainer width="60%" height={220}>
        <PieChart>
          <Pie
            data={chartData}
            dataKey="value"
            nameKey="name"
            cx="50%"
            cy="50%"
            innerRadius={50}
            outerRadius={80}
            paddingAngle={2}
          >
            {chartData.map((_, idx) => (
              <Cell key={idx} fill={ACTION_COLORS[idx % ACTION_COLORS.length]} />
            ))}
          </Pie>
          <RechartsTooltip
            content={({ active, payload }) => {
              if (!active || !payload || payload.length === 0) return null;
              const d = payload[0].payload;
              return (
                <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-2 text-xs shadow-lg">
                  <p className="font-medium text-[var(--text-primary)]">{d.name}</p>
                  <p className="text-[var(--text-secondary)]">
                    {d.value} 次 ({(d.ratio * 100).toFixed(1)}%)
                  </p>
                </div>
              );
            }}
          />
        </PieChart>
      </ResponsiveContainer>
      <div className="flex-1 space-y-1.5">
        {chartData.map((item, idx) => (
          <div key={item.name} className="flex items-center gap-2 text-xs">
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-sm"
              style={{ backgroundColor: ACTION_COLORS[idx % ACTION_COLORS.length] }}
            />
            <span className="text-[var(--text-secondary)]">{item.name}</span>
            <span className="ml-auto font-medium text-[var(--text-primary)]">
              {item.value}
            </span>
            <span className="text-[var(--text-muted)]">
              {(item.ratio * 100).toFixed(0)}%
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ==========================================
// Query Frequency Line Chart
// ==========================================

interface QueryFrequencyChartProps {
  frequency: { period: string; count: number }[];
  dimension: FrequencyDimension;
}

function QueryFrequencyChart({ frequency, dimension }: QueryFrequencyChartProps) {
  // Group data by the selected dimension
  const chartData = useMemo(
    () => aggregateFrequency(frequency, dimension),
    [frequency, dimension],
  );

  if (chartData.length === 0) {
    return <EmptyChart />;
  }

  return (
    <ResponsiveContainer width="100%" height={240}>
      <LineChart data={chartData} margin={{ top: 5, right: 10, left: 0, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border-default)" />
        <XAxis
          dataKey="label"
          tick={{ fontSize: 11, fill: "var(--text-muted)" }}
        />
        <YAxis tick={{ fontSize: 11, fill: "var(--text-muted)" }} />
        <RechartsTooltip
          content={({ active, payload, label }) => {
            if (!active || !payload || payload.length === 0) return null;
            return (
              <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-2 text-xs shadow-lg">
                <p className="font-medium text-[var(--text-primary)]">{label}</p>
                <p className="text-[var(--text-secondary)]">
                  查询次数: {payload[0].value}
                </p>
              </div>
            );
          }}
        />
        <Line
          type="monotone"
          dataKey="count"
          stroke="#3b82f6"
          strokeWidth={2}
          dot={{ r: 3, fill: "#3b82f6" }}
          activeDot={{ r: 5 }}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}

// ==========================================
// Helpers
// ==========================================

/** Aggregate raw frequency entries by day/week/month. */
function aggregateFrequency(
  frequency: { period: string; count: number }[],
  dimension: FrequencyDimension,
): { label: string; count: number }[] {
  if (frequency.length === 0) return [];

  if (dimension === "day") {
    return frequency.map((d) => ({
      label: formatDateLabel(d.period),
      count: d.count,
    }));
  }

  // Group by week or month
  const groups = new Map<string, number>();
  for (const entry of frequency) {
    const date = new Date(entry.period);
    if (isNaN(date.getTime())) continue;

    let key: string;
    if (dimension === "week") {
      // Get the Monday of the week
      const monday = new Date(date);
      const day = monday.getDay();
      const diff = day === 0 ? -6 : 1 - day;
      monday.setDate(monday.getDate() + diff);
      key = monday.toISOString().slice(0, 10);
    } else {
      key = date.toISOString().slice(0, 7); // YYYY-MM
    }

    groups.set(key, (groups.get(key) ?? 0) + entry.count);
  }

  return Array.from(groups.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, count]) => ({
      label: formatDateLabel(key),
      count,
    }));
}

function formatDateLabel(dateStr: string): string {
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return dateStr;
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

function EmptyChart() {
  return (
    <div className="flex h-[200px] items-center justify-center">
      <span className="text-sm text-[var(--text-muted)]">暂无数据</span>
    </div>
  );
}
