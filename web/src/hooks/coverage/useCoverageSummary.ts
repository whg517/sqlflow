import { useState, useEffect, useCallback, useRef } from "react";
import type { CoverageSummary } from "@/types/coverage";
import { getCoverageSummary } from "@/api/coverage";

interface UseCoverageSummaryResult {
  summary: CoverageSummary | null;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

/**
 * Hook to fetch project-level coverage summary.
 * Loading starts true; only async callbacks update state.
 */
export function useCoverageSummary(
  project: string,
  testType?: string,
): UseCoverageSummaryResult {
  const [summary, setSummary] = useState<CoverageSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tick, setTick] = useState(0);
  const fetchIdRef = useRef(0);

  useEffect(() => {
    const fetchId = ++fetchIdRef.current;

    getCoverageSummary(project, testType)
      .then((res) => {
        if (fetchId !== fetchIdRef.current) return;
        if (res.code === 0) {
          setSummary(res.data);
          setError(null);
        } else {
          setError("获取覆盖度概览失败");
        }
      })
      .catch((err) => {
        if (fetchId !== fetchIdRef.current) return;
        setError(err instanceof Error ? err.message : "获取覆盖度概览失败");
      })
      .finally(() => {
        if (fetchId !== fetchIdRef.current) return;
        setLoading(false);
      });
  }, [project, testType, tick]);

  const refresh = useCallback(() => {
    setLoading(true);
    setTick((t) => t + 1);
  }, []);

  return { summary, loading, error, refresh };
}
