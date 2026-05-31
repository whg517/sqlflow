import { useMemo } from "react";
import {
  Check,
  SkipForward,
  Bot,
  Clock,
  ChevronRight,
} from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  parseApprovalChain,
  getRoleLabel,
  type ApprovalRecord,
} from "@/api/approval";
import { formatTime } from "@/api/ticket";

interface ApprovalStepperProps {
  currentStage: number;
  totalStages: number;
  approvalChain: string; // JSON
  records: ApprovalRecord[];
  autoApproved: boolean;
  autoApproveReason: string;
  currentUserId?: number;
}

export default function ApprovalStepper({
  currentStage,
  totalStages,
  approvalChain,
  records,
  autoApproved,
  autoApproveReason,
  currentUserId,
}: ApprovalStepperProps) {
  const stages = useMemo(() => parseApprovalChain(approvalChain), [approvalChain]);

  // If auto-approved with no chain, show auto-approve info
  if (autoApproved && stages.length === 0) {
    return (
      <div className="rounded-md border border-blue-500/20 bg-blue-500/5 px-3 py-2">
        <div className="flex items-center gap-2 text-xs">
          <Bot size={14} className="text-blue-500" />
          <span className="font-medium text-blue-400">自动审批通过</span>
        </div>
        {autoApproveReason && (
          <p className="mt-1 text-xs text-blue-400/70">{autoApproveReason}</p>
        )}
      </div>
    );
  }

  if (stages.length === 0 && totalStages <= 1) {
    return null;
  }

  // Build stage info from records
  const stageRecords = new Map<number, ApprovalRecord>();
  for (const r of records) {
    if (!stageRecords.has(r.stage)) {
      stageRecords.set(r.stage, r);
    }
  }

  return (
    <div className="space-y-2">
      {/* Stepper nodes */}
      <div className="flex items-center gap-1">
        {stages.map((stage, idx) => {
          const stageNum = idx;
          const record = stageRecords.get(stageNum);
          const isCurrent = stageNum === currentStage && !record?.action;
          const isCompleted = !!record?.action && record.action !== "rejected";
          const isRejected = record?.action === "rejected";

          return (
            <div key={idx} className="flex items-center">
              <Tooltip>
                <TooltipTrigger asChild>
                  <div
                    className={`
                      flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium transition-all
                      ${isCompleted && record?.action === "approved"
                        ? "bg-emerald-500/15 text-emerald-500 border border-emerald-500/30"
                        : isCompleted && record?.action === "auto_approved"
                          ? "bg-blue-500/15 text-blue-500 border border-blue-500/30"
                          : isCompleted && record?.action === "skipped"
                            ? "bg-zinc-500/15 text-zinc-400 border border-zinc-500/30"
                            : isRejected
                              ? "bg-red-500/15 text-red-500 border border-red-500/30"
                              : isCurrent
                                ? "bg-orange-500/15 text-orange-500 border border-orange-500/30 ring-2 ring-orange-500/20"
                                : "bg-zinc-500/10 text-zinc-400 border border-zinc-500/20"
                      }
                      ${isCurrent ? "animate-pulse" : ""}
                    `}
                    style={
                      isCurrent
                        ? { animationDuration: "1.5s" }
                        : undefined
                    }
                  >
                    {/* Icon */}
                    {isCompleted && record?.action === "approved" && (
                      <Check size={12} />
                    )}
                    {isCompleted && record?.action === "auto_approved" && (
                      <Bot size={12} />
                    )}
                    {isCompleted && record?.action === "skipped" && (
                      <SkipForward size={12} />
                    )}
                    {isRejected && <span className="text-xs">✕</span>}
                    {isCurrent && <Clock size={12} />}

                    {/* Label */}
                    <span>{getRoleLabel(stage.role)}</span>

                    {/* Highlight if current user is the approver */}
                    {record?.approver_id === currentUserId && (
                      <span className="h-1 w-1 rounded-full bg-orange-500" />
                    )}
                  </div>
                </TooltipTrigger>
                <TooltipContent side="bottom" className="text-xs">
                  {record ? (
                    <div className="space-y-1">
                      <p>
                        {record.action === "approved" && "通过"}
                        {record.action === "rejected" && "驳回"}
                        {record.action === "auto_approved" && "自动通过"}
                        {record.action === "skipped" && "跳过"}
                        {" — "}
                        {record.approver_name || getRoleLabel(stage.role)}
                      </p>
                      <p className="text-zinc-400">
                        {formatTime(record.created_at)}
                      </p>
                      {record.comment && (
                        <p className="text-zinc-300">
                          &ldquo;{record.comment}&rdquo;
                        </p>
                      )}
                    </div>
                  ) : isCurrent ? (
                    <p>等待 {getRoleLabel(stage.role)} 审批中...</p>
                  ) : (
                    <p>等待中</p>
                  )}
                </TooltipContent>
              </Tooltip>

              {/* Connector */}
              {idx < stages.length - 1 && (
                <ChevronRight
                  size={14}
                  className={`mx-0.5 ${
                    isCompleted ? "text-emerald-500/50" : "text-zinc-600"
                  }`}
                />
              )}
            </div>
          );
        })}
      </div>

      {/* Auto approve reason for partial auto-approve */}
      {autoApproved && autoApproveReason && (
        <p className="text-xs text-zinc-400 pl-1">{autoApproveReason}</p>
      )}
    </div>
  );
}
