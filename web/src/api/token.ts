import { api } from "./client";

// --- Types ---

export interface APIToken {
  id: number;
  user_id: number;
  username?: string;
  name: string;
  token_prefix: string;
  scopes: string;
  expires_at: string;
  last_used_at?: string;
  use_count: number;
  is_active: boolean;
  description?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateTokenRequest {
  name: string;
  description?: string;
  scopes: string[];
  expires_days: number;
}

export interface CreateTokenResponse {
  id: number;
  name: string;
  token: string; // plaintext, only returned once
  token_prefix: string;
  scopes: string;
  expires_at: string;
  created_at: string;
}

export interface TokenStats {
  total_tokens: number;
  active_tokens: number;
  total_usage: number;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

// --- API Functions ---

export async function createToken(req: CreateTokenRequest): Promise<CreateTokenResponse> {
  const res = await api.post<{ code: number; data: CreateTokenResponse }>("/tokens", req);
  return res.data;
}

export async function listMyTokens(): Promise<APIToken[]> {
  const res = await api.get<{ code: number; data: APIToken[] }>("/tokens");
  return res.data;
}

export async function listAllTokens(page = 1, pageSize = 20): Promise<PaginatedResponse<APIToken>> {
  const res = await api.get<{ code: number; data: PaginatedResponse<APIToken> }>(
    `/admin/tokens?page=${page}&page_size=${pageSize}`
  );
  return res.data;
}

export async function revokeMyToken(id: number): Promise<void> {
  await api.del(`/tokens/${id}`);
}

export async function revokeAnyToken(id: number): Promise<void> {
  await api.del(`/admin/tokens/${id}`);
}

export async function getTokenStats(): Promise<TokenStats> {
  const res = await api.get<{ code: number; data: TokenStats }>("/tokens/stats");
  return res.data;
}

// --- Constants ---

export const SCOPE_OPTIONS: { value: string; label: string; desc: string }[] = [
  { value: "read:query", label: "查询读取", desc: "查看查询结果" },
  { value: "execute:query", label: "查询执行", desc: "执行 SQL 查询" },
  { value: "read:ticket", label: "工单查看", desc: "查看工单信息" },
  { value: "write:ticket", label: "工单管理", desc: "创建和管理工单" },
  { value: "read:datasource", label: "数据源查看", desc: "查看数据源元数据" },
  { value: "read:audit", label: "审计日志", desc: "查看审计日志" },
  { value: "admin", label: "管理员", desc: "完全管理权限" },
];

export function formatScopes(scopesStr: string): string {
  if (!scopesStr) return "无";
  const parts = scopesStr.split(",");
  return parts
    .map((s) => {
      const opt = SCOPE_OPTIONS.find((o) => o.value === s);
      return opt ? opt.label : s;
    })
    .join("、");
}

export function formatDate(dateStr: string): string {
  if (!dateStr) return "—";
  return new Date(dateStr).toLocaleString("zh-CN");
}

export function isExpired(expiresAt: string): boolean {
  return new Date(expiresAt) < new Date();
}

export function daysUntilExpiry(expiresAt: string): number {
  const diff = new Date(expiresAt).getTime() - Date.now();
  return Math.ceil(diff / (1000 * 60 * 60 * 24));
}
