import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { ChevronRight, ArrowUpDown, AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { CoverageBadge } from "./CoverageBadge";
import { CoverageProgressBar } from "./CoverageProgressBar";
import type { ModuleItem, ModuleListParams } from "@/types/coverage";

interface ModuleTableProps {
  modules: ModuleItem[];
  loading: boolean;
  error: string | null;
  sort: ModuleListParams["sort"];
  onSortChange: (sort: ModuleListParams["sort"]) => void;
  project: string;
}

const SORT_OPTIONS: { value: ModuleListParams["sort"]; label: string }[] = [
  { value: "line_rate:asc", label: "行覆盖率 ↑" },
  { value: "line_rate:desc", label: "行覆盖率 ↓" },
  { value: "branch_rate:asc", label: "分支覆盖率 ↑" },
  { value: "branch_rate:desc", label: "分支覆盖率 ↓" },
];

/** Module-level coverage table with sort controls and click-to-drill-down. */
export function ModuleTable({
  modules,
  loading,
  error,
  sort,
  onSortChange,
  project,
}: ModuleTableProps) {
  const navigate = useNavigate();
  const [hoveredRow, setHoveredRow] = useState<string | null>(null);

  if (loading) {
    return (
      <div className="space-y-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full rounded-md" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-12 text-center">
        <AlertTriangle size={32} className="text-red-400" />
        <p className="text-sm text-[var(--text-secondary)]">{error}</p>
        <p className="text-xs text-[var(--text-tertiary)]">
          请检查网络连接或稍后重试
        </p>
      </div>
    );
  }

  if (modules.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-12 text-center">
        <div className="text-4xl">📊</div>
        <p className="text-sm text-[var(--text-secondary)]">暂无覆盖度数据</p>
        <p className="text-xs text-[var(--text-tertiary)]">
          等待 CI 流水线上传测试覆盖率报告
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {/* Sort controls */}
      <div className="flex items-center gap-2">
        <ArrowUpDown size={14} className="text-[var(--text-tertiary)]" />
        <span className="text-xs text-[var(--text-tertiary)]">排序：</span>
        <div className="flex gap-1">
          {SORT_OPTIONS.map((opt) => (
            <Button
              key={opt.value}
              variant={sort === opt.value ? "default" : "ghost"}
              size="sm"
              className={cn(
                "h-7 px-2.5 text-xs",
                sort !== opt.value && "text-[var(--text-tertiary)]",
              )}
              onClick={() => onSortChange(opt.value)}
            >
              {opt.label}
            </Button>
          ))}
        </div>
      </div>

      {/* Table */}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-12" />
            <TableHead>模块路径</TableHead>
            <TableHead className="w-36">行覆盖率</TableHead>
            <TableHead className="w-36">分支覆盖率</TableHead>
            <TableHead className="w-20 text-center">文件数</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {modules.map((mod) => {
            const isCritical = mod.status === "critical";
            const isHovered = hoveredRow === mod.module_path;

            return (
              <TableRow
                key={mod.module_path}
                className={cn(
                  "cursor-pointer transition-colors",
                  isCritical && "bg-red-500/5 hover:bg-red-500/10",
                  isHovered && !isCritical && "bg-[var(--table-row-hover)]",
                )}
                onClick={() =>
                  navigate(`/coverage/modules/${encodeURIComponent(mod.module_path)}`, {
                    state: { project },
                  })
                }
                onMouseEnter={() => setHoveredRow(mod.module_path)}
                onMouseLeave={() => setHoveredRow(null)}
              >
                {/* Status indicator */}
                <TableCell>
                  {isCritical ? (
                    <AlertTriangle size={16} className="text-red-400" />
                  ) : (
                    <StatusDot status={mod.status} />
                  )}
                </TableCell>

                {/* Module path — critical gets red left border */}
                <TableCell>
                  <div className="relative flex items-center gap-2 pl-2">
                    {isCritical && (
                      <span className="absolute left-0 top-0 h-full w-1 rounded-r bg-red-500" />
                    )}
                    <span
                      className={cn(
                        "font-mono text-sm",
                        isCritical && "font-semibold",
                      )}
                    >
                      {mod.module_path}
                    </span>
                  </div>
                </TableCell>

                {/* Line coverage — reuses CoverageProgressBar */}
                <TableCell>
                  <CoverageProgressBar rate={mod.line_rate} status={mod.status} />
                </TableCell>

                {/* Branch coverage */}
                <TableCell>
                  <CoverageBadge status={mod.status} rate={mod.branch_rate} />
                </TableCell>

                {/* File count */}
                <TableCell className="text-center tabular-nums text-[var(--text-secondary)]">
                  {mod.file_count}
                </TableCell>

                {/* Navigate arrow */}
                <TableCell>
                  <ChevronRight
                    size={16}
                    className={cn(
                      "text-[var(--text-muted)] transition-colors",
                      isHovered && "text-[var(--text-secondary)]",
                    )}
                  />
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}

function StatusDot({ status }: { status: ModuleItem["status"] }) {
  const colors: Record<string, string> = {
    pass: "bg-emerald-500",
    warning: "bg-amber-500",
    critical: "bg-red-500",
  };
  return (
    <span className={cn("inline-block h-2 w-2 rounded-full", colors[status])} />
  );
}
