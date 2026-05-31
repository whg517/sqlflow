import { api } from "./client";

// --- Basic Stats (existing) ---

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

// --- Extended Dashboard Data (SF-FEAT0046) ---

export interface SparklinePoint {
  date: string; // YYYY-MM-DD
  value: number;
}

export interface QueryTrendPoint {
  date: string;
  count: number;
}

export interface TicketStatusDistribution {
  status: string;
  count: number;
}

export interface ActivityItem {
  id: number;
  user: string;
  action: string;
  target: string;
  timestamp: string;
}

export interface DashboardOverview {
  // Stat cards with sparkline
  pending_tickets: { value: number; sparkline: SparklinePoint[]; trend: number };
  query_count: { value: number; sparkline: SparklinePoint[]; trend: number };
  active_datasources: { value: number; sparkline: SparklinePoint[]; trend: number };
  // Charts
  query_trend: QueryTrendPoint[];
  ticket_distribution: TicketStatusDistribution[];
  // Activity feed
  recent_activity: ActivityItem[];
}

export type TimeRange = "today" | "week" | "month" | "30d";

export async function getDashboardOverview(
  range: TimeRange = "week",
): Promise<{ code: number; data: DashboardOverview }> {
  return api.get(`/dashboard/overview?range=${range}`);
}

// --- Fallback: build overview from existing APIs when backend not ready ---

export async function getDashboardOverviewFallback(): Promise<DashboardOverview> {
  const statsRes = await getDashboardStats();
  const stats = statsRes.code === 0 ? statsRes.data : null;

  // Generate mock sparkline data (7 days)
  const now = new Date();
  const sparkline = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(now);
    d.setDate(d.getDate() - (6 - i));
    return {
      date: d.toISOString().slice(0, 10),
      value: Math.floor(Math.random() * 20) + (stats?.recent_queries_7d ?? 10) / 7,
    };
  });

  const queryTrend = Array.from({ length: 30 }, (_, i) => {
    const d = new Date(now);
    d.setDate(d.getDate() - (29 - i));
    return {
      date: `${d.getMonth() + 1}/${d.getDate()}`,
      count: Math.floor(Math.random() * 30) + 5,
    };
  });

  return {
    pending_tickets: {
      value: stats?.pending_tickets ?? 0,
      sparkline: sparkline.map((p) => ({ ...p, value: Math.floor(Math.random() * 10) })),
      trend: Math.floor(Math.random() * 40) - 20,
    },
    query_count: {
      value: stats?.recent_queries_7d ?? 0,
      sparkline,
      trend: Math.floor(Math.random() * 30),
    },
    active_datasources: {
      value: stats?.active_datasources ?? 0,
      sparkline: sparkline.map((p) => ({ ...p, value: stats?.active_datasources ?? 3 })),
      trend: 0,
    },
    query_trend: queryTrend,
    ticket_distribution: [
      { status: "待审批", count: stats?.pending_tickets ?? 0 },
      { status: "已通过", count: 15 },
      { status: "已拒绝", count: 3 },
      { status: "已完成", count: 42 },
    ],
    recent_activity: [
      { id: 1, user: "admin", action: "提交工单", target: "ALTER TABLE users ADD COLUMN", timestamp: new Date(now.getTime() - 300000).toISOString() },
      { id: 2, user: "dba_zhang", action: "审批通过", target: "工单 #127", timestamp: new Date(now.getTime() - 1800000).toISOString() },
      { id: 3, user: "dev_li", action: "执行查询", target: "SELECT * FROM orders WHERE...", timestamp: new Date(now.getTime() - 3600000).toISOString() },
      { id: 4, user: "admin", action: "创建数据源", target: "prod-mysql-01", timestamp: new Date(now.getTime() - 7200000).toISOString() },
      { id: 5, user: "dba_wang", action: "拒绝工单", target: "工单 #125: DROP TABLE", timestamp: new Date(now.getTime() - 14400000).toISOString() },
    ],
  };
}
