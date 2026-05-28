import { AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { CoverageBadge } from "./CoverageBadge";
import { CoverageProgressBar } from "./CoverageProgressBar";
import type { FileItem } from "@/types/coverage";

interface FileTableProps {
  files: FileItem[];
  loading: boolean;
  error: string | null;
  modulePath: string;
}

/** File-level coverage table shown when drilling into a module. */
export function FileTable({ files, loading, error, modulePath }: FileTableProps) {
  if (loading) {
    return (
      <div className="space-y-3">
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className="h-10 w-full rounded-md" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-8 text-center">
        <AlertTriangle size={24} className="text-red-400" />
        <p className="text-sm text-[var(--text-secondary)]">{error}</p>
      </div>
    );
  }

  if (files.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-8 text-center">
        <p className="text-sm text-[var(--text-secondary)]">
          模块「{modulePath}」暂无文件级覆盖度数据
        </p>
      </div>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>文件路径</TableHead>
          <TableHead className="w-36">行覆盖率</TableHead>
          <TableHead className="w-28">分支覆盖率</TableHead>
          <TableHead className="w-24 text-right">覆盖行 / 总行</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {files.map((file) => {
          const isCritical = file.status === "critical";
          return (
            <TableRow key={file.file_path}>
              <TableCell>
                <span
                  className={cn(
                    "font-mono text-sm",
                    isCritical && "font-semibold",
                  )}
                >
                  {file.file_path}
                </span>
              </TableCell>
              <TableCell>
                <CoverageProgressBar rate={file.line_rate} status={file.status} />
              </TableCell>
              <TableCell>
                <CoverageBadge status={file.status} rate={file.branch_rate} />
              </TableCell>
              <TableCell className="text-right tabular-nums text-[var(--text-secondary)]">
                <span className={cn(isCritical && "text-red-400 font-semibold")}>
                  {file.lines_covered.toLocaleString()}
                </span>
                <span className="text-[var(--text-muted)]"> / </span>
                <span>{file.lines_total.toLocaleString()}</span>
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
