import { Download, Loader2, Play, ShieldAlert, GitBranch } from "lucide-react";
import { useState, useEffect, useMemo } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { exportQuery } from "@/api/query";
import type { QueryResult } from "@/api/query";
import { listSensitiveTables, type SensitiveTable } from "@/api/maskRule";

interface StatusBarProps {
  executing: boolean;
  error: string | null;
  result: QueryResult | null;
  datasourceId: number | null;
  database: string;
  sql: string;
  onExecute: () => void;
  onExplain?: () => void;
  explaining?: boolean;
  isMongo?: boolean;
  mongoCollection?: string;
}

export default function StatusBar({
  executing,
  error,
  result,
  datasourceId,
  database,
  sql,
  onExecute,
  onExplain,
  explaining,
  isMongo,
  mongoCollection,
}: StatusBarProps) {
  const hasResult = result !== null && result.rows.length > 0;

  const [exportFormat, setExportFormat] = useState<"csv" | "json" | null>(null);
  const [confirmLargeExport, setConfirmLargeExport] = useState(false);
  const [exporting, setExporting] = useState(false);

  // Sensitive table detection
  const [sensitiveTables, setSensitiveTables] = useState<SensitiveTable[]>([]);

  useEffect(() => {
    if (!datasourceId) {
      const id = requestAnimationFrame(() => {
        setSensitiveTables([]);
      });
      return () => cancelAnimationFrame(id);
    }
    listSensitiveTables({ datasource_id: String(datasourceId), page_size: 500 })
      .then((res) => setSensitiveTables(res.data ?? []))
      .catch(() => setSensitiveTables([]));
  }, [datasourceId]);

  // Derive detected sensitive tables from SQL and sensitive tables list
  const detectedSensitive = useMemo(() => {
    if (!sql.trim() || sensitiveTables.length === 0) return [];
    // Extract table names from SQL (simple FROM/JOIN parsing)
    const sqlLower = sql.toLowerCase();
    const found = new Set<string>();
    const tableRegex = /(?:from|join)\s+`?([\w]+)`?/gi;
    let match;
    while ((match = tableRegex.exec(sqlLower)) !== null) {
      const tableName = match[1];
      if (
        sensitiveTables.some((t) => t.table_name.toLowerCase() === tableName)
      ) {
        found.add(tableName);
      }
    }
    return Array.from(found);
  }, [sql, sensitiveTables]);

  const canExecute = isMongo
    ? !executing && !!datasourceId && !!mongoCollection?.trim()
    : !executing && !!sql.trim() && !!datasourceId;

  async function doExport(format: "csv" | "json") {
    if (!datasourceId) {
      toast.error("请先选择数据源");
      return;
    }
    if (!sql.trim()) {
      toast.error(isMongo ? "请填写查询内容" : "请输入 SQL");
      return;
    }
    setExporting(true);
    try {
      await exportQuery({
        datasource_id: datasourceId,
        database,
        sql: sql.trim(),
        format,
      });
      toast.success("导出完成");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "导出失败");
    } finally {
      setExporting(false);
      setConfirmLargeExport(false);
      setExportFormat(null);
    }
  }

  function handleExport(format: "csv" | "json") {
    if (result && result.total > 10000) {
      setExportFormat(format);
      setConfirmLargeExport(true);
      return;
    }
    doExport(format);
  }

  return (
    /* §3.4: h-7 border-t border-default bg-surface px-3 */
    <div className="flex h-8 items-center justify-between border-t border-[var(--border-default)] bg-[var(--bg-surface)] px-3">
      <div className="flex items-center gap-3 text-xs text-[var(--text-secondary)]">
        {/* Execute button */}
        <Button
          size="sm"
          disabled={!canExecute}
          onClick={onExecute}
          className="h-7 gap-1 bg-[var(--accent-primary)] px-3 text-xs text-white hover:bg-[var(--accent-hover)] disabled:opacity-50"
        >
          {executing ? (
            <>
              <Loader2 size={12} className="animate-spin" />
              执行中
            </>
          ) : (
            <>
              <Play size={12} />
              执行
            </>
          )}
        </Button>

        {/* Stats */}
        {result && (
          <>
            <span>
              {result.execution_time_ms >= 1000
                ? `${(result.execution_time_ms / 1000).toFixed(2)}s`
                : `${result.execution_time_ms}ms`}
            </span>
            <span>{result.total} 行</span>
            {result.affected_rows > 0 && (
              <span>影响 {result.affected_rows} 行</span>
            )}
            {result.desensitized && result.desensitized_fields.length > 0 && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="cursor-default text-[var(--warning)]">
                    已脱敏 {result.desensitized_fields.length} 字段
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  脱敏字段: {result.desensitized_fields.join(", ")}
                </TooltipContent>
              </Tooltip>
            )}
          </>
        )}

        {/* Error */}
        {error && <span className="text-[var(--danger)]">{error}</span>}

        {/* Warnings */}
        {result?.warnings && result.warnings.length > 0 && (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-default text-[var(--warning)]">
                {result.warnings.length} 条警告
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {result.warnings.map((w, i) => (
                <div key={i}>{w}</div>
              ))}
            </TooltipContent>
          </Tooltip>
        )}

        {/* Sensitive table warning */}
        {detectedSensitive.length > 0 && !executing && (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="inline-flex cursor-default items-center gap-1 rounded bg-red-500/15 px-1.5 py-0.5 text-[11px] font-medium text-red-400">
                <ShieldAlert size={12} />
                敏感表: {detectedSensitive.join(", ")}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              <div className="text-xs">
                <p className="mb-1 font-medium text-red-400">⚠ 检测到敏感表</p>
                <p>
                  查询涉及 {detectedSensitive.length}{" "}
                  个敏感表，查询结果将自动脱敏处理。
                </p>
              </div>
            </TooltipContent>
          </Tooltip>
        )}

        {!result && !error && !executing && detectedSensitive.length === 0 && (
          <span className="text-[var(--text-muted)]">Ctrl+Enter 执行</span>
        )}
      </div>

      {/* Explain button */}
      {!isMongo && onExplain && (
        <Button
          variant="ghost"
          size="sm"
          disabled={!sql.trim() || !datasourceId || explaining}
          onClick={onExplain}
          className="h-7 gap-1 px-2 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)] disabled:opacity-30"
        >
          {explaining ? (
            <>
              <Loader2 size={12} className="animate-spin" />
              分析中...
            </>
          ) : (
            <>
              <GitBranch size={12} />
              执行计划
            </>
          )}
        </Button>
      )}

      {/* Export */}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            disabled={!hasResult || exporting}
            className="h-7 gap-1 px-2 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)] disabled:opacity-30"
          >
            {exporting ? (
              <>
                <Loader2 size={12} className="animate-spin" />
                正在导出...
              </>
            ) : (
              <>
                <Download size={12} />
                导出
              </>
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => handleExport("csv")}>
            CSV
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => handleExport("json")}>
            JSON
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {/* Large export confirmation */}
      <AlertDialog
        open={confirmLargeExport}
        onOpenChange={setConfirmLargeExport}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              确认导出
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              当前结果共 {result?.total ?? 0} 行，导出可能耗时较长，是否继续？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => exportFormat && doExport(exportFormat)}
              disabled={exporting}
              className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
            >
              {exporting ? "导出中..." : "确认导出"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
