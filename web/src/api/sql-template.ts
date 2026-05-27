import { api } from "./client";

// --- Types ---

export interface SQLTemplate {
  id: number;
  user_id: number;
  name: string;
  description: string;
  sql_content: string;
  db_type: string;
  category: string;
  params_json: string;
  is_public: boolean;
  created_at: string;
  updated_at: string;
}

export interface TemplateParam {
  name: string;
  default: string;
}

export interface CreateTemplateRequest {
  name: string;
  description?: string;
  sql_content: string;
  db_type: string;
  category?: string;
  is_public?: boolean;
}

export interface RenderResult {
  rendered_sql: string;
  param_values: unknown[];
  sql: string;
}

export interface PaginatedTemplateResponse {
  items: SQLTemplate[];
  total: number;
  page: number;
  page_size: number;
}

// --- API Functions ---

export async function createTemplate(req: CreateTemplateRequest): Promise<SQLTemplate> {
  const res = await api.post<{ code: number; data: SQLTemplate }>("/sql-templates", req);
  return res.data;
}

export async function getTemplate(id: number): Promise<SQLTemplate> {
  const res = await api.get<{ code: number; data: SQLTemplate }>(`/sql-templates/${id}`);
  return res.data;
}

export async function listTemplates(
  category = "",
  page = 1,
  pageSize = 20,
): Promise<PaginatedTemplateResponse> {
  const params = new URLSearchParams();
  if (category) params.set("category", category);
  params.set("page", String(page));
  params.set("page_size", String(pageSize));
  const res = await api.get<{ code: number; data: SQLTemplate[]; page: number; page_size: number; total: number }>(
    `/sql-templates?${params.toString()}`,
  );
  return {
    items: res.data,
    total: res.total,
    page: res.page,
    page_size: res.page_size,
  };
}

export async function updateTemplate(
  id: number,
  req: CreateTemplateRequest,
): Promise<void> {
  await api.put(`/sql-templates/${id}`, req);
}

export async function deleteTemplate(id: number): Promise<void> {
  await api.del(`/sql-templates/${id}`);
}

export async function renderTemplate(
  id: number,
  params: Record<string, string>,
): Promise<RenderResult> {
  const res = await api.post<{ code: number; data: RenderResult }>(
    `/sql-templates/${id}/render`,
    { params },
  );
  return res.data;
}

// --- Helpers ---

export function parseParamsJSON(json: string): TemplateParam[] {
  try {
    return JSON.parse(json) as TemplateParam[];
  } catch {
    return [];
  }
}
