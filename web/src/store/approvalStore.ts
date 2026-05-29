/**
 * Approval flow Zustand store — SF-FEAT0044
 * Manages approval flow state for ticket detail pages
 */

import { create } from "zustand";
import type {
  ApprovalFlow,
  ApprovalHistoryEntry,
} from "@/types/approval";
import * as approvalApi from "@/api/approval";

interface ApprovalState {
  // ─── Data ───────────────────────────────────────────────────────────────
  flow: ApprovalFlow | null;
  history: ApprovalHistoryEntry[];
  loading: boolean;
  historyLoading: boolean;
  error: string | null;

  // ─── Actions ────────────────────────────────────────────────────────────
  fetchFlow: (ticketId: number) => Promise<void>;
  fetchHistory: (ticketId: number) => Promise<void>;
  approve: (ticketId: number, comment: string) => Promise<void>;
  reject: (ticketId: number, reason: string) => Promise<void>;
  resubmit: (
    ticketId: number,
    sql: string,
    changeReason: string,
  ) => Promise<number>; // returns new revision
  reset: () => void;
}

export const useApprovalStore = create<ApprovalState>((set) => ({
  flow: null,
  history: [],
  loading: false,
  historyLoading: false,
  error: null,

  fetchFlow: async (ticketId: number) => {
    set({ loading: true, error: null });
    try {
      const res = await approvalApi.getApprovalFlow(ticketId);
      set({ flow: res.data, loading: false });
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : "获取审批流程失败",
        loading: false,
      });
    }
  },

  fetchHistory: async (ticketId: number) => {
    set({ historyLoading: true });
    try {
      const res = await approvalApi.getApprovalHistory(ticketId);
      set({ history: res.data ?? [], historyLoading: false });
    } catch {
      set({ historyLoading: false });
    }
  },

  approve: async (ticketId: number, comment: string) => {
    set({ loading: true, error: null });
    try {
      const res = await approvalApi.approveStage(ticketId, comment);
      set({ flow: res.data, loading: false });
      const historyRes = await approvalApi.getApprovalHistory(ticketId);
      set({ history: historyRes.data ?? [] });
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : "审批操作失败",
        loading: false,
      });
      throw err;
    }
  },

  reject: async (ticketId: number, reason: string) => {
    set({ loading: true, error: null });
    try {
      const res = await approvalApi.rejectStage(ticketId, reason);
      set({ flow: res.data, loading: false });
      const historyRes = await approvalApi.getApprovalHistory(ticketId);
      set({ history: historyRes.data ?? [] });
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : "审批操作失败",
        loading: false,
      });
      throw err;
    }
  },

  resubmit: async (ticketId: number, sql: string, changeReason: string) => {
    set({ loading: true, error: null });
    try {
      const res = await approvalApi.resubmitTicket(ticketId, {
        sql,
        change_reason: changeReason,
      });
      const [flowRes, historyRes] = await Promise.all([
        approvalApi.getApprovalFlow(ticketId),
        approvalApi.getApprovalHistory(ticketId),
      ]);
      set({
        flow: flowRes.data,
        history: historyRes.data ?? [],
        loading: false,
      });
      return res.data.revision;
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : "重提失败",
        loading: false,
      });
      throw err;
    }
  },

  reset: () => {
    set({
      flow: null,
      history: [],
      loading: false,
      historyLoading: false,
      error: null,
    });
  },
}));
