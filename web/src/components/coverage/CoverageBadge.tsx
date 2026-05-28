import type { CoverageStatus } from "@/types/coverage";
import { Badge } from "@/components/ui/badge";

interface CoverageBadgeProps {
  status: CoverageStatus;
  rate: number;
  className?: string;
}

const STATUS_VARIANT: Record<CoverageStatus, "success" | "warning" | "danger"> = {
  pass: "success",
  warning: "warning",
  critical: "danger",
};

const STATUS_ICON: Record<CoverageStatus, string> = {
  pass: "🟢",
  warning: "🟡",
  critical: "🔴",
};

/** Coverage status badge with color-coded indicator. */
export function CoverageBadge({ status, rate, className }: CoverageBadgeProps) {
  const pct = `${(rate * 100).toFixed(2)}%`;

  return (
    <Badge variant={STATUS_VARIANT[status]} className={className}>
      <span>{STATUS_ICON[status]}</span>
      <span className="tabular-nums">{pct}</span>
    </Badge>
  );
}
