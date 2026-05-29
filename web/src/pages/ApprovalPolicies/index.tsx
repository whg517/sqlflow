/**
 * ApprovalPoliciesPage — SF-FEAT0044 Module D
 */

import { useState, useEffect } from "react";
import {
  Plus,
  GripVertical,
  Trash2,
  Loader2,
  Shield,
  Settings,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Separator } from "@/components/ui/separator";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from "@/components/ui/sheet";
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import ConditionBuilder, { EMPTY_GROUP } from "@/components/approval/ConditionBuilder";
import { usePolicyStore } from "@/store/policyStore";
import { conditionSummary } from "@/api/policy";
import type {
  ApprovalPolicy,
  ApprovalNode,
  ConditionGroup,
  PolicyCreateRequest,
  PolicyUpdateRequest,
} from "@/types/approval";
import { cn } from "@/lib/utils";

function ApproverRow({
  node,
  index,
  onChange,
  onRemove,
}: {
  node: ApprovalNode;
  index: number;
  onChange: (idx: number, node: ApprovalNode) => void;
  onRemove: (idx: number) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <Select
        value={node.role}
        onValueChange={(v) => onChange(index, { ...node, role: v as "dba" | "admin", user_ids: null, user_names: null })}
      >
        <SelectTrigger className="h-7 w-28 border-[var(--border-default)] bg-[var(--bg-base)] text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="dba">
            <span className="flex items-center gap-1.5">
              <Badge className="bg-blue-500/20 text-blue-400 border-0 text-[9px] px-1 py-0">DBA</Badge>
              DBA 审批
            </span>
          </SelectItem>
          <SelectItem value="admin">
            <span className="flex items-center gap-1.5">
              <Badge className="bg-purple-500/20 text-purple-400 border-0 text-[9px] px-1 py-0">Admin</Badge>
              Admin 审批
            </span>
          </SelectItem>
        </SelectContent>
      </Select>
      <Input
        value={node.user_names?.join(", ") ?? ""}
        onChange={(e) => {
          const names = e.target.value.split(",").map((s) => s.trim()).filter(Boolean);
          onChange(index, { ...node, user_names: names.length > 0 ? names : null });
        }}
        className="h-7 w-40 border-[var(--border-default)] bg-[var(--bg-base)] text-xs"
        placeholder="指定审批人（可选）"
      />
      <Button variant="ghost" size="sm" className="h-7 w-7 p-0 text-[var(--text-muted)] hover:text-red-400" onClick={() => onRemove(index)}>
        <Trash2 size={12} />
      </Button>
    </div>
  );
}

