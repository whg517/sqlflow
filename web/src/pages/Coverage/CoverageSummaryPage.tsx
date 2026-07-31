import { useState } from "react";
import { BarChart3, DatabaseZap, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  useCoverageSummary,
  useModuleList,
} from "@/hooks/coverage";
import { useCoverageAvailability } from "@/hooks/useCoverageAvailability";
import { CoverageSummaryCard } from "@/components/coverage/CoverageSummaryCard";
import { ModuleTable } from "@/components/coverage/ModuleTable";
import type { ModuleListParams } from "@/types/coverage";

/**
 * CR fix: project name read from env/config, not hardcoded.
 * Falls back to "sqlflow" only when env var is absent.
 */
const PROJECT = import.meta.env.VITE_COVERAGE_PROJECT ?? "sqlflow";

/**
 * Module-level coverage list page.
 * Shows summary cards for each test type + sortable module table.
 */
export default function CoverageSummaryPage() {
  const [testType, setTestType] = useState<string>("unit");
  const [sort, setSort] = useState<ModuleListParams["sort"]>("line_rate:asc");
  const {
    enabled: coverageEnabled,
    loading: availabilityLoading,
    reason: unavailableReason,
  } = useCoverageAvailability();

  const {
    summary,
    loading: summaryLoading,
    error: summaryError,
    refresh: refreshSummary,
  } = useCoverageSummary(PROJECT, undefined, coverageEnabled);

  const {
    modules,
    loading: modulesLoading,
    error: modulesError,
    setParams,
  } = useModuleList(
    PROJECT,
    { sort, test_type: testType },
    coverageEnabled,
  );

  const handleSortChange = (newSort: ModuleListParams["sort"]) => {
    setSort(newSort);
    setParams({ sort: newSort });
  };

  const handleTestTypeChange = (newType: string) => {
    setTestType(newType);
    setParams({ test_type: newType });
  };

  return (
    <div className="mx-auto max-w-[1200px] space-y-6 page-transition">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <BarChart3 size={22} className="text-[var(--accent-primary)]" />
          <h1 className="text-xl font-semibold text-[var(--text-primary)]">
            覆盖度审计
          </h1>
          <span className="rounded-md bg-[var(--bg-elevated)] px-2 py-0.5 text-xs text-[var(--text-tertiary)]">
            {PROJECT}
          </span>
        </div>
        {coverageEnabled && (
          <div className="flex items-center gap-3">
            <Select value={testType} onValueChange={handleTestTypeChange}>
              <SelectTrigger className="h-8 w-32 border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="unit">单元测试</SelectItem>
                <SelectItem value="integration">集成测试</SelectItem>
              </SelectContent>
            </Select>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 gap-1.5 text-[var(--text-secondary)]"
              onClick={() => {
                refreshSummary();
                setParams({});
              }}
            >
              <RefreshCw size={14} />
              刷新
            </Button>
          </div>
        )}
      </div>

      {availabilityLoading ? (
        <Skeleton className="h-52 rounded-lg" />
      ) : !coverageEnabled ? (
        <div className="flex min-h-64 flex-col items-center justify-center rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] px-6 text-center">
          <div className="mb-4 flex size-12 items-center justify-center rounded-xl bg-[var(--bg-elevated)] text-[var(--text-tertiary)]">
            <DatabaseZap size={23} />
          </div>
          <h2 className="text-base font-medium text-[var(--text-primary)]">
            覆盖度审计尚未启用
          </h2>
          <p className="mt-2 max-w-lg text-sm leading-6 text-[var(--text-secondary)]">
            {unavailableReason ?? "当前环境未配置覆盖度数据服务。"}
            配置完成后，系统会开放覆盖度概览、模块下钻和文件明细。
          </p>
        </div>
      ) : (
        <>
      {/* Summary cards */}
      {summaryLoading ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <Skeleton className="h-44 rounded-lg" />
          <Skeleton className="h-44 rounded-lg" />
        </div>
      ) : summaryError ? (
        <div className="flex items-center justify-center rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] py-12 text-center">
          <p className="text-sm text-red-400">{summaryError}</p>
        </div>
      ) : summary ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {Object.entries(summary.test_types).map(([type, data]) => (
            <CoverageSummaryCard key={type} testType={type} data={data} />
          ))}
        </div>
      ) : null}

      {/* Module table */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-5">
        <div className="mb-4">
          <h2 className="text-sm font-medium text-[var(--text-primary)]">
            模块覆盖度列表
          </h2>
          <p className="mt-0.5 text-xs text-[var(--text-tertiary)]">
            点击模块行可下钻查看文件级覆盖度
          </p>
        </div>
        <ModuleTable
          modules={modules}
          loading={modulesLoading}
          error={modulesError}
          sort={sort}
          onSortChange={handleSortChange}
          project={PROJECT}
        />
      </div>
        </>
      )}
    </div>
  );
}
