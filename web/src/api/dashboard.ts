import { api } from "./client";

// --- Basic Stats (existing, unchanged) ---

export interface DashboardStats {
  pending_tickets: number;
  recent_queries_7d: number;
  active_datasources: number;
  total_users: number;
  sensitive_tables?: number;
}

export async function getDashboardStats(): Promise<{
  code: number;
  data: DashboardStats;
}> {
  return api.get("/dashboard/stats");
}

// --- Full Dashboard Data (SF-FEAT0046-BE backed) ---

export interface TimeRange {
  key: string;
  label: string;
  startDate: string;
  endDate: string;
}

export function getTimeRanges(): TimeRange[] {
  const today = new Date();
  const fmt = (d: Date) => d.toISOString().slice(0, 10);

  const daysAgo = (n: number) => {
    const d = new Date(today);
    d.setDate(d.getDate() - n);
    return fmt(d);
  };

  return [
    { key: "today", label: "今日", startDate: fmt(today), endDate: fmt(today) },
    { key: "week", label: "本周", startDate: daysAgo(6), endDate: fmt(today) },
    { key: "month", label: "本月", startDate: daysAgo(29), endDate: fmt(today) },
    { key: "30d", label: "近30天", startDate: daysAgo(29), endDate: fmt(today) },
  ];
}

// Backend response shape (DashboardFullStats)
export interface DashboardFullStatsRaw {
  pending_tickets: number;
  recent_queries_7d: number;
  active_datasources: number;
  pending_ticket_sparkline: number[];  // 7 elements
  query_sparkline: number[];           // 7 elements
  datasource_sparkline: number[];      // 7 elements
  ticket_status_distribution: Record<string, number>;
  query_trend: { date: string; count: number }[];
  recent_activity: {
    id: number;
    user_id: number;
    action: string;
    ip_address: string;
    created_at: string;
  }[];
}

// Frontend-friendly shape
export interface SparklinePoint {
  date: string;
  value: number;
}

export interface StatCardData {
  value: number;
  sparkline: SparklinePoint[];
  trend: number;
}

export interface DashboardOverview {
  pending_tickets: StatCardData;
  query_count: StatCardData;
  active_datasources: StatCardData;
  query_trend: { date: string; count: number }[];
  ticket_distribution: { status: string; count: number }[];
  recent_activity: {
    id: number;
    user: string;
    action: string;
    target: string;
    timestamp: string;
  }[];
}

// Convert sparkline int[7] → SparklinePoint[7] with dates
function sparklineToPoints(values: number[]): SparklinePoint[] {
  const today = new Date();
  return values.map((v, i) => {
    const d = new Date(today);
    d.setDate(d.getDate() - (values.length - 1 - i));
    return { date: d.toISOString().slice(0, 10), value: v };
  });
}

// Compute trend percentage from sparkline (last vs first, or last 3 avg vs first 3 avg)
function computeTrend(values: number[]): number {
  if (!values || values.length < 2) return 0;
  const first = values[0] || 0;
  const last = values[values.length - 1] || 0;
  if (first === 0 && last === 0) return 0;
  if (first === 0) return 100;
  return Math.round(((last - first) / first) * 100);
}

// Map backend status keys to display labels
const STATUS_LABELS: Record<string, string> = {
  PENDING_APPROVAL: "待审批",
  APPROVED: "已通过",
  REJECTED: "已拒绝",
  DONE: "已完成",
  EXECUTING: "执行中",
  CANCELLED: "已取消",
  SUBMITTED: "已提交",
  AI_REVIEWED: "AI 已评审",
};

export async function getDashboardOverview(
  timeRange: TimeRange,
): Promise<{ code: number; data: DashboardOverview }> {
  const res = await api.get<{ code: number; data: DashboardFullStatsRaw }>(
    `/dashboard/overview?start_date=${timeRange.startDate}&end_date=${timeRange.endDate}`,
  );

  const raw = res.data;

  // Transform to frontend shape
  const overview: DashboardOverview = {
    pending_tickets: {
      value: raw.pending_tickets,
      sparkline: sparklineToPoints(raw.pending_ticket_sparkline),
      trend: computeTrend(raw.pending_ticket_sparkline),
    },
    query_count: {
      value: raw.recent_queries_7d,
      sparkline: sparklineToPoints(raw.query_sparkline),
      trend: computeTrend(raw.query_sparkline),
    },
    active_datasources: {
      value: raw.active_datasources,
      sparkline: sparklineToPoints(raw.datasource_sparkline),
      trend: computeTrend(raw.datasource_sparkline),
    },
    query_trend: raw.query_trend,
    ticket_distribution: Object.entries(raw.ticket_status_distribution).map(
      ([status, count]) => ({
        status: STATUS_LABELS[status] || status,
        count,
      }),
    ),
    recent_activity: raw.recent_activity.map((a) => ({
      id: Number(a.id),
      user: `用户#${a.user_id}`,
      action: a.action,
      target: a.ip_address || "",
      timestamp: a.created_at,
    })),
  };

  return { code: 0, data: overview };
}