function PolicyEditorSheet({
  open,
  onOpenChange,
  editingPolicy,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editingPolicy: ApprovalPolicy | null;
}) {
  const store = usePolicyStore();
  const [confirmDiscard, setConfirmDiscard] = useState(false);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [conditions, setConditions] = useState<ConditionGroup>(EMPTY_GROUP);
  const [chain, setChain] = useState<ApprovalNode[]>([
    { role: "dba", user_ids: null, user_names: null },
  ]);

  const isDirty = editingPolicy
    ? name !== editingPolicy.name ||
      description !== (editingPolicy.description ?? "") ||
      JSON.stringify(conditions) !== JSON.stringify(editingPolicy.conditions ?? EMPTY_GROUP) ||
      JSON.stringify(chain) !== JSON.stringify(editingPolicy.approval_chain)
    : name !== "" || description !== "" || conditions.conditions.length > 0;

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen && isDirty) { setConfirmDiscard(true); return; }
    onOpenChange(false);
  }

  async function handleSave() {
    if (!name.trim()) { toast.error("请输入策略名称"); return; }
    if (chain.length === 0) { toast.error("至少需要一个审批节点"); return; }
    try {
      if (editingPolicy) {
        const req: PolicyUpdateRequest = {
          name: name.trim(),
          description: description.trim() || undefined,
          conditions: conditions.conditions.length > 0 ? conditions : null,
          approval_chain: chain,
          version: editingPolicy.version,
        };
        await store.updatePolicy(editingPolicy.id, req);
      } else {
        const req: PolicyCreateRequest = {
          name: name.trim(),
          description: description.trim() || undefined,
          conditions: conditions.conditions.length > 0 ? conditions : null,
          approval_chain: chain,
        };
        await store.createPolicy(req);
      }
      onOpenChange(false);
    } catch { /* handled by store */ }
  }

  return (
    <>
      <Sheet open={open} onOpenChange={handleOpenChange}>
        <SheetContent side="right" showCloseButton className="w-[50%] max-w-[640px] border-[var(--border-default)] bg-[var(--bg-surface)] flex flex-col">
          <SheetHeader className="px-6 pt-6 pb-0">
            <SheetTitle className="text-[var(--text-primary)]">
              {editingPolicy ? `编辑策略: ${editingPolicy.name}` : "新建审批策略"}
            </SheetTitle>
          </SheetHeader>
          <ScrollArea className="flex-1">
            <div className="space-y-5 px-6 py-4">
              <div>
                <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">策略名称 <span className="text-red-400">*</span></label>
                <Input value={name} onChange={(e) => setName(e.target.value)} className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm text-[var(--text-primary)]" placeholder="输入策略名称..." />
              </div>
              <div>
                <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">描述</label>
                <Textarea value={description} onChange={(e) => setDescription(e.target.value)} className="min-h-[60px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs text-[var(--text-primary)]" placeholder="策略用途说明..." />
              </div>
              <Separator className="bg-[var(--border-default)]" />
              <div>
                <label className="mb-2 block text-xs font-medium text-[var(--text-secondary)]">匹配条件</label>
                <ConditionBuilder value={conditions} onChange={setConditions} />
              </div>
              <Separator className="bg-[var(--border-default)]" />
              <div>
                <label className="mb-2 block text-xs font-medium text-[var(--text-secondary)]">审批链 <span className="text-red-400">*</span></label>
                <div className="space-y-2">
                  {chain.map((node, idx) => (
                    <div key={idx} className="flex items-center gap-1">
                      <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--bg-elevated)] text-[10px] text-[var(--text-muted)]">{idx + 1}</span>
                      <ApproverRow node={node} index={idx} onChange={(i, n) => setChain((prev) => prev.map((item, j) => j === i ? n : item))} onRemove={(i) => setChain((prev) => prev.filter((_, j) => j !== i))} />
                    </div>
                  ))}
                  <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs text-[var(--text-muted)] hover:text-[var(--accent-primary)]" onClick={() => setChain((prev) => [...prev, { role: "dba", user_ids: null, user_names: null }])}>
                    <Plus size={12} /> 添加审批节点
                  </Button>
                </div>
              </div>
            </div>
          </ScrollArea>
          <SheetFooter className="border-t border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-3">
            <div className="flex w-full items-center justify-end gap-2">
              <Button variant="ghost" size="sm" className="h-8 px-3 text-xs text-[var(--text-muted)]" onClick={() => handleOpenChange(false)}>取消</Button>
              <Button size="sm" className="h-8 gap-1.5 bg-[var(--accent-primary)] px-4 text-xs text-white hover:bg-[var(--accent-hover)]" onClick={handleSave} disabled={store.saving}>
                {store.saving ? <Loader2 size={14} className="animate-spin" /> : null}
                {editingPolicy ? "保存" : "创建"}
              </Button>
            </div>
          </SheetFooter>
        </SheetContent>
      </Sheet>
      <AlertDialog open={confirmDiscard} onOpenChange={setConfirmDiscard}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>放弃修改？</AlertDialogTitle>
            <AlertDialogDescription>你有未保存的修改，确认放弃吗？</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>继续编辑</AlertDialogCancel>
            <AlertDialogAction onClick={() => { setConfirmDiscard(false); onOpenChange(false); }}>放弃</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

