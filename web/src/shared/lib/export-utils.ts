/**
 * Shared export utility functions.
 * Extracted from Audit/Ticket pages for reuse across export flows.
 */

/** Download a Blob as a file via a temporary anchor element. */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

/** Format byte count to human-readable file size. */
export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Export format type — mirrors backend ExportFormat enum. */
export type ExportFormat = "csv" | "xlsx";

/** Column definition for export field selection. */
export interface ExportColumn {
  /** Backend column key — must match server-side field whitelist. */
  key: string;
  /** Display label shown in the checkbox group. */
  label: string;
}

/** Get file extension for a given export format. */
export function getExportExtension(format: ExportFormat): string {
  return format === "xlsx" ? ".xlsx" : ".csv";
}

/** Get MIME type for a given export format. */
export function getExportMimeType(format: ExportFormat): string {
  return format === "xlsx"
    ? "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
    : "text/csv";
}

/** Build export filename with date stamp and correct extension. */
export function buildExportFilename(
  prefix: string,
  format: ExportFormat,
): string {
  const date = new Date().toISOString().slice(0, 10);
  return `${prefix}_${date}${getExportExtension(format)}`;
}
