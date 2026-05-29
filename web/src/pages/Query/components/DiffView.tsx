import { useMemo, useState } from "react";
import {
  ChevronLeft,
  ChevronRight,
  Plus,
  Minus,
  ArrowRightLeft,
  Filter,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { DiffRow, CompareResult } from "@/api/snapshot";

interface DiffViewProps {
  result: CompareResult;
}

type FilterMode = "all" | "changed" | "added" | "removed" | "modified";

const ROWS_PER_PAGE = 50;

export default function DiffView({ result }: DiffViewProps) {
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<FilterMode>("changed");

  const filteredRows = useMemo(() => {
    if (filter === "all") return result.diffRows;
    if (filter === "changed")
      return result.diffRows.filter((r) => r.type !== "unchanged");
    return result.diffRows.filter((r) => r.type === filter);
  }, [result.diffRows, filter]);

  const totalPages = Math.max(1, Math.ceil(filteredRows.length / ROWS_PER_PAGE));
  const pageRows = filteredRows.slice(
    (page - 1) * ROWS_PER_PAGE,
    page * ROWS_PER_PAGE,
  );

  const rowBg = (type: DiffRow["type"]) => {
    switch (type) {
      case "added":
        return "bg-[var(--success)]/10";
      case "removed":
        return "bg-[var(--danger)]/10";
      case "modified":
        return "bg-[var(--warning)]/10";
      default:
        return "";
    }
  };

  const rowBadge = (type: DiffRow["type"]) => {
    switch (type) {
      case "added":
        return <Badge className="bg-[var(--success)]/20 text-[var(--success)] border-0 text-[10px] px-1.5">+ 新增</Badge>;
      case "removed":
        return <Badge className="bg-[var(--danger)]/20 text-[var(--danger)] border-0 text-[10px] px-1.5">- 删除</Badge>;
      case "modified":
        return <Badge className="bg-[var(--warning)]/20 text-[var(--warning)] border-0 text-[10px] px-1.5">~ 修改</Badge>;
      default:
        return null;
    }
  };

  const formatCell = (value: unknown, changed?: boolean) => {
    const str = value === null || value === undefined ? "NULL" : String(value);
    if (changed) {
      return <span className="bg-[var(--warning)]/20 text-[var(--warning)] px-0.5 rounded text-[11px]">{str}</span>;
    }
    return <span className="text-[var(--text-primary)]">{str}</span>;
  };

  return (
    <div className="flex flex-col h-full">
      {/* Summary bar */}
      <div className="flex items-center gap-4 px-4 py-2 border-b border-[var(--border-default)] bg-[var(--bg-surface)]">
        <div className="flex items-center gap-3 text-xs">
          <span className="text-[var(--text-secondary)]">
            左 {result.totalLeft} 行 → 右 {result.totalRight} 行
          </span>
          <div className="h-3 w-px bg-[var(--border-default)]" />
          <span className="flex items-center gap-1 text-[var(--success)]">
            <Plus size={12} />{result.summary.added} 新增
          </span>
          <span className="flex items-center gap-1 text-[var(--danger)]">
            <Minus size={12} />{result.summary.removed} 删除
          </span>
          <span className="flex items-center gap-1 text-[var(--warning)]">
            <ArrowRightLeft size={12} />{result.summary.modified} 修改
          </span>
          <span className="text-[var(--text-muted)]">
            {result.summary.unchanged} 未变
          </span>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Filter size={12} className="text-[var(--text-muted)]" />
          <Select value={filter} onValueChange={(v) => { setFilter(v as FilterMode); setPage(1); }}>
            <SelectTrigger className="h-6 w-24 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="changed">差异项</SelectItem>
              <SelectItem value="all">全部</SelectItem>
              <SelectItem value="added">新增</SelectItem>
              <SelectItem value="removed">删除</SelectItem>
              <SelectItem value="modified">修改</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Diff table */}
      <div className="flex-1 overflow-auto">
        <table className="w-full text-xs border-collapse">
          <thead className="sticky top-0 z-10 bg-[var(--bg-surface)] border-b border-[var(--border-default)]">
            <tr>
              <th className="w-10 px-2 py-2 text-left text-[var(--text-muted)] font-normal">#</th>
              <th className="w-16 px-2 py-2 text-left text-[var(--text-muted)] font-normal">状态</th>
              {result.columns.map((col) => (
                <th
                  key={col}
                  className="px-3 py-2 text-left text-[var(--text-secondary)] font-medium whitespace-nowrap"
                >
                  {col}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {pageRows.length === 0 ? (
              <tr>
                <td colSpan={result.columns.length + 2} className="text-center py-8 text-[var(--text-muted)]">
                  没有匹配的差异记录
                </td>
              </tr>
            ) : (
              pageRows.map((row) => {
                const displayData = row.type === "removed" ? row.left : row.right;
                return (
                  <tr
                    key={row.rowIndex}
                    className={`border-b border-[var(--border-subtle)] ${rowBg(row.type)}`}
                  >
                    <td className="px-2 py-1.5 text-[var(--text-muted)]">{row.rowIndex + 1}</td>
                    <td className="px-2 py-1.5">{rowBadge(row.type)}</td>
                    {result.columns.map((col) => {
                      const leftVal = row.left?.[col];
                      const rightVal = row.right?.[col];
                      const changed =
                        row.type === "modified" &&
                        row.changedFields?.includes(col);
                      const value =
                        row.type === "removed" ? leftVal : rightVal;
                      return (
                        <td key={col} className="px-3 py-1.5 max-w-[200px] truncate">
                          {formatCell(value, changed)}
                        </td>
                      );
                    })}
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between px-4 py-2 border-t border-[var(--border-default)] bg-[var(--bg-surface)]">
          <span className="text-xs text-[var(--text-muted)]">
            共 {filteredRows.length} 条差异，第 {page}/{totalPages} 页
          </span>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
            >
              <ChevronLeft size={12} />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0"
              disabled={page >= totalPages}
              onClick={() => setPage(page + 1)}
            >
              <ChevronRight size={12} />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
