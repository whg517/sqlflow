/**
 * Approval flow types for SF-FEAT0044
 * Multi-stage approval + resubmission + policy engine
 */

// ─── Approval Stage ───────────────────────────────────────────────────────────

export type StageStatus =
  | "PENDING"
  | "APPROVED"
  | "REJECTED"
  | "SKIPPED"
  | "AUTO_APPROVED";

export interface ApprovalStage {
  /** Stage index (0-based) */
  stage: number;
  /** Stage label, e.g. "DBA 审批" */
  label: string;
  /** Approver role */
  approver_role: "dba" | "admin";
  /** Approver user id (null if not yet assigned) */
  approver_id: number | null;
  /** Approver display name */
  approver_name: string | null;
  /** Current status */
  status: StageStatus;
  /** Timestamp of action */
  acted_at: string | null;
  /** Comment / reason */
  comment: string | null;
  /** Auto-approval reason (only when status=AUTO_APPROVED) */
  auto_reason: string | null;
}

// ─── Approval Flow ────────────────────────────────────────────────────────────

export interface ApprovalFlow {
  /** Ticket ID */
  ticket_id: number;
  /** Total stages */
  total_stages: number;
  /** Current stage index (0-based), -1 if all done */
  current_stage: number;
  /** Matched policy name */
  policy_name: string | null;
  /** Policy match reason */
  policy_reason: string | null;
  /** Ordered stages */
  stages: ApprovalStage[];
  /** Current revision number */
  revision: number;
}

// ─── Approval History ─────────────────────────────────────────────────────────

export type ApprovalAction =
  | "APPROVED"
  | "REJECTED"
  | "SKIPPED"
  | "AUTO_APPROVED"
  | "RESUBMITTED"
  | "CANCELLED";

export interface ApprovalHistoryEntry {
  /** Unique entry ID */
  id: number;
  /** Ticket ID */
  ticket_id: number;
  /** Revision number */
  revision: number;
  /** Who performed the action */
  actor_id: number;
  actor_name: string;
  /** Actor role */
  actor_role: string;
  /** What happened */
  action: ApprovalAction;
  /** Stage index (if applicable) */
  stage: number | null;
  /** Stage label */
  stage_label: string | null;
  /** Comment */
  comment: string | null;
  /** Timestamp */
  created_at: string;
  /** SQL content at this revision (only for RESUBMITTED) */
  sql_content: string | null;
  /** Change reason at this revision (only for RESUBMITTED) */
  change_reason: string | null;
}

// ─── Resubmit ─────────────────────────────────────────────────────────────────

export interface ResubmitRequest {
  /** Revised SQL content */
  sql: string;
  /** Updated change reason */
  change_reason: string;
}

export interface ResubmitResponse {
  code: number;
  message: string;
  data: {
    ticket_id: number;
    revision: number;
  };
}

// ─── Policy ───────────────────────────────────────────────────────────────────

export type FieldType =
  | "risk_level"
  | "sql_type"
  | "environment"
  | "affected_tables"
  | "database"
  | "submitter"
  | "affected_rows";

export type Operator =
  | "eq"
  | "neq"
  | "in"
  | "not_in"
  | "contains"
  | "not_contains"
  | "matches_regex"
  | "gt"
  | "lt"
  | "between"
  | "is"
  | "is_not"
  | "in_group";

export interface ConditionValue {
  /** For single value fields */
  value?: string;
  /** For multi-value fields (in/not_in) */
  values?: string[];
  /** For range fields (between) */
  range?: [number, number];
}

export interface Condition {
  /** Field name */
  field: FieldType;
  /** Operator */
  operator: Operator;
  /** Value(s) */
  value: ConditionValue;
}

export type LogicOperator = "AND" | "OR";

export interface ConditionGroup {
  /** Group logic operator */
  logic: LogicOperator;
  /** Conditions in this group */
  conditions: Condition[];
  /** Nested sub-groups (max 1 level deep) */
  groups?: ConditionGroup[];
}

