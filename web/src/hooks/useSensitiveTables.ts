import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { listSensitiveTables, type SensitiveTable } from '@/api/maskRule'

const EMPTY_MAP = new Map<string, SensitiveTable>()

/**
 * Hook to fetch and cache sensitive tables by datasource id.
 * Returns a map: tableName -> SensitiveTable for quick lookup.
 */
export function useSensitiveTables(datasourceId: number | null) {
  const [fetchedMap, setFetchedMap] = useState<Map<string, SensitiveTable>>(EMPTY_MAP)
  const [loading, setLoading] = useState(false)
  const cacheRef = useRef<Map<number, Map<string, SensitiveTable>>>(new Map())

  const fetchSensitive = useCallback(async (dsId: number) => {
    // Check cache first
    const cached = cacheRef.current.get(dsId)
    if (cached) {
      setFetchedMap(cached)
      return
    }

    setLoading(true)
    try {
      const res = await listSensitiveTables({
        datasource_id: String(dsId),
        page_size: 500,
      })
      const tableMap = new Map<string, SensitiveTable>()
      for (const t of res.data ?? []) {
        tableMap.set(t.table_name.toLowerCase(), t)
      }
      cacheRef.current.set(dsId, tableMap)
      setFetchedMap(tableMap)
    } catch {
      // Silently fail — non-critical feature
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (datasourceId) {
      fetchSensitive(datasourceId)
    }
  }, [datasourceId, fetchSensitive])

  // When datasourceId is null, expose an empty map without needing to reset state
  const sensitiveMap = useMemo(
    () => (datasourceId ? fetchedMap : EMPTY_MAP),
    [datasourceId, fetchedMap],
  )

  /**
   * Check if a table name is marked as sensitive for the current datasource.
   * Case-insensitive matching.
   */
  const isSensitive = useCallback((tableName: string): SensitiveTable | undefined => {
    return sensitiveMap.get(tableName.toLowerCase())
  }, [sensitiveMap])

  return { sensitiveMap, isSensitive, loading, refetch: fetchSensitive }
}
