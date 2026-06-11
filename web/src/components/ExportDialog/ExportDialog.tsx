/**
 * ExportDialog — Reusable export dialog with format selection and field selection.
 *
 * Features:
 * - Format radio group: CSV / Excel (.xlsx)
 * - Field checkbox group: select which columns to export (default: all)
 * - Async export polling for large datasets (>10,000 rows)
 * - Consistent UX across Audit and Ticket export flows
 */

import React, { useCallback, useEffect, useRef, useState } from "react";
import { Download, Loader2, FileSpreadsheet, FileText } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import type {
  ExportColumn,
  ExportFormat,
} from "@/lib/export-utils";
import {
  buildExportFilename,
  downloadBlob,
  formatFileSize,
} from "@/lib/export-utils";

// --- Types ---

export type ExportType = "audit" | "ticket";

export interface ExportDialogProps {
  /** Whether the dialog is open. */
  open: boolean;
  /** Callback to close the dialog. */
  onOpenChange: (open: boolean) => void;
  /** Export context type — determines default filename prefix and field list. */
  exportType: ExportType;
  /** Available columns for field selection. */
  columns: ExportColumn[];
  /** Filename prefix (e.g., "audit_logs", "tickets"). */
  filenamePrefix: string;
  /** Perform sync export — returns a Blob. */
  syncExport: (format: ExportFormat, columns: string[]) => Promise<Blob>;
  /** Create async export task — returns the task object. */
  asyncExport: (format: ExportFormat, columns: string[]) => Promise<ExportTaskLike>;
  /** Get export task status for polling. */
  getTask: (taskId: number) => Promise<ExportTaskLike>;
  /** Download completed async export file. */
  downloadTask: (taskId: number) => Promise<Blob>;
  /** Whether the parent page is still loading data. */
  disabled?: boolean;
}

/** Minimal task shape needed for async polling. */
export interface ExportTaskLike {
  id: number;
  status: "pending" | "processing" | "completed" | "failed";
  filename: string;
  total_rows: number;
  file_bytes: number;
  error_msg?: string;
}

// --- Component ---

