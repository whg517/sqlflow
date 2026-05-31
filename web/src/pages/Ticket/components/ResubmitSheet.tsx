import { useState, useMemo } from "react";
import { Loader2, AlertTriangle, RotateCcw } from "lucide-react";
import { toast } from "sonner";
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
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { resubmitTicket } from "@/api/approval";
import type { Ticket } from "@/api/ticket";

interface ResubmitSheetProps {
  ticket: Ticket;
  onSuccess: () => void;
}

function computeDiff(oldSql: string, newSql: string) {
  const oldLines = oldSql.split("\n");
  const newLines = newSql.split("\n");
  const diffLines: {
    type: "unchanged" | "added" | "removed";
    content: string;
    lineNum: number;
  }[] = [];

  let oi = 0;
  let ni = 0;
  let unchangedCount = 0;

  while (oi < oldLines.length || ni < newLines.length) {
    const oldLine = oi < oldLines.length ? oldLines[oi] : null;
    const newLine = ni < newLines.length ? newLines[ni] : null;

    if (oldLine === newLine) {
      unchangedCount++;
      // Collapse consecutive unchanged lines > 3
      if (unchangedCount <= 3 || oi >= oldLines.length - 1 || ni >= newLines.length - 1) {
        diffLines.push({ type: "unchanged", content: oldLine ?? "", lineNum: oi + 1 });
      } else if (unchangedCount === 4) {
        diffLines.push({ type: "unchanged", content: "...", lineNum: -1 });
      }
      oi++;
      ni++;
    } else {
      unchangedCount = 0;
      if (ni < newLines.length && (oldLine === undefined || oldLine !== newLine)) {
        diffLines.push({ type: "added", content: newLine!, lineNum: ni + 1 });
        ni++;
      }
      if (oi < oldLines.length && oldLine !== newLine) {
        diffLines.push({ type: "removed", content: oldLine!, lineNum: oi + 1 });
        oi++;
      }
    }
  }
  return diffLines;
}

export default function ResubmitSheet({ ticket, onSuccess }: ResubmitSheetProps) {
  const [sqlContent, setSqlContent] = useState(ticket.sql_content);
  const [changeReason, setChangeReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const sqlChanged = useMemo(
    () => sqlContent.trim() !== ticket.sql_content.trim(),
    [sqlContent, ticket.sql_content],
  );

  const diffLines = useMemo(
    () => computeDiff(ticket.sql_content, sqlContent),
    [ticket.sql_content, sqlContent],
  );

  const canSubmit = sqlContent.trim() && changeReason.trim();

  async function handleSubmit() {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      await resubmitTicket(ticket.id, {
        sql_content: sqlContent.trim(),
        change_reason: changeReason.trim(),
      });
      toast.success("工单已重提，等待审批");
      setConfirmOpen(false);
      onSuccess();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "重提失败");
    } finally {
      setSubmitting(false);
    }
  }

  // Get latest reject comment from review_comment
  const rejectComment = ticket.review_comment;

  return (
    <div className="space-y-4">
      {/* Reject reason alert */}
      {rejectComment && (
        <div className="rounded-md border border-amber-500/20 bg-amber-500/5 px-3 py-2">
          <div className="flex items-center gap-2 text-xs font-medium text-amber-400">
            <AlertTriangle size={12} />
            驳回意见
          </div>
          <p className="mt-1 text-xs text-amber-300/70 whitespace-pre-wrap">
            {rejectComment}
          </p>
        </div>
      )}

      {/* SQL Editor */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
          修改 SQL <span className="text-red-400">*</span>
        </label>

        {/* Diff view */}
        <div className="rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] overflow-hidden">
          <div className="flex items-center justify-between border-b border-[var(--border-default)] px-3 py-1.5">
            <span className="text-xs text-[var(--text-muted)]">
              {sqlChanged ? "SQL 变更对比" : "SQL 内容（未修改）"}
            </span>
            {!sqlChanged && (
              <span className="flex items-center gap-1 text-xs text-amber-400">
                <AlertTriangle size={12} />
                未修改
              </span>
            )}
          </div>

          {sqlChanged ? (
            <div className="max-h-48 overflow-auto p-2 font-mono text-xs">
              {diffLines.map((line, idx) => (
                <div
                  key={idx}
                  className={`
                    flex min-h-[20px] items-start border-l-2 px-2
                    ${line.type === "added"
                      ? "border-l-emerald-500 bg-emerald-500/10 text-emerald-400"
                      : line.type === "removed"
                        ? "border-l-red-500 bg-red-500/10 text-red-400 line-through"
                        : line.content === "..."
                          ? "border-l-transparent text-zinc-500 text-center"
                          : "border-l-transparent text-[var(--text-muted)]"
                    }
                  `}
                >
                  {line.lineNum > 0 && (
                    <span className="mr-2 w-6 text-right text-zinc-600 select-none">
                      {line.lineNum}
                    </span>
                  )}
                  <span className="whitespace-pre-wrap break-all">
                    {line.type === "added" && "+ "}
                    {line.type === "removed" && "- "}
                    {line.content}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <div className="max-h-48 overflow-auto p-3">
              <Textarea
                value={sqlContent}
                onChange={(e) => setSqlContent(e.target.value)}
                className="min-h-[120px] w-full border-0 bg-transparent p-0 text-xs font-mono text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-0 resize-none"
                placeholder="输入修改后的 SQL..."
              />
            </div>
          )}
        </div>
      </div>

      {/* Change reason */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
          变更原因 <span className="text-red-400">*</span>
        </label>
        <Textarea
          value={changeReason}
          onChange={(e) => setChangeReason(e.target.value)}
          placeholder="说明此次修改的原因..."
          className="min-h-[60px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
        />
      </div>

      {/* Submit button */}
      <div className="flex justify-end gap-2">
        <Button
          size="sm"
          className="h-8 gap-1.5 bg-[var(--accent-primary)] px-4 text-xs text-white hover:bg-[var(--accent-hover)]"
          disabled={!canSubmit || submitting}
          onClick={() => setConfirmOpen(true)}
        >
          {submitting ? (
            <Loader2 size={14} className="animate-spin" />
          ) : (
            <RotateCcw size={14} />
          )}
          提交重提
        </Button>
      </div>

      {/* Confirm dialog */}
      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认重提工单</AlertDialogTitle>
            <AlertDialogDescription>
              确认要重提工单 #{ticket.id} 吗？
              <br />
              {sqlChanged
                ? `SQL 已修改，Revision 将从 ${ticket.revision} 变为 ${ticket.revision + 1}`
                : "⚠️ SQL 未做修改，审批人可能再次驳回"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-1 text-xs text-[var(--text-muted)]">
            <p>• 风险等级: {ticket.risk_level || "待评估"}</p>
            <p>• 审批流程: 将根据策略重新匹配</p>
            <p>• Revision: {ticket.revision} → {ticket.revision + 1}</p>
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={submitting}>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleSubmit}
              disabled={submitting}
              className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
            >
              {submitting && <Loader2 size={14} className="animate-spin" />}
              确认重提
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
