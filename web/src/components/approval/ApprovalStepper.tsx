/**
 * ApprovalStepper — SF-FEAT0044 Module A
 * Horizontal stepper showing multi-stage approval progress
 */

import type { ApprovalFlow, ApprovalStage, StageStatus } from "@/types/approval";
import {
  stageStatusLabel,
  stageStatusColor,
} from "@/types/approval";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Check, SkipForward, Bot, Clock } from "lucide-react";
import { cn } from "@/lib/utils";

function StageIcon({ status }: { status: StageStatus }) {
  switch (status) {
    case "APPROVED":
      return <Check size={14} className="text-emerald-500" />;
    case "REJECTED":
      return <span className="text-red-500 text-xs">✕</span>;
    case "SKIPPED":
      return <SkipForward size={14} className="text-zinc-500" />;
    case "AUTO_APPROVED":
      return <Bot size={14} className="text-blue-500" />;
    case "PENDING":
      return <Clock size={14} className="text-orange-500" />;
  }
}

function StageNode({
  stage,
  isCurrent,
  isCurrentUserApprover,
  isLast,
}: {
  stage: ApprovalStage;
  isCurrent: boolean;
  isCurrentUserApprover: boolean;
  isLast: boolean;
}) {
  const isPending = stage.status === "PENDING";

  return (
    <div className="flex items-center gap-0">
      <Tooltip>
        <TooltipTrigger asChild>
          <div
            className={cn(
              "relative flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 transition-all",
              stage.status === "APPROVED" && "border-emerald-500 bg-emerald-500/10",
              stage.status === "REJECTED" && "border-red-500 bg-red-500/10",
              stage.status === "SKIPPED" && "border-zinc-500 bg-zinc-500/10",
              stage.status === "AUTO_APPROVED" && "border-blue-500 bg-blue-500/10",
              stage.status === "PENDING" && "border-orange-500 bg-orange-500/10",
              isCurrentUserApprover && isPending && "border-[3px] border-orange-400 bg-orange-400/15",
            )}
            style={
              isCurrent && isPending
                ? { animation: "pulse-subtle 1.5s ease-in-out infinite" }
                : undefined
            }
          >
            <StageIcon status={stage.status} />
          </div>
        </TooltipTrigger>
        <TooltipContent side="bottom" className="text-xs max-w-[240px]">
          <div className="space-y-1">
            <p className="font-medium">{stage.label}</p>
            <p className={stageStatusColor[stage.status]}>
              {stageStatusLabel[stage.status]}
            </p>
            {stage.approver_name && (
              <p className="text-[var(--text-muted)]">审批人: {stage.approver_name}</p>
            )}
            {stage.acted_at && (
              <p className="text-[var(--text-muted)]">
                {new Date(stage.acted_at).toLocaleString("zh-CN")}
              </p>
            )}
            {stage.comment && (
              <p className="text-[var(--text-secondary)] italic">"{stage.comment}"</p>
            )}
            {stage.auto_reason && (
              <p className="text-blue-400 text-[11px]">自动通过原因: {stage.auto_reason}</p>
            )}
          </div>
        </TooltipContent>
      </Tooltip>

      {!isLast && (
        <div
          className={cn(
            "h-0.5 w-12 min-w-[32px]",
            stage.status === "APPROVED" || stage.status === "AUTO_APPROVED"
              ? "bg-emerald-500/50"
              : stage.status === "SKIPPED"
                ? "bg-zinc-500/50"
                : stage.status === "REJECTED"
                  ? "bg-red-500/30"
                  : "bg-[var(--border-default)]",
          )}
        />
      )}
    </div>
  );
}

interface ApprovalStepperProps {
  flow: ApprovalFlow;
  currentUserId?: number;
  className?: string;
}

export default function ApprovalStepper({
  flow,
  currentUserId,
  className,
}: ApprovalStepperProps) {
  return (
    <div className={cn("space-y-3", className)}>
      <div className="flex items-center justify-center gap-0">
        {flow.stages.map((stage, idx) => (
          <StageNode
            key={stage.stage}
            stage={stage}
            isCurrent={stage.stage === flow.current_stage}
            isCurrentUserApprover={
              stage.approver_id != null && stage.approver_id === currentUserId
            }
            isLast={idx === flow.stages.length - 1}
          />
        ))}
      </div>
      <div className="flex items-center justify-center gap-6 text-[10px] text-[var(--text-muted)]">
        {flow.stages.map((stage) => (
          <span key={stage.stage} className="text-center whitespace-nowrap">
            {stage.label}
          </span>
        ))}
      </div>
      {flow.policy_name && (
        <div className="flex items-center justify-center gap-1.5 text-xs text-zinc-400">
          <Bot size={12} />
          <span>
            匹配策略: {flow.policy_name}
            {flow.policy_reason && (
              <span className="ml-1 text-zinc-500">— {flow.policy_reason}</span>
            )}
          </span>
        </div>
      )}
    </div>
  );
}