export default function ApprovalPoliciesPage() {
  const store = usePolicyStore();
  const [editorOpen, setEditorOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ApprovalPolicy | null>(null);
  const [deleteConfirmName, setDeleteConfirmName] = useState("");
  const [disableTarget, setDisableTarget] = useState<ApprovalPolicy | null>(null);
  const [dragIdx, setDragIdx] = useState<number | null>(null);
  const [dropTarget, setDropTarget] = useState<number | null>(null);

  useEffect(() => { store.fetchPolicies(); }, []);

  function handleDragEnd() {
    if (dragIdx !== null && dropTarget !== null && dragIdx !== dropTarget) {
      const ids = store.policies.map((p) => p.id);
      const [moved] = ids.splice(dragIdx, 1);
      ids.splice(dropTarget, 0, moved);
      store.reorderPolicies(ids);
    }
    setDragIdx(null);
    setDropTarget(null);
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    if (store.policies.length > 5 && deleteConfirmName !== deleteTarget.name) {
      toast.error("请输入正确的策略名称以确认删除");
      return;
    }
    try { await store.deletePolicy(deleteTarget.id); setDeleteTarget(null); setDeleteConfirmName(""); } catch {}
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between mb-5">
        <div>
          <h1 className="text-xl font-semibold text-[var(--text-primary)]">审批策略管理</h1>
          <p className="mt-1 text-xs text-[var(--text-muted)]">配置多级审批流程和匹配规则</p>
        </div>
        <Button size="sm" className="h-8 gap-1.5 bg-[var(--accent-primary)] px-3 text-xs text-white hover:bg-[var(--accent-hover)]" onClick={() => { store.setEditingPolicy(null); setEditorOpen(true); }}>
          <Plus size={14} /> 新建策略
        </Button>
      </div>
      <div className="flex-1 overflow-hidden rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] flex flex-col">
        {store.loading ? (
          <div className="flex h-32 items-center justify-center"><Loader2 className="h-5 w-5 animate-spin text-[var(--text-muted)]" /></div>
        ) : store.policies.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 py-12">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-[var(--bg-elevated)]"><Shield size={24} className="text-[var(--text-muted)]" /></div>
            <div className="text-center">
              <p className="text-sm font-medium text-[var(--text-secondary)]">暂无审批策略</p>
              <p className="mt-1 text-xs text-[var(--text-muted)]">创建第一条策略来定义审批流程</p>
            </div>
            <Button size="sm" className="h-8 gap-1.5 bg-[var(--accent-primary)] px-3 text-xs text-white" onClick={() => { store.setEditingPolicy(null); setEditorOpen(true); }}>
              <Plus size={14} /> 新建策略
            </Button>
          </div>
        ) : (
          <div className="flex-1 overflow-auto">
            <Table>
              <TableHeader>
                <TableRow className="border-[var(--border-default)] bg-[var(--bg-surface)] hover:bg-[var(--bg-surface)]">
                  <TableHead className="w-8" />
                  <TableHead className="w-12 text-xs text-[var(--text-secondary)]">优先级</TableHead>
                  <TableHead className="text-xs text-[var(--text-secondary)]">策略名称</TableHead>
                  <TableHead className="text-xs text-[var(--text-secondary)]">匹配条件</TableHead>
                  <TableHead className="text-xs text-[var(--text-secondary)]">审批链</TableHead>
                  <TableHead className="w-20 text-xs text-[var(--text-secondary)]">状态</TableHead>
                  <TableHead className="w-24 text-xs text-[var(--text-secondary)]">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {store.policies.map((policy, idx) => (
                  <TableRow
                    key={policy.id}
                    className={cn(
                      "cursor-pointer border-[var(--border-subtle)] hover:bg-[var(--bg-elevated)] transition-opacity",
                      dragIdx === idx && "opacity-50",
                      !policy.enabled && "opacity-50",
                      dropTarget === idx && "border-t-2 border-t-orange-500",
                    )}
                    draggable
                    onDragStart={() => setDragIdx(idx)}
                    onDragOver={(e) => { e.preventDefault(); setDropTarget(idx); }}
                    onDragEnd={handleDragEnd}
                    onClick={() => { store.setEditingPolicy(policy); setEditorOpen(true); }}
                  >
                    <TableCell className="w-8 px-2" onClick={(e) => e.stopPropagation()}>
                      <GripVertical size={14} className="text-[var(--text-muted)] cursor-grab" />
                    </TableCell>
                    <TableCell className="text-xs text-[var(--text-muted)]">{idx + 1}</TableCell>
                    <TableCell className="text-xs">
                      <span className={cn("font-medium", policy.enabled ? "text-[var(--text-primary)]" : "text-[var(--text-muted)] line-through")}>{policy.name}</span>
                    </TableCell>
                    <TableCell className="text-xs text-[var(--text-muted)] max-w-[200px]">
                      <span className="block truncate">{conditionSummary(policy.conditions)}</span>
                    </TableCell>
                    <TableCell className="text-xs">
                      <div className="flex items-center gap-1">
                        {policy.approval_chain.map((node, i) => (
                          <span key={i} className="flex items-center gap-0.5">
                            {i > 0 && <span className="text-[var(--text-muted)]">→</span>}
                            <Badge variant="outline" className={cn("text-[9px] px-1 py-0 border-0", node.role === "dba" ? "bg-blue-500/15 text-blue-400" : "bg-purple-500/15 text-purple-400")}>
                              {node.role === "dba" ? "DBA" : "Admin"}
                            </Badge>
                          </span>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell onClick={(e) => e.stopPropagation()}>
                      <div className="flex items-center gap-1.5">
                        <Switch
                          checked={policy.enabled}
                          onCheckedChange={() => {
                            if (policy.enabled) setDisableTarget(policy);
                            else store.togglePolicy(policy.id);
                          }}
                          className="scale-75"
                        />
                        <span className={cn("text-[10px]", policy.enabled ? "text-emerald-400" : "text-[var(--text-muted)]")}>
                          {policy.enabled ? "启用" : "禁用"}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell onClick={(e) => e.stopPropagation()}>
                      <div className="flex items-center gap-1">
                        <Button variant="ghost" size="sm" className="h-6 w-6 p-0 text-[var(--text-muted)] hover:text-[var(--text-primary)]" onClick={() => { store.setEditingPolicy(policy); setEditorOpen(true); }}>
                          <Settings size={12} />
                        </Button>
                        <Button variant="ghost" size="sm" className="h-6 w-6 p-0 text-[var(--text-muted)] hover:text-red-400" onClick={() => { setDeleteTarget(policy); setDeleteConfirmName(""); }}>
                          <Trash2 size={12} />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      <PolicyEditorSheet open={editorOpen} onOpenChange={setEditorOpen} editingPolicy={store.editingPolicy} />

      <AlertDialog open={disableTarget !== null} onOpenChange={(open) => { if (!open) setDisableTarget(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>禁用策略</AlertDialogTitle>
            <AlertDialogDescription>确认禁用策略「{disableTarget?.name}」？禁用后该策略将不会匹配任何工单。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={async () => { if (disableTarget) { await store.togglePolicy(disableTarget.id); setDisableTarget(null); } }}>确认禁用</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) { setDeleteTarget(null); setDeleteConfirmName(""); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除策略</AlertDialogTitle>
            <AlertDialogDescription>确认删除策略「{deleteTarget?.name}」？此操作不可恢复。</AlertDialogDescription>
          </AlertDialogHeader>
          {store.policies.length > 5 && (
            <div>
              <label className="mb-1 block text-xs text-[var(--text-secondary)]">
                请输入策略名称「<span className="font-medium">{deleteTarget?.name}</span>」以确认
              </label>
              <Input value={deleteConfirmName} onChange={(e) => setDeleteConfirmName(e.target.value)} className="h-8 border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm" placeholder={deleteTarget?.name} />
            </div>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete} variant="destructive" disabled={store.policies.length > 5 && deleteConfirmName !== deleteTarget?.name}>
              {store.saving ? <Loader2 size={14} className="animate-spin" /> : null}
              确认删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
