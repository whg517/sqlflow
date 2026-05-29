import { api } from "./client";

// --- Types ---

export interface SharedResultResponse {
  id: number;
  user_id: number;
  username: string;
  token: string;
  row_count: number;
  expires_at: string;
  sql_summary: string;
  datasource_name: string;
  revoked: boolean;
  revoked_at: string | null;
  created_at: string;
}

export interface SharedResultPublic {
  id: number;
  columns: string[];
  rows: Record<string, unknown>[];
  row_count: number;
  sql_summary: string;
  datasource_name: string;
  expires_at: string;
  has_password: boolean;
  created_at: string;
}

interface CreateShareRequest {
  columns: string[];
  rows: Record<string, unknown>[];
  expires_in_hours: number;
  password?: string;
  sql_summary?: string;
  datasource_name?: string;
}

// --- API ---

export async function createShare(req: CreateShareRequest): Promise<SharedResultResponse> {
  const res = await api.post<{ code: number; message: string; data: SharedResultResponse }>(
    "/query/share",
    req,
  );
  return res.data;
}

export async function listMyShares(): Promise<SharedResultResponse[]> {
  const res = await api.get<{ code: number; message: string; data: SharedResultResponse[] }>(
    "/query/share",
  );
  return res.data;
}

export async function revokeShare(id: number): Promise<void> {
  await api.delete(`/query/share/${id}`);
}

// Public API (no auth required) — uses fetch directly with full URL
export async function getSharedResult(token: string): Promise<SharedResultPublic> {
  const res = await fetch(`/s/${token}`);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message || `请求失败 (${res.status})`);
  }
  const json = await res.json();
  return json.data;
}

export async function verifySharePassword(
  token: string,
  password: string,
): Promise<void> {
  const res = await fetch(`/s/${token}/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ password }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message || `密码验证失败 (${res.status})`);
  }
}
