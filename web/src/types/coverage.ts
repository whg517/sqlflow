/**
 * SF-QA0025 — Coverage audit TypeScript type definitions.
 * Mirrors the backend API response shapes defined in the technical spec §3.
 */

// --- Coverage status (threshold-based) ---

export type CoverageStatus = "pass" | "warning" | "critical";

// --- Project summary ---

export interface TestTypeSummary {
  line_rate: number;
  branch_rate: number;
  lines_total: number;
  lines_covered: number;
  modules_total: number;
  modules_below_threshold: number;
}

export interface CoverageSummary {
  project: string;
  snapshot_at: string;
  test_types: Record<string, TestTypeSummary>;
}

// --- Module level ---

export interface ModuleItem {
  module_path: string;
  line_rate: number;
  branch_rate: number;
  lines_total: number;
  lines_covered: number;
  file_count: number;
  status: CoverageStatus;
}

export interface ModuleListResponse {
  items: ModuleItem[];
  total: number;
  page: number;
  page_size: number;
}

// --- File level ---

export interface UncoveredRange {
  start: number;
  end: number;
}

export interface FileItem {
  file_path: string;
  line_rate: number;
  branch_rate: number;
  lines_total: number;
  lines_covered: number;
  uncovered_ranges: UncoveredRange[] | null;
  status: CoverageStatus;
}

export interface FileListResponse {
  items: FileItem[];
  total: number;
  page: number;
  page_size: number;
}

// --- Query params ---

export interface ModuleListParams {
  test_type?: string;
  sort?: "line_rate:asc" | "line_rate:desc" | "branch_rate:asc" | "branch_rate:desc";
  status?: CoverageStatus;
  page?: number;
  page_size?: number;
}

export interface FileListParams {
  module_path: string;
  test_type?: string;
  sort?: string;
  page?: number;
  page_size?: number;
}

// --- Helpers ---

export function computeStatus(rate: number): CoverageStatus {
  if (rate >= 0.8) return "pass";
  if (rate >= 0.6) return "warning";
  return "critical";
}
