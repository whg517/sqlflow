import { useState, useCallback, type FormEvent } from "react";
import { toast } from "sonner";
import {
  Webhook,
  Loader2,
  Send,
  Plus,
  Pencil,
  Trash2,
  Eye,
  EyeOff,
  AlertTriangle,
  Copy,
  CheckCircle2,
  ShieldCheck,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Checkbox } from "@/components/ui/checkbox";
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
  listSubscriptions,
  createSubscription,
  updateSubscription,
  deleteSubscription,
  toggleSubscription,
  testSubscription,
  validateWebhookURL,
  getFailureStatus,
  maskUrl,
  WEBHOOK_EVENTS,
  EVENT_CATEGORIES,
  type WebhookSubscription,
  type CreateSubscriptionResponse,
} from "@/api/webhookSubscription";

// ==========================================
// Types
// ==========================================

interface CurrentUser {
  id: number;
  username: string;
  role: string;
}

// ==========================================
// Failure Status Badge
// ==========================================

function FailureBadge({ count }: { count: number }) {
  const status = getFailureStatus(count);

  if (status.level === "normal") {
    return (
      <Badge className="border-0 bg-emerald-500/20 text-emerald-400 text-[10px]">
        正常
      </Badge>
    );
  }

  const colorMap = {
    warning: "bg-yellow-500/20 text-yellow-400",
    danger: "bg-orange-500/20 text-orange-400",
    critical: "bg-red-500/20 text-red-400",
  };

  return (
    <Badge className={cn("border-0 text-[10px]", colorMap[status.level])}>
      {status.label}
    </Badge>
  );
}

// ==========================================
// Event Tags
// ==========================================

function EventTags({ events }: { events: string[] }) {
  const eventMap = new Map(WEBHOOK_EVENTS.map((e) => [e.key, e.label]));

  return (
    <div className="flex flex-wrap gap-1">
      {events.map((evt) => {
        const label = eventMap.get(evt) ?? evt;
        const isSla = evt.startsWith("sla.");
        return (
          <Badge
            key={evt}
            className={cn(
              "text-[10px] border-0",
              isSla
                ? "bg-yellow-500/15 text-yellow-500"
                : "bg-blue-500/15 text-blue-400",
            )}
          >
            {label}
          </Badge>
        );
      })}
    </div>
  );
}

// ==========================================
// Integrations Tab
// ==========================================

interface IntegrationsTabProps {
  user: CurrentUser | null;
}

export default function IntegrationsTab({ user }: IntegrationsTabProps) {
  const isAdmin = user?.role === "admin";

  // Non-admin: show permission denied
  if (!isAdmin) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="text-center">
          <ShieldCheck size={32} className="mx-auto mb-2 text-[var(--text-muted)]" />
          <p className="text-sm text-[var(--text-muted)]">
            仅管理员可管理 Webhook 集成
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <Webhook size={18} className="text-[var(--accent-primary)]" />
          <h2 className="text-lg font-semibold text-[var(--text-primary)]">
            集成管理
          </h2>
        </div>
        <p className="text-sm text-[var(--text-secondary)]">
          配置出站 Webhook 订阅，将事件推送到外部系统（CI/CD、告警平台等）。
        </p>
      </div>

      {/* Subscriptions */}
      <SubscriptionList />
    </div>
  );
}

// ==========================================
// Subscription List
// ==========================================

