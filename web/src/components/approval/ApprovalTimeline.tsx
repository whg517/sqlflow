/**
 * ApprovalTimeline — SF-FEAT0044 Module E
 */

import { useState, useMemo } from "react";
import {
  ChevronDown,
  ChevronRight,
  RotateCcw,
  Bot,
  CheckCircle2,
  XCircle,
  SkipForward,
  Ban,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { ApprovalHistoryEntry, ApprovalAction } from "@/types/approval";
import { actionLabel, actionColor, actionDot } from "@/types/approval";

function relativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const diffMin = Math.floor(diffMs / 60_000);
  const diffHour = Math.floor(diffMs / 3_600_000);
  const diffDay = Math.floor(diffMs / 86_400_000);
  if (diffMin < 1) return "刚刚";
  if (diffMin < 60) return `${diffMin} 分钟前`;
  if (diffHour < 24) return `${diffHour} 小时前`;
  if (diffDay < 7) return `${diffDay} 天前`;
  return new Date(iso).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}

function ActionIcon({ action }: { action: ApprovalAction }) {
  switch (action) {
    case "APPROVED": return <CheckCircle2 size={14} className="text-emerald-500" />;
    case "REJECTED": return <XCircle size={14} className="text-red-500" />;
    case "SKIPPED": return <SkipForward size={14} className="text-zinc-500" />;
    case "AUTO_APPROVED": return <Bot size={14} className="text-blue-500" />;
    case "RESUBMITTED": return <RotateCcw size={14} className="text-orange-500" />;
    case "CANCELLED": return <Ban size={14} className="text-gray-500" />;
  }
}

function CommentContent({ content }: { content: string }) {
  const lines = content.split("\n");
  const isLong = lines.length > 3;
  const [expanded, setExpanded] = useState(false);
  const displayLines = isLong && !expanded ? lines.slice(0, 3) : lines;

  return (
    <div className="space-y-0.5">
      {displayLines.map((line, i) => (
        <p key={i} className="text-xs text-[var(--text-secondary)]">
          {line.split(/(`[^`]+`)/g).map((part, j) =>
            part.startsWith("`") && part.endsWith("`")
              ? <code key={j} className="rounded bg-[var(--bg-elevated)] px-1 py-0.5 text-[10px] font-mono text-[var(--accent-primary)]">{part.slice(1, -1)}</code>
              : <span key={j}>{part}</span>
          )}
        </p>
      ))}
      {isLong && (
        <Button variant="link" size="sm" className="h-auto p-0 text-[10px] text-[var(--accent-primary)]" onClick={() => setExpanded(!expanded)}>
          {expanded ? "收起" : "展开全文"}
        </Button>
      )}
    </div>
  );
}

function RevisionGroup({
  revision,
  entries,
  isLatest,
}: {
  revision: number;
  entries: ApprovalHistoryEntry[];
  isLatest: boolean;
}) {
  const [open, setOpen] = useState(isLatest);
  const resubmitEntry = entries.find((e) => e.action === "RESUBMITTED");

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <button className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs hover:bg-[var(--bg-elevated)] transition-colors">
          {open ? <ChevronDown size={12} className="text-[var(--text-muted)]" /> : <ChevronRight size={12} className="text-[var(--text-muted)]" />}
          <span className="font-medium text-[var(--text-primary)]">Revision {revision}</span>
          {isLatest && <span className="rounded bg-[var(--accent-primary)]/10 px-1.5 py-0.5 text-[10px] text-[var(--accent-primary)]">当前</span>}
          <span className="ml-auto text-[var(--text-muted)]">{entries.length} 条记录</span>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="ml-2 mt-1 space-y-0">
        {resubmitEntry && resubmitEntry.sql_content && (
          <div className="mb-2 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] p-2">
            <p className="text-[10px] font-medium text-[var(--text-muted)] mb-1">重提 SQL:</p>
            <pre className="max-h-24 overflow-auto text-[10px] font-mono text-[var(--text-secondary)] whitespace-pre-wrap break-all">
              {resubmitEntry.sql_content}
            </pre>
            {resubmitEntry.change_reason && (
              <p className="mt-1 text-[10px] text-[var(--text-muted)]">变更原因: {resubmitEntry.change_reason}</p>
            )}
          </div>
        )}
        <div className="relative space-y-0 border-l-2 border-[var(--border-default)] pl-4">
          {entries.filter((e) => e.action !== "RESUBMITTED").map((entry) => (
            <div key={entry.id} className="relative pb-3">
              <div className={cn("absolute -left-[21px] top-0.5 h-2.5 w-2.5 rounded-full", actionDot[entry.action])} />
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <ActionIcon action={entry.action} />
                  <span className="text-xs font-medium text-[var(--text-primary)]">{entry.actor_name}</span>
                  <span className={cn("text-[10px]", actionColor[entry.action])}>{actionLabel[entry.action]}</span>
                  {entry.stage_label && (
                    <span className="rounded bg-[var(--bg-elevated)] px-1 py-0.5 text-[10px] text-[var(--text-muted)]">{entry.stage_label}</span>
                  )}
                </div>
                {entry.comment && <CommentContent content={entry.comment} />}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="text-[10px] text-[var(--text-muted)] cursor-default">{relativeTime(entry.created_at)}</span>
                  </TooltipTrigger>
                  <TooltipContent className="text-xs">{new Date(entry.created_at).toLocaleString("zh-CN")}</TooltipContent>
                </Tooltip>
              </div>
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

interface ApprovalTimelineProps {
  entries: ApprovalHistoryEntry[];
  loading?: boolean;
  className?: string;
}

export default function ApprovalTimeline({ entries, loading = false, className }: ApprovalTimelineProps) {
  const revisionGroups = useMemo(() => {
    const map = new Map<number, ApprovalHistoryEntry[]>();
    const sorted = [...entries].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
    for (const entry of sorted) {
      const rev = entry.revision;
      if (!map.has(rev)) map.set(rev, []);
      map.get(rev)!.push(entry);
    }
    return Array.from(map.entries()).sort((a, b) => b[0] - a[0]);
  }, [entries]);

  const latestRevision = revisionGroups[0]?.[0] ?? 0;

  if (loading) {
    return (
      <div className={cn("space-y-2", className)}>
        {[1, 2, 3].map((i) => (
          <div key={i} className="h-8 animate-pulse rounded bg-[var(--bg-elevated)]" />
        ))}
      </div>
    );
  }

  if (entries.length === 0) {
    return (
      <div className={cn("flex flex-col items-center gap-2 py-8", className)}>
        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--bg-elevated)]">
          <Ban size={16} className="text-[var(--text-muted)]" />
        </div>
        <p className="text-xs text-[var(--text-muted)]">暂无审批记录</p>
      </div>
    );
  }

  return (
    <div className={cn("space-y-2", className)}>
      {revisionGroups.map(([revision, groupEntries]) => (
        <RevisionGroup key={revision} revision={revision} entries={groupEntries} isLatest={revision === latestRevision} />
      ))}
    </div>
  );
}
