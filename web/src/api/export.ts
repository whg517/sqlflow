import { api } from "./client";
import type { ExportFormat } from "@/lib/export-utils";

// --- Types ---

export interface AuditExportParams {
  user_id?: string;
  action?: string;
  datasource_id?: string;
  start?: string;
  end?: string;
  keyword?: string;
  /** Export format — csv (default) or xlsx. */
  format?: ExportFormat;
  /** Selected columns to export. Empty/undefined = all columns. */
  columns?: string[];
}

export interface TicketExportParams {
  status?: string;
  datasource_id?: string;
  risk_level?: string;
  keyword?: string;
  /** Export format — csv (default) or xlsx. */
  format?: ExportFormat;
  /** Selected columns to export. Empty/undefined = all columns. */
  columns?: string[];
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

// --- Internal: build query string with shared export params ---

function buildExportQuery(params: Record<string, unknown>): string {
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === "") continue;
    if (Array.isArray(value) && value.length > 0) {
      // columns 需要JSON序列化，后端用 json.Unmarshal 解析
      if (key === "columns") {
        qs.set(key, JSON.stringify(value));
      } else {
        qs.set(key, value.join(","));
      }
    } else if (!Array.isArray(value)) {
      qs.set(key, String(value));
    }
  }
  const query = qs.toString();
  return query ? `?${query}` : "";
}

// --- Sync Export API Functions ---

/**
 * Export audit logs via the backend export API.
 * Returns the Blob (CSV or XLSX) for download.
 * Backend adds BOM header and user watermark for audit compliance.
 * Requires admin or dba role.
 */
export async function exportAuditLogs(
  params: AuditExportParams = {},
): Promise<Blob> {
  const query = buildExportQuery(params);
  return api.getBlob(`/export/audit${query}`);
}

/**
 * Export tickets via the backend export API.
 * Returns the Blob (CSV or XLSX) for download.
 * Backend adds BOM header and user watermark for audit compliance.
 * All authenticated users can export tickets.
 */
export async function exportTickets(
  params: TicketExportParams = {},
): Promise<Blob> {
  const query = buildExportQuery(params);
  return api.getBlob(`/export/tickets${query}`);
}

// --- Async Export API Functions ---

/**
 * Create an async audit export task.
 * Returns the created task with its ID.
 */
export async function createAsyncAuditExport(
  params: AuditExportParams = {},
): Promise<ExportTask> {
  const query = buildExportQuery({ ...params, async: "1" });
  const res = await api.get<ApiResponse<ExportTask>>(
    `/export/audit${query}`,
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
  const query = buildExportQuery({ ...params, async: "1" });
  const res = await api.get<ApiResponse<ExportTask>>(
    `/export/tickets${query}`,
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
