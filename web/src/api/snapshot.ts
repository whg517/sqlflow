import { api } from "./client";

// --- Types ---

export interface QuerySnapshot {
  id: number;
  query_history_id: number;
  columns_json: string[];
  rows_json: Record<string, unknown>[];
  total_rows: number;
  created_at: string;
  /** Joined from query_history for display */
  sql_content?: string;
  sql_summary?: string;
  database?: string;
}

export interface CreateSnapshotRequest {
  query_history_id: number;
}

export interface CreateSnapshotResponse {
  code: number;
  message: string;
  data: QuerySnapshot;
}

export interface SnapshotListResponse {
  code: number;
  message: string;
  data: QuerySnapshot[];
  total: number;
}

export interface DiffRow {
  type: "added" | "removed" | "modified" | "unchanged";
  rowIndex: number;
  left?: Record<string, unknown>;
  right?: Record<string, unknown>;
  changedFields?: string[];
}

export interface CompareResult {
  columns: string[];
  totalLeft: number;
  totalRight: number;
  diffRows: DiffRow[];
  summary: {
    added: number;
    removed: number;
    modified: number;
    unchanged: number;
  };
}

export interface CompareRequest {
  left_snapshot_id: number;
  right_snapshot_id: number;
}

export interface CompareResponse {
  code: number;
  message: string;
  data: CompareResult;
}

// --- API Functions ---

export async function createSnapshot(
  req: CreateSnapshotRequest,
): Promise<CreateSnapshotResponse> {
  return api.post<CreateSnapshotResponse>("/query/snapshots", req);
}

export async function listSnapshots(
  page = 1,
  pageSize = 20,
): Promise<SnapshotListResponse> {
  return api.get<SnapshotListResponse>(
    `/query/snapshots?page=${page}&page_size=${pageSize}`,
  );
}

export async function deleteSnapshot(id: number): Promise<{ code: number; message: string }> {
  return api.del<{ code: number; message: string }>(`/query/snapshots/${id}`);
}

export async function compareSnapshots(
  req: CompareRequest,
): Promise<CompareResponse> {
  return api.post<CompareResponse>("/query/compare", req);
}
