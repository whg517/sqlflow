import { useState, useCallback, useRef } from "react";
import { fetchDatasourceTables } from "@/api/maskRule";
import { api } from "@/api/client";

// --- Types ---

export interface SchemaData {
  tables: string[];
  columns: Map<string, string[]>;
  fetchedAt: number;
}

type SchemaCache = Map<number, SchemaData>;

const CACHE_TTL = 5 * 60 * 1000; // 5 minutes

/**
 * Hook to fetch and cache database schema (tables + columns) for autocompletion.
 *
 * Currently the backend only supports GET /api/datasources/:id/tables (returns string[]).
 * Column fetching is designed to call GET /api/datasources/:id/tables/:name/columns if available,
 * otherwise returns empty columns gracefully.
 */
export function useSchemaCompletion() {
  const cacheRef = useRef<SchemaCache>(new Map());
  const [loading, setLoading] = useState(false);

  /** Check if cache entry is still valid */
  const isCacheValid = useCallback((dsId: number): boolean => {
    const entry = cacheRef.current.get(dsId);
    if (!entry) return false;
    return Date.now() - entry.fetchedAt < CACHE_TTL;
  }, []);

  /** Fetch table list for a datasource */
  const fetchTables = useCallback(
    async (datasourceId: number): Promise<SchemaData | null> => {
      // Return cached if valid
      if (isCacheValid(datasourceId)) {
        return cacheRef.current.get(datasourceId) ?? null;
      }

      setLoading(true);
      try {
        const res = await fetchDatasourceTables(datasourceId);
        const tables: string[] = res.data ?? [];

        const schema: SchemaData = {
          tables,
          columns: new Map(),
          fetchedAt: Date.now(),
        };

        cacheRef.current.set(datasourceId, schema);
        return schema;
      } catch {
        return null;
      } finally {
        setLoading(false);
      }
    },
    [isCacheValid],
  );

  /** Fetch columns for a specific table. Currently best-effort (returns cached or empty). */
  const fetchColumns = useCallback(
    async (datasourceId: number, tableName: string): Promise<string[]> => {
      const schema = cacheRef.current.get(datasourceId);
      if (!schema) return [];

      // Return cached columns if available
      const cached = schema.columns.get(tableName.toLowerCase());
      if (cached) return cached;

      // Try to fetch from backend: GET /api/datasources/:id/tables/:name/columns
      // If the endpoint doesn't exist (404), we gracefully return empty array
      try {
        const res = await api.get<{ code: number; data: string[] }>(
          `/datasources/${datasourceId}/tables/${encodeURIComponent(tableName)}/columns`,
        );
        const cols = res.data ?? [];
        schema.columns.set(tableName.toLowerCase(), cols);
        return cols;
      } catch {
        // Endpoint may not exist — cache empty result to avoid repeated calls
        schema.columns.set(tableName.toLowerCase(), []);
        return [];
      }
    },
    [],
  );

  /** Clear all cached schema data */
  const clearCache = useCallback(() => {
    cacheRef.current.clear();
  }, []);

  /** Clear cache for a specific datasource */
  const clearDatasourceCache = useCallback((datasourceId: number) => {
    cacheRef.current.delete(datasourceId);
  }, []);

  return {
    loading,
    fetchTables,
    fetchColumns,
    clearCache,
    clearDatasourceCache,
  };
}
