import { cn } from "@/lib/utils";
import { Card, CardContent } from "@/components/ui/card";
import type { TestTypeSummary } from "@/types/coverage";
import { computeStatus, type CoverageStatus } from "@/types/coverage";
import { CoverageProgressBar } from "./CoverageProgressBar";

/** CR fix: CoverageSummaryCard reuses CoverageProgressBar instead of a duplicate CoverageBar. */

interface CoverageSummaryCardProps {
  testType: string;
  data: TestTypeSummary;
}

const TYPE_LABELS: Record<string, string> = {
  unit: "单元测试",
  integration: "集成测试",
  e2e: "E2E 测试",
};

function rateClassName(status: CoverageStatus) {
  return cn(
    "tabular-nums font-medium",
    status === "critical" && "text-red-400 font-semibold",
  );
}

/** Summary card for one test type's coverage overview. */
export function CoverageSummaryCard({ testType, data }: CoverageSummaryCardProps) {
  const label = TYPE_LABELS[testType] ?? testType;
  const lineStatus = computeStatus(data.line_rate);
  const branchStatus = computeStatus(data.branch_rate);

  return (
    <Card className="gap-4 py-5">
      <CardContent className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium text-[var(--text-secondary)]">
            {label}
          </h3>
          <span className="text-xs text-[var(--text-tertiary)]">
            {data.modules_total} 个模块
          </span>
        </div>

        {/* Line coverage */}
        <div className="space-y-1.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-[var(--text-secondary)]">行覆盖率</span>
            <span className={rateClassName(lineStatus)}>
              {(data.line_rate * 100).toFixed(2)}%
            </span>
          </div>
          <CoverageProgressBar rate={data.line_rate} status={lineStatus} showLabel={false} />
        </div>

        {/* Branch coverage */}
        <div className="space-y-1.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-[var(--text-secondary)]">分支覆盖率</span>
            <span className={rateClassName(branchStatus)}>
              {(data.branch_rate * 100).toFixed(2)}%
            </span>
          </div>
          <CoverageProgressBar rate={data.branch_rate} status={branchStatus} showLabel={false} />
        </div>

        {/* Stats footer */}
        <div className="flex items-center gap-3 border-t border-[var(--border-subtle)] pt-3 text-xs text-[var(--text-tertiary)]">
          <span>
            {data.lines_covered.toLocaleString()} / {data.lines_total.toLocaleString()} 行
          </span>
          {data.modules_below_threshold > 0 && (
            <span className="text-red-400 font-medium">
              ⚠ {data.modules_below_threshold} 个模块低于阈值
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
