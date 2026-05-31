import { cn } from "@/lib/utils";

// --- Risk Score Gauge ---

interface RiskScoreGaugeProps {
  score: number; // 0-100
  riskLevel: "low" | "medium" | "high";
  size?: "sm" | "md";
}

export function RiskScoreGauge({
  score,
  riskLevel,
  size = "md",
}: RiskScoreGaugeProps) {
  const clampedScore = Math.max(0, Math.min(100, score));
  const isSmall = size === "sm";

  const colorConfig = {
    low: {
      track: "bg-emerald-500/10",
      fill: "bg-emerald-500",
      text: "text-emerald-400",
      glow: "shadow-emerald-500/20",
    },
    medium: {
      track: "bg-amber-500/10",
      fill: "bg-amber-500",
      text: "text-amber-400",
      glow: "shadow-amber-500/20",
    },
    high: {
      track: "bg-red-500/10",
      fill: "bg-red-500",
      text: "text-red-400",
      glow: "shadow-red-500/20",
    },
  };

  const colors = colorConfig[riskLevel] || colorConfig.medium;

  return (
    <div className={cn("flex items-center gap-2", isSmall ? "gap-1.5" : "gap-2")}>
      {/* Score number */}
      <span
        className={cn(
          "font-mono font-bold tabular-nums",
          colors.text,
          isSmall ? "text-lg" : "text-2xl",
        )}
      >
        {clampedScore}
      </span>

      {/* Gauge bar */}
      <div className="flex flex-col gap-0.5">
        <div
          className={cn(
            "rounded-full overflow-hidden",
            colors.track,
            isSmall ? "w-20 h-1.5" : "w-32 h-2.5",
          )}
        >
          <div
            className={cn("h-full rounded-full transition-all duration-700 ease-out", colors.fill)}
            style={{ width: `${clampedScore}%` }}
          />
        </div>
        <span className={cn("text-[10px] tabular-nums", colors.text)}>
          风险分数
        </span>
      </div>
    </div>
  );
}

// --- Suggestion Category Tag ---

type SuggestionCategory = "security" | "performance" | "syntax" | "impact" | "general";

interface CategoryConfig {
  label: string;
  color: string;
  bgColor: string;
}

const CATEGORY_MAP: Record<SuggestionCategory, CategoryConfig> = {
  security: {
    label: "安全",
    color: "text-red-400",
    bgColor: "bg-red-500/15",
  },
  performance: {
    label: "性能",
    color: "text-amber-400",
    bgColor: "bg-amber-500/15",
  },
  syntax: {
    label: "语法",
    color: "text-blue-400",
    bgColor: "bg-blue-500/15",
  },
  impact: {
    label: "影响范围",
    color: "text-purple-400",
    bgColor: "bg-purple-500/15",
  },
  general: {
    label: "建议",
    color: "text-zinc-400",
    bgColor: "bg-zinc-500/15",
  },
};

function categorizeSuggestion(text: string): SuggestionCategory {
  const lower = text.toLowerCase();
  if (
    lower.includes("安全") ||
    lower.includes("敏感") ||
    lower.includes("权限") ||
    lower.includes("注入") ||
    lower.includes("security") ||
    lower.includes("sensitive")
  ) {
    return "security";
  }
  if (
    lower.includes("索引") ||
    lower.includes("性能") ||
    lower.includes("limit") ||
    lower.includes("全表扫描") ||
    lower.includes("优化") ||
    lower.includes("慢查询") ||
    lower.includes("performance") ||
    lower.includes("index")
  ) {
    return "performance";
  }
  if (
    lower.includes("语法") ||
    lower.includes("syntax") ||
    lower.includes("格式") ||
    lower.includes("错误")
  ) {
    return "syntax";
  }
  if (
    lower.includes("影响") ||
    lower.includes("范围") ||
    lower.includes("行数") ||
    lower.includes("表结构") ||
    lower.includes("impact")
  ) {
    return "impact";
  }
  return "general";
}

// --- Suggestion Card ---

interface SuggestionCardProps {
  text: string;
  index: number;
  riskLevel: "low" | "medium" | "high";
  animate?: boolean;
}

export function SuggestionCard({
  text,
  index,
  riskLevel,
  animate = true,
}: SuggestionCardProps) {
  const category = categorizeSuggestion(text);
  const config = CATEGORY_MAP[category];

  const severityMap = {
    low: "低",
    medium: "中",
    high: "高",
  };

  return (
    <div
      className={cn(
        "rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3",
        animate && "animate-fade-in",
      )}
      style={
        animate
          ? {
              animationDelay: `${index * 120}ms`,
              animationFillMode: "backwards",
            }
          : undefined
      }
    >
      <div className="flex items-center gap-2 mb-1.5">
        {/* Category tag */}
        <span
          className={cn(
            "inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium",
            config.bgColor,
            config.color,
          )}
        >
          {config.label}
        </span>

        {/* Severity */}
        <span
          className={cn(
            "text-[10px]",
            riskLevel === "high"
              ? "text-red-400"
              : riskLevel === "medium"
                ? "text-amber-400"
                : "text-emerald-400",
          )}
        >
          严重度: {severityMap[riskLevel]}
        </span>
      </div>

      {/* Suggestion text */}
      <p className="text-xs text-[var(--text-primary)] leading-relaxed">
        {text}
      </p>
    </div>
  );
}

// --- Rollback SQL Block ---

interface RollbackSQLBlockProps {
  sql: string;
}

export function RollbackSQLBlock({ sql }: RollbackSQLBlockProps) {
  if (!sql.trim()) return null;

  return (
    <div className="rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] overflow-hidden">
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-[var(--border-default)]">
        <span className="text-xs font-medium text-[var(--text-secondary)]">
          回滚 SQL
        </span>
        <button
          className="flex items-center gap-1 text-[10px] text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors"
          onClick={() => {
            navigator.clipboard.writeText(sql);
          }}
        >
          📋 复制
        </button>
      </div>
      <pre className="max-h-32 overflow-auto p-3 text-xs text-[var(--text-primary)] whitespace-pre-wrap break-all font-mono">
        {sql}
      </pre>
    </div>
  );
}

// --- CSS Animation Keyframes (inject once) ---

export function AIReviewAnimations() {
  return (
    <style>{`
      @keyframes fade-in {
        from { opacity: 0; transform: translateY(8px); }
        to { opacity: 1; transform: translateY(0); }
      }
      .animate-fade-in {
        animation: fade-in 300ms ease-out forwards;
      }
    `}</style>
  );
}
