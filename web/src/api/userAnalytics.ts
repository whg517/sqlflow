import { api } from "./client";

// --- Types (mirrors backend UserAnalytics struct) ---

export interface ActiveUserEntry {
  rank: number;
  user_id: number;
  username: string;
  query_count: number;
  approval_count: number;
  active_days: number;
  total_actions: number;
}

export interface QueryFrequencyEntry {
  period: string;
  count: number;
}

export interface ActionTypeEntry {
  action: string;
  count: number;
  ratio: number;
}

export interface AnomalyEntry {
  user_id: number;
  username: string;
  anomaly_type: string;
  description: string;
  count: number;
  time_window: string;
}

export interface UserAnalytics {
  generated_at: string;
  time_range: string;
  start_date: string;
  end_date: string;
  user_id?: number;
  top_active_users: ActiveUserEntry[];
  query_frequency: QueryFrequencyEntry[];
  action_type_breakdown: ActionTypeEntry[];
  anomalous_behaviors: AnomalyEntry[];
}

// --- API Functions ---

export async function getUserAnalytics(params: {
  time_range?: string;
  user_id?: number;
  start_date?: string;
  end_date?: string;
}): Promise<{ code: number; data: UserAnalytics }> {
  const qs = new URLSearchParams();
  if (params.time_range) qs.set("time_range", params.time_range);
  if (params.user_id) qs.set("user_id", String(params.user_id));
  if (params.start_date) qs.set("start_date", params.start_date);
  if (params.end_date) qs.set("end_date", params.end_date);
  const query = qs.toString();
  return api.get(`/audit/user-analytics${query ? `?${query}` : ""}`);
}

// --- Display Helpers -----

export const TIME_RANGE_OPTIONS = [
  { value: "7d", label: "最近 7 天" },
  { value: "30d", label: "最近 30 天" },
  { value: "90d", label: "最近 90 天" },
] as const;

/** Map action codes to Chinese labels for charts. */
const ACTION_LABELS: Record<string, string> = {
  query: "查询",
  approve: "审批",
  reject: "驳回",
  export: "导出",
  login: "登录",
  setting_change: "设置变更",
  create_ticket: "创建工单",
  execute: "执行",
};

export function getActionLabel(action: string): string {
  return ACTION_LABELS[action] ?? action;
}

/** Pie chart color palette for action types. */
export const ACTION_COLORS = [
  "#3b82f6", // blue
  "#22c55e", // green
  "#f59e0b", // amber
  "#ef4444", // red
  "#8b5cf6", // violet
  "#06b6d4", // cyan
  "#ec4899", // pink
  "#6b7280", // gray
];

/** Anomaly type labels. */
const ANOMALY_LABELS: Record<string, string> = {
  high_volume: "高频查询",
  off_hours: "非工作时间",
  bulk_export: "批量导出",
  failed_burst: "连续失败",
  sensitive_access: "敏感数据访问",
};

export function getAnomalyLabel(type: string): string {
  return ANOMALY_LABELS[type] ?? type;
}
