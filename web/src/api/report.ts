import { api } from "./client";

// --- Usage Stats ---

export interface UserActionStat {
  user_id: number;
  username: string;
  count: number;
}

export interface ActionStat {
  action: string;
  count: number;
}

export interface DatabaseStat {
  database: string;
  count: number;
}

export interface DailyAuditTrend {
  date: string;
  count: number;
}

export interface UsageStatsResponse {
  code: number;
  message: string;
  data: {
    total_actions: number;
    unique_users: number;
    unique_ips: number;
    top_users: UserActionStat[];
    top_actions: ActionStat[];
    top_databases: DatabaseStat[];
    daily_trend: DailyAuditTrend[];
  };
}

// --- Error Stats ---

export interface ErrorTypeStat {
  action: string;
  count: number;
}

export interface RecentErrorEntry {
  id: number;
  action: string;
  database: string;
  error_message: string;
  username: string;
  created_at: string;
}

export interface ErrorStatsResponse {
  code: number;
  message: string;
  data: {
    total_errors: number;
    error_rate: number;
    top_error_types: ErrorTypeStat[];
    recent_errors: RecentErrorEntry[];
    daily_error_trend: DailyAuditTrend[];
  };
}

// --- Performance Report ---

export interface DailyPerfTrend {
  date: string;
  avg_time_ms: number;
  max_time_ms: number;
  query_count: number;
  result_rows: number;
}

export interface PerformanceReportResponse {
  code: number;
  message: string;
  data: {
    avg_execution_ms: number;
    max_execution_ms: number;
    p95_execution_ms: number;
    total_result_rows: number;
    total_affected_rows: number;
    daily_perf_trend: DailyPerfTrend[];
  };
}

// --- Ticket Report ---

export interface DailyTicketTrend {
  date: string;
  created: number;
  approved: number;
  rejected: number;
}

export interface RiskDistEntry {
  risk_level: string;
  count: number;
}

export interface TicketReportResponse {
  code: number;
  message: string;
  data: {
    total_tickets: number;
    pending_count: number;
    approved_count: number;
    rejected_count: number;
    done_count: number;
    cancelled_count: number;
    avg_approval_time_h: number;
    daily_ticket_trend: DailyTicketTrend[];
    risk_distribution: RiskDistEntry[];
  };
}

// --- API Functions ---

export async function getUsageStats(days = 7): Promise<UsageStatsResponse["data"]> {
  const res = await api.get<UsageStatsResponse>(`/reports/usage?days=${days}`);
  return res.data;
}

export async function getErrorStats(days = 7): Promise<ErrorStatsResponse["data"]> {
  const res = await api.get<ErrorStatsResponse>(`/reports/errors?days=${days}`);
  return res.data;
}

export async function getPerformanceReport(days = 7): Promise<PerformanceReportResponse["data"]> {
  const res = await api.get<PerformanceReportResponse>(`/reports/performance?days=${days}`);
  return res.data;
}

export async function getTicketReport(days = 7): Promise<TicketReportResponse["data"]> {
  const res = await api.get<TicketReportResponse>(`/reports/tickets?days=${days}`);
  return res.data;
}

// --- Helpers ---

export function formatMs(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.round(ms)}ms`;
}

export function formatPercent(v: number): string {
  return `${v.toFixed(1)}%`;
}

export function formatNumber(n: number): string {
  return n.toLocaleString("zh-CN");
}
