import { useState, useCallback, useEffect, type FormEvent } from "react";
import { toast } from "sonner";
import {
  Plus,
  Pencil,
  Trash2,
  Clock,
  Loader2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  listSLAConfigs,
  createSLAConfig,
  updateSLAConfig,
  deleteSLAConfig,
  type SLAConfig,
} from "@/api/sla";

// --- Constants ---

const PRIORITY_OPTIONS = [
  { value: "low", label: "低风险 (Low)" },
  { value: "medium", label: "中风险 (Medium)" },
  { value: "high", label: "高风险 (High)" },
];

const ROLE_OPTIONS = [
  { value: "admin", label: "管理员" },
  { value: "dba", label: "DBA" },
];

// --- Component ---

export default function SLATab() {
  const [configs, setConfigs] = useState<SLAConfig[]>([]);
  const [loading, setLoading] = useState(false);

  // Dialog
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState({
    priority: "medium",
    timeout_minutes: "120",
    reminder_percent: "80",
    escalate_to_role: "admin",
    escalate_to_user: "",
    enabled: true,
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);

  // Delete
  const [deleteTarget, setDeleteTarget] = useState<SLAConfig | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Notifications list
  const [notifications, setNotifications] = useState<
    { id: number; ticket_id: number; notification_type: string; notified_user: string; notified_at: string }[]
  >([]);
  const [notifTotal, setNotifTotal] = useState(0);
  const [showNotifs, setShowNotifs] = useState(false);
  const [notifLoading, setNotifLoading] = useState(false);

  const fetchConfigs = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listSLAConfigs();
      setConfigs(res.data ?? []);
    } catch {
      toast.error("获取 SLA 配置失败");
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchNotifications = useCallback(async () => {
    setNotifLoading(true);
    try {
      const { listSLANotifications } = await import("@/api/sla");
      const res = await listSLANotifications(1, 10);
      setNotifications(res.data ?? []);
      setNotifTotal(res.total ?? 0);
    } catch {
      // ignore
    } finally {
      setNotifLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchConfigs();
  }, [fetchConfigs]);

  useEffect(() => {
    if (showNotifs) fetchNotifications();
  }, [showNotifs, fetchNotifications]);

  // --- Handlers ---

  function openAdd() {
    setEditingId(null);
    setForm({
      priority: "medium",
      timeout_minutes: "120",
      reminder_percent: "80",
      escalate_to_role: "admin",
      escalate_to_user: "",
      enabled: true,
    });
    setErrors({});
    setDialogOpen(true);
  }

  function openEdit(cfg: SLAConfig) {
    setEditingId(cfg.id);
    setForm({
      priority: cfg.priority,
      timeout_minutes: String(cfg.timeout_minutes),
      reminder_percent: String(cfg.reminder_percent),
      escalate_to_role: cfg.escalate_to_role,
      escalate_to_user: cfg.escalate_to_user,
      enabled: cfg.enabled,
    });
    setErrors({});
    setDialogOpen(true);
  }

  function validate(): boolean {
    const errs: Record<string, string> = {};
    const timeout = Number(form.timeout_minutes);
    if (timeout <= 0) errs.timeout_minutes = "超时时间必须大于0";
    if (timeout > 43200) errs.timeout_minutes = "超时时间不能超过30天";
    const pct = Number(form.reminder_percent);
    if (pct < 1 || pct > 99) errs.reminder_percent = "提醒比例范围 1-99";
    setErrors(errs);
    return Object.keys(errs).length === 0;
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!validate()) return;
    setSubmitting(true);
    const body = {
      priority: form.priority,
      timeout_minutes: Number(form.timeout_minutes),
      reminder_percent: Number(form.reminder_percent),
      escalate_to_role: form.escalate_to_role,
      escalate_to_user: form.escalate_to_user,
      enabled: form.enabled,
    };
    try {
      if (editingId) {
        await updateSLAConfig(editingId, body);
        toast.success("SLA 配置更新成功");
      } else {
        await createSLAConfig(body);
        toast.success("SLA 配置创建成功");
      }
      setDialogOpen(false);
      fetchConfigs();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deleteSLAConfig(deleteTarget.id);
      toast.success("SLA 配置已删除");
      setDeleteTarget(null);
      fetchConfigs();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    } finally {
      setDeleting(false);
    }
  }

  const priorityLabel = (p: string) =>
    PRIORITY_OPTIONS.find((o) => o.value === p)?.label ?? p;

  // --- Render ---

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-[var(--text-primary)]">
          SLA 配置
        </h2>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="h-8 gap-1.5 border-[var(--border-default)] px-3 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)]"
            onClick={() => setShowNotifs(!showNotifs)}
          >
            <Clock size={14} />
            通知记录 {notifTotal > 0 && `(${notifTotal})`}
          </Button>
          <Button
            onClick={openAdd}
            size="sm"
            className="h-8 gap-1 bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
          >
            <Plus size={14} />
            添加规则
          </Button>
        </div>
      </div>

      {/* Notifications panel */}
      {showNotifs && (
        <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)]">
          <div className="border-b border-[var(--border-default)] px-4 py-2">
            <span className="text-sm font-medium text-[var(--text-primary)]">
              最近通知记录
            </span>
          </div>
          {notifLoading ? (
            <div className="flex h-16 items-center justify-center text-xs text-[var(--text-muted)]">
              <Loader2 size={14} className="mr-2 animate-spin" />
              加载中...
            </div>
          ) : notifications.length === 0 ? (
            <div className="flex h-16 items-center justify-center text-xs text-[var(--text-muted)]">
              暂无通知记录
            </div>
          ) : (
            <div className="divide-y divide-[var(--border-subtle)]">
              {notifications.map((n) => (
                <div
                  key={n.id}
                  className="flex items-center gap-4 px-4 py-2 text-xs"
                >
                  <Badge
                    className={
                      n.notification_type === "escalate"
                        ? "bg-red-500/20 text-red-400 border-0 text-[10px]"
                        : "bg-yellow-500/20 text-yellow-400 border-0 text-[10px]"
                    }
                  >
                    {n.notification_type === "escalate" ? "升级" : "提醒"}
                  </Badge>
                  <span className="text-[var(--text-primary)]">
                    工单 #{n.ticket_id}
                  </span>
                  <span className="text-[var(--text-muted)]">
                    通知: {n.notified_user}
                  </span>
                  <span className="ml-auto text-[var(--text-muted)]">
                    {new Date(n.notified_at).toLocaleString("zh-CN")}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Table */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] table-responsive">
        <Table>
          <TableHeader>
            <TableRow className="border-[var(--border-default)] hover:bg-transparent">
              <TableHead className="text-[var(--text-secondary)]">
                优先级
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                审批时限
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                提醒比例
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                升级角色
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                状态
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                操作
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && !configs.length ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="h-24 text-center text-[var(--text-muted)]"
                >
                  加载中...
                </TableCell>
              </TableRow>
            ) : !configs.length ? (
              <TableRow>
                <TableCell colSpan={6} className="h-32">
                  <div className="flex flex-col items-center gap-2">
                    <Clock
                      size={20}
                      className="text-[var(--text-muted)]"
                    />
                    <span className="text-sm text-[var(--text-muted)]">
                      暂无 SLA 规则
                    </span>
                    <span className="text-xs text-[var(--text-muted)]">
                      点击「添加规则」配置审批超时告警
                    </span>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              configs.map((cfg) => (
                <TableRow
                  key={cfg.id}
                  className="border-[var(--border-default)]"
                >
                  <TableCell>
                    <span className="font-medium text-[var(--text-primary)]">
                      {priorityLabel(cfg.priority)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-[var(--text-secondary)]">
                      {cfg.timeout_minutes >= 60
                        ? `${cfg.timeout_minutes / 60}h`
                        : `${cfg.timeout_minutes}m`}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-[var(--text-secondary)]">
                      {cfg.reminder_percent}%
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-[var(--text-secondary)]">
                      {cfg.escalate_to_role === "dba" ? "DBA" : "管理员"}
                    </span>
                  </TableCell>
                  <TableCell>
                    <Badge
                      className={
                        cfg.enabled
                          ? "bg-emerald-500/20 text-emerald-400 border-0"
                          : "bg-muted text-[var(--text-muted)] border-0"
                      }
                    >
                      {cfg.enabled ? "启用" : "禁用"}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                        onClick={() => openEdit(cfg)}
                      >
                        <Pencil size={13} />
                        编辑
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-red-400"
                        onClick={() => setDeleteTarget(cfg)}
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

      {/* Add / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-[var(--text-primary)]">
              {editingId ? "编辑 SLA 规则" : "添加 SLA 规则"}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">优先级</Label>
                <Select
                  value={form.priority}
                  onValueChange={(v) =>
                    setForm((f) => ({ ...f, priority: v }))
                  }
                >
                  <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PRIORITY_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">
                  审批时限（分钟）
                </Label>
                <Input
                  type="number"
                  value={form.timeout_minutes}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      timeout_minutes: e.target.value,
                    }))
                  }
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.timeout_minutes && (
                  <p className="text-xs text-red-400">
                    {errors.timeout_minutes}
                  </p>
                )}
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">
                  提醒比例（%）
                </Label>
                <Input
                  type="number"
                  value={form.reminder_percent}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      reminder_percent: e.target.value,
                    }))
                  }
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.reminder_percent && (
                  <p className="text-xs text-red-400">
                    {errors.reminder_percent}
                  </p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">
                  升级角色
                </Label>
                <Select
                  value={form.escalate_to_role}
                  onValueChange={(v) =>
                    setForm((f) => ({ ...f, escalate_to_role: v }))
                  }
                >
                  <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {ROLE_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="flex items-center gap-3">
              <label className="flex items-center gap-2 text-sm text-[var(--text-secondary)] cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, enabled: e.target.checked }))
                  }
                  className="h-4 w-4 rounded border-[var(--border-default)]"
                />
                启用此规则
              </label>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                className="border-[var(--border-default)]"
                onClick={() => setDialogOpen(false)}
              >
                取消
              </Button>
              <Button
                type="submit"
                disabled={submitting}
                className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
              >
                {submitting ? "保存中..." : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm */}
      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              确认删除 SLA 规则
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要删除「{deleteTarget?.priority}
              」优先级的 SLA 规则吗？删除后对应优先级的工单将不再有超时告警。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              disabled={deleting}
              className="bg-[var(--danger)] text-white hover:bg-[var(--danger)]/80"
            >
              {deleting ? "删除中..." : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
