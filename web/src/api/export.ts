import { api } from "./client";

// --- Types ---

export interface AuditExportParams {
  user_id?: string;
  action?: string;
  datasource_id?: string;
  start?: string;
  end?: string;
  keyword?: string;
}

export interface TicketExportParams {
  status?: string;
  datasource_id?: string;
  risk_level?: string;
  keyword?: string;
}

export type ExportTaskStatus =
  | "pending"
  | "processing"
  | "completed"
  | "failed";

export interface ExportTask {
  id: number;
  user_id: number;
  username: string;
  export_type: "audit" | "ticket";
  status: ExportTaskStatus;
  filename: string;
  total_rows: number;
  file_bytes: number;
  error_msg?: string;
  created_at: string;
  completed_at?: string;
}

export interface ExportTaskSlim {
  id: number;
  export_type: "audit" | "ticket";
  status: ExportTaskStatus;
  filename: string;
  total_rows: number;
  file_bytes: number;
  error_msg?: string;
  created_at: string;
  completed_at?: string;
}

interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

// --- Sync Export API Functions ---

/**
 * Export audit logs as CSV via the backend export API.
 * Returns the CSV Blob for download.
 * Backend adds BOM header and user watermark for audit compliance.
 * Requires admin or dba role.
 */
export async function exportAuditLogs(
  params: AuditExportParams = {},
): Promise<Blob> {
  const qs = new URLSearchParams();
  if (params.user_id) qs.set("user_id", params.user_id);
  if (params.action) qs.set("action", params.action);
  if (params.datasource_id) qs.set("datasource_id", params.datasource_id);
  if (params.start) qs.set("start", params.start);
  if (params.end) qs.set("end", params.end);
  if (params.keyword) qs.set("keyword", params.keyword);
  const query = qs.toString();
  const url = `/export/audit${query ? `?${query}` : ""}`;
  return api.getBlob(url);
}

/**
 * Export tickets as CSV via the backend export API.
 * Returns the CSV Blob for download.
 * Backend adds BOM header and user watermark for audit compliance.
 * All authenticated users can export tickets.
 */
export async function exportTickets(
  params: TicketExportParams = {},
): Promise<Blob> {
  const qs = new URLSearchParams();
  if (params.status) qs.set("status", params.status);
  if (params.datasource_id) qs.set("datasource_id", params.datasource_id);
  if (params.risk_level) qs.set("risk_level", params.risk_level);
  if (params.keyword) qs.set("keyword", params.keyword);
  const query = qs.toString();
  const url = `/export/tickets${query ? `?${query}` : ""}`;
  return api.getBlob(url);
}

// --- Async Export API Functions ---

/**
 * Create an async audit export task.
 * Returns the created task with its ID.
 */
export async function createAsyncAuditExport(
  params: AuditExportParams = {},
): Promise<ExportTask> {
  const qs = new URLSearchParams();
  qs.set("async", "1");
  if (params.user_id) qs.set("user_id", params.user_id);
  if (params.action) qs.set("action", params.action);
  if (params.datasource_id) qs.set("datasource_id", params.datasource_id);
  if (params.start) qs.set("start", params.start);
  if (params.end) qs.set("end", params.end);
  if (params.keyword) qs.set("keyword", params.keyword);
  const res = await api.get<ApiResponse<ExportTask>>(
    `/export/audit?${qs.toString()}`,
  );
  return res.data;
}

/**
 * Create an async ticket export task.
 * Returns the created task with its ID.
 */
export async function createAsyncTicketExport(
  params: TicketExportParams = {},
): Promise<ExportTask> {
  const qs = new URLSearchParams();
  qs.set("async", "1");
  if (params.status) qs.set("status", params.status);
  if (params.datasource_id) qs.set("datasource_id", params.datasource_id);
  if (params.risk_level) qs.set("risk_level", params.risk_level);
  if (params.keyword) qs.set("keyword", params.keyword);
  const res = await api.get<ApiResponse<ExportTask>>(
    `/export/tickets?${qs.toString()}`,
  );
  return res.data;
}

/**
 * Get the status of an async export task.
 */
export async function getExportTask(taskId: number): Promise<ExportTask> {
  const res = await api.get<ApiResponse<ExportTask>>(
    `/export/tasks/${taskId}`,
  );
  return res.data;
}

/**
 * List recent export tasks for the current user.
 */
export async function listExportTasks(): Promise<ExportTaskSlim[]> {
  const res = await api.get<ApiResponse<ExportTaskSlim[]>>(
    `/export/tasks`,
  );
  return res.data;
}

/**
 * Download a completed async export file as a Blob.
 */
export async function downloadExportFile(taskId: number): Promise<Blob> {
  return api.getBlob(`/export/tasks/${taskId}/download`);
}
