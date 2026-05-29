/**
 * ResubmitForm — SF-FEAT0044 Module B
 * Resubmit flow for rejected tickets
 */

import { useState, useMemo } from "react";
import { Loader2, AlertTriangle, Edit3, X, GitCompare } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";

interface ResubmitFormProps {
  originalSql: string;
  rejectionComment: string | null;
  originalChangeReason: string;
  riskLevel: string;
  revision: number;
  onResubmit: (sql: string, changeReason: string) => Promise<number>;
  loading: boolean;
  onSuccess?: () => void;
}

interface DiffLine {
  type: "added" | "removed" | "unchanged";
  content: string;
}

function computeDiff(oldText: string, newText: string): DiffLine[] {
  const oldLines = oldText.split("\n");
  const newLines = newText.split("\n");
  const result: DiffLine[] = [];
  let oi = 0;
  let ni = 0;

  while (oi < oldLines.length || ni < newLines.length) {
    if (oi >= oldLines.length) {
      result.push({ type: "added", content: newLines[ni] });
      ni++;
    } else if (ni >= newLines.length) {
      result.push({ type: "removed", content: oldLines[oi] });
      oi++;
    } else if (oldLines[oi] === newLines[ni]) {
      result.push({ type: "unchanged", content: oldLines[oi] });
      oi++;
      ni++;
    } else {
      const oldInNew = newLines.indexOf(oldLines[oi], ni);
      const newInOld = oldLines.indexOf(newLines[ni], oi);
      if (oldInNew === -1 && newInOld === -1) {
        result.push({ type: "removed", content: oldLines[oi] });
        result.push({ type: "added", content: newLines[ni] });
        oi++;
        ni++;
      } else if (newInOld === -1 || (oldInNew !== -1 && oldInNew - ni <= newInOld - oi)) {
        for (let k = ni; k < oldInNew; k++) {
          result.push({ type: "added", content: newLines[k] });
        }
        ni = oldInNew;
      } else {
        for (let k = oi; k < newInOld; k++) {
          result.push({ type: "removed", content: oldLines[k] });
        }
        oi = newInOld;
      }
    }
  }
  return result;
}

function collapseUnchanged(diff: DiffLine[], threshold = 3): DiffLine[] {
  const result: DiffLine[] = [];
  let unchangedBuf: DiffLine[] = [];

  for (const line of diff) {
    if (line.type === "unchanged") {
      unchangedBuf.push(line);
    } else {
      if (unchangedBuf.length > threshold) {
        result.push(unchangedBuf[0]);
        result.push({ type: "unchanged", content: `  ... ${unchangedBuf.length - 2} 行未变更 ...` });
        result.push(unchangedBuf[unchangedBuf.length - 1]);
      } else {
        result.push(...unchangedBuf);
      }
      unchangedBuf = [];
      result.push(line);
    }
  }
  if (unchangedBuf.length > threshold) {
    result.push(unchangedBuf[0]);
    result.push({ type: "unchanged", content: `  ... ${unchangedBuf.length - 2} 行未变更 ...` });
    result.push(unchangedBuf[unchangedBuf.length - 1]);
  } else {
    result.push(...unchangedBuf);
  }
  return result;
}