export interface ApprovalNode {
  /** Approver role */
  role: "dba" | "admin";
  /** Specific user IDs (optional, null = anyone with role) */
  user_ids: number[] | null;
  /** User display names (for UI) */
  user_names: string[] | null;
}

export interface ApprovalPolicy {
  /** Policy ID */
  id: number;
  /** Policy name */
  name: string;
  /** Priority (lower = higher priority) */
  priority: number;
  /** Whether enabled */
  enabled: boolean;
  /** Condition tree */
  conditions: ConditionGroup | null;
  /** Approval chain */
  approval_chain: ApprovalNode[];
  /** Description */
  description: string;
  /** Created at */
  created_at: string;
  /** Updated at */
  updated_at: string;
  /** Created by */
  created_by: string;
  /** Version for optimistic concurrency */
  version: number;
}

// ─── API Response Types ───────────────────────────────────────────────────────

export interface ApprovalFlowResponse {
  code: number;
  message: string;
  data: ApprovalFlow;
}

export interface ApprovalHistoryResponse {
  code: number;
  message: string;
  data: ApprovalHistoryEntry[];
}

export interface PolicyListResponse {
  code: number;
  message: string;
  data: ApprovalPolicy[];
}

export interface PolicyDetailResponse {
  code: number;
  message: string;
  data: ApprovalPolicy;
}

export interface PolicyCreateRequest {
  name: string;
  description?: string;
  conditions: ConditionGroup | null;
  approval_chain: ApprovalNode[];
}

export interface PolicyUpdateRequest extends PolicyCreateRequest {
  version: number;
}

export interface PolicyReorderRequest {
  /** Ordered policy IDs */
  ordered_ids: number[];
}

export interface PolicyReorderResponse {
  code: number;
  message: string;
  data: { reordered: number };
}

// ─── Field Definition for ConditionBuilder ────────────────────────────────────

export interface FieldDefinition {
  field: FieldType;
  label: string;
  operators: { value: Operator; label: string }[];
  valueType: "single_select" | "multi_select" | "text" | "number" | "user_search";
  options?: { value: string; label: string }[];
}

// ─── Status helpers ───────────────────────────────────────────────────────────

export const stageStatusLabel: Record<StageStatus, string> = {
  PENDING: "等待中",
  APPROVED: "已通过",
  REJECTED: "已拒绝",
  SKIPPED: "已跳过",
  AUTO_APPROVED: "自动通过",
};

export const stageStatusColor: Record<StageStatus, string> = {
  PENDING: "text-orange-500",
  APPROVED: "text-emerald-500",
  REJECTED: "text-red-500",
  SKIPPED: "text-zinc-500",
  AUTO_APPROVED: "text-blue-500",
};

export const stageStatusBg: Record<StageStatus, string> = {
  PENDING: "bg-orange-500",
  APPROVED: "bg-emerald-500",
  REJECTED: "bg-red-500",
  SKIPPED: "bg-zinc-500",
  AUTO_APPROVED: "bg-blue-500",
};

export const actionLabel: Record<ApprovalAction, string> = {
  APPROVED: "通过",
  REJECTED: "拒绝",
  SKIPPED: "跳过",
  AUTO_APPROVED: "自动通过",
  RESUBMITTED: "重提",
  CANCELLED: "取消",
};

export const actionColor: Record<ApprovalAction, string> = {
  APPROVED: "text-emerald-500",
  REJECTED: "text-red-500",
  SKIPPED: "text-zinc-500",
  AUTO_APPROVED: "text-blue-500",
  RESUBMITTED: "text-orange-500",
  CANCELLED: "text-gray-500",
};

export const actionDot: Record<ApprovalAction, string> = {
  APPROVED: "bg-emerald-500",
  REJECTED: "bg-red-500",
  SKIPPED: "bg-zinc-500",
  AUTO_APPROVED: "bg-blue-500",
  RESUBMITTED: "bg-orange-500",
  CANCELLED: "bg-gray-500",
};
