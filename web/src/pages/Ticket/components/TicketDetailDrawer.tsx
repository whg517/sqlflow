import { useState, useEffect } from "react";
import { Loader2, CheckCircle2, XCircle, Ban, Play, Copy } from "lucide-react";
import { toast } from "sonner";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { ScrollArea } from "@/components/ui/scroll-area";
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
  getTicket,
  approveTicket,
  rejectTicket,
  cancelTicket,
  executeTicket,
  getStatusLabel,
  getStatusColor,
  getRiskLabel,
  getRiskColor,
  getRiskDot,
  formatTime,
  type Ticket,
  type TicketStatus,
} from "@/api/ticket";
import CommentSection from "./CommentSection";

interface TicketDetailDrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  ticketId: number | null;
  userRole: string;
  userId: number;
  onActionComplete: () => void;
}

export default function TicketDetailDrawer({
  open,
  onOpenChange,
  ticketId,
  userRole,
  userId,
  onActionComplete,
}: TicketDetailDrawerProps) {
  const [ticket, setTicket] = useState<Ticket | null>(null);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);

  // Approval dialog
  const [approveOpen, setApproveOpen] = useState(false);
  const [approveComment, setApproveComment] = useState("");

  // Reject dialog
  const [rejectOpen, setRejectOpen] = useState(false);
  const [rejectReason, setRejectReason] = useState("");

  // Cancel dialog
  const [cancelOpen, setCancelOpen] = useState(false);
  const [cancelReason, setCancelReason] = useState("");

  // Execute confirm
  const [execOpen, setExecOpen] = useState(false);

  useEffect(() => {
    if (open && ticketId) {
      const id = requestAnimationFrame(() => {
        setLoading(true);
        getTicket(ticketId)
          .then((res) => setTicket(res.data))
          .catch((err) =>
            toast.error(err instanceof Error ? err.message : "获取工单失败"),
          )
          .finally(() => setLoading(false));
      });
      return () => cancelAnimationFrame(id);
    } else {
      const id = requestAnimationFrame(() => {
        setTicket(null);
      });
      return () => cancelAnimationFrame(id);
    }
  }, [open, ticketId]);

  function resetDialogs() {
    setApproveOpen(false);
    setApproveComment("");
    setRejectOpen(false);
    setRejectReason("");
    setCancelOpen(false);
    setCancelReason("");
    setExecOpen(false);
  }

  async function handleAction(fn: () => Promise<unknown>, msg: string) {
    setActionLoading(true);
    try {
      await fn();
      toast.success(msg);
      resetDialogs();
      onActionComplete();
      // Refresh ticket detail
      if (ticketId) {
        const res = await getTicket(ticketId);
        setTicket(res.data);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    } finally {
      setActionLoading(false);
    }
  }

  function handleApprove() {
    handleAction(() => approveTicket(ticketId!, approveComment), "审批通过");
  }

  function handleReject() {
    if (!rejectReason.trim()) {
      toast.error("请填写驳回原因");
      return;
    }
    handleAction(() => rejectTicket(ticketId!, rejectReason.trim()), "已驳回");
  }

  function handleCancel() {
    if (!cancelReason.trim()) {
      toast.error("请填写取消原因");
      return;
    }
    handleAction(
      () => cancelTicket(ticketId!, cancelReason.trim()),
      "工单已取消",
    );
  }

  function handleExecute() {
    handleAction(() => executeTicket(ticketId!), "工单已执行");
  }

  // Permission checks
  const isDBA = userRole === "admin" || userRole === "dba";
  const isSubmitter = ticket?.submitter_id === userId;
  const status = ticket?.status;

  const canApprove = isDBA && status === "PENDING_APPROVAL";
  const canReject = isDBA && status === "PENDING_APPROVAL";
  const canCancel =
    (isSubmitter || isDBA) &&
    ["SUBMITTED", "AI_REVIEWED", "PENDING_APPROVAL", "APPROVED"].includes(
      status ?? "",
    );
  const canExecute = (isSubmitter || isDBA) && status === "APPROVED";

  // Parse AI review result
  let aiReview: {
    summary?: string;
    suggestions?: string[];
    impact_analysis?: string;
  } | null = null;
  if (ticket?.ai_review_result) {
    try {
      aiReview = JSON.parse(ticket.ai_review_result);
    } catch {
      /* ignore */
    }
  }

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          side="right"
          showCloseButton
          className="w-[60%] max-w-[720px] border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-[720px] flex flex-col"
        >
          <SheetHeader className="px-6 pt-6 pb-0">
            <SheetTitle className="text-[var(--text-primary)]">
              工单 #{ticket?.id ?? "..."}
            </SheetTitle>
          </SheetHeader>

          {loading ? (
            <div className="flex flex-1 items-center justify-center">
              <Loader2 className="h-6 w-6 animate-spin text-[var(--text-muted)]" />
            </div>
          ) : !ticket ? (
            <div className="flex flex-1 items-center justify-center text-sm text-[var(--text-muted)]">
              工单不存在
            </div>
          ) : (
            <ScrollArea className="flex-1">
              <div className="space-y-5 px-6 py-4">
                {/* Status + Meta */}
                <div className="flex items-center gap-3">
                  <Badge
                    className={`${getStatusColor(status as TicketStatus)} border-0 text-xs`}
                  >
                    {getStatusLabel(status as TicketStatus)}
                  </Badge>
                  {ticket.risk_level && (
                    <span
                      className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ${getRiskColor(ticket.risk_level)}`}
                    >
                      <span
                        className={`inline-block h-1.5 w-1.5 rounded-full ${getRiskDot(ticket.risk_level)}`}
                      />
                      {getRiskLabel(ticket.risk_level)}
                    </span>
                  )}
                </div>

                <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-[var(--text-secondary)]">
                  <span>
                    提交人:{" "}
                    {ticket.submitter_name || `用户#${ticket.submitter_id}`}
                  </span>
                  <span>提交时间: {formatTime(ticket.created_at)}</span>
                  <span>
                    数据库: {ticket.db_type?.toUpperCase() ?? "MySQL"} &gt;{" "}
                    {ticket.database || "—"}
                  </span>
                </div>

                <Separator className="bg-[var(--border-default)]" />

                {/* SQL Content */}
                <div>
                  <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
                    SQL 内容
                  </label>
                  <div className="relative">
                    <pre className="max-h-48 overflow-auto rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3 text-xs text-[var(--text-primary)] whitespace-pre-wrap break-all">
                      {ticket.sql_content}
                    </pre>
                    <button
                      className="absolute top-2 right-2 rounded p-1 text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-base)] hover:text-[var(--text-primary)]"
                      onClick={() => {
                        navigator.clipboard.writeText(ticket.sql_content);
                        toast.success("已复制");
                      }}
                    >
                      <Copy size={12} />
                    </button>
                  </div>
                </div>

                {/* AI Review */}
                {aiReview && (
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
                      AI 评审
                    </label>
                    <div className="rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3 space-y-2">
                      {aiReview.summary && (
                        <p className="text-xs text-[var(--text-primary)]">
                          {aiReview.summary}
                        </p>
                      )}
                      {aiReview.suggestions &&
                        aiReview.suggestions.length > 0 && (
                          <ul className="space-y-1 pl-3">
                            {aiReview.suggestions.map((s, i) => (
                              <li
                                key={i}
                                className="text-xs text-[var(--text-muted)] list-disc"
                              >
                                {s}
                              </li>
                            ))}
                          </ul>
                        )}
                      {aiReview.impact_analysis && (
                        <p className="text-xs text-[var(--text-muted)]">
                          影响分析: {aiReview.impact_analysis}
                        </p>
                      )}
                    </div>
                  </div>
                )}

                {/* Change Reason */}
                {ticket.change_reason && (
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
                      变更原因
                    </label>
                    <p className="text-xs text-[var(--text-muted)]">
                      {ticket.change_reason}
                    </p>
                  </div>
                )}

                {/* Review Record */}
                {ticket.reviewer_id > 0 && (
                  <>
                    <Separator className="bg-[var(--border-default)]" />
                    <div>
                      <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
                        审批记录
                      </label>
                      <div className="space-y-1 text-xs text-[var(--text-muted)]">
                        <p>
                          {ticket.status === "APPROVED" ||
                          ticket.status === "DONE" ||
                          ticket.status === "EXECUTING"
                            ? `${formatTime(ticket.updated_at)} ${ticket.reviewer_name || `用户#${ticket.reviewer_id}`} 审批通过`
                            : `${formatTime(ticket.updated_at)} ${ticket.reviewer_name || `用户#${ticket.reviewer_id}`} 已拒绝`}
                        </p>
                        {ticket.review_comment && (
                          <p className="pl-3 text-[var(--text-secondary)]">
                            "{ticket.review_comment}"
                          </p>
                        )}
                      </div>
                    </div>
                  </>
                )}

                {/* Executed At */}
                {ticket.executed_at && (
                  <div className="text-xs text-[var(--text-muted)]">
                    执行时间: {formatTime(ticket.executed_at)}
                  </div>
                )}

                {/* Comments Section */}
                <Separator className="bg-[var(--border-default)]" />
                <CommentSection
                  orderId={ticket.id}
                  currentUserId={userId}
                  currentUserRole={userRole}
                />
              </div>
            </ScrollArea>
          )}

          {/* Footer Actions */}
          {ticket && (canApprove || canReject || canCancel || canExecute) && (
            <SheetFooter className="border-t border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-3">
              <div className="flex w-full items-center gap-2">
                {canApprove && (
                  <Button
                    size="sm"
                    className="h-8 gap-1.5 bg-emerald-600 px-3 text-xs text-white hover:bg-emerald-700"
                    onClick={() => setApproveOpen(true)}
                    disabled={actionLoading}
                  >
                    <CheckCircle2 size={14} />
                    通过
                  </Button>
                )}
                {canReject && (
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-8 gap-1.5 border-red-500/50 px-3 text-xs text-red-400 hover:bg-red-500/10"
                    onClick={() => setRejectOpen(true)}
                    disabled={actionLoading}
                  >
                    <XCircle size={14} />
                    拒绝
                  </Button>
                )}
                {canExecute && (
                  <Button
                    size="sm"
                    className="h-8 gap-1.5 bg-[var(--accent-primary)] px-3 text-xs text-white hover:bg-[var(--accent-hover)]"
                    onClick={() => setExecOpen(true)}
                    disabled={actionLoading}
                  >
                    <Play size={14} />
                    执行
                  </Button>
                )}
                {canCancel && (
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-8 gap-1.5 px-3 text-xs text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                    onClick={() => setCancelOpen(true)}
                    disabled={actionLoading}
                  >
                    <Ban size={14} />
                    取消工单
                  </Button>
                )}
              </div>
            </SheetFooter>
          )}
        </SheetContent>
      </Sheet>

      {/* Approve Dialog */}
      <AlertDialog open={approveOpen} onOpenChange={setApproveOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>审批通过</AlertDialogTitle>
            <AlertDialogDescription>
              确认通过工单 #{ticketId} 的变更申请？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
              审批备注（可选）
            </label>
            <Textarea
              value={approveComment}
              onChange={(e) => setApproveComment(e.target.value)}
              placeholder="填写审批备注..."
              className="min-h-[80px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={actionLoading}>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleApprove} disabled={actionLoading}>
              {actionLoading ? (
                <Loader2 size={14} className="animate-spin" />
              ) : null}
              确认通过
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Reject Dialog */}
      <AlertDialog open={rejectOpen} onOpenChange={setRejectOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>驳回工单</AlertDialogTitle>
            <AlertDialogDescription>
              驳回工单 #{ticketId}，请填写驳回原因。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
              驳回原因 <span className="text-red-400">*</span>
            </label>
            <Textarea
              value={rejectReason}
              onChange={(e) => setRejectReason(e.target.value)}
              placeholder="请说明驳回原因..."
              className="min-h-[80px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={actionLoading}>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleReject}
              disabled={actionLoading || !rejectReason.trim()}
              variant="destructive"
            >
              {actionLoading ? (
                <Loader2 size={14} className="animate-spin" />
              ) : null}
              确认驳回
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Cancel Dialog */}
      <AlertDialog open={cancelOpen} onOpenChange={setCancelOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>取消工单</AlertDialogTitle>
            <AlertDialogDescription>
              取消工单 #{ticketId}，此操作不可恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
              取消原因 <span className="text-red-400">*</span>
            </label>
            <Textarea
              value={cancelReason}
              onChange={(e) => setCancelReason(e.target.value)}
              placeholder="请说明取消原因..."
              className="min-h-[80px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={actionLoading}>返回</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleCancel}
              disabled={actionLoading || !cancelReason.trim()}
            >
              {actionLoading ? (
                <Loader2 size={14} className="animate-spin" />
              ) : null}
              确认取消
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Execute Confirm */}
      <AlertDialog open={execOpen} onOpenChange={setExecOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>执行工单</AlertDialogTitle>
            <AlertDialogDescription>
              确认执行工单 #{ticketId} 的
              SQL？此操作将直接在目标数据库上执行变更。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={actionLoading}>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleExecute} disabled={actionLoading}>
              {actionLoading ? (
                <Loader2 size={14} className="animate-spin" />
              ) : null}
              确认执行
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