function SubscriptionList() {
  const [subs, setSubs] = useState<WebhookSubscription[]>([]);
  const [staleIds, setStaleIds] = useState<Set<number>>(new Set());
  const [loading, setLoading] = useState(true);

  // Dialog states
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState({ name: "", url: "", events: [] as string[] });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [urlWarning, setUrlWarning] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Secret reveal (only after creation)
  const [createdSecret, setCreatedSecret] = useState<string | null>(null);
  const [secretCopied, setSecretCopied] = useState(false);

  // URL reveal
  const [revealedUrls, setRevealedUrls] = useState<Set<number>>(new Set());

  // Testing
  const [testingId, setTestingId] = useState<number | null>(null);

  // Delete
  const [deleteTarget, setDeleteTarget] = useState<WebhookSubscription | null>(null);
  const [deleting, setDeleting] = useState(false);

  const fetchSubs = useCallback(async () => {
    try {
      const res = await listSubscriptions();
      const items = res.data ?? [];
      setSubs(items);
      // Compute stale IDs outside render
      const threshold = Date.now() - 24 * 60 * 60 * 1000;
      const stale = new Set<number>();
      for (const sub of items) {
        if (sub.last_triggered_at && new Date(sub.last_triggered_at).getTime() < threshold && sub.enabled) {
          stale.add(sub.id);
        }
      }
      setStaleIds(stale);
    } catch {
      toast.error("获取订阅列表失败");
    } finally {
      setLoading(false);
    }
  }, []);

  if (loading && subs.length === 0) {
    void fetchSubs();
  }

  // --- Handlers ---

  function openAdd() {
    setEditingId(null);
    setForm({ name: "", url: "", events: [] });
    setErrors({});
    setUrlWarning(null);
    setCreatedSecret(null);
    setSecretCopied(false);
    setDialogOpen(true);
  }

  function openEdit(sub: WebhookSubscription) {
    setEditingId(sub.id);
    setForm({
      name: sub.name,
      url: "", // URL is masked, user re-enters to change
      events: sub.events ?? [],
    });
    setErrors({});
    setUrlWarning(null);
    setCreatedSecret(null);
    setDialogOpen(true);
  }

  function validate(): boolean {
    const errs: Record<string, string> = {};
    let warning: string | null = null;
    if (!form.name.trim()) errs.name = "请输入名称";
    else if (form.name.trim().length < 2 || form.name.trim().length > 50)
      errs.name = "名称需 2-50 个字符";

    if (!editingId) {
      const result = validateWebhookURL(form.url);
      if (result.error) errs.url = result.error;
      else warning = result.warning;
    } else if (form.url) {
      const result = validateWebhookURL(form.url);
      if (result.error) errs.url = result.error;
      else warning = result.warning;
    }

    if (!form.events || form.events.length === 0)
      errs.events = "请至少选择一个事件";

    setErrors(errs);
    setUrlWarning(warning);
    return Object.keys(errs).length === 0;
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!validate()) return;
    setSubmitting(true);

    try {
      if (editingId) {
        const updateData: Record<string, unknown> = {
          name: form.name.trim(),
          events: form.events,
        };
        if (form.url) {
          updateData.url = form.url.trim();
        }
        await updateSubscription(editingId, updateData);
        toast.success("订阅更新成功");
        setDialogOpen(false);
      } else {
        const res = await createSubscription({
          name: form.name.trim(),
          url: form.url.trim(),
          events: form.events,
        });
        const data = res.data as CreateSubscriptionResponse;
        if (data.secret) {
          setCreatedSecret(data.secret);
          setSecretCopied(false);
          // Don't close dialog — show secret first
          toast.success("订阅创建成功，请保存 Secret");
        } else {
          setDialogOpen(false);
          toast.success("订阅创建成功");
        }
      }
      fetchSubs();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleSecretCopy() {
    if (!createdSecret) return;
    try {
      await navigator.clipboard.writeText(createdSecret);
      setSecretCopied(true);
      toast.success("Secret 已复制到剪贴板");
    } catch {
      toast.error("复制失败，请手动选中复制");
    }
  }

  function handleSecretDone() {
    setCreatedSecret(null);
    setDialogOpen(false);
  }

  async function handleToggle(sub: WebhookSubscription) {
    try {
      await toggleSubscription(sub.id);
      toast.success(sub.enabled ? "已禁用" : "已启用");
      fetchSubs();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    }
  }

  async function handleTest(sub: WebhookSubscription) {
    setTestingId(sub.id);
    try {
      await testSubscription(sub.id);
      toast.success("测试发送成功");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "测试发送失败");
    } finally {
      setTestingId(null);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deleteSubscription(deleteTarget.id);
      toast.success("订阅已删除");
      setDeleteTarget(null);
      fetchSubs();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败");
    } finally {
      setDeleting(false);
    }
  }

  function toggleUrlReveal(id: number) {
    setRevealedUrls((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleEvent(eventKey: string) {
    setForm((f) => ({
      ...f,
      events: f.events.includes(eventKey)
        ? f.events.filter((e) => e !== eventKey)
        : [...f.events, eventKey],
    }));
  }

  function toggleCategory(category: string) {
    const categoryEvents = WEBHOOK_EVENTS.filter(
      (e) => e.category === category,
    ).map((e) => e.key);
    const allSelected = categoryEvents.every((e) => form.events.includes(e));

    setForm((f) => ({
      ...f,
      events: allSelected
        ? f.events.filter((e) => !categoryEvents.includes(e))
        : [...new Set([...f.events, ...categoryEvents])],
    }));
  }

  // --- Render ---

  if (loading && subs.length === 0) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-[var(--text-muted)]" />
      </div>
    );
  }

  // Group events by category for the form
  const eventsByCategory = WEBHOOK_EVENTS.reduce<
    Record<string, typeof WEBHOOK_EVENTS>
  >((acc, evt) => {
    if (!acc[evt.category]) acc[evt.category] = [];
    acc[evt.category].push(evt);
    return acc;
  }, {});

  return (
    <div className="space-y-5">
      {/* Action bar */}
      <div className="flex items-center justify-between">
        <span className="text-sm text-[var(--text-secondary)]">
          共 {subs.length} 个订阅
        </span>
        <Button
          onClick={openAdd}
          size="sm"
          className="gap-1 bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
        >
          <Plus size={14} />
          新增订阅
        </Button>
      </div>

      {/* Subscription table */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] table-responsive">
        <Table>
          <TableHeader>
            <TableRow className="border-[var(--border-default)] hover:bg-transparent">
              <TableHead className="text-[var(--text-secondary)]">
                名称
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                URL
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                订阅事件
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                状态
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                失败
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                启用
              </TableHead>
              <TableHead className="text-[var(--text-secondary)]">
                操作
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {subs.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="h-32">
                  <div className="flex flex-col items-center gap-2">
                    <Webhook size={20} className="text-[var(--text-muted)]" />
                    <span className="text-sm text-[var(--text-muted)]">
                      暂无 Webhook 订阅
                    </span>
                    <span className="text-xs text-[var(--text-muted)]">
                      点击「新增订阅」配置外部系统集成
                    </span>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              subs.map((sub) => {
                const isRevealed = revealedUrls.has(sub.id);
                const failureStatus = getFailureStatus(sub.failure_count);
                const isStale = staleIds.has(sub.id);

                return (
                  <TableRow
                    key={sub.id}
                    className="border-[var(--border-default)]"
                  >
                    <TableCell>
                      <span className="font-medium text-[var(--text-primary)]">
                        {sub.name}
                      </span>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1.5">
                        <span className="max-w-[200px] truncate font-mono text-xs text-[var(--text-secondary)]">
                          {isRevealed ? sub.url : maskUrl(sub.url)}
                        </span>
                        <button
                          type="button"
                          className="text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                          onClick={() => toggleUrlReveal(sub.id)}
                        >
                          {isRevealed ? (
                            <EyeOff size={12} />
                          ) : (
                            <Eye size={12} />
                          )}
                        </button>
                      </div>
                    </TableCell>
                    <TableCell>
                      <EventTags events={sub.events ?? []} />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1.5">
                        <span
                          className={cn(
                            "h-2 w-2 rounded-full",
                            !sub.enabled
                              ? "bg-gray-400"
                              : failureStatus.level === "normal"
                                ? "bg-emerald-400"
                                : failureStatus.level === "warning"
                                  ? "bg-yellow-400"
                                  : failureStatus.level === "danger"
                                    ? "bg-orange-400"
                                    : "bg-red-400",
                          )}
                        />
                        {isStale && sub.enabled && (
                          <span className="text-[10px] text-[var(--text-muted)]">
                            超过24h未触发
                          </span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <FailureBadge count={sub.failure_count} />
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={sub.enabled}
                        onCheckedChange={() => handleToggle(sub)}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                          onClick={() => openEdit(sub)}
                        >
                          <Pencil size={13} />
                          编辑
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                          disabled={testingId === sub.id || !sub.enabled}
                          onClick={() => handleTest(sub)}
                        >
                          {testingId === sub.id ? (
                            <Loader2 size={13} className="animate-spin" />
                          ) : (
                            <Send size={13} />
                          )}
                          测试
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-red-400"
                          onClick={() => setDeleteTarget(sub)}
                        >
                          <Trash2 size={13} />
                          删除
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </div>

      {/* Empty state guidance */}
      {!loading && subs.length === 0 && (
        <div className="rounded-lg border border-dashed border-[var(--border-default)] bg-[var(--bg-elevated)]/50 p-6">
          <div className="flex flex-col items-center gap-3 text-center">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--accent-primary)]/10">
              <AlertTriangle size={18} className="text-[var(--accent-primary)]" />
            </div>
            <div>
              <p className="text-sm font-medium text-[var(--text-primary)]">
                配置 Webhook 集成
              </p>
              <p className="mt-1 text-xs text-[var(--text-muted)]">
                将 SQLFlow 事件推送到外部系统：CI/CD、告警聚合、运维平台等
              </p>
              <p className="mt-0.5 text-xs text-[var(--text-muted)]">
                支持 HMAC-SHA256 签名验证，确保消息安全
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              className="gap-1 border-[var(--accent-primary)]/30 text-[var(--accent-primary)] hover:bg-[var(--accent-primary)]/10"
              onClick={openAdd}
            >
              <Plus size={14} />
              创建第一个订阅
            </Button>
          </div>
        </div>
      )}

      {/* Add / Edit Dialog */}
      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          if (!open && !createdSecret) setDialogOpen(false);
        }}
      >
        <DialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-[var(--text-primary)]">
              {createdSecret
                ? "🔐 请保存 Secret"
                : editingId
                  ? "编辑 Webhook 订阅"
                  : "新增 Webhook 订阅"}
            </DialogTitle>
          </DialogHeader>

          {createdSecret ? (
            /* Secret display (one-time) */
            <div className="space-y-4">
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-4">
                <div className="flex items-start gap-2">
                  <AlertTriangle
                    size={16}
                    className="mt-0.5 shrink-0 text-amber-400"
                  />
                  <div className="space-y-2">
                    <p className="text-sm font-medium text-amber-400">
                      Secret 仅展示一次，关闭后无法再次查看
                    </p>
                    <div className="flex items-center gap-2">
                      <code className="flex-1 rounded bg-[var(--bg-elevated)] px-3 py-2 font-mono text-xs text-[var(--text-primary)] break-all">
                        {createdSecret}
                      </code>
                      <Button
                        variant="outline"
                        size="sm"
                        className="shrink-0 gap-1"
                        onClick={handleSecretCopy}
                      >
                        {secretCopied ? (
                          <CheckCircle2 size={14} className="text-emerald-400" />
                        ) : (
                          <Copy size={14} />
                        )}
                        {secretCopied ? "已复制" : "复制"}
                      </Button>
                    </div>
                    <p className="text-xs text-[var(--text-muted)]">
                      用于验证 Webhook 消息的 HMAC-SHA256 签名，请妥善保管。
                    </p>
                  </div>
                </div>
              </div>
              <div className="flex justify-end">
                <Button
                  onClick={handleSecretDone}
                  className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
                >
                  我已保存
                </Button>
              </div>
            </div>
          ) : (
            /* Create/Edit form */
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">名称</Label>
                <Input
                  value={form.name}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, name: e.target.value }))
                  }
                  placeholder="例如：Jenkins CI、告警平台"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.name && (
                  <p className="text-xs text-red-400">{errors.name}</p>
                )}
              </div>

              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">
                  Webhook URL{" "}
                  {editingId && (
                    <span className="font-normal text-[var(--text-muted)]">
                      (留空保持不变)
                    </span>
                  )}
                </Label>
                <Input
                  value={form.url}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, url: e.target.value }))
                  }
                  placeholder="https://example.com/webhook"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.url && (
                  <p className="text-xs text-red-400">{errors.url}</p>
                )}
                {!errors.url && urlWarning && (
                  <p className="text-xs text-amber-400">
                    ⚠️ {urlWarning}
                  </p>
                )}
              </div>

              <div className="space-y-2">
                <Label className="text-[var(--text-secondary)]">
                  订阅事件
                </Label>
                {errors.events && (
                  <p className="text-xs text-red-400">{errors.events}</p>
                )}
                {Object.entries(eventsByCategory).map(
                  ([category, events]) => {
                    const catInfo =
                      EVENT_CATEGORIES[category as keyof typeof EVENT_CATEGORIES];
                    const allSelected = events.every((e) =>
                      form.events.includes(e.key),
                    );

                    return (
                      <div
                        key={category}
                        className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-elevated)]/50 p-3"
                      >
                        <div className="mb-2 flex items-center gap-2">
                          <Checkbox
                            checked={allSelected}
                            onCheckedChange={() => toggleCategory(category)}
                          />
                          <span className="text-xs font-medium text-[var(--text-secondary)]">
                            {catInfo?.label ?? category}
                          </span>
                        </div>
                        <div className="flex flex-wrap gap-3 pl-6">
                          {events.map((evt) => (
                            <div
                              key={evt.key}
                              className="flex items-center gap-1.5"
                            >
                              <Checkbox
                                checked={form.events.includes(evt.key)}
                                onCheckedChange={() => toggleEvent(evt.key)}
                              />
                              <span className="text-xs text-[var(--text-primary)]">
                                {evt.label}
                              </span>
                            </div>
                          ))}
                        </div>
                      </div>
                    );
                  },
                )}
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
                  {submitting
                    ? "保存中..."
                    : editingId
                      ? "更新"
                      : "创建"}
                </Button>
              </DialogFooter>
            </form>
          )}
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
              确认删除 Webhook 订阅
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要删除订阅「{deleteTarget?.name}」吗？删除后对应的外部系统将不再收到事件通知。
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
