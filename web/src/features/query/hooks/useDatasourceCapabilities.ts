import { useEffect, useState } from "react";
import {
  fetchDatasourceCapabilities,
  type DatasourceCapabilities,
} from "@/features/query/api/capabilities";

// Capabilities are a property of the datasource's driver, so they never change
// for a given id within a session. Cache them module-wide: switching tabs or
// reopening the workbench should not re-ask.
const cache = new Map<number, DatasourceCapabilities>();
const inflight = new Map<number, Promise<DatasourceCapabilities>>();

function load(datasourceId: number): Promise<DatasourceCapabilities> {
  const cached = cache.get(datasourceId);
  if (cached) return Promise.resolve(cached);

  const pending = inflight.get(datasourceId);
  if (pending) return pending;

  const request = fetchDatasourceCapabilities(datasourceId)
    .then((caps) => {
      cache.set(datasourceId, caps);
      return caps;
    })
    .finally(() => {
      inflight.delete(datasourceId);
    });
  inflight.set(datasourceId, request);
  return request;
}

export interface UseDatasourceCapabilities {
  capabilities: DatasourceCapabilities | null;
  loading: boolean;
  error: string | null;
}

/**
 * useDatasourceCapabilities reports what a datasource's driver supports.
 *
 * Consumers branch on the returned capabilities instead of on datasource type
 * names, so a new driver becomes usable without touching the UI.
 */
export function useDatasourceCapabilities(
  datasourceId: number | null | undefined,
): UseDatasourceCapabilities {
  const [capabilities, setCapabilities] =
    useState<DatasourceCapabilities | null>(() =>
      datasourceId ? (cache.get(datasourceId) ?? null) : null,
    );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!datasourceId) {
      setCapabilities(null);
      setError(null);
      return;
    }

    const cached = cache.get(datasourceId);
    if (cached) {
      setCapabilities(cached);
      setError(null);
      return;
    }

    let cancelled = false;
    setLoading(true);
    load(datasourceId)
      .then((caps) => {
        if (!cancelled) {
          setCapabilities(caps);
          setError(null);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setCapabilities(null);
          setError(err instanceof Error ? err.message : "获取数据源能力失败");
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [datasourceId]);

  return { capabilities, loading, error };
}

/** Exposed for tests that need a clean cache between cases. */
export function __resetCapabilitiesCache() {
  cache.clear();
  inflight.clear();
}
