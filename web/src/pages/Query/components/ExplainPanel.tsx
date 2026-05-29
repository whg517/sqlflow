"use no memo";

import { useState, useMemo } from "react";
import { AlertTriangle, Loader2, Table2, FileText, GitBranch } from "lucide-react";
import ExplainTreeView from "./ExplainTreeView";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { ExplainRow } from "@/api/explain";

// --- Highlight Helpers ---

type IssueLevel = "danger" | "warning";

interface ExplainIssue {
  row: number;
  field: string;
  message: string;
  level: IssueLevel;
}

function analyzeRow(row: ExplainRow, idx: number): ExplainIssue[] {
  const issues: ExplainIssue[] = [];

  if (row.type === "ALL") {
    issues.push({
      row: idx,
      field: "type",
      message: "全表扫描 — 考虑添加合适的索引",
      level: "danger",
    });
  } else if (row.type === "index") {
    issues.push({
      row: idx,
      field: "type",
      message: "索引全扫描 — 检查是否缺少查询条件",
      level: "warning",
    });
  }

  if (row.key === null) {
    issues.push({
      row: idx,
      field: "key",
      message: "未使用索引",
      level: "danger",
    });
  }

  if (row.extra && row.extra.includes("Using filesort")) {
    issues.push({
      row: idx,
      field: "extra",
      message: "额外排序（filesort）— 考虑优化 ORDER BY 子句",
      level: "warning",
    });
  }

  if (row.extra && row.extra.includes("Using temporary")) {
    issues.push({
      row: idx,
      field: "extra",
      message: "使用临时表 — 考虑优化 GROUP BY 或 DISTINCT",
      level: "warning",
    });
  }

  if (row.rows > 10000) {
    issues.push({
      row: idx,
      field: "rows",
      message: `扫描行数较多（${row.rows.toLocaleString()}）`,
      level: "warning",
    });
  }

  return issues;
}

function HighlightedCell({
  value,
  field,
  issues,
}: {
  value: string;
  field: keyof ExplainRow;
  issues: ExplainIssue[];
}) {
  const issue = issues.find((i) => i.field === field);
  if (!issue) return <span>{value}</span>;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={`inline-flex items-center gap-1 font-medium ${
            issue.level === "danger"
              ? "text-red-400"
              : "text-yellow-400"
          }`}
        >
          {value}
          <AlertTriangle size={11} />
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs text-xs">
        {issue.message}
      </TooltipContent>
    </Tooltip>
  );
}

// --- Structured View ---

function StructuredView({ plan }: { plan: ExplainRow[] }) {
  const columns = [
    { key: "id", label: "id" },
    { key: "select_type", label: "select_type" },
    { key: "table", label: "table" },
    { key: "type", label: "type" },
    { key: "possible_keys", label: "possible_keys" },
    { key: "key", label: "key" },
    { key: "key_len", label: "key_len" },
    { key: "ref", label: "ref" },
    { key: "rows", label: "rows" },
    { key: "filtered", label: "filtered" },
    { key: "extra", label: "Extra" },
  ] as const;

  const allIssues = useMemo(
    () => plan.flatMap((row, idx) => analyzeRow(row, idx)),
    [plan],
  );

  const dangerCount = allIssues.filter((i) => i.level === "danger").length;
  const warningCount = allIssues.filter((i) => i.level === "warning").length;

  return (
    <div className="flex flex-col gap-3">
      {/* Summary */}
      {allIssues.length > 0 && (
        <div className="flex items-center gap-2">
          {dangerCount > 0 && (
            <Badge variant="danger" className="gap-1">
              <AlertTriangle size={11} />
              {dangerCount} 个严重问题
            </Badge>
          )}
          {warningCount > 0 && (
            <Badge variant="warning" className="gap-1">
              <AlertTriangle size={11} />
              {warningCount} 个警告
            </Badge>
          )}
        </div>
      )}

      {/* Table */}
      <div className="overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-[var(--border-default)]">
              {columns.map((col) => (
                <th
                  key={col.key}
                  className="whitespace-nowrap px-2 py-1.5 text-left font-medium text-[var(--text-secondary)]"
                >
                  {col.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {plan.map((row, idx) => {
              const rowIssues = allIssues.filter((i) => i.row === idx);
              const hasDanger = rowIssues.some((i) => i.level === "danger");
              const hasWarning = rowIssues.some((i) => i.level === "warning");
              return (
                <tr
                  key={idx}
                  className={`border-b border-[var(--border-default)] ${
                    hasDanger
                      ? "bg-red-500/5"
                      : hasWarning
                        ? "bg-yellow-500/5"
                        : "hover:bg-[var(--bg-elevated)]/50"
                  }`}
                >
                  {columns.map((col) => (
                    <td
                      key={col.key}
                      className="whitespace-nowrap px-2 py-1.5 font-mono"
                    >
                      <HighlightedCell
                        value={row[col.key] === null ? "NULL" : String(row[col.key])}
                        field={col.key}
                        issues={rowIssues}
                      />
                    </td>
                  ))}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// --- Props ---

interface ExplainPanelProps {
  plan: ExplainRow[];
  formatted: string;
  loading?: boolean;
  error?: string | null;
}

// --- Main Component ---

type ViewMode = "structured" | "tree" | "text";

export default function ExplainPanel({
  plan,
  formatted,
  loading,
  error,
}: ExplainPanelProps) {
  const [viewMode, setViewMode] = useState<ViewMode>("structured");

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 size={20} className="animate-spin text-[var(--text-muted)]" />
        <span className="ml-2 text-sm text-[var(--text-muted)]">
          正在获取执行计划...
        </span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center py-12">
        <span className="text-sm text-[var(--danger)]">{error}</span>
      </div>
    );
  }

  if (plan.length === 0) {
    return (
      <div className="flex items-center justify-center py-12">
        <span className="text-sm text-[var(--text-muted)]">无执行计划数据</span>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      {/* View mode toggle */}
      <div className="flex items-center gap-1">
        <Button
          variant={viewMode === "structured" ? "secondary" : "ghost"}
          size="sm"
          className="h-7 gap-1 text-xs"
          onClick={() => setViewMode("structured")}
        >
          <Table2 size={12} />
          结构化
        </Button>
        <Button
          variant={viewMode === "tree" ? "secondary" : "ghost"}
          size="sm"
          className="h-7 gap-1 text-xs"
          onClick={() => setViewMode("tree")}
        >
          <GitBranch size={12} />
          树形
        </Button>
        <Button
          variant={viewMode === "text" ? "secondary" : "ghost"}
          size="sm"
          className="h-7 gap-1 text-xs"
          onClick={() => setViewMode("text")}
        >
          <FileText size={12} />
          文本
        </Button>
      </div>

      {/* Content */}
      {viewMode === "structured" ? (
        <StructuredView plan={plan} />
      ) : viewMode === "tree" ? (
        <ExplainTreeView plan={plan} />
      ) : (
        <pre className="overflow-x-auto rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3 font-mono text-xs text-[var(--text-primary)]">
          {formatted}
        </pre>
      )}
    </div>
  );
}
