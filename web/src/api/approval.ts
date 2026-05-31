import { api } from "./client";

// --- Types ---

export interface ApprovalPolicy {
  id: number;
  name: string;
  description: string;
  enabled: boolean;
  priority: number;
  conditions: string; // JSON
  approval_chain: string; // JSON array of { role, auto_skip_same_submitter? }
  auto_approve_enabled: boolean;
  auto_approve_reason: string;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

export interface ApprovalRecord {
  id: number;
  ticket_id: number;
  policy_id: number;
  stage: number;
  total_stages: number;
  approver_role: string;
  approver_id: number;
  approver_name: string;
  action: "approved" | "rejected" | "auto_approved" | "skipped";
  comment: string;
  auto_approved: boolean;
  auto_reason: string;
  created_at: string;
}

export interface TicketRevision {
  id: number;
  ticket_id: number;
  revision: number;
  sql_content: string;
  sql_summary: string;
  change_reason: string;
  risk_level: string;
  ai_review_result: string;
  reviewer_id: number;
  reviewer_name: string;
  review_comment: string;
  created_at: string;
}

export interface ApprovalStage {
  role: string;
  auto_skip_same_submitter?: boolean;
}

// Parsed condition for condition builder
export interface ConditionField {
  field: string;
  operator: string;
  value: string | string[];
}

export interface ConditionGroup {
  logic: "and" | "or";
  conditions: ConditionField[];
}

// --- API Functions ---

export async function listPolicies(): Promise<ApprovalPolicy[]> {
  const res = await api.get<ApprovalPolicy[]>("/approval/policies");
  return res ?? [];
}

export async function getPolicy(id: number): Promise<ApprovalPolicy> {
  return api.get<ApprovalPolicy>(`/approval/policies/${id}`);
}

export async function createPolicy(
  policy: Omit<ApprovalPolicy, "id" | "created_at" | "updated_at">,
): Promise<ApprovalPolicy> {
  return api.post<ApprovalPolicy>("/approval/policies", policy);
}

export async function updatePolicy(
  id: number,
  policy: Partial<ApprovalPolicy>,
): Promise<ApprovalPolicy> {
  return api.put<ApprovalPolicy>(`/approval/policies/${id}`, policy);
}

export async function deletePolicy(id: number): Promise<void> {
  await api.del(`/approval/policies/${id}`);
}

export async function getApprovalHistory(
  ticketId: number,
): Promise<ApprovalRecord[]> {
  const res = await api.get<ApprovalRecord[]>(
    `/tickets/${ticketId}/approval-history`,
  );
  return res ?? [];
}

export async function resubmitTicket(
  ticketId: number,
  data: { sql_content: string; change_reason: string },
): Promise<unknown> {
  return api.put(`/tickets/${ticketId}/resubmit`, data);
}

export async function listRevisions(
  ticketId: number,
): Promise<TicketRevision[]> {
  const res = await api.get<TicketRevision[]>(
    `/tickets/${ticketId}/revisions`,
  );
  return res ?? [];
}

export async function engineApprove(
  ticketId: number,
  action: "approved" | "rejected",
  comment: string,
): Promise<ApprovalRecord> {
  return api.post<ApprovalRecord>(`/tickets/${ticketId}/engine-approve`, {
    action,
    comment,
  });
}

// --- Helpers ---

export function parseApprovalChain(chainJson: string): ApprovalStage[] {
  try {
    return JSON.parse(chainJson) as ApprovalStage[];
  } catch {
    return [];
  }
}

export function parseConditions(conditionsJson: string): ConditionGroup {
  try {
    return JSON.parse(conditionsJson) as ConditionGroup;
  } catch {
    return { logic: "and", conditions: [] };
  }
}

export function stringifyConditions(group: ConditionGroup): string {
  return JSON.stringify(group);
}

export function stringifyApprovalChain(stages: ApprovalStage[]): string {
  return JSON.stringify(stages);
}

export function getStageStatusLabel(
  action: ApprovalRecord["action"],
): string {
  switch (action) {
    case "approved":
      return "已通过";
    case "rejected":
      return "已驳回";
    case "auto_approved":
      return "自动通过";
    case "skipped":
      return "已跳过";
    default:
      return action;
  }
}

export function getStageStatusColor(
  action: ApprovalRecord["action"],
): string {
  switch (action) {
    case "approved":
      return "text-emerald-500";
    case "rejected":
      return "text-red-500";
    case "auto_approved":
      return "text-blue-500";
    case "skipped":
      return "text-zinc-400";
    default:
      return "text-zinc-400";
  }
}

export function getStageDotColor(
  action: ApprovalRecord["action"],
): string {
  switch (action) {
    case "approved":
      return "bg-emerald-500";
    case "rejected":
      return "bg-red-500";
    case "auto_approved":
      return "bg-blue-500";
    case "skipped":
      return "bg-zinc-400";
    default:
      return "bg-zinc-400";
  }
}

export function getRoleLabel(role: string): string {
  switch (role) {
    case "admin":
      return "管理员";
    case "dba":
      return "DBA";
    case "developer":
      return "开发者";
    default:
      return role;
  }
}
