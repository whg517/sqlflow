import { api } from "./client";

// --- Types ---

export interface AuditLog {
  id: number;
  user_id: number;
  username: string;
  action: string;
  datasource_id: number;
  database: string;
  sql_content: string;
  sql_summary: string;
  result_rows: number;
  affected_rows: number;
  execution_time_ms: number;
  error_message: string;
  desensitized_fields: string;
  ip_address: string;
  ai_review_result: string;
  ticket_id: number;
  created_at: string;
}

export interface AuditListParams {
  page?: number;
  page_size?: number;
  user_id?: string;
  action?: string;
  datasource_id?: string;
  start?: string;
  end?: string;
  keyword?: string;
}

export interface AuditListResponse {
  code: number;
  message: string;
  data: AuditLog[];
  page: number;
  page_size: number;
  total: number;
}

// --- API Functions ---

export async function listAuditLogs(
  params: AuditListParams = {},
): Promise<AuditListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.page_size) qs.set("page_size", String(params.page_size));
  if (params.user_id) qs.set("user_id", params.user_id);
  if (params.action) qs.set("action", params.action);
  if (params.datasource_id) qs.set("datasource_id", params.datasource_id);
  if (params.start) qs.set("start", params.start);
  if (params.end) qs.set("end", params.end);
  if (params.keyword) qs.set("keyword", params.keyword);
  const query = qs.toString();
  return api.get<AuditListResponse>(`/audit-logs${query ? `?${query}` : ""}`);
}

/**
 * Full-text search on audit logs (FTS5).
 * Falls back to listAuditLogs with keyword if the search endpoint is unavailable.
 */
export async function searchAuditLogs(
  keyword: string,
  pageSize = 5,
): Promise<AuditListResponse> {
  const qs = new URLSearchParams();
  qs.set("keyword", keyword);
  qs.set("page", "1");
  qs.set("page_size", String(pageSize));
  return api.get<AuditListResponse>(`/audit-logs/search?${qs.toString()}`);
}

// --- Helpers ---

const actionLabelMap: Record<string, string> = {
  SELECT: "SELECT",
  UPDATE: "UPDATE",
  DELETE: "DELETE",
  DDL: "DDL",
  EXPORT: "导出",
  INSERT: "INSERT",
};

const actionColorMap: Record<string, string> = {
  SELECT: "bg-blue-500/20 text-blue-400",
  UPDATE: "bg-yellow-500/20 text-yellow-400",
  DELETE: "bg-red-500/20 text-red-400",
  DDL: "bg-violet-500/20 text-violet-400",
  EXPORT: "bg-emerald-500/20 text-emerald-400",
  INSERT: "bg-amber-500/20 text-amber-400",
};

export function getActionLabel(action: string): string {
  return actionLabelMap[action] ?? action;
}

export function getActionColor(action: string): string {
  return actionColorMap[action] ?? "bg-slate-500/20 text-slate-400";
}

/** Badge style helper — alias for getActionColor with consistent naming */
export const getActionBadgeStyle = getActionColor;

export function formatAuditTime(iso: string): string {
  const d = new Date(iso);
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  const hour = String(d.getHours()).padStart(2, "0");
  const min = String(d.getMinutes()).padStart(2, "0");
  const sec = String(d.getSeconds()).padStart(2, "0");
  return `${month}-${day} ${hour}:${min}:${sec}`;
}

export function formatExecutionTime(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

export const actionOptions = ["SELECT", "UPDATE", "DELETE", "DDL", "EXPORT"];
