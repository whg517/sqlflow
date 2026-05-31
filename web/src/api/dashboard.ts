import { api } from "./client";

export interface DashboardStats {
  pending_tickets: number;
  recent_queries_7d: number;
  active_datasources: number;
  total_users: number;
  sensitive_tables?: number;
}

export interface DailyCount {
  date: string;
  count: number;
}

export interface TicketStatusCount {
  status: string;
  count: number;
}

export interface AuditLogEntry {
  id: number;
  created_at: string;
  username: string;
  action: string;
  summary: string;
}

export interface DashboardOverview {
  stats: DashboardStats;
  query_trend: DailyCount[];
  query_sparkline: DailyCount[];
  ticket_sparkline: DailyCount[];
  ticket_status_dist: TicketStatusCount[];
  recent_activities: AuditLogEntry[];
}

export type TimeRange = "today" | "this_week" | "this_month" | "last_30d";

export async function getDashboardStats(): Promise<{
  code: number;
  data: DashboardStats;
}> {
  return api.get("/dashboard/stats");
}

export async function getDashboardOverview(
  timeRange: TimeRange = "last_30d",
): Promise<{
  code: number;
  data: DashboardOverview;
}> {
  return api.get(`/dashboard/overview?range=${timeRange}`);
}
