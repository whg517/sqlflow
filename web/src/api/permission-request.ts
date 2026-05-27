import { api } from "./client";

export interface PermissionRequest {
  id: number;
  applicant_id: number;
  applicant_name?: string;
  approver_id?: number;
  approver_name?: string;
  datasource_id: number;
  datasource_name?: string;
  database: string;
  table_name: string;
  actions: string;
  reason: string;
  status: "PENDING" | "APPROVED" | "REJECTED" | "EXPIRED" | "REVOKED";
  approve_comment?: string;
  approved_at?: string;
  expires_at: string;
  revoked_at?: string;
  revoked_by?: number;
  revoke_reason?: string;
  created_at: string;
  updated_at: string;
}

export interface CreatePermReqRequest {
  datasource_id: number;
  database: string;
  table_name: string;
  actions: string;
  reason: string;
  duration_hours: number;
}

export interface ActivePermission {
  items: PermissionRequest[];
  total: number;
}

export const STATUS_MAP: Record<string, { label: string; color: string; bg: string }> = {
  PENDING: { label: "待审批", color: "text-amber-400", bg: "bg-amber-500/10" },
  APPROVED: { label: "已批准", color: "text-emerald-400", bg: "bg-emerald-500/10" },
  REJECTED: { label: "已拒绝", color: "text-red-400", bg: "bg-red-500/10" },
  EXPIRED: { label: "已过期", color: "text-gray-400", bg: "bg-gray-500/10" },
  REVOKED: { label: "已撤销", color: "text-orange-400", bg: "bg-orange-500/10" },
};

export const ACTION_LABELS: Record<string, string> = {
  select: "查询",
  update: "更新",
  delete: "删除",
  ddl: "DDL",
  export: "导出",
};

export function formatActions(actions: string): string {
  return actions
    .split(",")
    .map((a) => ACTION_LABELS[a.trim()] || a.trim())
    .join("、");
}

export function formatDuration(hours: number): string {
  if (hours < 1) return `${Math.round(hours * 60)} 分钟`;
  if (hours < 24) return `${hours} 小时`;
  return `${Math.round(hours / 24 * 10) / 10} 天`;
}

export function timeLeft(expiresAt: string): string {
  const diff = new Date(expiresAt).getTime() - Date.now();
  if (diff <= 0) return "已过期";
  const hours = Math.floor(diff / (1000 * 60 * 60));
  if (hours < 1) return `${Math.floor(diff / (1000 * 60))} 分钟`;
  if (hours < 24) return `${hours} 小时`;
  return `${Math.floor(hours / 24)} 天`;
}

export function formatDateTime(dateStr: string): string {
  if (!dateStr) return "—";
  return new Date(dateStr).toLocaleString("zh-CN");
}

export async function createPermReq(req: CreatePermReqRequest): Promise<PermissionRequest> {
  const res = await api.post<{ code: number; data: PermissionRequest }>("/permission-requests", req);
  return res.data;
}

export async function listPermReqs(page = 1, pageSize = 20, status = "") {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  if (status) params.set("status", status);
  const res = await api.get<{
    code: number;
    data: { items: PermissionRequest[]; total: number };
    page: number;
    page_size: number;
    total: number;
  }>(`/permission-requests?${params}`);
  return { items: res.data.items, total: res.total, page, pageSize };
}

export async function myPermReqs(): Promise<ActivePermission> {
  const res = await api.get<{ code: number; data: ActivePermission }>("/permission-requests/mine");
  return res.data;
}

export async function myActivePermissions(): Promise<PermissionRequest[]> {
  const res = await api.get<{ code: number; data: PermissionRequest[] }>("/permission-requests/active");
  return res.data;
}

export async function approvePermReq(id: number, comment = ""): Promise<PermissionRequest> {
  const res = await api.post<{ code: number; data: PermissionRequest }>(`/permission-requests/${id}/approve`, { comment });
  return res.data;
}

export async function rejectPermReq(id: number, comment = ""): Promise<PermissionRequest> {
  const res = await api.post<{ code: number; data: PermissionRequest }>(`/permission-requests/${id}/reject`, { comment });
  return res.data;
}

export async function revokePermReq(id: number, reason = ""): Promise<PermissionRequest> {
  const res = await api.post<{ code: number; data: PermissionRequest }>(`/permission-requests/${id}/revoke`, { reason });
  return res.data;
}
