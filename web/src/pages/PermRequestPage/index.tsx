import { useState, useEffect, useCallback } from "react";
import {
  ShieldAlert,
  Plus,
  Check,
  X,
  Ban,
  Clock,
  Loader2,
  RefreshCw,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
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
import {
  createPermReq,
  listPermReqs,
  myPermReqs,
  approvePermReq,
  rejectPermReq,
  revokePermReq,
  STATUS_MAP,
  formatActions,
  formatDateTime,
  timeLeft,
  type PermissionRequest,
} from "@/api/permission-request";
import { api } from "@/api/client";

// --- Status Badge ---

function StatusBadge({ status }: { status: string }) {
  const info = STATUS_MAP[status] || { label: status, color: "text-gray-400", bg: "bg-gray-500/10" };
  return (
    <Badge variant="outline" className={`${info.bg} ${info.color} border-0`}>
      {info.label}
    </Badge>
  );
}

// --- Create Request Dialog ---

function CreatePermReqDialog({ onCreated }: { onCreated: () => void }) {
  const [open, setOpen] = useState(false);
  const [database, setDatabase] = useState("");
  const [tableName, setTableName] = useState("");
  const [actions, setActions] = useState("select");
  const [reason, setReason] = useState("");
  const [durationH, setDurationH] = useState("2");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [datasources, setDatasources] = useState<{ id: number; name: string }[]>([]);
  const [dsId, setDsId] = useState("0");

  useEffect(() => {
    if (open) {
      api.get<{ code: number; data: { items: { id: number; name: string }[] } }>("/datasources")
        .then((res) => setDatasources(res.data.items || []))
        .catch(() => {});
    }
  }, [open]);

  const handleSubmit = async () => {
    setError("");
    if (!dsId || dsId === "0") { setError("请选择数据源"); return; }
    if (!database.trim()) { setError("请输入数据库名"); return; }
    if (!actions) { setError("请选择操作类型"); return; }

    setLoading(true);
    try {
      await createPermReq({
        datasource_id: parseInt(dsId),
        database: database.trim(),
        table_name: tableName.trim(),
        actions,
        reason: reason.trim(),
        duration_hours: parseInt(durationH) || 2,
      });
      setOpen(false);
      onCreated();
      setDatabase(""); setTableName(""); setActions("select"); setReason(""); setDurationH("2"); setDsId("0");
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "创建失败");
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button><Plus size={14} className="mr-1.5" />申请临时权限</Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader><DialogTitle>申请临时权限</DialogTitle></DialogHeader>
        <div className="space-y-4 pt-2">
          {error && <div className="rounded-lg bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>}
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-[var(--text-primary)]">数据源 *</label>
            <Select value={dsId} onValueChange={setDsId}>
              <SelectTrigger><SelectValue placeholder="选择数据源" /></SelectTrigger>
              <SelectContent>
                {datasources.map((ds) => (
                  <SelectItem key={ds.id} value={String(ds.id)}>{ds.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-[var(--text-primary)]">数据库 *</label>
              <Input placeholder="数据库名" value={database} onChange={(e) => setDatabase(e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-[var(--text-primary)]">表名（可选）</label>
              <Input placeholder="表名" value={tableName} onChange={(e) => setTableName(e.target.value)} />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-[var(--text-primary)]">操作类型 *</label>
              <Select value={actions} onValueChange={setActions}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="select">查询 (SELECT)</SelectItem>
                  <SelectItem value="select,update">查询 + 更新</SelectItem>
                  <SelectItem value="select,update,delete">查询 + 更新 + 删除</SelectItem>
                  <SelectItem value="select,ddl">查询 + DDL</SelectItem>
                  <SelectItem value="export">导出</SelectItem>
                  <SelectItem value="select,update,delete,ddl,export">全部权限</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-[var(--text-primary)]">有效期</label>
              <Select value={durationH} onValueChange={setDurationH}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="1">1 小时</SelectItem>
                  <SelectItem value="2">2 小时</SelectItem>
                  <SelectItem value="4">4 小时</SelectItem>
                  <SelectItem value="8">8 小时</SelectItem>
                  <SelectItem value="24">1 天</SelectItem>
                  <SelectItem value="48">2 天</SelectItem>
                  <SelectItem value="72">3 天（最大）</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-[var(--text-primary)]">申请理由</label>
            <Input placeholder="说明为什么需要临时访问权限" value={reason} onChange={(e) => setReason(e.target.value)} />
          </div>
          <div className="flex justify-end gap-3 pt-2">
            <Button variant="ghost" onClick={() => setOpen(false)}>取消</Button>
            <Button onClick={handleSubmit} disabled={loading}>
              {loading && <Loader2 size={14} className="mr-1.5 animate-spin" />}
              提交申请
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// --- Request Table ---

function RequestTable({
  requests,
  loading,
  isAdmin,
  onAction,
}: {
  requests: PermissionRequest[];
  loading: boolean;
  isAdmin: boolean;
  onAction?: () => void;
}) {
  if (loading) {
    return <div className="flex h-40 items-center justify-center"><Loader2 size={20} className="animate-spin text-[var(--text-muted)]" /></div>;
  }
  if (requests.length === 0) {
    return (
      <div className="flex h-40 flex-col items-center justify-center gap-2 text-[var(--text-muted)]">
        <ShieldAlert size={32} strokeWidth={1} />
        <span className="text-sm">暂无权限申请记录</span>
      </div>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow className="border-b border-[var(--border-default)]">
          <TableHead className="text-[var(--text-secondary)]">申请人</TableHead>
          <TableHead className="text-[var(--text-secondary)]">数据库/表</TableHead>
          <TableHead className="text-[var(--text-secondary)]">操作</TableHead>
          <TableHead className="text-[var(--text-secondary)]">状态</TableHead>
          <TableHead className="text-[var(--text-secondary)]">有效期</TableHead>
          <TableHead className="text-[var(--text-secondary)]">申请时间</TableHead>
          {isAdmin && <TableHead className="text-right text-[var(--text-secondary)]">操作</TableHead>}
        </TableRow>
      </TableHeader>
      <TableBody>
        {requests.map((r) => (
          <TableRow key={r.id}>
            <TableCell className="text-[var(--text-primary)]">{r.applicant_name || `#${r.applicant_id}`}</TableCell>
            <TableCell>
              <div className="text-[var(--text-primary)]">{r.database}</div>
              {r.table_name && <div className="text-xs text-[var(--text-muted)]">{r.table_name}</div>}
            </TableCell>
            <TableCell>
              <span className="text-xs text-[var(--text-secondary)]">{formatActions(r.actions)}</span>
            </TableCell>
            <TableCell><StatusBadge status={r.status} /></TableCell>
            <TableCell>
              {r.status === "APPROVED" ? (
                <span className="flex items-center gap-1 text-xs text-emerald-400"><Clock size={12} />{timeLeft(r.expires_at)}</span>
              ) : r.status === "PENDING" ? (
                <span className="text-xs text-[var(--text-muted)]">{formatDateTime(r.expires_at)}</span>
              ) : (
                <span className="text-xs text-[var(--text-muted)]">{formatDateTime(r.expires_at)}</span>
              )}
            </TableCell>
            <TableCell className="text-xs text-[var(--text-muted)]">{formatDateTime(r.created_at)}</TableCell>
            {isAdmin && (
              <TableCell className="text-right">
                {r.status === "PENDING" && (
                  <div className="flex items-center justify-end gap-1">
                    <Button size="sm" variant="ghost" className="text-emerald-400 hover:bg-emerald-500/10"
                      onClick={async () => { await approvePermReq(r.id); onAction?.(); }}>
                      <Check size={14} />
                    </Button>
                    <Button size="sm" variant="ghost" className="text-red-400 hover:bg-red-500/10"
                      onClick={async () => { await rejectPermReq(r.id); onAction?.(); }}>
                      <X size={14} />
                    </Button>
                  </div>
                )}
                {r.status === "APPROVED" && (
                  <Button size="sm" variant="ghost" className="text-orange-400 hover:bg-orange-500/10"
                    onClick={async () => { await revokePermReq(r.id); onAction?.(); }}>
                    <Ban size={14} />
                  </Button>
                )}
              </TableCell>
            )}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

// --- Main Page ---

export default function PermReqPage() {
  const [isAdmin, setIsAdmin] = useState(false);
  const [activeTab, setActiveTab] = useState<"mine" | "all">("mine");
  const [requests, setRequests] = useState<PermissionRequest[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [filterStatus, setFilterStatus] = useState("");
  const [loading, setLoading] = useState(true);

  const fetchAdmin = useCallback(async (p = 1) => {
    setLoading(true);
    try {
      const res = await listPermReqs(p, 20, filterStatus);
      setRequests(res.items);
      setTotal(res.total);
    } catch { /* error */ }
    setLoading(false);
  }, [filterStatus]);

  const fetchMine = useCallback(async () => {
    setLoading(true);
    try {
      const res = await myPermReqs();
      setRequests(res.items);
      setTotal(res.total);
    } catch { /* error */ }
    setLoading(false);
  }, []);

  useEffect(() => {
    api.get<{ code: number; data: { role: string } }>("/auth/me")
      .then((res) => setIsAdmin(res.data?.role === "admin"))
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (activeTab === "all" && isAdmin) fetchAdmin(page);
    else fetchMine();
  }, [activeTab, isAdmin, fetchAdmin, fetchMine, page]);

  const totalPages = Math.ceil(total / 20);

  return (
    <div className="mx-auto max-w-[1200px] space-y-6 page-transition">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">临时权限管理</h1>
        <CreatePermReqDialog onCreated={() => { if (activeTab === "mine") fetchMine(); else fetchAdmin(page); }} />
      </div>

      {isAdmin && (
        <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-1">
          <div className="flex gap-1">
            <button onClick={() => setActiveTab("mine")} className={`flex-1 rounded-md px-3 py-2 text-sm font-medium transition-colors ${activeTab === "mine" ? "bg-[var(--bg-elevated)] text-[var(--text-primary)] shadow-sm" : "text-[var(--text-muted)] hover:text-[var(--text-secondary)]"}`}>
              我的申请
            </button>
            <button onClick={() => setActiveTab("all")} className={`flex-1 rounded-md px-3 py-2 text-sm font-medium transition-colors ${activeTab === "all" ? "bg-[var(--bg-elevated)] text-[var(--text-primary)] shadow-sm" : "text-[var(--text-muted)] hover:text-[var(--text-secondary)]"}`}>
              全部申请（管理）
            </button>
          </div>
        </div>
      )}

      {activeTab === "all" && isAdmin && (
        <div className="flex items-center gap-3">
          <Select value={filterStatus || "_all"} onValueChange={(v) => { setFilterStatus(v === "_all" ? "" : v); setPage(1); }}>
            <SelectTrigger className="w-32 border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm"><SelectValue placeholder="全部状态" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="_all">全部状态</SelectItem>
              <SelectItem value="PENDING">待审批</SelectItem>
              <SelectItem value="APPROVED">已批准</SelectItem>
              <SelectItem value="REJECTED">已拒绝</SelectItem>
              <SelectItem value="REVOKED">已撤销</SelectItem>
              <SelectItem value="EXPIRED">已过期</SelectItem>
            </SelectContent>
          </Select>
          <Button size="sm" variant="outline" onClick={() => fetchAdmin(page)}><RefreshCw size={14} /></Button>
          <span className="text-sm text-[var(--text-muted)]">共 {total} 条</span>
        </div>
      )}

      <Card>
        <CardContent className="p-4">
          <RequestTable
            requests={requests}
            loading={loading}
            isAdmin={activeTab === "all" && isAdmin}
            onAction={() => { if (activeTab === "mine") fetchMine(); else fetchAdmin(page); }}
          />
        </CardContent>
      </Card>

      {activeTab === "all" && isAdmin && totalPages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>上一页</Button>
          <span className="text-xs text-[var(--text-muted)]">{page} / {totalPages}</span>
          <Button size="sm" variant="outline" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>下一页</Button>
        </div>
      )}

      <Card>
        <CardContent className="space-y-3 p-4">
          <h3 className="text-sm font-medium text-[var(--text-primary)]">使用说明</h3>
          <div className="space-y-2 text-xs text-[var(--text-secondary)]">
            <p>临时权限用于在限定时间内访问敏感表，需要管理员审批后生效。</p>
            <ul className="ml-4 list-disc space-y-1">
              <li>提交申请后，管理员会审批并授予临时 Casbin 策略</li>
              <li>权限到期后自动过期，关联策略会自动移除</li>
              <li>管理员可以随时撤销已批准的临时权限</li>
              <li>有效期最长 72 小时</li>
            </ul>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
