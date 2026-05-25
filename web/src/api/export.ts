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

// --- API Functions ---

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
