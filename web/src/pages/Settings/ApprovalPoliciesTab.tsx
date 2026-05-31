import { useState, useCallback, useMemo } from "react";
import {
  Plus,
  Pencil,
  Trash2,
  GripVertical,
  ShieldCheck,
  Loader2,
  Bot,
  ToggleLeft,
  ToggleRight,
  Search,
} from "lucide-react";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@/components/ui/table";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from "@/components/ui/sheet";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  listPolicies,
  createPolicy,
  updatePolicy,
  deletePolicy,
  parseApprovalChain,
  stringifyApprovalChain,
  getRoleLabel,
  type ApprovalPolicy,
  type ApprovalStage,
} from "@/api/approval";

// --- Condition Builder ---

interface ConditionRow {
  field: string;
  operator: string;
  value: string;
}

const CONDITION_FIELDS = [
  { value: "risk_level", label: "风险等级", operators: ["equals", "not_equals", "in"], valueOptions: ["HIGH", "MODERATE", "LOW"] },
  { value: "sql_type", label: "SQL 类型", operators: ["equals", "not_equals", "in"], valueOptions: ["DDL", "DML", "SELECT"] },
  { value: "datasource_environment", label: "环境", operators: ["equals", "not_equals"], valueOptions: ["dev", "staging", "prod"] },
  { value: "has_sensitive_tables", label: "包含敏感表", operators: ["equals"], valueOptions: ["true", "false"] },
  { value: "submitter_role", label: "提交人角色", operators: ["equals", "not_equals"], valueOptions: ["admin", "dba", "developer"] },
];

