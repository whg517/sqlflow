import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import type { FileListParams } from "@/types/coverage";
import { getFileList } from "@/api/coverage";

interface FileItem {
  file_path: string;
  line_rate: number;
  branch_rate: number;
  lines_total: number;
  lines_covered: number;
  uncovered_ranges: { start: number; end: number }[] | null;
  status: "pass" | "warning" | "critical";
}

interface UseFileListResult {
  files: FileItem[];
  total: number;
  page: number;
  pageSize: number;
  loading: boolean;
  error: string | null;
  refresh: () => void;
  setParams: (params: Partial<FileListParams>) => void;
}

/**
 * Hook to fetch file-level coverage list for a specific module.
 *
 * CR fix: module_path is derived from props and merged directly into
 * the fetch effect rather than syncing via intermediate setState.
 * Loading starts true; only async callbacks update state.
 */
export function useFileList(
  project: string,
  modulePath: string,
): UseFileListResult {
  const [extraParams, setExtraParams] = useState<
    Omit<FileListParams, "module_path">
  >({ page: 1, page_size: 50 });
  const [files, setFiles] = useState<FileItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const fetchIdRef = useRef(0);

  // Stable params object via useMemo — avoids re-creating on every render
  const params: FileListParams = useMemo(
    () => ({ ...extraParams, module_path: modulePath }),
    [extraParams, modulePath],
  );

  useEffect(() => {
    if (!params.module_path) return;
    const fetchId = ++fetchIdRef.current;

    getFileList(project, params)
      .then((res) => {
        if (fetchId !== fetchIdRef.current) return;
        if (res.code === 0) {
          setFiles(res.data.items);
          setTotal(res.data.total);
          setPage(res.data.page);
          setPageSize(res.data.page_size);
          setError(null);
        } else {
          setError("获取文件列表失败");
        }
      })
      .catch((err) => {
        if (fetchId !== fetchIdRef.current) return;
        setError(err instanceof Error ? err.message : "获取文件列表失败");
      })
      .finally(() => {
        if (fetchId !== fetchIdRef.current) return;
        setLoading(false);
      });
  }, [project, params]);

  const setParams = useCallback(
    (next: Partial<FileListParams>) => {
      setExtraParams((prev) => ({ ...prev, ...next }));
    },
    [],
  );

  const refresh = useCallback(() => {
    setExtraParams((prev) => ({ ...prev }));
  }, []);

  return { files, total, page, pageSize, loading, error, refresh, setParams };
}
