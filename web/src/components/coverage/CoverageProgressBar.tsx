import { cn } from "@/lib/utils";
import type { CoverageStatus } from "@/types/coverage";

interface CoverageProgressBarProps {
  rate: number;
  status: CoverageStatus;
  className?: string;
  /** Show percentage label on the right (default true) */
  showLabel?: boolean;
  /** Bar height: "sm" = 2px (compact), "md" = 8px (default) */
  size?: "sm" | "md";
}

const STATUS_COLORS: Record<CoverageStatus, string> = {
  pass: "bg-emerald-500",
  warning: "bg-amber-500",
  critical: "bg-red-500",
};

const STATUS_TRACK: Record<CoverageStatus, string> = {
  pass: "bg-emerald-500/20",
  warning: "bg-amber-500/20",
  critical: "bg-red-500/20",
};

const SIZE_MAP = { sm: "h-2", md: "h-2" } as const;

/**
 * Reusable coverage progress bar.
 * CR fix: single source of truth — used by CoverageSummaryCard AND ModuleTable/FileTable.
 */
export function CoverageProgressBar({
  rate,
  status,
  className,
  showLabel = true,
  size = "sm",
}: CoverageProgressBarProps) {
  const pct = Math.round(rate * 100);

  return (
    <div className={cn("flex items-center gap-2", className)}>
      <div
        className={cn(
          "flex-1 overflow-hidden rounded-full",
          STATUS_TRACK[status],
          SIZE_MAP[size],
        )}
      >
        <div
          className={cn(
            "h-full rounded-full transition-all duration-300",
            STATUS_COLORS[status],
          )}
          style={{ width: `${pct}%` }}
        />
      </div>
      {showLabel && (
        <span
          className={cn(
            "w-14 text-right tabular-nums text-xs font-medium",
            status === "critical" && "text-red-400 font-semibold",
          )}
        >
          {pct}%
        </span>
      )}
    </div>
  );
}
