/**
 * Approval policy API — SF-FEAT0044
 * CRUD + reorder + toggle for approval policies
 */

import { api } from "./client";
import type {
  PolicyListResponse,
  PolicyDetailResponse,
  PolicyCreateRequest,
  PolicyUpdateRequest,
  PolicyReorderRequest,
  PolicyReorderResponse,
  ApprovalPolicy,
  FieldDefinition,
  FieldType,
  Operator,
} from "@/types/approval";

// ─── List all policies (ordered by priority) ─────────────────────────────────

export async function listPolicies(): Promise<PolicyListResponse> {
  return api.get<PolicyListResponse>("/approval-policies");
}

// ─── Get single policy ───────────────────────────────────────────────────────

export async function getPolicy(
  id: number,
): Promise<PolicyDetailResponse> {
  return api.get<PolicyDetailResponse>(`/approval-policies/${id}`);
}

// ─── Create policy ───────────────────────────────────────────────────────────

export async function createPolicy(
  req: PolicyCreateRequest,
): Promise<PolicyDetailResponse> {
  return api.post<PolicyDetailResponse>("/approval-policies", req);
}

// ─── Update policy (optimistic concurrency via version) ──────────────────────

export async function updatePolicy(
  id: number,
  req: PolicyUpdateRequest,
): Promise<PolicyDetailResponse> {
  return api.put<PolicyDetailResponse>(`/approval-policies/${id}`, req);
}

// ─── Delete policy ───────────────────────────────────────────────────────────

export async function deletePolicy(
  id: number,
): Promise<{ code: number; message: string }> {
  return api.del<{ code: number; message: string }>(`/approval-policies/${id}`);
}

// ─── Toggle policy enabled/disabled ──────────────────────────────────────────

export async function togglePolicy(
  id: number,
  enabled: boolean,
  version: number,
): Promise<PolicyDetailResponse> {
  return api.put<PolicyDetailResponse>(`/approval-policies/${id}`, {
    enabled,
    version,
  });
}

// ─── Reorder policies (drag & drop priority) ─────────────────────────────────

export async function reorderPolicies(
  req: PolicyReorderRequest,
): Promise<PolicyReorderResponse> {
  return api.post<PolicyReorderResponse>("/approval-policies/reorder", req);
}

// ─── Field definitions for condition builder ─────────────────────────────────

const FIELD_DEFINITIONS: FieldDefinition[] = [
  {
    field: "risk_level",
    label: "风险等级",
    operators: [
      { value: "eq", label: "等于" },
      { value: "neq", label: "不等于" },
      { value: "in", label: "属于" },
    ],
    valueType: "single_select",
    options: [
      { value: "HIGH", label: "高风险" },
      { value: "MODERATE", label: "中风险" },
      { value: "LOW", label: "低风险" },
    ],
  },
  {
    field: "sql_type",
    label: "SQL 类型",
    operators: [
      { value: "eq", label: "等于" },
      { value: "neq", label: "不等于" },
      { value: "in", label: "属于" },
    ],
    valueType: "multi_select",
    options: [
      { value: "DDL", label: "DDL" },
      { value: "DML", label: "DML" },
      { value: "SELECT", label: "SELECT" },
    ],
  },
  {
    field: "environment",
    label: "环境",
    operators: [
      { value: "eq", label: "等于" },
      { value: "neq", label: "不等于" },
    ],
    valueType: "single_select",
    options: [
      { value: "DEV", label: "开发" },
      { value: "STAGING", label: "预发" },
      { value: "PROD", label: "生产" },
    ],
  },
  {
    field: "affected_tables",
    label: "影响表",
    operators: [
      { value: "contains", label: "包含" },
      { value: "not_contains", label: "不包含" },
      { value: "matches_regex", label: "匹配正则" },
    ],
    valueType: "text",
  },
  {
    field: "database",
    label: "数据库",
    operators: [
      { value: "eq", label: "等于" },
      { value: "neq", label: "不等于" },
      { value: "in", label: "属于" },
    ],
    valueType: "text",
  },
  {
    field: "submitter",
    label: "提交人",
    operators: [
      { value: "is", label: "是" },
      { value: "is_not", label: "不是" },
      { value: "in_group", label: "属于组" },
    ],
    valueType: "user_search",
  },
  {
    field: "affected_rows",
    label: "影响行数",
    operators: [
      { value: "gt", label: "大于" },
      { value: "lt", label: "小于" },
      { value: "between", label: "范围" },
    ],
    valueType: "number",
  },
];

export function getFieldDefinitions(): FieldDefinition[] {
  return FIELD_DEFINITIONS;
}

export function getFieldDefinition(field: FieldType): FieldDefinition | undefined {
  return FIELD_DEFINITIONS.find((f) => f.field === field);
}

export function getOperatorsForField(field: FieldType): { value: Operator; label: string }[] {
  return FIELD_DEFINITIONS.find((f) => f.field === field)?.operators ?? [];
}

// ─── Condition display helpers ────────────────────────────────────────────────

export function conditionSummary(conditions: ApprovalPolicy["conditions"]): string {
  if (!conditions || conditions.conditions.length === 0) {
    return "匹配所有工单";
  }

  const parts = conditions.conditions.map((c) => {
    const fieldDef = getFieldDefinition(c.field);
    const fieldLabel = fieldDef?.label ?? c.field;
    const opLabel = fieldDef?.operators.find((o) => o.value === c.operator)?.label ?? c.operator;
    let valueLabel = "";
    if (c.value.values && c.value.values.length > 0) {
      valueLabel = c.value.values.join(", ");
    } else if (c.value.range) {
      valueLabel = `${c.value.range[0]} ~ ${c.value.range[1]}`;
    } else if (c.value.value) {
      const opt = fieldDef?.options?.find((o) => o.value === c.value.value);
      valueLabel = opt?.label ?? c.value.value;
    }
    return `${fieldLabel} ${opLabel} ${valueLabel}`;
  });

  return parts.join(` ${conditions.logic} `);
}
