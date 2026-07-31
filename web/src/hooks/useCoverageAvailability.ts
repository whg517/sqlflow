import { useEffect, useState } from "react";
import { getCoverageAvailability } from "@/api/coverage";

interface UseCoverageAvailabilityResult {
  enabled: boolean;
  loading: boolean;
  reason: string | null;
}

export function useCoverageAvailability(): UseCoverageAvailabilityResult {
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [reason, setReason] = useState<string | null>(null);

  useEffect(() => {
    let active = true;

    getCoverageAvailability()
      .then((status) => {
        if (!active) return;
        setEnabled(status.enabled);
        setReason(status.reason ?? null);
      })
      .catch(() => {
        if (!active) return;
        setEnabled(false);
        setReason("无法获取覆盖度服务状态");
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, []);

  return { enabled, loading, reason };
}
