import { useState, useEffect, useCallback } from "react";
import {
  Clock,
  TrendingUp,
  AlertTriangle,
  Gauge,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import {
  getSlowQueries,
  getPerformanceStats,
  type PerformanceStats,
  type SlowQueryItem,
} from "@/api/performance";

// --- Stat Cards ---

function StatCard({
  icon: Icon,
  label,
  value,
  color,
  bg,
  sub,
}: {
  icon: React.ElementType;
  label: string;
  value: string | number;
  color: string;
  bg: string;
  sub?: string;
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 py-5">
        <div
          className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${bg}`}
        >
          <Icon size={20} className={color} />
        </div>
        <div>
          <div className="text-2xl font-bold text-[var(--text-primary)]">
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

// --- Slow Query Table ---

function SlowQueryTable({
  items,
  loading,
}: {
  items: SlowQueryItem[];
  loading: boolean;
}) {
  if (loading) {
    return (
      <div className="flex h-32 items-center justify-center text-sm text-[var(--text-muted)]">
        加载中...
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="flex h-32 flex-col items-center justify-center gap-2 text-sm text-[var(--text-muted)]">
        <Clock size={24} />
        <span>暂无慢查询记录</span>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
            <th className="whitespace-nowrap px-3 py-2 font-medium">SQL 摘要</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">执行时间</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">数据库</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">类型</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">时间</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr
              key={item.id}
              className="border-b border-[var(--border-subtle)] transition-colors hover:bg-[var(--bg-surface)]"
            >
              <td className="max-w-[300px] truncate px-3 py-2 font-mono text-xs text-[var(--text-primary)]">
                {item.sql_summary || item.sql_content}
              </td>
              <td className="whitespace-nowrap px-3 py-2">
                <span
                  className={`rounded px-1.5 py-0.5 text-xs font-medium ${
                    item.execution_time >= 3000
                      ? "bg-red-500/10 text-red-500"
                      : item.execution_time >= 1000
                        ? "bg-orange-500/10 text-orange-500"
                        : "bg-yellow-500/10 text-yellow-500"
                  }`}
                >
                  {item.execution_time}ms
                </span>
              </td>
              <td className="whitespace-nowrap px-3 py-2 text-[var(--text-secondary)]">
                {item.database || "—"}
              </td>
              <td className="whitespace-nowrap px-3 py-2 text-[var(--text-muted)]">
                {item.db_type}
              </td>
              <td className="whitespace-nowrap px-3 py-2 text-[var(--text-muted)]">
                {item.created_at}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// --- Trend Table ---

function TrendTable({ stats }: { stats: PerformanceStats | null }) {
  if (!stats || stats.daily_trend.length === 0) {
    return (
      <div className="flex h-24 items-center justify-center text-sm text-[var(--text-muted)]">
        暂无趋势数据
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
            <th className="whitespace-nowrap px-3 py-2 font-medium">日期</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">查询数</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">平均耗时</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">慢查询数</th>
          </tr>
        </thead>
        <tbody>
          {stats.daily_trend.map((d) => (
            <tr
              key={d.date}
              className="border-b border-[var(--border-subtle)] hover:bg-[var(--bg-surface)]"
            >
              <td className="whitespace-nowrap px-3 py-2 text-[var(--text-primary)]">
                {d.date}
              </td>
              <td className="whitespace-nowrap px-3 py-2 text-[var(--text-secondary)]">
                {d.count}
              </td>
              <td className="whitespace-nowrap px-3 py-2 text-[var(--text-secondary)]">
                {d.avg_time}ms
              </td>
              <td className="whitespace-nowrap px-3 py-2">
                {d.slow_count > 0 ? (
                  <span className="rounded bg-orange-500/10 px-1.5 py-0.5 text-xs font-medium text-orange-500">
                    {d.slow_count}
                  </span>
                ) : (
                  <span className="text-[var(--text-muted)]">0</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// --- Top Slow Queries ---

function TopSlowTable({ stats }: { stats: PerformanceStats | null }) {
  if (!stats || stats.top_slow_queries.length === 0) {
    return (
      <div className="flex h-24 items-center justify-center text-sm text-[var(--text-muted)]">
        暂无数据
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-[var(--border-default)] text-left text-[var(--text-secondary)]">
            <th className="px-3 py-2 font-medium">#</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">SQL 摘要</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">耗时</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">数据源</th>
            <th className="whitespace-nowrap px-3 py-2 font-medium">时间</th>
          </tr>
        </thead>
        <tbody>
          {stats.top_slow_queries.map((q, i) => (
            <tr
              key={q.id}
              className="border-b border-[var(--border-subtle)] hover:bg-[var(--bg-surface)]"
            >
              <td className="px-3 py-2 text-[var(--text-muted)]">{i + 1}</td>
              <td className="max-w-[260px] truncate px-3 py-2 font-mono text-xs text-[var(--text-primary)]">
                {q.sql_summary}
              </td>
              <td className="whitespace-nowrap px-3 py-2">
                <span className="rounded bg-red-500/10 px-1.5 py-0.5 text-xs font-medium text-red-500">
                  {q.execution_time}ms
                </span>
              </td>
              <td className="whitespace-nowrap px-3 py-2 text-[var(--text-secondary)]">
                {q.datasource_name}
              </td>
              <td className="whitespace-nowrap px-3 py-2 text-[var(--text-muted)]">
                {q.created_at}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// --- Main Page ---

export default function PerformancePage() {
  const [stats, setStats] = useState<PerformanceStats | null>(null);
  const [slowQueries, setSlowQueries] = useState<SlowQueryItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [slowLoading, setSlowLoading] = useState(false);

  const [threshold, setThreshold] = useState(1000);
  const [days, setDays] = useState(7);
  const [page, setPage] = useState(1);
  const pageSize = 20;

  // Fetch stats
  useEffect(() => {
    setLoading(true);
    getPerformanceStats(days)
      .then((data) => setStats(data))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [days]);

  // Fetch slow queries
  const fetchSlowQueries = useCallback(() => {
    setSlowLoading(true);
    getSlowQueries({ threshold, page, page_size: pageSize })
      .then((res) => {
        setSlowQueries(res.data || []);
        setTotal(res.total);
      })
      .catch(() => {})
      .finally(() => setSlowLoading(false));
  }, [threshold, page]);

  useEffect(() => {
    fetchSlowQueries();
  }, [fetchSlowQueries]);

  const totalPages = Math.ceil(total / pageSize);

  function formatTime(ms: number): string {
    if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
    return `${ms}ms`;
  }

  return (
    <div className="mx-auto max-w-[1200px] space-y-6 page-transition">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          性能分析
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
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Stat Cards */}
      <div className={`grid grid-cols-2 gap-5 md:grid-cols-4${loading ? " animate-pulse" : ""}`}>
        <StatCard
          icon={TrendingUp}
          label="总查询数"
          value={stats?.total_queries ?? 0}
          color="text-blue-500"
          bg="bg-blue-500/10"
        />
        <StatCard
          icon={AlertTriangle}
          label="慢查询数"
          value={stats?.slow_queries ?? 0}
          color="text-red-500"
          bg="bg-red-500/10"
          sub={`占比 ${stats?.slow_query_rate.toFixed(1) ?? 0}%`}
        />
        <StatCard
          icon={Gauge}
          label="平均耗时"
          value={formatTime(stats?.avg_time ?? 0)}
          color="text-green-500"
          bg="bg-green-500/10"
        />
        <StatCard
          icon={Clock}
          label="统计天数"
          value={days}
          color="text-purple-500"
          bg="bg-purple-500/10"
        />
      </div>

      {/* Slow Query List */}
      <Card>
        <CardContent className="space-y-4 p-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-medium text-[var(--text-primary)]">
              慢查询列表
            </h2>
            <div className="flex items-center gap-3">
              <span className="text-xs text-[var(--text-muted)]">阈值</span>
              <Select
                value={String(threshold)}
                onValueChange={(v) => {
                  setThreshold(Number(v));
                  setPage(1);
                }}
              >
                <SelectTrigger className="h-7 w-24 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="500">500ms</SelectItem>
                  <SelectItem value="1000">1000ms</SelectItem>
                  <SelectItem value="3000">3000ms</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <SlowQueryTable items={slowQueries} loading={slowLoading} />
          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between pt-2">
              <span className="text-xs text-[var(--text-muted)]">
                共 {total} 条，第 {page}/{totalPages} 页
              </span>
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 w-7 p-0"
                  disabled={page <= 1}
                  onClick={() => setPage(page - 1)}
                >
                  <ChevronLeft size={14} />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 w-7 p-0"
                  disabled={page >= totalPages}
                  onClick={() => setPage(page + 1)}
                >
                  <ChevronRight size={14} />
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Bottom grid: Daily Trend + Top Slow */}
      <div className="grid gap-5 md:grid-cols-2">
        <Card>
          <CardContent className="space-y-3 p-4">
            <h2 className="text-sm font-medium text-[var(--text-primary)]">
              按天趋势
            </h2>
            <TrendTable stats={stats} />
          </CardContent>
        </Card>
        <Card>
          <CardContent className="space-y-3 p-4">
            <h2 className="text-sm font-medium text-[var(--text-primary)]">
              TOP 10 慢查询
            </h2>
            <TopSlowTable stats={stats} />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