function ConditionBuilder({
  conditionsJson,
  onChange,
}: {
  conditionsJson: string;
  onChange: (json: string) => void;
}) {
  const initialConditions = useMemo(() => {
    try {
      const parsed = JSON.parse(conditionsJson);
      if (parsed.conditions && Array.isArray(parsed.conditions)) {
        return parsed.conditions.map((c: Record<string, string>) => ({
          field: c.field || "risk_level",
          operator: c.operator || "equals",
          value: c.value || "",
        }));
      }
      return [];
    } catch {
      return [];
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const [conditions, setConditions] = useState<ConditionRow[]>(initialConditions);

  // Sync from external changes (policy switch)
  const [prevJson, setPrevJson] = useState(conditionsJson);
  if (prevJson !== conditionsJson) {
    setPrevJson(conditionsJson);
    try {
      const parsed = JSON.parse(conditionsJson);
      if (parsed.conditions && Array.isArray(parsed.conditions)) {
        setConditions(
          parsed.conditions.map((c: Record<string, string>) => ({
            field: c.field || "risk_level",
            operator: c.operator || "equals",
            value: c.value || "",
          })),
        );
      } else {
        setConditions([]);
      }
    } catch {
      setConditions([]);
    }
  }

  function updateConditions(rows: ConditionRow[]) {
    setConditions(rows);
    const filtered = rows.filter((r) => r.field && r.value);
    if (filtered.length === 0) {
      onChange("{}");
    } else {
      onChange(
        JSON.stringify({
          logic: "and",
          conditions: filtered.map((r) => ({
            field: r.field,
            operator: r.operator,
            value: r.value,
          })),
        }),
      );
    }
  }

  function addCondition() {
    updateConditions([...conditions, { field: "risk_level", operator: "equals", value: "" }]);
  }

  function removeCondition(idx: number) {
    updateConditions(conditions.filter((_, i) => i !== idx));
  }

  function updateRow(idx: number, field: string, value: string) {
    const next = [...conditions];
    next[idx] = { ...next[idx], [field]: value };
    updateConditions(next);
  }

  const fieldConfig = (f: string) => CONDITION_FIELDS.find((c) => c.value === f);

  return (
    <div className="space-y-2">
      {conditions.length === 0 && (
        <p className="text-xs text-zinc-500 italic">
          尚未添加条件，将匹配所有工单
        </p>
      )}
      {conditions.map((row, idx) => {
        const cfg = fieldConfig(row.field);
        return (
          <div key={idx} className="flex items-center gap-2">
            <Select
              value={row.field}
              onValueChange={(v) => updateRow(idx, "field", v)}
            >
              <SelectTrigger className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CONDITION_FIELDS.map((f) => (
                  <SelectItem key={f.value} value={f.value}>
                    {f.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={row.operator}
              onValueChange={(v) => updateRow(idx, "operator", v)}
            >
              <SelectTrigger className="h-7 w-24 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(cfg?.operators ?? ["equals"]).map((op) => (
                  <SelectItem key={op} value={op}>
                    {op === "equals"
                      ? "等于"
                      : op === "not_equals"
                        ? "不等于"
                        : op === "in"
                          ? "属于"
                          : op}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {cfg?.valueOptions ? (
              <Select
                value={row.value}
                onValueChange={(v) => updateRow(idx, "value", v)}
              >
                <SelectTrigger className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
                  <SelectValue placeholder="选择值" />
                </SelectTrigger>
                <SelectContent>
                  {cfg.valueOptions.map((v) => (
                    <SelectItem key={v} value={v}>
                      {v}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                value={row.value}
                onChange={(e) => updateRow(idx, "value", e.target.value)}
                placeholder="值"
                className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
              />
            )}

            <Button
              variant="ghost"
              size="sm"
              className="h-7 w-7 p-0 text-zinc-500 hover:text-red-400"
              onClick={() => removeCondition(idx)}
            >
              ×
            </Button>
          </div>
        );
      })}
      <Button
        variant="ghost"
        size="sm"
        className="h-7 gap-1 text-xs text-zinc-400 hover:text-[var(--text-primary)]"
        onClick={addCondition}
      >
        <Plus size={12} />
        添加条件
      </Button>
    </div>
  );
}

// --- Stage Editor ---

function StageEditor({
  chainJson,
  onChange,
}: {
  chainJson: string;
  onChange: (json: string) => void;
}) {
  const initialStages = useMemo(() => parseApprovalChain(chainJson), []); // eslint-disable-line react-hooks/exhaustive-deps
  const [stages, setStages] = useState<ApprovalStage[]>(initialStages);

  // Sync from external changes
  const [prevChain, setPrevChain] = useState(chainJson);
  if (prevChain !== chainJson) {
    setPrevChain(chainJson);
    setStages(parseApprovalChain(chainJson));
  }

  function updateStages(newStages: ApprovalStage[]) {
    setStages(newStages);
    onChange(stringifyApprovalChain(newStages));
  }

  function addStage() {
    updateStages([...stages, { role: "dba", auto_skip_same_submitter: false }]);
  }

  function removeStage(idx: number) {
    updateStages(stages.filter((_, i) => i !== idx));
  }

  function updateStage(idx: number, field: string, value: unknown) {
    const next = [...stages];
    next[idx] = { ...next[idx], [field]: value };
    updateStages(next);
  }

  return (
    <div className="space-y-2">
      {stages.length === 0 && (
        <div className="flex items-center gap-2 rounded-md border border-blue-500/20 bg-blue-500/5 px-3 py-2">
          <Bot size={14} className="text-blue-500" />
          <span className="text-xs text-blue-400">空审批链 = 自动审批通过</span>
        </div>
      )}
      {stages.map((stage, idx) => (
        <div
          key={idx}
          className="flex items-center gap-2 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 py-2"
        >
          <GripVertical size={14} className="text-zinc-500 cursor-grab" />
          <span className="text-xs text-zinc-500 w-6">#{idx + 1}</span>

          <Select
            value={stage.role}
            onValueChange={(v) => updateStage(idx, "role", v)}
          >
            <SelectTrigger className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="dba">DBA</SelectItem>
              <SelectItem value="admin">管理员</SelectItem>
            </SelectContent>
          </Select>

          <label className="flex items-center gap-1.5 text-xs text-[var(--text-secondary)]">
            <input
              type="checkbox"
              checked={stage.auto_skip_same_submitter ?? false}
              onChange={(e) =>
                updateStage(idx, "auto_skip_same_submitter", e.target.checked)
              }
              className="rounded border-zinc-600"
            />
            提交人=审批人时跳过
          </label>

          <Button
            variant="ghost"
            size="sm"
            className="ml-auto h-7 w-7 p-0 text-zinc-500 hover:text-red-400"
            onClick={() => removeStage(idx)}
          >
            ×
          </Button>
        </div>
      ))}
      <Button
        variant="ghost"
        size="sm"
        className="h-7 gap-1 text-xs text-zinc-400 hover:text-[var(--text-primary)]"
        onClick={addStage}
      >
        <Plus size={12} />
        添加审批节点
      </Button>
    </div>
  );
}

// --- Policy Sheet (Editor) ---

function PolicySheet({
  open,
  onOpenChange,
  policy,
  onSave,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  policy: ApprovalPolicy | null;
  onSave: () => void;
}) {
  const [form, setForm] = useState({
    name: "",
    description: "",
    priority: 99,
    conditions: "{}",
    approval_chain: "[{\"role\":\"dba\"}]",
    auto_approve_enabled: false,
    auto_approve_reason: "",
    enabled: true,
  });
  const [saving, setSaving] = useState(false);

  // Sync form when policy changes or sheet opens
  const [prevPolicy, setPrevPolicy] = useState<{ id: number | null; open: boolean }>({ id: null, open: false });
  const policyKey = policy ? policy.id : null;
  if (prevPolicy.id !== policyKey || prevPolicy.open !== open) {
    setPrevPolicy({ id: policyKey, open });
    if (policy) {
      setForm({
        name: policy.name,
        description: policy.description || "",
        priority: policy.priority,
        conditions: policy.conditions || "{}",
        approval_chain: policy.approval_chain || "[]",
        auto_approve_enabled: policy.auto_approve_enabled,
        auto_approve_reason: policy.auto_approve_reason || "",
        enabled: policy.enabled,
      });
    } else {
      setForm({
        name: "",
        description: "",
        priority: 99,
        conditions: "{}",
        approval_chain: "[{\"role\":\"dba\"}]",
        auto_approve_enabled: false,
        auto_approve_reason: "",
        enabled: true,
      });
    }
  }

  async function handleSave() {
    if (!form.name.trim()) {
      toast.error("策略名称不能为空");
      return;
    }
    setSaving(true);
    try {
      if (policy) {
        await updatePolicy(policy.id, {
          name: form.name.trim(),
          description: form.description.trim(),
          priority: form.priority,
          conditions: form.conditions,
          approval_chain: form.approval_chain,
          auto_approve_enabled: form.auto_approve_enabled,
          auto_approve_reason: form.auto_approve_reason.trim(),
          enabled: form.enabled,
        });
        toast.success("策略已更新");
      } else {
        await createPolicy({
          name: form.name.trim(),
          description: form.description.trim(),
          priority: form.priority,
          conditions: form.conditions,
          approval_chain: form.approval_chain,
          auto_approve_enabled: form.auto_approve_enabled,
          auto_approve_reason: form.auto_approve_reason.trim(),
          enabled: form.enabled,
          is_default: false,
        });
        toast.success("策略已创建");
      }
      onOpenChange(false);
      onSave();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        showCloseButton
        className="w-[50%] max-w-[640px] border-[var(--border-default)] bg-[var(--bg-surface)] flex flex-col"
      >
        <SheetHeader className="px-6 pt-6 pb-0">
          <SheetTitle className="text-[var(--text-primary)]">
            {policy ? "编辑策略" : "新建策略"}
          </SheetTitle>
        </SheetHeader>

        <div className="flex-1 overflow-auto px-6 py-4 space-y-5">
          {/* Basic Info */}
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">策略名称</Label>
                <Input
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  placeholder="如：生产高风险DDL"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
                />
              </div>
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">优先级</Label>
                <Input
                  type="number"
                  value={form.priority}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, priority: Number(e.target.value) }))
                  }
                  className="w-24 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
                />
                <p className="text-[10px] text-zinc-500">数字越小优先级越高</p>
              </div>
            </div>
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">描述</Label>
              <Textarea
                value={form.description}
                onChange={(e) =>
                  setForm((f) => ({ ...f, description: e.target.value }))
                }
                placeholder="策略用途说明..."
                className="min-h-[60px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
              />
            </div>
          </div>

          <div className="h-px bg-[var(--border-default)]" />

          {/* Condition Builder */}
          <div>
            <Label className="mb-2 block text-sm font-medium text-[var(--text-secondary)]">
              匹配条件
            </Label>
            <ConditionBuilder
              conditionsJson={form.conditions}
              onChange={(json) => setForm((f) => ({ ...f, conditions: json }))}
            />
          </div>

          <div className="h-px bg-[var(--border-default)]" />

          {/* Approval Chain */}
          <div>
            <Label className="mb-2 block text-sm font-medium text-[var(--text-secondary)]">
              审批链
            </Label>
            <StageEditor
              chainJson={form.approval_chain}
              onChange={(json) =>
                setForm((f) => ({ ...f, approval_chain: json }))
              }
            />
          </div>

          <div className="h-px bg-[var(--border-default)]" />

          {/* Auto Approve */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label className="text-sm font-medium text-[var(--text-secondary)]">
                自动审批
              </Label>
              <Switch
                checked={form.auto_approve_enabled}
                onCheckedChange={(v) =>
                  setForm((f) => ({ ...f, auto_approve_enabled: v }))
                }
              />
            </div>
            {form.auto_approve_enabled && (
              <Input
                value={form.auto_approve_reason}
                onChange={(e) =>
                  setForm((f) => ({ ...f, auto_approve_reason: e.target.value }))
                }
                placeholder="自动审批原因（如：开发环境免审批）"
                className="border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
              />
            )}
          </div>

          {/* Enabled */}
          <div className="flex items-center justify-between">
            <Label className="text-sm font-medium text-[var(--text-secondary)]">
              启用策略
            </Label>
            <Switch
              checked={form.enabled}
              onCheckedChange={(v) =>
                setForm((f) => ({ ...f, enabled: v }))
              }
            />
          </div>
        </div>

        <SheetFooter className="border-t border-[var(--border-default)] px-6 py-3">
          <div className="flex w-full justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              className="h-8 border-[var(--border-default)] text-xs"
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button
              size="sm"
              className="h-8 gap-1 bg-[var(--accent-primary)] text-xs text-white hover:bg-[var(--accent-hover)]"
              disabled={saving || !form.name.trim()}
              onClick={handleSave}
            >
              {saving && <Loader2 size={14} className="animate-spin" />}
              保存
            </Button>
          </div>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

// --- Policy List ---

export default function ApprovalPoliciesTab() {
  const [policies, setPolicies] = useState<ApprovalPolicy[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState("");

  // Editor sheet
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<ApprovalPolicy | null>(null);

  // Delete confirm
  const [deleteTarget, setDeleteTarget] = useState<ApprovalPolicy | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteConfirmName, setDeleteConfirmName] = useState("");

  const fetchPolicies = useCallback(async () => {
    setLoading(true);
    try {
      const list = await listPolicies();
      setPolicies(list);
    } catch {
      toast.error("获取审批策略失败");
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial fetch on mount
  const [inited, setInited] = useState(false);
  if (!inited && !loading) {
    setInited(true);
    fetchPolicies();
  }

  const filteredPolicies = searchKeyword
    ? policies.filter(
        (p) =>
          p.name.toLowerCase().includes(searchKeyword.toLowerCase()) ||
          p.description?.toLowerCase().includes(searchKeyword.toLowerCase()),
      )
    : policies;

  async function handleToggleEnabled(policy: ApprovalPolicy) {
    if (policy.enabled) {
      // Disable needs confirm
      setDeleteTarget(policy);
      return;
    }
    // Enable directly
    try {
      await updatePolicy(policy.id, { enabled: true });
      toast.success("策略已启用");
      fetchPolicies();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    }
  }

  async function handleDisable() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await updatePolicy(deleteTarget.id, { enabled: false });
      toast.success("策略已禁用");
      setDeleteTarget(null);
      fetchPolicies();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    } finally {
      setDeleting(false);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    if (
      policies.length > 5 &&
      deleteConfirmName !== deleteTarget.name
    ) {
      toast.error("请输入策略名称确认删除");
      return;
    }
    setDeleting(true);
    try {
      await deletePolicy(deleteTarget.id);
      toast.success("策略已删除");
      setDeleteTarget(null);
      setDeleteConfirmName("");
      fetchPolicies();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败");
    } finally {
      setDeleting(false);
    }
  }

  function openCreate() {
    setEditingPolicy(null);
    setEditorOpen(true);
  }

  function openEdit(policy: ApprovalPolicy) {
    setEditingPolicy(policy);
    setEditorOpen(true);
  }

  function getConditionSummary(conditionsJson: string): string {
    try {
      const parsed = JSON.parse(conditionsJson);
      if (!parsed.conditions || parsed.conditions.length === 0) {
        return "匹配所有";
      }
      return parsed.conditions
        .map(
          (c: Record<string, string>) =>
            `${CONDITION_FIELDS.find((f) => f.value === c.field)?.label ?? c.field} ${c.operator === "equals" ? "=" : c.operator === "not_equals" ? "≠" : "∈"} ${c.value}`,
        )
        .join(" ∧ ");
    } catch {
      return "匹配所有";
    }
  }

  function getChainSummary(chainJson: string): string {
    const stages = parseApprovalChain(chainJson);
    if (stages.length === 0) return "自动审批";
    return stages.map((s) => getRoleLabel(s.role)).join(" → ");
  }

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-[var(--text-primary)]">
          审批策略管理
        </h2>
        <Button
          onClick={openCreate}
          size="sm"
          className="gap-1 bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
        >
          <Plus size={14} />
          新建策略
        </Button>
      </div>

      {/* Search */}
      <div className="relative">
        <Search
          size={14}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]"
        />
        <Input
          value={searchKeyword}
          onChange={(e) => setSearchKeyword(e.target.value)}
          placeholder="搜索策略..."
          className="h-8 w-64 rounded-md border-[var(--border-default)] bg-[var(--bg-elevated)] pl-8 text-xs"
        />
      </div>

      {/* Policy Table */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] table-responsive">
        <Table>
          <TableHeader>
            <TableRow className="border-[var(--border-default)] hover:bg-transparent">
              <TableHead className="w-12 text-xs text-[var(--text-secondary)]">
                优先级
              </TableHead>
              <TableHead className="text-xs text-[var(--text-secondary)]">
                策略名称
              </TableHead>
              <TableHead className="text-xs text-[var(--text-secondary)]">
                匹配条件
              </TableHead>
              <TableHead className="text-xs text-[var(--text-secondary)]">
                审批链
              </TableHead>
              <TableHead className="w-20 text-xs text-[var(--text-secondary)]">
                状态
              </TableHead>
              <TableHead className="w-32 text-xs text-[var(--text-secondary)]">
                操作
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && !policies.length ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="h-24 text-center text-[var(--text-muted)]"
                >
                  <Loader2 size={16} className="mx-auto animate-spin" />
                </TableCell>
              </TableRow>
            ) : filteredPolicies.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="h-32">
                  <div className="flex flex-col items-center gap-2">
                    <ShieldCheck size={20} className="text-[var(--text-muted)]" />
                    <span className="text-sm text-[var(--text-muted)]">
                      {searchKeyword ? "没有匹配的策略" : "暂无审批策略"}
                    </span>
                    <span className="text-xs text-[var(--text-muted)]">
                      {searchKeyword
                        ? "尝试其他关键词"
                        : "点击「新建策略」添加审批规则"}
                    </span>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              filteredPolicies
                .sort((a, b) => a.priority - b.priority)
                .map((p) => (
                  <TableRow
                    key={p.id}
                    className={cn(
                      "border-[var(--border-default)]",
                      !p.enabled && "opacity-50",
                    )}
                  >
                    <TableCell className="text-xs font-medium text-[var(--text-primary)]">
                      {p.priority}
                    </TableCell>
                    <TableCell>
                      <span
                        className={cn(
                          "text-xs font-medium",
                          p.enabled
                            ? "text-[var(--text-primary)]"
                            : "line-through text-zinc-500",
                        )}
                      >
                        {p.name}
                      </span>
                      {p.is_default && (
                        <Badge className="ml-1.5 bg-zinc-500/20 text-zinc-400 border-0 text-[9px] px-1">
                          默认
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell className="text-xs text-[var(--text-secondary)] max-w-[200px] truncate">
                      {getConditionSummary(p.conditions)}
                    </TableCell>
                    <TableCell className="text-xs text-[var(--text-secondary)]">
                      {getChainSummary(p.approval_chain)}
                    </TableCell>
                    <TableCell>
                      <button
                        onClick={() => handleToggleEnabled(p)}
                        className="flex items-center gap-1"
                        title={p.enabled ? "点击禁用" : "点击启用"}
                      >
                        {p.enabled ? (
                          <ToggleRight
                            size={18}
                            className="text-emerald-500"
                          />
                        ) : (
                          <ToggleLeft size={18} className="text-zinc-500" />
                        )}
                      </button>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                          onClick={() => openEdit(p)}
                        >
                          <Pencil size={13} />
                          编辑
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 gap-1 text-xs text-zinc-500 hover:text-red-400"
                          onClick={() => {
                            setDeleteTarget(p);
                            setDeleteConfirmName("");
                          }}
                        >
                          <Trash2 size={13} />
                          删除
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
            )}
          </TableBody>
        </Table>
      </div>

      {/* Policy Editor Sheet */}
      <PolicySheet
        open={editorOpen}
        onOpenChange={setEditorOpen}
        policy={editingPolicy}
        onSave={fetchPolicies}
      />

      {/* Disable / Delete Confirm Dialog */}
      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
            setDeleteConfirmName("");
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {deleteTarget?.enabled ? "禁用策略" : "删除策略"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget?.enabled
                ? `确定要禁用策略「${deleteTarget?.name}」吗？禁用后将不再匹配。`
                : `确定要删除策略「${deleteTarget?.name}」吗？此操作不可恢复。`}
            </AlertDialogDescription>
          </AlertDialogHeader>

          {/* Name confirm for delete when > 5 policies */}
          {!deleteTarget?.enabled && policies.length > 5 && (
            <div className="space-y-1.5">
              <Label className="text-xs text-[var(--text-secondary)]">
                请输入策略名称确认: <span className="font-medium">{deleteTarget?.name}</span>
              </Label>
              <Input
                value={deleteConfirmName}
                onChange={(e) => setDeleteConfirmName(e.target.value)}
                placeholder={deleteTarget?.name}
                className="border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs"
              />
            </div>
          )}

          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={deleteTarget?.enabled ? handleDisable : handleDelete}
              disabled={
                deleting ||
                (!deleteTarget?.enabled &&
                  policies.length > 5 &&
                  deleteConfirmName !== deleteTarget?.name)
              }
              className={
                deleteTarget?.enabled
                  ? "bg-[var(--accent-primary)] text-white"
                  : "bg-red-600 text-white hover:bg-red-700"
              }
            >
              {deleting
                ? "处理中..."
                : deleteTarget?.enabled
                  ? "确认禁用"
                  : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
