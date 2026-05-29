/**
 * Approval flow API — SF-FEAT0044
 * Multi-stage approval, resubmit, history
 */

import { api } from "./client";
import type {
  ApprovalFlowResponse,
  ApprovalHistoryResponse,
  ResubmitRequest,
  ResubmitResponse,
} from "@/types/approval";

// ─── Get approval flow for a ticket ──────────────────────────────────────────

export async function getApprovalFlow(
  ticketId: number,
): Promise<ApprovalFlowResponse> {
  return api.get<ApprovalFlowResponse>(`/tickets/${ticketId}/approval-flow`);
}

// ─── Approve at current stage ────────────────────────────────────────────────

export async function approveStage(
  ticketId: number,
  comment: string,
): Promise<ApprovalFlowResponse> {
  return api.post<ApprovalFlowResponse>(
    `/tickets/${ticketId}/approval/approve`,
    { comment },
  );
}

// ─── Reject at current stage ─────────────────────────────────────────────────

export async function rejectStage(
  ticketId: number,
  reason: string,
): Promise<ApprovalFlowResponse> {
  return api.post<ApprovalFlowResponse>(
    `/tickets/${ticketId}/approval/reject`,
    { reason },
  );
}

// ─── Resubmit a rejected ticket ──────────────────────────────────────────────

export async function resubmitTicket(
  ticketId: number,
  req: ResubmitRequest,
): Promise<ResubmitResponse> {
  return api.post<ResubmitResponse>(
    `/tickets/${ticketId}/approval/resubmit`,
    req,
  );
}

// ─── Get approval history ────────────────────────────────────────────────────

export async function getApprovalHistory(
  ticketId: number,
): Promise<ApprovalHistoryResponse> {
  return api.get<ApprovalHistoryResponse>(
    `/tickets/${ticketId}/approval/history`,
  );
}
