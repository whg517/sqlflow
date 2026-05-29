/**
 * StageBadge — SF-FEAT0044
 * Compact badge for ticket list showing approval stage
 */

import { Bot, CheckCircle2 } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface StageBadgeProps {
  currentStage: number;
  totalStages: number;
  autoApproved?: boolean;
}

export default function StageBadge({ currentStage, totalStages, autoApproved }: StageBadgeProps) {
  if (autoApproved) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex items-center gap-1 rounded-full bg-blue-500/15 px-2 py-0.5 text-[10px] font-medium text-blue-400">
            <Bot size={10} />
            自动通过
          </span>
        </TooltipTrigger>
        <TooltipContent className="text-xs">匹配自动审批策略，无需人工审批</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-orange-500/15 px-2 py-0.5 text-[10px] font-medium text-orange-400">
      <CheckCircle2 size={10} />
      {currentStage + 1}/{totalStages}
    </span>
  );
}
