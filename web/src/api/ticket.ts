import { api } from "./client";
import type { GitLink } from "./git";

// --- Types ---

export type TicketStatus =
  | "SUBMITTED"
  | "AI_REVIEWED"
  | "PENDING_APPROVAL"
  | "APPROVED"
  | "REJECTED"
  | "EXECUTING"
  | "DONE"
  | "CANCELLED";

export interface Ticket {
  id: number;
  submitter_id: number;
  submitter_name: string;
  datasource_id: number;
  database: string;
  sql_content: string;
  sql_summary: string;
  db_type: string;
  change_reason: string;
  status: TicketStatus;
  risk_level: string;
  ai_review_result: string;
  reviewer_id: number;
  reviewer_name: string;
  review_comment: string;
  executed_at: string | null;
  created_at: string;
  updated_at: string;
  git_links?: GitLink[];
}

export interface TicketListParams {
  page?: number;
  page_size?: number;
  status?: string;
  datasource_id?: string;
  submitter_id?: string;
  risk_level?: string;
  keyword?: string;
  scope?: "mine" | "pending";
}

export interface TicketListResponse {
  code: number;
  message: string;
  data: Ticket[];
  page: number;
  page_size: number;
  total: number;
}

export interface TicketDetailResponse {
  code: number;
  message: string;
  data: Ticket;
}

export interface CreateTicketRequest {
  datasource_id: number;
  database: string;
  sql: string;
  db_type: string;
  change_reason: string;
  risk_level?: string;
  ai_review_result?: string;
}

export interface CreateTicketResponse {
  code: number;
  message: string;
  data: Ticket;
}

export interface TicketActionResponse {
  code: number;
  message: string;
  data: Ticket;
}

// --- API Functions ---

export async function listTickets(
  params: TicketListParams = {},
): Promise<TicketListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.page_size) qs.set("page_size", String(params.page_size));
  if (params.status) qs.set("status", params.status);
  if (params.datasource_id) qs.set("datasource_id", params.datasource_id);
  if (params.submitter_id) qs.set("submitter_id", params.submitter_id);
  if (params.risk_level) qs.set("risk_level", params.risk_level);
  if (params.keyword) qs.set("keyword", params.keyword);
  if (params.scope) qs.set("scope", params.scope);
  const query = qs.toString();
  return api.get<TicketListResponse>(`/tickets${query ? `?${query}` : ""}`);
}

export async function getTicket(id: number): Promise<TicketDetailResponse> {
  return api.get<TicketDetailResponse>(`/tickets/${id}`);
}

export async function createTicket(
  req: CreateTicketRequest,
): Promise<CreateTicketResponse> {
  return api.post<CreateTicketResponse>("/tickets", req);
}

export async function approveTicket(
  id: number,
  comment: string,
): Promise<TicketActionResponse> {
  return api.post<TicketActionResponse>(`/tickets/${id}/approve`, { comment });
}

export async function rejectTicket(
  id: number,
  reason: string,
): Promise<TicketActionResponse> {
  return api.post<TicketActionResponse>(`/tickets/${id}/reject`, { reason });
}

export async function cancelTicket(
  id: number,
  reason: string,
): Promise<TicketActionResponse> {
  return api.post<TicketActionResponse>(`/tickets/${id}/cancel`, { reason });
}

export async function executeTicket(id: number): Promise<TicketActionResponse> {
  return api.post<TicketActionResponse>(`/tickets/${id}/execute`, {});
}

// --- Helpers ---

const statusLabelMap: Record<TicketStatus, string> = {
  SUBMITTED: "已提交",
  AI_REVIEWED: "AI 已评审",
  PENDING_APPROVAL: "待审批",
  APPROVED: "已通过",
  REJECTED: "已拒绝",
  EXECUTING: "执行中",
  DONE: "已完成",
  CANCELLED: "已取消",
};

const statusColorMap: Record<TicketStatus, string> = {
  SUBMITTED: "bg-slate-500/20 text-slate-400",
  AI_REVIEWED: "bg-violet-500/20 text-violet-400",
  PENDING_APPROVAL: "bg-blue-500/20 text-blue-400",
  APPROVED: "bg-green-500/20 text-green-400",
  REJECTED: "bg-red-500/20 text-red-400",
  EXECUTING: "bg-yellow-500/20 text-yellow-400",
  DONE: "bg-emerald-500/20 text-emerald-400",
  CANCELLED: "bg-gray-500/20 text-gray-400",
};

export function getStatusLabel(status: TicketStatus): string {
  return statusLabelMap[status] ?? status;
}

export function getStatusColor(status: TicketStatus): string {
  return statusColorMap[status] ?? "bg-gray-500/20 text-gray-400";
}

const riskLabelMap: Record<string, string> = {
  low: "低风险",
  medium: "中风险",
  high: "高风险",
};

const riskColorMap: Record<string, string> = {
  low: "bg-emerald-500/20 text-emerald-400",
  medium: "bg-yellow-500/20 text-yellow-400",
  high: "bg-red-500/20 text-red-400",
};

const riskDotMap: Record<string, string> = {
  low: "bg-emerald-400",
  medium: "bg-yellow-400",
  high: "bg-red-400",
};

export function getRiskLabel(level: string): string {
  return riskLabelMap[level] ?? level;
}

export function getRiskColor(level: string): string {
  return riskColorMap[level] ?? "bg-gray-500/20 text-gray-400";
}

export function getRiskDot(level: string): string {
  return riskDotMap[level] ?? "bg-gray-400";
}

export function formatTime(iso: string): string {
  const d = new Date(iso);
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  const hour = String(d.getHours()).padStart(2, "0");
  const min = String(d.getMinutes()).padStart(2, "0");
  return `${month}-${day} ${hour}:${min}`;
}
