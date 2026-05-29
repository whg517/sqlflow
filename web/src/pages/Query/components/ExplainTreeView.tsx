"use no memo";

import { useMemo, useState } from "react";
import {
  AlertTriangle,
  ChevronRight,
  ChevronDown,
  Database,
  Zap,
  Filter,
  Clock,
  HardDrive,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { ExplainRow } from "@/api/explain";

// --- Tree Node Types ---

interface TreeNode extends ExplainRow {
  children: TreeNode[];
  issues: NodeIssue[];
  depth: number;
}

interface NodeIssue {
  field: string;
  message: string;
  level: "danger" | "warning";
}

// --- Issue Analysis ---

function analyzeNodeIssues(row: ExplainRow): NodeIssue[] {
  const issues: NodeIssue[] = [];

  if (row.type === "ALL") {
    issues.push({
      field: "type",
      message: "全表扫描 — 考虑添加合适的索引",
      level: "danger",
    });
  } else if (row.type === "index") {
    issues.push({
      field: "type",
      message: "索引全扫描 — 检查是否缺少查询条件",
      level: "warning",
    });
  }

  if (row.key === null) {
    issues.push({
      field: "key",
      message: "未使用索引",
      level: "danger",
    });
  }

  if (row.extra && row.extra.includes("Using filesort")) {
    issues.push({
      field: "extra",
      message: "额外排序（filesort）— 考虑优化 ORDER BY",
      level: "warning",
    });
  }

  if (row.extra && row.extra.includes("Using temporary")) {
    issues.push({
      field: "extra",
      message: "使用临时表 — 考虑优化 GROUP BY",
      level: "warning",
    });
  }

  if (row.rows > 10000) {
    issues.push({
      field: "rows",
      message: `扫描行数较多（${row.rows.toLocaleString()}）`,
      level: "warning",
    });
  }

  return issues;
}

// --- Type Access Method Badge ---

function getAccessTypeIcon(type: string) {
  switch (type) {
    case "ALL":
      return <HardDrive size={12} className="text-red-400" />;
    case "index":
      return <Zap size={12} className="text-yellow-400" />;
    case "range":
      return <Filter size={12} className="text-blue-400" />;
    case "ref":
      return <Filter size={12} className="text-green-400" />;
    case "eq_ref":
      return <Filter size={12} className="text-green-400" />;
    case "const":
      return <Zap size={12} className="text-emerald-400" />;
    case "system":
      return <Zap size={12} className="text-emerald-400" />;
    default:
      return <Database size={12} className="text-[var(--text-muted)]" />;
  }
}

function getAccessTypeColor(type: string): string {
  switch (type) {
    case "ALL":
      return "bg-red-500/10 text-red-400 border-red-500/20";
    case "index":
      return "bg-yellow-500/10 text-yellow-400 border-yellow-500/20";
    case "range":
      return "bg-blue-500/10 text-blue-400 border-blue-500/20";
    case "ref":
    case "eq_ref":
      return "bg-green-500/10 text-green-400 border-green-500/20";
    case "const":
    case "system":
      return "bg-emerald-500/10 text-emerald-400 border-emerald-500/20";
    default:
      return "bg-[var(--bg-elevated)] text-[var(--text-secondary)] border-[var(--border-default)]";
  }
}

// --- Tree Building Logic ---

function buildTree(plan: ExplainRow[]): TreeNode[] {
  // MySQL EXPLAIN: id is the step identifier.
  // Same id means same select/subquery level.
  // Build a tree based on id grouping with depth tracking.
  const nodes: TreeNode[] = [];

  // Group by id to understand structure
  const idGroups = new Map<number, ExplainRow[]>();
  for (const row of plan) {
    const list = idGroups.get(row.id) || [];
    list.push(row);
    idGroups.set(row.id, list);
  }

  // Build tree: root level nodes are those that appear as the first
  // occurrence of a new id. Children are subsequent rows with the same id.
  let currentParent: TreeNode | null = null;
  let currentId: number | null = null;

  for (const row of plan) {
    const node: TreeNode = {
      ...row,
      children: [],
      issues: analyzeNodeIssues(row),
      depth: 0,
    };

    if (row.id !== currentId) {
      // New id — this is a top-level or sibling node
      node.depth = 0;
      nodes.push(node);
      currentParent = node;
      currentId = row.id;
    } else {
      // Same id — this is a child of the current parent
      node.depth = (currentParent?.depth ?? 0) + 1;
      currentParent?.children.push(node);
    }
  }

  return nodes;
}

// --- Issue Count Summary ---

function countIssues(nodes: TreeNode[]): { danger: number; warning: number } {
  let danger = 0;
  let warning = 0;
  function walk(list: TreeNode[]) {
    for (const n of list) {
      danger += n.issues.filter((i) => i.level === "danger").length;
      warning += n.issues.filter((i) => i.level === "warning").length;
      walk(n.children);
    }
  }
  walk(nodes);
  return { danger, warning };
}

// --- Single Tree Node Card ---

function TreeCard({
  node,
  defaultExpanded,
}: {
  node: TreeNode;
  defaultExpanded: boolean;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const hasChildren = node.children.length > 0;
  const hasDanger = node.issues.some((i) => i.level === "danger");
  const hasWarning = node.issues.some((i) => i.level === "warning");

  const borderColor = hasDanger
    ? "border-l-red-400"
    : hasWarning
      ? "border-l-yellow-400"
      : "border-l-[var(--border-default)]";

  const bgColor = hasDanger
    ? "bg-red-500/5"
    : hasWarning
      ? "bg-yellow-500/5"
      : "bg-[var(--bg-elevated)]";

  return (
    <div className={`${node.depth > 0 ? "ml-6" : ""}`}>
      <div
        className={`flex items-start gap-2 rounded-md border-l-2 ${borderColor} ${bgColor} px-3 py-2`}
      >
        {/* Expand toggle */}
        {hasChildren ? (
          <button
            onClick={() => setExpanded(!expanded)}
            className="mt-0.5 flex-shrink-0 text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors"
          >
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
        ) : (
          <span className="mt-0.5 w-3.5 flex-shrink-0" />
        )}

        {/* Main content */}
        <div className="flex-1 min-w-0">
          {/* Header row: table name + access type */}
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-mono text-sm font-medium text-[var(--text-primary)]">
              {node.table || "NULL"}
            </span>
            <Tooltip>
              <TooltipTrigger asChild>
                <span
                  className={`inline-flex items-center gap-1 rounded-full border px-1.5 py-0.5 text-[10px] font-mono ${getAccessTypeColor(node.type)}`}
                >
                  {getAccessTypeIcon(node.type)}
                  {node.type}
                </span>
              </TooltipTrigger>
              <TooltipContent className="text-xs">访问类型: {node.type}</TooltipContent>
            </Tooltip>
            <span className="text-[10px] text-[var(--text-muted)] font-mono">
              id={node.id}
            </span>
            {node.select_type && node.select_type !== "SIMPLE" && (
              <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                {node.select_type}
              </Badge>
            )}

            {/* Issue indicators */}
            {node.issues.map((issue, i) => (
              <Tooltip key={`${node.id}-${node.table}-${i}`}>
                <TooltipTrigger asChild>
                  <span
                    className={`inline-flex items-center gap-0.5 ${issue.level === "danger" ? "text-red-400" : "text-yellow-400"}`}
                  >
                    <AlertTriangle size={11} />
                  </span>
                </TooltipTrigger>
                <TooltipContent className="max-w-xs text-xs">
                  {issue.message}
                </TooltipContent>
              </Tooltip>
            ))}
          </div>

          {/* Detail row */}
          <div className="mt-1 flex items-center gap-3 flex-wrap text-[11px] text-[var(--text-secondary)]">
            {node.key !== null ? (
              <span className="inline-flex items-center gap-1 font-mono">
                <Filter size={10} />
                key: {node.key}
              </span>
            ) : (
              <span className="inline-flex items-center gap-1 font-mono text-red-400/70">
                <Filter size={10} />
                key: NULL
              </span>
            )}
            {node.possible_keys && (
              <span className="font-mono truncate max-w-[200px]">
                possible: {node.possible_keys}
              </span>
            )}
            <span className="inline-flex items-center gap-1 font-mono">
              <HardDrive size={10} />
              rows: {node.rows.toLocaleString()}
            </span>
            {node.filtered > 0 && (
              <span className="inline-flex items-center gap-1 font-mono">
                filtered: {node.filtered}%
              </span>
            )}
          </div>

          {/* Extra info */}
          {node.extra && (
            <div className="mt-1 text-[11px] text-[var(--text-muted)] font-mono flex items-center gap-1">
              <Clock size={10} />
              {node.extra}
            </div>
          )}

          {/* Issue details */}
          {node.issues.length > 0 && expanded && (
            <div className="mt-1.5 space-y-0.5">
              {node.issues.map((issue, i) => (
                <div
                  key={i}
                  className={`text-[11px] flex items-center gap-1 ${
                    issue.level === "danger" ? "text-red-400/80" : "text-yellow-400/80"
                  }`}
                >
                  <AlertTriangle size={10} />
                  <span>{issue.message}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Children */}
      {hasChildren && expanded && (
        <div className="mt-1 space-y-1 border-l border-[var(--border-default)] ml-4 pl-2">
          {node.children.map((child, i) => (
            <TreeCard
              key={`${child.id}-${child.table}-${i}`}
              node={child}
              defaultExpanded={true}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// --- Main Tree View Component ---

export default function ExplainTreeView({ plan }: { plan: ExplainRow[] }) {
  const tree = useMemo(() => buildTree(plan), [plan]);
  const { danger, warning } = useMemo(() => countIssues(tree), [tree]);

  return (
    <div className="flex flex-col gap-3">
      {/* Summary badges */}
      {(danger > 0 || warning > 0) && (
        <div className="flex items-center gap-2">
          {danger > 0 && (
            <Badge variant="danger" className="gap-1">
              <AlertTriangle size={11} />
              {danger} 个严重问题
            </Badge>
          )}
          {warning > 0 && (
            <Badge variant="warning" className="gap-1">
              <AlertTriangle size={11} />
              {warning} 个警告
            </Badge>
          )}
        </div>
      )}

      {/* Legend */}
      <div className="flex items-center gap-3 flex-wrap text-[10px] text-[var(--text-muted)]">
        <span className="inline-flex items-center gap-1">
          <span className="w-2 h-2 rounded-full bg-red-400" /> 全表扫描 / 严重问题
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="w-2 h-2 rounded-full bg-yellow-400" /> 警告
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="w-2 h-2 rounded-full bg-green-400" /> 索引命中
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="w-2 h-2 rounded-full bg-emerald-400" /> 常量 / 系统
        </span>
      </div>

      {/* Tree */}
      <div className="space-y-2">
        {tree.map((node, i) => (
          <TreeCard
            key={`${node.id}-${node.table}-${i}`}
            node={node}
            defaultExpanded={true}
          />
        ))}
      </div>

      {tree.length === 0 && (
        <div className="text-center py-8 text-sm text-[var(--text-muted)]">
          无执行计划数据
        </div>
      )}
    </div>
  );
}