export function ExportDialog({
  open,
  onOpenChange,
  exportType,
  columns,
  filenamePrefix,
  syncExport,
  asyncExport,
  getTask,
  downloadTask,
  disabled = false,
}: ExportDialogProps) {
  // Format state — default CSV for backwards compatibility
  const [format, setFormat] = useState<ExportFormat>("csv");
  // Column selection — default all selected
  const [selectedColumns, setSelectedColumns] = useState<Set<string>>(
    () => new Set(columns.map((c) => c.key)),
  );
  // Export progress state
  const [exporting, setExporting] = useState(false);
  const [asyncTask, setAsyncTask] = useState<ExportTaskLike | null>(null);
  const [polling, setPolling] = useState(false);
  const pollingRef = useRef(false);

  // Reset state when dialog opens
  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (open) {
      setFormat("csv");
      setSelectedColumns(new Set(columns.map((c) => c.key)));
      setExporting(false);
      setAsyncTask(null);
      setPolling(false);
      pollingRef.current = false;
    }
  }, [open, columns]);
  /* eslint-enable react-hooks/set-state-in-effect */

  // Stop polling on unmount
  useEffect(() => {
    return () => {
      pollingRef.current = false;
    };
  }, []);

  const toggleColumn = useCallback((key: string, checked: boolean) => {
    setSelectedColumns((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(key);
      } else {
        next.delete(key);
      }
      return next;
    });
  }, []);

  const selectAll = useCallback(() => {
    setSelectedColumns(new Set(columns.map((c) => c.key)));
  }, [columns]);

  const deselectAll = useCallback(() => {
    setSelectedColumns(new Set());
  }, []);

  // Build column list for API call
  const columnList = Array.from(selectedColumns);

  // Poll async task
  const startPolling = useCallback((taskId: number) => {
    pollingRef.current = true;
    setPolling(true);
    const poll = async () => {
      if (!pollingRef.current) return;
      try {
        const task = await getTask(taskId);
        setAsyncTask(task);
        if (task.status === "completed") {
          pollingRef.current = false;
          setPolling(false);
          toast.success(`导出完成！共 ${task.total_rows} 条数据`);
          try {
            const blob = await downloadTask(taskId);
            downloadBlob(blob, task.filename);
          } catch {
            toast.error("下载导出文件失败，请稍后重试");
          }
          return;
        }
        if (task.status === "failed") {
          pollingRef.current = false;
          setPolling(false);
          toast.error(task.error_msg || "导出任务失败");
          return;
        }
        setTimeout(poll, 2000);
      } catch {
        pollingRef.current = false;
        setPolling(false);
        toast.error("查询导出状态失败");
      }
    };
    setTimeout(poll, 1500);
  }, [getTask, downloadTask]);

  // Main export handler
  const handleExport = useCallback(async () => {
    setExporting(true);
    setAsyncTask(null);
    try {
      const blob = await syncExport(format, columnList);
      if (blob.size === 0) {
        toast.info("没有可导出的数据");
        return;
      }
      const filename = buildExportFilename(filenamePrefix, format);
      downloadBlob(blob, filename);
      const formatLabel = format === "xlsx" ? "Excel" : "CSV";
      toast.success(`${exportType === "audit" ? "审计日志" : "工单"}导出成功（${formatLabel}，含水印）`);
      onOpenChange(false);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "导出失败";
      // If error indicates data exceeds limit, try async export
      if (msg.includes("10000") || msg.includes("超过")) {
        try {
          toast.info("数据量较大，正在后台生成导出文件...");
          const task = await asyncExport(format, columnList);
          setAsyncTask(task);
          startPolling(task.id);
        } catch {
          toast.error("创建异步导出任务失败");
        }
        return;
      }
      toast.error(msg);
    } finally {
      setExporting(false);
    }
  }, [format, columnList, filenamePrefix, exportType, syncExport, asyncExport, onOpenChange, startPolling]);

  // Download completed async export
  const handleDownloadAsync = useCallback(async (task: ExportTaskLike) => {
    try {
      const blob = await downloadTask(task.id);
      downloadBlob(blob, task.filename);
    } catch {
      toast.error("下载失败，文件可能已过期");
    }
  }, [downloadTask]);

  const isExporting = exporting || polling;
  const canExport = !isExporting && !disabled && selectedColumns.size > 0;

  const formatLabel = format === "xlsx" ? "Excel" : "CSV";
  const exportBtnText = polling
    ? "后台生成中..."
    : exporting
      ? "导出中..."
      : `导出 ${formatLabel}`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>导出数据</DialogTitle>
          <DialogDescription>
            选择导出格式和需要导出的字段
          </DialogDescription>
        </DialogHeader>

        {/* Format Selection */}
        <div className="space-y-2">
          <Label className="text-sm font-medium text-[var(--text-primary)]">
            导出格式
          </Label>
          <div className="flex gap-3">
            <label
              className={
                "flex flex-1 cursor-pointer items-center gap-2.5 rounded-lg border p-3 transition-colors " +
                (format === "csv"
                  ? "border-[var(--accent-primary)] bg-[var(--accent-primary)]/5"
                  : "border-[var(--border-default)] hover:border-[var(--border-hover)]")
              }
            >
              <input
                type="radio"
                name="export-format"
                value="csv"
                checked={format === "csv"}
                onChange={() => setFormat("csv")}
                className="sr-only"
              />
              <FileText
                size={18}
                className={
                  format === "csv"
                    ? "text-[var(--accent-primary)]"
                    : "text-[var(--text-muted)]"
                }
              />
              <div className="flex flex-col">
                <span className="text-sm font-medium text-[var(--text-primary)]">
                  CSV
                </span>
                <span className="text-xs text-[var(--text-muted)]">
                  通用文本格式，兼容性好
                </span>
              </div>
            </label>
            <label
              className={
                "flex flex-1 cursor-pointer items-center gap-2.5 rounded-lg border p-3 transition-colors " +
                (format === "xlsx"
                  ? "border-[var(--accent-primary)] bg-[var(--accent-primary)]/5"
                  : "border-[var(--border-default)] hover:border-[var(--border-hover)]")
              }
            >
              <input
                type="radio"
                name="export-format"
                value="xlsx"
                checked={format === "xlsx"}
                onChange={() => setFormat("xlsx")}
                className="sr-only"
              />
              <FileSpreadsheet
                size={18}
                className={
                  format === "xlsx"
                    ? "text-[var(--accent-primary)]"
                    : "text-[var(--text-muted)]"
                }
              />
              <div className="flex flex-col">
                <span className="text-sm font-medium text-[var(--text-primary)]">
                  Excel
                </span>
                <span className="text-xs text-[var(--text-muted)]">
                  .xlsx 格式，支持样式和筛选
                </span>
              </div>
            </label>
          </div>
        </div>

        {/* Field Selection */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <Label className="text-sm font-medium text-[var(--text-primary)]">
              导出字段
              <span className="ml-1.5 text-xs font-normal text-[var(--text-muted)]">
                （已选 {selectedColumns.size}/{columns.length}）
              </span>
            </Label>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={selectAll}
                className="text-xs text-[var(--accent-primary)] hover:underline"
              >
                全选
              </button>
              <span className="text-xs text-[var(--border-default)]">|</span>
              <button
                type="button"
                onClick={deselectAll}
                className="text-xs text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:underline"
              >
                清空
              </button>
            </div>
          </div>
          <ScrollArea className="h-[200px] rounded-md border border-[var(--border-default)] p-3">
            <div className="space-y-2">
              {columns.map((col) => (
                <label
                  key={col.key}
                  className="flex cursor-pointer items-center gap-2.5 rounded px-1.5 py-1 hover:bg-[var(--bg-elevated)]"
                >
                  <Checkbox
                    checked={selectedColumns.has(col.key)}
                    onCheckedChange={(checked: boolean) =>
                      toggleColumn(col.key, checked)
                    }
                  />
                  <span className="text-sm text-[var(--text-primary)]">
                    {col.label}
                  </span>
                </label>
              ))}
            </div>
          </ScrollArea>
          {selectedColumns.size === 0 && (
            <p className="text-xs text-amber-500">
              请至少选择一个导出字段
            </p>
          )}
        </div>

        {/* Async Task Status */}
        {asyncTask && (
          <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3">
            {asyncTask.status === "completed" && (
              <div className="flex items-center justify-between">
                <span className="text-sm text-[var(--text-secondary)]">
                  导出完成！共 {asyncTask.total_rows} 条数据
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-7 gap-1.5 px-2 text-xs"
                  onClick={() => handleDownloadAsync(asyncTask)}
                >
                  <Download size={12} />
                  下载 ({formatFileSize(asyncTask.file_bytes)})
                </Button>
              </div>
            )}
            {asyncTask.status === "processing" && (
              <div className="flex items-center gap-2">
                <Loader2 size={14} className="animate-spin text-[var(--accent-primary)]" />
                <span className="text-sm text-[var(--text-secondary)]">
                  后台导出任务 #{asyncTask.id} 处理中...
                </span>
              </div>
            )}
            {asyncTask.status === "pending" && (
              <div className="flex items-center gap-2">
                <Loader2 size={14} className="animate-spin text-[var(--text-muted)]" />
                <span className="text-sm text-[var(--text-secondary)]">
                  排队中...
                </span>
              </div>
            )}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            className="h-8"
            onClick={() => onOpenChange(false)}
            disabled={isExporting}
          >
            取消
          </Button>
          <Button
            size="sm"
            className="h-8 gap-1.5"
            onClick={handleExport}
            disabled={!canExport}
          >
            {isExporting ? (
              <Loader2 size={14} className="animate-spin" />
            ) : (
              <Download size={14} />
            )}
            {exportBtnText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
