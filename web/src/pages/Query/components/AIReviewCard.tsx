import { useState } from "react";
import {
  Shield,
  ShieldAlert,
  ShieldCheck,
  ChevronDown,
  ChevronRight,
  Sparkles,
  AlertTriangle,
  XCircle,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import type { AIReviewResult, ReviewDecision } from "@/api/query";
import {
  RiskScoreGauge,
  SuggestionCard,
  RollbackSQLBlock,
  AIReviewAnimations,
} from "@/components/AIReview/RiskScoreGauge";
import { ThinkingDots } from "@/components/AIReview/ThinkingDots";

// --- Risk config ---

const riskConfig: Record<
  string,
  {
    label: string;
    color: string;
    bgColor: string;
    borderColor: string;
    icon: typeof ShieldCheck;
    badgeClass: string;
    gradientFrom: string;
    gradientTo: string;
  }
> = {
  low: {
    label: "低风险",
    color: "text-emerald-400",
    bgColor: "bg-emerald-500/5",
    borderColor: "border-emerald-500/30",
    icon: ShieldCheck,
    badgeClass: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
    gradientFrom: "from-emerald-500/5",
    gradientTo: "to-transparent",
  },
  medium: {
    label: "中风险",
    color: "text-amber-400",
    bgColor: "bg-amber-500/5",
    borderColor: "border-amber-500/30",
    icon: Shield,
    badgeClass: "bg-amber-500/20 text-amber-400 border-amber-500/30",
    gradientFrom: "from-amber-500/5",
    gradientTo: "to-transparent",
  },
  high: {
    label: "高风险",
    color: "text-red-400",
    bgColor: "bg-red-500/5",
    borderColor: "border-red-500/30",
    icon: ShieldAlert,
    badgeClass: "bg-red-500/20 text-red-400 border-red-500/30",
    gradientFrom: "from-red-500/5",
    gradientTo: "to-transparent",
  },
};

// --- Props ---

interface AIReviewCardProps {
  status: "idle" | "reviewing" | "done" | "error";
  result: AIReviewResult | null;
  streamingContent: string;
  error: string | null;
  onConfirm: () => void;
  onAutoExecute: () => void;
  onSubmitTicket: () => void;
  onDismiss: () => void;
}

// --- Component ---

export default function AIReviewCard({
  status,
  result,
  streamingContent,
  error,
  onConfirm,
  onAutoExecute,
  onSubmitTicket,
  onDismiss,
}: AIReviewCardProps) {
  const [suggestionsOpen, setSuggestionsOpen] = useState(true);
  const [impactOpen, setImpactOpen] = useState(false);
  const [rollbackOpen, setRollbackOpen] = useState(false);

  if (status === "idle") return null;

  // Reviewing state — thinking animation + streaming content
  if (status === "reviewing") {
    return (
      <>
        <AIReviewAnimations />
        <div className="border-b border-violet-500/30 bg-gradient-to-r from-violet-500/5 to-transparent">
          <div className="flex items-center gap-2 px-4 py-3">
            <div className="relative flex items-center gap-2">
              <Sparkles size={16} className="text-violet-400" />
              <ThinkingDots />
            </div>
            <span className="text-xs font-medium text-violet-400">
              AI 正在分析 SQL...
            </span>
            <div className="flex-1" />
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-xs text-[var(--text-muted)]"
              onClick={onDismiss}
            >
              取消
            </Button>
          </div>
          {streamingContent && (
            <div className="border-t border-violet-500/20 px-4 py-2">
              <ScrollArea className="max-h-32">
                <pre className="whitespace-pre-wrap font-mono text-xs text-[var(--text-muted)] leading-relaxed">
                  {streamingContent}
                </pre>
              </ScrollArea>
            </div>
          )}
        </div>
      </>
    );
  }

  // Error state
  if (status === "error") {
    return (
      <div className="border-b border-red-500/30 bg-red-500/5">
        <div className="flex items-center gap-2 px-4 py-3">
          <AlertTriangle size={16} className="text-red-400" />
          <span className="text-xs text-red-400">
            {error || "AI 评审失败"}
          </span>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            关闭
          </Button>
        </div>
      </div>
    );
  }

  // Done state — enhanced result card
  if (!result) return null;

  const risk = riskConfig[result.risk_level] || riskConfig.medium;
  const RiskIcon = risk.icon;

  return (
    <>
      <AIReviewAnimations />
      <div
        className={cn(
          "border-b bg-gradient-to-r",
          risk.borderColor,
          risk.gradientFrom,
          risk.gradientTo,
        )}
      >
        {/* Header with risk gauge */}
        <div className="flex items-center gap-3 px-4 py-3">
          <RiskIcon size={18} className={risk.color} />
          <span className={cn("text-xs font-semibold", risk.color)}>
            AI 评审完成
          </span>
          <Badge variant="outline" className={cn("text-[10px]", risk.badgeClass)}>
            {risk.label}
          </Badge>
          {result.review_source !== "ai" && (
            <Badge
              variant="outline"
              className="border-[var(--border-default)] bg-[var(--bg-elevated)] text-[10px] text-[var(--text-muted)]"
            >
              {result.review_source === "static" ? "静态规则" : "降级模式"}
            </Badge>
          )}
          <div className="flex-1" />
          <RiskScoreGauge
            score={result.risk_score}
            riskLevel={result.risk_level}
            size="sm"
          />
          <Button
            variant="ghost"
            size="sm"
            className="h-6 w-6 p-0 text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            <XCircle size={14} />
          </Button>
        </div>

        {/* Summary */}
        <div className="px-4 pb-3">
          <p className="text-xs text-[var(--text-secondary)] leading-relaxed">
            {result.summary}
          </p>
        </div>

        {/* Warnings */}
        {result.warnings && result.warnings.length > 0 && (
          <div className="px-4 pb-2 space-y-1">
            {result.warnings.map((w, i) => (
              <div
                key={i}
                className="flex items-start gap-1.5 text-xs text-amber-400"
              >
                <AlertTriangle size={12} className="shrink-0 mt-0.5" />
                <span>{w}</span>
              </div>
            ))}
          </div>
        )}

        <Separator className="bg-[var(--border-default)]" />

        {/* Suggestions — structured cards */}
        {result.suggestions && result.suggestions.length > 0 && (
          <div className="px-4 py-2">
            <button
              className="flex items-center gap-1.5 text-xs font-medium text-[var(--accent-primary)] hover:underline"
              onClick={() => setSuggestionsOpen(!suggestionsOpen)}
            >
              {suggestionsOpen ? (
                <ChevronDown size={12} />
              ) : (
                <ChevronRight size={12} />
              )}
              {result.suggestions.length} 条优化建议
            </button>

            {suggestionsOpen && (
              <div className="mt-2 space-y-2">
                {result.suggestions.map((s, i) => (
                  <SuggestionCard
                    key={i}
                    text={s}
                    index={i}
                    riskLevel={result.risk_level}
                  />
                ))}
              </div>
            )}
          </div>
        )}

        {/* Impact analysis — collapsible */}
        {result.impact_analysis && (
          <div className="px-4 py-2">
            <button
              className="flex items-center gap-1.5 text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
              onClick={() => setImpactOpen(!impactOpen)}
            >
              {impactOpen ? (
                <ChevronDown size={12} />
              ) : (
                <ChevronRight size={12} />
              )}
              影响分析
            </button>
            {impactOpen && (
              <div className="mt-1.5 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3 animate-fade-in">
                <p className="text-xs text-[var(--text-secondary)] leading-relaxed">
                  {result.impact_analysis}
                </p>
              </div>
            )}
          </div>
        )}

        {/* Rollback SQL — collapsible */}
        {result.rollback_sql && (
          <div className="px-4 py-2">
            <button
              className="flex items-center gap-1.5 text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
              onClick={() => setRollbackOpen(!rollbackOpen)}
            >
              {rollbackOpen ? (
                <ChevronDown size={12} />
              ) : (
                <ChevronRight size={12} />
              )}
              回滚 SQL
            </button>
            {rollbackOpen && (
              <div className="mt-1.5 animate-fade-in">
                <RollbackSQLBlock sql={result.rollback_sql} />
              </div>
            )}
          </div>
        )}

        <Separator className="bg-[var(--border-default)]" />

        {/* Action buttons per decision flow */}
        <div className="flex items-center gap-2 px-4 py-2.5">
          <ActionButtons
            decision={result.decision}
            onConfirm={onConfirm}
            onAutoExecute={onAutoExecute}
            onSubmitTicket={onSubmitTicket}
            onDismiss={onDismiss}
          />
        </div>
      </div>
    </>
  );
}

// --- Action Buttons ---

function ActionButtons({
  decision,
  onConfirm,
  onAutoExecute,
  onSubmitTicket,
  onDismiss,
}: {
  decision: ReviewDecision;
  onConfirm: () => void;
  onAutoExecute: () => void;
  onSubmitTicket: () => void;
  onDismiss: () => void;
}) {
  switch (decision) {
    case "execute":
      return (
        <>
          <span className="flex items-center gap-1.5 text-xs text-emerald-400">
            <ShieldCheck size={14} />
            安全 — 可自动执行
          </span>
          <div className="flex-1" />
          <Button
            size="sm"
            className="h-7 gap-1 bg-[var(--success)] px-3 text-xs text-white hover:bg-[var(--success)]/80"
            onClick={onAutoExecute}
          >
            立即执行
          </Button>
        </>
      );
    case "confirm":
      return (
        <>
          <span className="flex items-center gap-1.5 text-xs text-amber-400">
            <AlertTriangle size={14} />
            需要确认后执行
          </span>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            取消
          </Button>
          <Button
            size="sm"
            className="h-7 gap-1 bg-[var(--accent-primary)] px-3 text-xs text-white hover:bg-[var(--accent-hover)]"
            onClick={onConfirm}
          >
            确认执行
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-7 gap-1 border-amber-500/50 px-3 text-xs text-amber-400 hover:bg-amber-500/10"
            onClick={onSubmitTicket}
          >
            提交工单
          </Button>
        </>
      );
    case "ticket":
      return (
        <>
          <span className="flex items-center gap-1.5 text-xs text-red-400">
            <ShieldAlert size={14} />
            高风险操作，需提交工单审批
          </span>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            取消
          </Button>
          <Button
            size="sm"
            className="h-7 gap-1 bg-[var(--danger)] px-3 text-xs text-white hover:bg-[var(--danger)]/80"
            onClick={onSubmitTicket}
          >
            提交工单
          </Button>
        </>
      );
    case "blocked":
      return (
        <>
          <span className="flex items-center gap-1.5 text-xs text-red-400">
            <XCircle size={14} />
            操作被安全规则拦截，禁止执行
          </span>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            关闭
          </Button>
        </>
      );
    default:
      return (
        <>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            关闭
          </Button>
        </>
      );
  }
}
