/**
 * ConditionBuilder — SF-FEAT0044 Module C
 * Visual condition builder for approval policies.
 */

import { useMemo, useCallback } from "react";
import { Plus, Trash2, Parentheses } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import {
  getFieldDefinitions,
  getOperatorsForField,
} from "@/api/policy";
import type {
  Condition,
  ConditionGroup,
  FieldType,
  Operator,
  ConditionValue,
} from "@/types/approval";

interface ConditionBuilderProps {
  value: ConditionGroup;
  onChange: (value: ConditionGroup) => void;
  maxDepth?: number;
  className?: string;
}

const EMPTY_GROUP: ConditionGroup = { logic: "AND", conditions: [], groups: [] };

function ValueInput({
  field,
  operator,
  value,
  onChange,
}: {
  field: FieldType;
  operator: Operator;
  value: ConditionValue;
  onChange: (v: ConditionValue) => void;
}) {
  const fieldDef = useMemo(() => getFieldDefinitions().find((f) => f.field === field), [field]);
  if (!fieldDef) return null;

  if (fieldDef.valueType === "single_select" && fieldDef.options) {
    return (
      <Select value={value.value ?? ""} onValueChange={(v) => onChange({ value: v })}>
        <SelectTrigger className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-base)] text-xs">
          <SelectValue placeholder="选择..." />
        </SelectTrigger>
        <SelectContent>
          {fieldDef.options.map((opt) => (
            <SelectItem key={opt.value} value={opt.value}>{opt.label}</SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  if (fieldDef.valueType === "multi_select" && fieldDef.options) {
    const selected = value.values ?? [];
    return (
      <div className="flex flex-wrap items-center gap-1">
        {fieldDef.options.map((opt) => {
          const isSelected = selected.includes(opt.value);
          return (
            <Badge
              key={opt.value}
              variant={isSelected ? "default" : "outline"}
              className={cn(
                "cursor-pointer text-[10px] px-1.5 py-0",
                isSelected
                  ? "bg-orange-500/20 text-orange-400 border-orange-500/30"
                  : "border-[var(--border-default)] text-[var(--text-muted)]",
              )}
              onClick={() => {
                const next = isSelected ? selected.filter((v) => v !== opt.value) : [...selected, opt.value];
                onChange({ values: next });
              }}
            >
              {opt.label}
            </Badge>
          );
        })}
      </div>
    );
  }

  if (fieldDef.valueType === "number" && operator === "between") {
    const range = value.range ?? [0, 0];
    return (
      <div className="flex items-center gap-1">
        <Input
          type="number"
          value={range[0]}
          onChange={(e) => onChange({ range: [Number(e.target.value), range[1]] })}
          className="h-7 w-20 border-[var(--border-default)] bg-[var(--bg-base)] text-xs"
        />
        <span className="text-[var(--text-muted)] text-xs">~</span>
        <Input
          type="number"
          value={range[1]}
          onChange={(e) => onChange({ range: [range[0], Number(e.target.value)] })}
          className="h-7 w-20 border-[var(--border-default)] bg-[var(--bg-base)] text-xs"
        />
      </div>
    );
  }

  if (fieldDef.valueType === "number") {
    return (
      <Input
        type="number"
        value={value.value ?? ""}
        onChange={(e) => onChange({ value: e.target.value })}
        className="h-7 w-24 border-[var(--border-default)] bg-[var(--bg-base)] text-xs"
        placeholder="数值"
      />
    );
  }

  return (
    <Input
      value={value.value ?? ""}
      onChange={(e) => onChange({ value: e.target.value })}
      className="h-7 w-40 border-[var(--border-default)] bg-[var(--bg-base)] text-xs"
      placeholder={fieldDef.valueType === "user_search" ? "搜索用户..." : "输入值..."}
    />
  );
}

function ConditionRow({
  condition,
  index,
  onChange,
  onRemove,
}: {
  condition: Condition;
  index: number;
  onChange: (idx: number, c: Condition) => void;
  onRemove: (idx: number) => void;
}) {
  const fieldDefs = useMemo(() => getFieldDefinitions(), []);
  const operators = useMemo(() => getOperatorsForField(condition.field), [condition.field]);

  return (
    <div className="flex items-center gap-2">
      <Select
        value={condition.field}
        onValueChange={(v) => {
          const newField = v as FieldType;
          const newOps = getOperatorsForField(newField);
          onChange(index, { ...condition, field: newField, operator: newOps[0]?.value ?? "eq", value: {} });
        }}
      >
        <SelectTrigger className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-base)] text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {fieldDefs.map((f) => (
            <SelectItem key={f.field} value={f.field}>{f.label}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={condition.operator}
        onValueChange={(v) => onChange(index, { ...condition, operator: v as Operator, value: {} })}
      >
        <SelectTrigger className="h-7 w-24 border-[var(--border-default)] bg-[var(--bg-base)] text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {operators.map((op) => (
            <SelectItem key={op.value} value={op.value}>{op.label}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      <ValueInput field={condition.field} operator={condition.operator} value={condition.value} onChange={(v) => onChange(index, { ...condition, value: v })} />

      <Button variant="ghost" size="sm" className="h-7 w-7 p-0 text-[var(--text-muted)] hover:text-red-400" onClick={() => onRemove(index)}>
        <Trash2 size={12} />
      </Button>
    </div>
  );
}

function ConditionGroupEditor({
  group,
  onChange,
  depth = 0,
  maxDepth = 1,
}: {
  group: ConditionGroup;
  onChange: (g: ConditionGroup) => void;
  depth?: number;
  maxDepth?: number;
}) {
  const isEmpty = group.conditions.length === 0 && (!group.groups || group.groups.length === 0);

  const updateCondition = useCallback((idx: number, c: Condition) => {
    const next = [...group.conditions];
    next[idx] = c;
    onChange({ ...group, conditions: next });
  }, [group, onChange]);

  const removeCondition = useCallback((idx: number) => {
    onChange({ ...group, conditions: group.conditions.filter((_, i) => i !== idx) });
  }, [group, onChange]);

  const addCondition = useCallback(() => {
    const fieldDefs = getFieldDefinitions();
    const defaultField = fieldDefs[0];
    onChange({
      ...group,
      conditions: [...group.conditions, { field: defaultField.field, operator: defaultField.operators[0].value, value: {} }],
    });
  }, [group, onChange]);

  const addSubGroup = useCallback(() => {
    onChange({ ...group, groups: [...(group.groups ?? []), { logic: "AND", conditions: [] }] });
  }, [group, onChange]);

  const updateSubGroup = useCallback((idx: number, g: ConditionGroup) => {
    const next = [...(group.groups ?? [])];
    next[idx] = g;
    onChange({ ...group, groups: next });
  }, [group, onChange]);

  const removeSubGroup = useCallback((idx: number) => {
    onChange({ ...group, groups: (group.groups ?? []).filter((_, i) => i !== idx) });
  }, [group, onChange]);

  const toggleLogic = useCallback(() => {
    onChange({ ...group, logic: group.logic === "AND" ? "OR" : "AND" });
  }, [group, onChange]);

  return (
    <div className={cn("space-y-2", depth > 0 && "ml-4 border-l-2 border-[var(--border-default)] pl-3")}>
      {group.conditions.length > 1 && (
        <button
          onClick={toggleLogic}
          className={cn(
            "h-7 rounded border px-2 text-[10px] font-medium transition-colors",
            group.logic === "AND"
              ? "border-orange-500/30 bg-orange-500/10 text-orange-400"
              : "border-blue-500/30 bg-blue-500/10 text-blue-400",
          )}
        >
          {group.logic}
        </button>
      )}
      {group.conditions.map((cond, idx) => (
        <ConditionRow key={idx} condition={cond} index={idx} onChange={updateCondition} onRemove={removeCondition} />
      ))}
      {(group.groups ?? []).map((subGroup, idx) => (
        <div key={`group-${idx}`} className="relative">
          <ConditionGroupEditor group={subGroup} onChange={(g) => updateSubGroup(idx, g)} depth={depth + 1} maxDepth={maxDepth} />
          <Button variant="ghost" size="sm" className="absolute -right-1 -top-1 h-5 w-5 p-0 text-[var(--text-muted)] hover:text-red-400" onClick={() => removeSubGroup(idx)}>
            <Trash2 size={10} />
          </Button>
        </div>
      ))}
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-[10px] text-[var(--text-muted)] hover:text-[var(--accent-primary)]" onClick={addCondition}>
          <Plus size={10} /> 添加条件
        </Button>
        {depth < maxDepth && (
          <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-[10px] text-[var(--text-muted)] hover:text-[var(--accent-primary)]" onClick={addSubGroup}>
            <Parentheses size={10} /> 添加条件组
          </Button>
        )}
      </div>
      {isEmpty && <p className="text-[10px] text-[var(--text-muted)] italic">尚未添加条件，将匹配所有工单</p>}
    </div>
  );
}

export default function ConditionBuilder({ value, onChange, maxDepth = 1, className }: ConditionBuilderProps) {
  return (
    <div className={cn("rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3", className)}>
      <ConditionGroupEditor group={value} onChange={onChange} depth={0} maxDepth={maxDepth} />
    </div>
  );
}

export { EMPTY_GROUP };
export type { ConditionBuilderProps };