function DiffView({ oldText, newText }: { oldText: string; newText: string }) {
  const diff = useMemo(() => collapseUnchanged(computeDiff(oldText, newText)), [oldText, newText]);
  const hasChanges = diff.some((d) => d.type !== "unchanged");
  const addedCount = diff.filter((d) => d.type === "added").length;
  const removedCount = diff.filter((d) => d.type === "removed").length;

  if (!hasChanges) {
    return (
      <div className="flex items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs text-amber-400">
        <AlertTriangle size={14} />
        SQL 内容未修改
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs text-[var(--text-muted)]">
        <GitCompare size={12} />
        <span className="text-emerald-400">+{addedCount}</span>
        <span className="text-red-400">-{removedCount}</span>
      </div>
      <div className="overflow-auto rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs font-mono">
        {diff.map((line, idx) => (
          <div
            key={idx}
            className={cn(
              "flex border-l-2 px-3 py-0.5",
              line.type === "added" && "border-l-emerald-500 bg-emerald-500/10 text-emerald-300",
              line.type === "removed" && "border-l-red-500 bg-red-500/10 text-red-300 line-through",
              line.type === "unchanged" && "border-l-transparent text-[var(--text-muted)]",
            )}
          >
            <span className="w-4 shrink-0 select-none text-[var(--text-muted)]">
              {line.type === "added" ? "+" : line.type === "removed" ? "-" : " "}
            </span>
            <span className="whitespace-pre-wrap break-all">{line.content}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export default function ResubmitForm({
  originalSql,
  rejectionComment,
  originalChangeReason,
  riskLevel,
  revision,
  onResubmit,
  loading,
  onSuccess,
}: ResubmitFormProps) {
  const [editing, setEditing] = useState(false);
  const [newSql, setNewSql] = useState(originalSql);
  const [changeReason, setChangeReason] = useState(originalChangeReason);
  const [showDiff, setShowDiff] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const sqlChanged = newSql !== originalSql;
  const sqlEmpty = !newSql.trim();
  const reasonEmpty = !changeReason.trim();
  const canSubmit = !sqlEmpty && !reasonEmpty && !loading;

  function handleSubmit() {
    if (!canSubmit) {
      if (sqlEmpty) toast.error("SQL 内容不能为空");
      if (reasonEmpty) toast.error("请填写变更原因");
      return;
    }
    setConfirmOpen(true);
  }

  async function handleConfirm() {
    try {
      const newRev = await onResubmit(newSql.trim(), changeReason.trim());
      toast.success(`工单已重提，Revision ${newRev}`);
      setEditing(false);
      setConfirmOpen(false);
      onSuccess?.();
    } catch {
      // error handled by store
    }
  }

  if (!editing) {
    return (
      <Button
        size="sm"
        className="h-8 gap-1.5 bg-orange-500 px-3 text-xs text-white hover:bg-orange-600"
        onClick={() => setEditing(true)}
      >
        <Edit3 size={14} />
        修改重提
      </Button>
    );
  }

  return (
    <div className="space-y-4 rounded-lg border border-[var(--border-default)] bg-[var(--bg-elevated)] p-4 animate-in fade-in slide-in-from-bottom-2 duration-300">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-[var(--text-primary)]">修改并重提</h3>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 w-7 p-0"
          onClick={() => { setEditing(false); setNewSql(originalSql); setChangeReason(originalChangeReason); }}
        >
          <X size={14} />
        </Button>
      </div>

      {rejectionComment && (
        <div className="flex items-start gap-2 rounded-md border border-amber-500/20 bg-amber-500/5 px-3 py-2">
          <AlertTriangle size={14} className="mt-0.5 shrink-0 text-amber-400" />
          <div className="text-xs">
            <span className="font-medium text-amber-400">最新驳回意见：</span>
            <span className="text-[var(--text-secondary)] ml-1">{rejectionComment}</span>
          </div>
        </div>
      )}

      <div>
        <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
          SQL 内容 <span className="text-red-400">*</span>
        </label>
        <Textarea
          value={newSql}
          onChange={(e) => setNewSql(e.target.value)}
          className="min-h-[160px] font-mono text-xs border-[var(--border-default)] bg-[var(--bg-base)] text-[var(--text-primary)]"
          placeholder="输入修改后的 SQL..."
        />
      </div>

      <Collapsible open={showDiff} onOpenChange={setShowDiff}>
        <CollapsibleTrigger asChild>
          <Button variant="ghost" size="sm" className="h-7 gap-1.5 px-2 text-xs text-[var(--text-muted)]">
            <GitCompare size={12} />
            {showDiff ? "隐藏变更对比" : "查看变更对比"}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="mt-2">
          <DiffView oldText={originalSql} newText={newSql} />
        </CollapsibleContent>
      </Collapsible>

      <div>
        <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
          变更原因 <span className="text-red-400">*</span>
        </label>
        <Textarea
          value={changeReason}
          onChange={(e) => setChangeReason(e.target.value)}
          className="min-h-[60px] text-xs border-[var(--border-default)] bg-[var(--bg-base)] text-[var(--text-primary)]"
          placeholder="说明此次修改原因..."
        />
      </div>

      <div className="flex items-center gap-2">
        <Button
          size="sm"
          className="h-8 gap-1.5 bg-orange-500 px-4 text-xs text-white hover:bg-orange-600"
          onClick={handleSubmit}
          disabled={!canSubmit}
        >
          {loading ? <Loader2 size={14} className="animate-spin" /> : null}
          提交重提
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 px-3 text-xs text-[var(--text-muted)]"
          onClick={() => { setEditing(false); setNewSql(originalSql); setChangeReason(originalChangeReason); }}
        >
          取消
        </Button>
      </div>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认重提</AlertDialogTitle>
            <AlertDialogDescription asChild>
              <div className="space-y-2">
                <p>请确认以下变更信息：</p>
                <div className="rounded-md bg-[var(--bg-elevated)] p-3 text-xs space-y-1">
                  <div className="flex justify-between">
                    <span className="text-[var(--text-muted)]">SQL 变更</span>
                    <span className={sqlChanged ? "text-orange-400" : "text-amber-400"}>
                      {sqlChanged ? "已修改" : "⚠️ 未修改"}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-[var(--text-muted)]">风险等级</span>
                    <Badge variant="outline" className="text-[10px]">{riskLevel || "—"}</Badge>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-[var(--text-muted)]">Revision</span>
                    <span className="text-[var(--text-primary)]">{revision} → {revision + 1}</span>
                  </div>
                </div>
                {!sqlChanged && (
                  <div className="flex items-center gap-1.5 text-xs text-amber-400">
                    <AlertTriangle size={12} />
                    SQL 内容未做修改，请确认是否仍需重提
                  </div>
                )}
              </div>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={loading}>返回修改</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirm} disabled={loading} className="bg-orange-500 hover:bg-orange-600">
              {loading ? <Loader2 size={14} className="animate-spin" /> : null}
              确认重提
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
