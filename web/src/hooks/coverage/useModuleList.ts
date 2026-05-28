import { useState, useEffect, useCallback, useRef } from "react";
import type { ModuleItem, ModuleListParams } from "@/types/coverage";
import { getModuleList } from "@/api/coverage";

interface UseModuleListResult {
  modules: ModuleItem[];
  total: number;
  page: number;
  pageSize: number;
  loading: boolean;
  error: string | null;
  refresh: () => void;
  setParams: (params: Partial<ModuleListParams>) => void;
}

/**
 * Hook to fetch module-level coverage list with pagination and sorting.
 * Loading starts true; only async callbacks update state.
 */
export function useModuleList(
  project: string,
  initialParams?: ModuleListParams,
): UseModuleListResult {
  const [params, setParamsState] = useState<ModuleListParams>(
    initialParams ?? { sort: "line_rate:asc", page: 1, page_size: 20 },
  );
  const [modules, setModules] = useState<ModuleItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const fetchIdRef = useRef(0);

  useEffect(() => {
    const fetchId = ++fetchIdRef.current;

    getModuleList(project, params)
      .then((res) => {
        if (fetchId !== fetchIdRef.current) return;
        if (res.code === 0) {
          setModules(res.data.items);
          setTotal(res.data.total);
          setPage(res.data.page);
          setPageSize(res.data.page_size);
          setError(null);
        } else {
          setError("获取模块列表失败");
        }
      })
      .catch((err) => {
        if (fetchId !== fetchIdRef.current) return;
        setError(err instanceof Error ? err.message : "获取模块列表失败");
      })
      .finally(() => {
        if (fetchId !== fetchIdRef.current) return;
        setLoading(false);
      });
  }, [project, params]);

  const setParams = useCallback(
    (next: Partial<ModuleListParams>) => {
      setParamsState((prev) => ({ ...prev, ...next }));
    },
    [],
  );

  const refresh = useCallback(() => {
    setParamsState((prev) => ({ ...prev }));
  }, []);

  return { modules, total, page, pageSize, loading, error, refresh, setParams };
}
