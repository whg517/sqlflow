import { useState, useEffect } from "react";
import {
  Check,
  X,
  SkipForward,
  Bot,
  ChevronDown,
  ChevronRight,
  MessageSquare,
} from "lucide-react";
import { cn } from "@/lib/utils";
import {
  getApprovalHistory,
  getStageStatusLabel,
  type ApprovalRecord,
} from "@/api/approval";
import { formatTime } from "@/api/ticket";

interface ApprovalHistoryProps {
  ticketId: number;
  revision: number;
}

function relativeTime(iso: string): string {
  const now = Date.now();
  const then = new Date(iso).getTime();
  const diff = now - then;
  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (days >= 7) {
    return formatTime(iso);
  }
  if (days > 0) return `${days} 天前`;
  if (hours > 0) return `${hours} 小时前`;
  if (minutes > 0) return `${minutes} 分钟前`;
  return "刚刚";
}

function ActionIcon({ action }: { action: ApprovalRecord["action"] }) {
  switch (action) {
    case "approved":
      return (
        <div className="flex h-6 w-6 items-center justify-center rounded-full bg-emerald-500/15">
          <Check size={12} className="text-emerald-500" />
        </div>
      );
    case "rejected":
      return (
        <div className="flex h-6 w-6 items-center justify-center rounded-full bg-red-500/15">
          <X size={12} className="text-red-500" />
        </div>
      );
    case "auto_approved":
      return (
        <div className="flex h-6 w-6 items-center justify-center rounded-full bg-blue-500/15">
          <Bot size={12} className="text-blue-500" />
        </div>
      );
    case "skipped":
      return (
        <div className="flex h-6 w-6 items-center justify-center rounded-full bg-zinc-500/15">
          <SkipForward size={12} className="text-zinc-400" />
        </div>
      );
    default:
      return (
        <div className="flex h-6 w-6 items-center justify-center rounded-full bg-zinc-500/15">
          <span className="text-xs text-zinc-400">?</span>
        </div>
      );
  }
}

function TimelineItem({ record }: { record: ApprovalRecord }) {
  const [expanded, setExpanded] = useState(false);
  const longComment = record.comment && record.comment.split("\n").length > 3;

  return (
    <div className="flex gap-3">
      <div className="flex flex-col items-center">
        <ActionIcon action={record.action} />
        <div className="w-px flex-1 bg-zinc-700/50" />
      </div>
      <div className="flex-1 pb-4">
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium text-[var(--text-primary)]">
            {record.approver_name || `角色: ${record.approver_role}`}
          </span>
          <span
            className={cn(
              "rounded-full px-1.5 py-0.5 text-[10px] font-medium",
              record.action === "approved" &&
                "bg-emerald-500/15 text-emerald-400",
              record.action === "rejected" &&
                "bg-red-500/15 text-red-400",
              record.action === "auto_approved" &&
                "bg-blue-500/15 text-blue-400",
              record.action === "skipped" &&
                "bg-zinc-500/15 text-zinc-400",
            )}
          >
            {getStageStatusLabel(record.action)}
          </span>
          <span className="text-xs text-zinc-500">
            阶段 {record.stage + 1}/{record.total_stages}
          </span>
        </div>

        {record.comment && (
          <div className="mt-1">
            <p
              className={cn(
                "text-xs text-zinc-400 whitespace-pre-wrap",
                !expanded && longComment && "line-clamp-3",
              )}
            >
              {record.comment}
            </p>
            {longComment && (
              <button
                className="mt-0.5 text-xs text-zinc-500 hover:text-zinc-300"
                onClick={() => setExpanded(!expanded)}
              >
                {expanded ? "收起" : "展开全文"}
              </button>
            )}
          </div>
        )}

        {record.auto_approved && record.auto_reason && (
          <p className="mt-0.5 text-xs text-blue-400/70">
            自动审批原因: {record.auto_reason}
          </p>
        )}

        <p className="mt-0.5 text-xs text-zinc-500">
          {relativeTime(record.created_at)}
        </p>
      </div>
    </div>
  );
}

export default function ApprovalHistory({
  ticketId,
  revision,
}: ApprovalHistoryProps) {
  // Fetch approval history when ticket changes
  const [records, setRecords] = useState<ApprovalRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    if (!ticketId) return;
    let active = true;
    const controller = new AbortController();
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true);
    getApprovalHistory(ticketId)
      .then((res) => {
        if (active) setRecords(res);
      })
      .catch(() => {
        if (active) setRecords([]);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [ticketId, revision]);

  if (loading) {
    return (
      <div className="space-y-3">
        {[1, 2].map((i) => (
          <div key={i} className="flex gap-3">
            <div className="h-6 w-6 rounded-full bg-zinc-800 animate-pulse" />
            <div className="flex-1 space-y-1">
              <div className="h-3 w-24 rounded bg-zinc-800 animate-pulse" />
              <div className="h-3 w-48 rounded bg-zinc-800 animate-pulse" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (records.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-6 text-center">
        <MessageSquare size={20} className="text-zinc-500 mb-2" />
        <p className="text-xs text-zinc-500">暂无审批记录</p>
      </div>
    );
  }

  // Group by revision
  const revisionGroups = new Map<number, ApprovalRecord[]>();
  for (const r of records) {
    // Use stage + total_stages to infer revision grouping
    // For now just show all records chronologically
    const group = revisionGroups.get(1) ?? [];
    group.push(r);
    revisionGroups.set(1, group);
  }

  return (
    <div className="space-y-1">
      <button
        className="flex items-center gap-1.5 text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
        onClick={() => setCollapsed(!collapsed)}
      >
        {collapsed ? (
          <ChevronRight size={14} />
        ) : (
          <ChevronDown size={14} />
        )}
        审批历史（{records.length} 条记录）
      </button>

      {!collapsed && (
        <div className="ml-1 mt-2">
          {records.map((r) => (
            <TimelineItem key={r.id} record={r} />
          ))}
        </div>
      )}
    </div>
  );
}
