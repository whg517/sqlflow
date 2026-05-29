/**
 * useApprovalFlow — SF-FEAT0044
 * Core hook for managing approval flow state per ticket.
 * Handles fetch, approve, reject, resubmit, and 10s polling for pending status.
 */

import { useCallback, useEffect, useRef } from "react";
import { useApprovalStore } from "@/store/approvalStore";

const POLL_INTERVAL_MS = 10_000;

export function useApprovalFlow(ticketId: number | null) {
  const store = useApprovalStore();
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Fetch flow + history when ticketId changes
  useEffect(() => {
    if (!ticketId) {
      store.reset();
      return;
    }
    store.fetchFlow(ticketId);
    store.fetchHistory(ticketId);
    return () => {
      store.reset();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ticketId]);

  // 10s polling: only when current stage is PENDING
  useEffect(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
    if (!ticketId || !store.flow) return;
    const hasPending = store.flow.stages.some((s) => s.status === "PENDING");
    if (!hasPending) return;
    pollRef.current = setInterval(() => {
      store.fetchFlow(ticketId);
    }, POLL_INTERVAL_MS);
    return () => {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
  }, [ticketId, store.flow?.current_stage, store.flow?.stages]);

  const approve = useCallback(
    async (comment: string) => {
      if (!ticketId) return;
      await store.approve(ticketId, comment);
    },
    [ticketId, store.approve],
  );

  const reject = useCallback(
    async (reason: string) => {
      if (!ticketId) return;
      await store.reject(ticketId, reason);
    },
    [ticketId, store.reject],
  );

  const resubmit = useCallback(
    async (sql: string, changeReason: string) => {
      if (!ticketId) return 0;
      return store.resubmit(ticketId, sql, changeReason);
    },
    [ticketId, store.resubmit],
  );

  return {
    flow: store.flow,
    history: store.history,
    loading: store.loading,
    historyLoading: store.historyLoading,
    error: store.error,
    approve,
    reject,
    resubmit,
  };
}
