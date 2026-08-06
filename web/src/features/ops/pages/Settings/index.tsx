import { useState, useEffect, useCallback, type FormEvent } from "react";
import { useLocation } from "react-router-dom";
import { toast } from "sonner";
import {
  Plus,
  Pencil,
  Trash2,
  Database,
  Plug,
  Loader2,
  ShieldAlert,
  CheckCircle2,
  XCircle,
  Copy,
  Power,
  PowerOff,
} from "lucide-react";
import { api } from "@/shared/api/client";
import { Button } from "@/shared/components/ui/button";
import { Input } from "@/shared/components/ui/input";
import { Label } from "@/shared/components/ui/label";
import { Badge } from "@/shared/components/ui/badge";
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@/shared/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/shared/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/shared/components/ui/alert-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/shared/components/ui/select";
import MaskRulesTab from "./MaskRulesTab";
import AIConfigTab from "./AIConfigTab";
import SLATab from "./SLATab";
import ApprovalPoliciesTab from "./ApprovalPoliciesTab";
import IntegrationsTab from "./IntegrationsTab";
import { listSensitiveTables } from "@/features/security/api/maskRule";

// --- Types ---

interface DataSourceItem {
  id: number;
  name: string;
  type: string;
  host: string;
  port: number;
  username: string;
  database: string;
  sslmode?: string;
  schema_name?: string;
  max_open: number;
  status: string;
  system?: boolean;
  created_at: string;
  // Driver-specific settings arrive as one object. They used to be named
  // fields — es_urls, es_auth_type, es_index_pattern, es_verify_certs — which
  // meant this type, the request structs, the model and five database columns
  // all had to learn a setting before any driver could use it.
  extra_config?: Record<string, unknown>;
}

// esConfig reads Elasticsearch's keys out of the generic object.
function esConfig(ds: DataSourceItem) {
  const extra = ds.extra_config ?? {};
  const urls = extra.urls;
  return {
    urls: Array.isArray(urls) ? urls.join(", ") : typeof urls === "string" ? urls : "",
    authType: typeof extra.auth_type === "string" ? extra.auth_type : "basic",
    indexPattern:
      typeof extra.index_pattern === "string" ? extra.index_pattern : "",
    verifyCerts: extra.verify_certs !== false,
  };
}

interface DataSourceListResponse {
  code: number;
  data: DataSourceItem[];
}

interface TestConnectionResponse {
  code: number;
  data: { message: string; success: boolean };
}

interface ConnectionTestState {
  status: "idle" | "testing" | "success" | "error";
  message: string;
  fingerprint: string;
}

interface ApiResponse {
  code: number;
  message: string;
}

type SettingsTab = "datasource" | "mask-rules" | "ai-config" | "sla" | "approval-policies" | "integrations";

// --- Constants ---

const SETTINGS_TABS: SettingsTab[] = [
  "datasource",
  "approval-policies",
  "mask-rules",
  "sla",
  "integrations",
  "ai-config",
];

const TYPE_BADGE: Record<string, { label: string; cls: string }> = {
  mysql: { label: "MySQL", cls: "bg-blue-500/20 text-blue-400" },
  postgresql: { label: "PostgreSQL", cls: "bg-cyan-500/20 text-cyan-400" },
  sqlite: { label: "SQLite", cls: "bg-violet-500/20 text-violet-400" },
  mongodb: { label: "MongoDB", cls: "bg-green-500/20 text-green-400" },
  elasticsearch: { label: "Elasticsearch", cls: "bg-orange-500/20 text-orange-400" },
};

const STATUS_BADGE: Record<string, { label: string; cls: string }> = {
  active: { label: "正常", cls: "bg-emerald-500/20 text-emerald-400" },
  disabled: { label: "已禁用", cls: "bg-muted text-[var(--text-muted)]" },
  error: { label: "异常", cls: "bg-red-500/20 text-red-400" },
};

const DEFAULT_BADGE = {
  label: "未知",
  cls: "bg-muted text-[var(--text-muted)]",
};

// --- Validators ---

function validateName(v: string): string | null {
  if (!v.trim()) return "请输入名称";
  if (v.trim().length < 2 || v.trim().length > 50) return "名称需 2-50 个字符";
  return null;
}

function validateHost(v: string): string | null {
  if (!v.trim()) return "请输入主机地址";
  return null;
}

function validatePort(v: string): string | null {
  const n = Number(v);
  if (!v || isNaN(n)) return "请输入端口号";
  if (n < 1 || n > 65535 || !Number.isInteger(n)) return "端口范围 1-65535";
  return null;
}

function datasourceAddress(ds: DataSourceItem): string {
  if (ds.type === "sqlite") return ds.database || "—";
  if (ds.type === "elasticsearch") return esConfig(ds).urls || "—";
  return `${ds.host}:${ds.port}`;
}

function datasourceDatabase(ds: DataSourceItem): string {
  if (ds.type === "sqlite") {
    const parts = ds.database.split(/[\\/]/).filter(Boolean);
    return parts.at(-1) || ds.database || "—";
  }
  if (ds.type === "elasticsearch") {
    return esConfig(ds).indexPattern || "全部索引";
  }
  return ds.database || "—";
}

async function copyDatasourceAddress(ds: DataSourceItem): Promise<void> {
  const address = datasourceAddress(ds);
  if (!address || address === "—") {
    toast.error("当前数据源没有可复制的地址");
    return;
  }

  try {
    await navigator.clipboard.writeText(address);
    toast.success("地址已复制");
  } catch {
    toast.error("复制失败，请手动复制");
  }
}

// --- DataSource Tab ---

function DataSourceTab() {
  const [sources, setSources] = useState<DataSourceItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [inited, setInited] = useState(false);

  const [sensitiveCounts, setSensitiveCounts] = useState<Map<number, number>>(
    new Map(),
  );

  // Dialog
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState({
    name: "",
    type: "mysql",
    host: "",
    port: "3306",
    username: "",
    password: "",
    database: "",
    sslmode: "",
    schema_name: "",
    max_open: "10",
    // ES fields
    es_urls: "",
    es_auth_type: "basic",
    es_api_key: "",
    es_index_pattern: "",
    es_verify_certs: true,
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const [connectionTest, setConnectionTest] =
    useState<ConnectionTestState>({
      status: "idle",
      message: "",
      fingerprint: "",
    });

  // Test
  const [testingId, setTestingId] = useState<number | null>(null);

  // Lifecycle
  const [disableTarget, setDisableTarget] = useState<DataSourceItem | null>(null);
  const [hardDeleteTarget, setHardDeleteTarget] =
    useState<DataSourceItem | null>(null);
  const [statusUpdatingId, setStatusUpdatingId] = useState<number | null>(null);
  const [disabling, setDisabling] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const fetchSensitiveCounts = useCallback(async (dsList: DataSourceItem[]) => {
    const counts = new Map<number, number>();
    await Promise.allSettled(
      dsList.map(async (ds) => {
        try {
          const res = await listSensitiveTables({
            datasource_id: String(ds.id),
            page_size: 1,
          });
          counts.set(ds.id, res.total ?? 0);
        } catch {
          counts.set(ds.id, 0);
        }
      }),
    );
    setSensitiveCounts(counts);
  }, []);

  const fetchSources = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.get<DataSourceListResponse>("/datasources");
      const list = res.data ?? [];
      setSources(list);
      setInited(true);
      fetchSensitiveCounts(list);
    } catch {
      toast.error("获取数据源列表失败");
    } finally {
      setLoading(false);
    }
  }, [fetchSensitiveCounts]);

  if (!inited && !loading) fetchSources();

  // --- Handlers ---

  function openAdd() {
    setEditingId(null);
    setForm({
      name: "",
      type: "mysql",
      host: "",
      port: "3306",
      username: "",
      password: "",
      database: "",
      sslmode: "",
      schema_name: "",
      max_open: "10",
      es_urls: "",
      es_auth_type: "basic",
      es_api_key: "",
      es_index_pattern: "",
      es_verify_certs: true,
    });
    setErrors({});
    setConnectionTest({ status: "idle", message: "", fingerprint: "" });
    setDialogOpen(true);
  }

  function openEdit(ds: DataSourceItem) {
    setEditingId(ds.id);
    setForm({
      name: ds.name,
      type: ds.type,
      host: ds.host,
      port: String(ds.port),
      username: ds.username,
      password: "",
      database: ds.database || "",
      sslmode: ds.sslmode || "prefer",
      schema_name: ds.schema_name || "public",
      max_open: String(ds.max_open || 10),
      es_urls: esConfig(ds).urls,
      es_auth_type: esConfig(ds).authType,
      es_api_key: "",
      es_index_pattern: esConfig(ds).indexPattern,
      es_verify_certs: esConfig(ds).verifyCerts,
    });
    setErrors({});
    setConnectionTest({ status: "idle", message: "", fingerprint: "" });
    setDialogOpen(true);
  }

  function handleTypeChange(type: string) {
    const defaultPorts: Record<string, string> = {
      mysql: "3306",
      postgresql: "5432",
      mongodb: "27017",
      sqlite: "",
      elasticsearch: "",
    };
    setForm((current) => ({
      ...current,
      type,
      host: "",
      port: defaultPorts[type] ?? "",
      username: "",
      password: "",
      database: "",
      sslmode: type === "postgresql" ? "prefer" : "",
      schema_name: type === "postgresql" ? "public" : "",
      es_urls: "",
      es_auth_type: "basic",
      es_api_key: "",
      es_index_pattern: "",
      es_verify_certs: true,
    }));
    setErrors({});
    setConnectionTest({ status: "idle", message: "", fingerprint: "" });
  }

  function validate(includeName = true): boolean {
    const errs: Record<string, string> = {};
    if (includeName) {
      const n = validateName(form.name);
      if (n) errs.name = n;
    }
    if (form.type === "sqlite") {
      if (!form.database.trim()) errs.database = "请输入 SQLite 文件路径";
    } else if (form.type === "elasticsearch") {
      if (!form.es_urls.trim()) errs.es_urls = "请输入 Elasticsearch 节点地址";
      if (form.es_auth_type === "basic" && !form.username.trim()) {
        errs.username = "请输入用户名";
      }
      if (
        !editingId &&
        form.es_auth_type === "basic" &&
        !form.password
      ) {
        errs.password = "请输入密码";
      }
      if (
        !editingId &&
        form.es_auth_type === "api_key" &&
        !form.es_api_key
      ) {
        errs.es_api_key = "请输入 API Key";
      }
    } else {
      const h = validateHost(form.host);
      if (h) errs.host = h;
      const p = validatePort(form.port);
      if (p) errs.port = p;
    }
    if (
      form.type !== "sqlite" &&
      form.type !== "elasticsearch" &&
      !form.username.trim()
    ) {
      errs.username = "请输入用户名";
    }
    if (
      !editingId &&
      !form.password &&
      form.type !== "elasticsearch" &&
      form.type !== "sqlite"
    ) {
      errs.password = "请输入密码";
    }
    setErrors(errs);
    return Object.keys(errs).length === 0;
  }

  function buildDatasourcePayload(): Record<string, unknown> {
    const body: Record<string, unknown> = {
      name: form.name.trim(),
      type: form.type,
    };
    if (form.type === "sqlite") {
      body.database = form.database.trim();
      body.max_open = 1;
    } else if (form.type === "elasticsearch") {
      // Settings go into extra_config; the API key does not. It is a credential,
      // stored encrypted, and extra_config holds what was written verbatim.
      const extra: Record<string, unknown> = {
        urls: form.es_urls
          .split(",")
          .map((u) => u.trim())
          .filter(Boolean),
        auth_type: form.es_auth_type,
        verify_certs: form.es_verify_certs,
      };
      if (form.es_index_pattern) {
        extra.index_pattern = form.es_index_pattern.trim();
      }
      body.extra_config = extra;
      body.username =
        form.es_auth_type === "basic" ? form.username.trim() : "";
      if (form.es_auth_type === "basic" && form.password) {
        body.password = form.password;
      }
      if (form.es_auth_type === "api_key" && form.es_api_key) {
        body.es_api_key = form.es_api_key;
      }
    } else {
      body.host = form.host.trim();
      body.port = Number(form.port);
      body.username = form.username.trim();
      if (form.password) body.password = form.password;
      body.database = form.database.trim();
      body.max_open = Number(form.max_open) || 10;
      if (form.type === "postgresql") {
        body.sslmode = form.sslmode;
        body.schema_name = form.schema_name.trim() || "public";
      }
    }
    return body;
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!validate()) return;
    setSubmitting(true);
    const body = buildDatasourcePayload();
    try {
      const connectionAvailable = await testCurrentConfig();
      if (!connectionAvailable) {
        toast.error("连接验证未通过，配置未保存", {
          id: "datasource-save-validation",
        });
        return;
      }
      if (editingId) {
        await api.put<ApiResponse>(`/datasources/${editingId}`, body);
        toast.success("数据源更新成功");
      } else {
        await api.post<ApiResponse>("/datasources", body);
        toast.success("数据源添加成功");
      }
      setDialogOpen(false);
      fetchSources();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    } finally {
      setSubmitting(false);
    }
  }

  async function testCurrentConfig(): Promise<boolean> {
    if (!validate(false)) return false;
    const fingerprint = JSON.stringify(form);
    setConnectionTest({
      status: "testing",
      message: "正在建立连接并验证配置…",
      fingerprint,
    });
    try {
      const payload = buildDatasourcePayload();
      if (editingId) payload.id = editingId;
      const res = await api.post<TestConnectionResponse>(
        "/datasources/test-config",
        payload,
      );
      setConnectionTest({
        status: res.data.success ? "success" : "error",
        message:
          res.data.message ||
          (res.data.success ? "连接成功，配置可用" : "连接失败"),
        fingerprint,
      });
      return res.data.success;
    } catch (err) {
      setConnectionTest({
        status: "error",
        message: err instanceof Error ? err.message : "连接测试失败",
        fingerprint,
      });
      return false;
    }
  }

  async function handleTestConfig() {
    await testCurrentConfig();
  }

  async function handleTest(id: number) {
    setTestingId(id);
    try {
      const res = await api.post<TestConnectionResponse>(
        `/datasources/${id}/test`,
        {},
      );
      if (res.data.success) {
        toast.success(res.data.message || "连接测试成功");
      } else {
        toast.error(res.data.message || "连接测试失败");
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "连接测试失败");
    } finally {
      setTestingId(null);
    }
  }

  async function updateDatasourceStatus(
    ds: DataSourceItem,
    status: "active" | "disabled",
  ) {
    setStatusUpdatingId(ds.id);
    try {
      await api.put<ApiResponse>(`/datasources/${ds.id}/status`, { status });
      toast.success(status === "active" ? "数据源已启用" : "数据源已禁用");
      setDisableTarget(null);
      fetchSources();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "操作失败");
    } finally {
      setStatusUpdatingId(null);
      setDisabling(false);
    }
  }

  async function handleDisable() {
    if (!disableTarget) return;
    setDisabling(true);
    await updateDatasourceStatus(disableTarget, "disabled");
  }

  async function handleDelete() {
    if (!hardDeleteTarget) return;
    setDeleting(true);
    try {
      await api.del<ApiResponse>(`/datasources/${hardDeleteTarget.id}`);
      toast.success("数据源已删除");
      setHardDeleteTarget(null);
      fetchSources();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除数据源失败");
    } finally {
      setDeleting(false);
    }
  }

  // --- Render ---
  const currentFormFingerprint = JSON.stringify(form);
  const activeConnectionTest =
    connectionTest.fingerprint === currentFormFingerprint
      ? connectionTest
      : { status: "idle" as const, message: "", fingerprint: "" };

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-[var(--text-primary)]">
          数据源配置
        </h2>
        <Button
          onClick={openAdd}
          size="sm"
          className="gap-1 bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
        >
          <Plus size={14} />
          添加数据源
        </Button>
      </div>

      {/* Table */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)]">
        <Table className="table-fixed">
          <TableHeader>
            <TableRow className="border-[var(--border-default)] hover:bg-transparent">
              <TableHead className="w-[16%] text-[var(--text-secondary)]">
                名称
              </TableHead>
              <TableHead className="w-[11%] text-[var(--text-secondary)]">
                类型
              </TableHead>
              <TableHead className="w-[22%] text-[var(--text-secondary)]">
                地址
              </TableHead>
              <TableHead className="w-[14%] text-[var(--text-secondary)]">
                数据库
              </TableHead>
              <TableHead className="w-[7%] text-[var(--text-secondary)]">
                敏感表
              </TableHead>
              <TableHead className="w-[8%] text-[var(--text-secondary)]">
                状态
              </TableHead>
              <TableHead className="w-[22%] text-[var(--text-secondary)]">
                操作
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && !sources.length ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className="h-24 text-center text-[var(--text-muted)]"
                >
                  加载中...
                </TableCell>
              </TableRow>
            ) : !sources.length ? (
              <TableRow>
                <TableCell colSpan={7} className="h-32">
                  <div className="flex flex-col items-center gap-2">
                    <Database size={20} className="text-[var(--text-muted)]" />
                    <span className="text-sm text-[var(--text-muted)]">
                      暂无数据源
                    </span>
                    <span className="text-xs text-[var(--text-muted)]">
                      点击上方「添加数据源」按钮配置数据库连接
                    </span>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              sources.map((ds) => {
                const tb = TYPE_BADGE[ds.type] ?? {
                  label: ds.type,
                  cls: DEFAULT_BADGE.cls,
                };
                const sb = STATUS_BADGE[ds.status] ?? DEFAULT_BADGE;
                const sCount = sensitiveCounts.get(ds.id) ?? 0;
                return (
                  <TableRow
                    key={ds.id}
                    className="border-[var(--border-default)]"
                  >
                    <TableCell>
                      <span className="font-medium text-[var(--text-primary)]">
                        {ds.name}
                      </span>
                    </TableCell>
                    <TableCell>
                      <Badge className={tb.cls}>{tb.label}</Badge>
                    </TableCell>
                    <TableCell className="max-w-64">
                      <div className="flex min-w-0 items-center gap-1">
                        <span
                          className="min-w-0 flex-1 truncate text-[var(--text-secondary)]"
                          title={datasourceAddress(ds)}
                        >
                          {datasourceAddress(ds)}
                        </span>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7 shrink-0 text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                          aria-label={`复制 ${ds.name} 的地址`}
                          title="复制地址"
                          onClick={() => void copyDatasourceAddress(ds)}
                        >
                          <Copy size={13} />
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell className="max-w-52">
                      <span
                        className="block truncate font-mono text-xs text-[var(--text-secondary)]"
                        title={
                          ds.type === "sqlite"
                            ? ds.database
                            : datasourceDatabase(ds)
                        }
                      >
                        {datasourceDatabase(ds)}
                      </span>
                    </TableCell>
                    <TableCell>
                      {sCount > 0 ? (
                        <span className="inline-flex items-center gap-1 rounded bg-red-500/15 px-1.5 py-0.5 text-xs font-medium text-red-400">
                          <ShieldAlert size={12} />
                          {sCount}
                        </span>
                      ) : (
                        <span className="text-[var(--text-muted)]">0</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Badge className={sb.cls}>{sb.label}</Badge>
                        {ds.system && (
                          <Badge className="bg-blue-500/15 text-blue-400">
                            系统
                          </Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        {!ds.system && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                            onClick={() => openEdit(ds)}
                          >
                            <Pencil size={13} />
                            编辑
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                          disabled={testingId === ds.id}
                          onClick={() => handleTest(ds.id)}
                        >
                          {testingId === ds.id ? (
                            <Loader2 size={13} className="animate-spin" />
                          ) : (
                            <Plug size={13} />
                          )}
                          测试
                        </Button>
                        {!ds.system && ds.status === "active" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-amber-400"
                            disabled={statusUpdatingId === ds.id}
                            onClick={() => setDisableTarget(ds)}
                          >
                            <PowerOff size={13} />
                            禁用
                          </Button>
                        )}
                        {!ds.system && ds.status === "disabled" && (
                          <>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-emerald-400"
                              disabled={statusUpdatingId === ds.id}
                              onClick={() =>
                                void updateDatasourceStatus(ds, "active")
                              }
                            >
                              {statusUpdatingId === ds.id ? (
                                <Loader2
                                  size={13}
                                  className="animate-spin"
                                />
                              ) : (
                                <Power size={13} />
                              )}
                              启用
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-red-400"
                              onClick={() => setHardDeleteTarget(ds)}
                            >
                              <Trash2 size={13} />
                              删除
                            </Button>
                          </>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </div>

      {/* Add / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-[var(--text-primary)]">
              {editingId ? "编辑数据源" : "添加数据源"}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="grid grid-cols-2 gap-6">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">名称</Label>
                <Input
                  value={form.name}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, name: e.target.value }))
                  }
                  placeholder="2-50 个字符"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.name && (
                  <p className="text-xs text-red-400">{errors.name}</p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">类型</Label>
                <Select
                  value={form.type}
                  onValueChange={handleTypeChange}
                  disabled={!!editingId}
                >
                  <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="mysql">MySQL</SelectItem>
                    <SelectItem value="postgresql">PostgreSQL</SelectItem>
                    <SelectItem value="sqlite">SQLite（只读）</SelectItem>
                    <SelectItem value="mongodb">MongoDB</SelectItem>
                    <SelectItem value="elasticsearch">Elasticsearch</SelectItem>
                  </SelectContent>
                </Select>
                {editingId && (
                  <p className="text-xs text-[var(--text-muted)]">
                    已有数据源不可修改类型
                  </p>
                )}
              </div>
            </div>
            {["mysql", "postgresql", "mongodb"].includes(form.type) && <div className="grid grid-cols-2 gap-6">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">主机</Label>
                <Input
                  value={form.host}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, host: e.target.value }))
                  }
                  placeholder="IP 或域名"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.host && (
                  <p className="text-xs text-red-400">{errors.host}</p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">端口</Label>
                <Input
                  type="number"
                  value={form.port}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, port: e.target.value }))
                  }
                  placeholder="1-65535"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.port && (
                  <p className="text-xs text-red-400">{errors.port}</p>
                )}
              </div>
            </div>}
            {["mysql", "postgresql", "mongodb"].includes(form.type) && <div className="grid grid-cols-2 gap-6">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">用户名</Label>
                <Input
                  value={form.username}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, username: e.target.value }))
                  }
                  placeholder="数据库用户名"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.username && (
                  <p className="text-xs text-red-400">{errors.username}</p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">
                  密码{" "}
                  {editingId && (
                    <span className="font-normal text-[var(--text-muted)]">
                      (留空不修改)
                    </span>
                  )}
                </Label>
                <Input
                  type="password"
                  value={form.password}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, password: e.target.value }))
                  }
                  placeholder={editingId ? "留空不修改" : "数据库密码"}
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.password && (
                  <p className="text-xs text-red-400">{errors.password}</p>
                )}
              </div>
            </div>}
            {form.type !== "elasticsearch" && (
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">
                {form.type === "sqlite"
                  ? "SQLite 文件路径"
                  : form.type === "postgresql"
                    ? "数据库"
                    : "默认数据库"}
              </Label>
              <Input
                value={form.database}
                onChange={(e) =>
                  setForm((f) => ({ ...f, database: e.target.value }))
                }
                placeholder={
                  form.type === "sqlite"
                    ? "/absolute/path/to/database.db"
                    : "数据库名（可选）"
                }
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
              {errors.database && (
                <p className="text-xs text-red-400">{errors.database}</p>
              )}
            </div>
            )}
            {form.type === "postgresql" && (
              <div className="grid grid-cols-2 gap-6">
                <div className="space-y-1.5">
                  <Label className="text-[var(--text-secondary)]">
                    SSL 模式
                  </Label>
                  <Select
                    value={form.sslmode}
                    onValueChange={(v) =>
                      setForm((f) => ({ ...f, sslmode: v }))
                    }
                  >
                    <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="disable">disable</SelectItem>
                      <SelectItem value="prefer">prefer</SelectItem>
                      <SelectItem value="require">require</SelectItem>
                      <SelectItem value="verify-ca">verify-ca</SelectItem>
                      <SelectItem value="verify-full">verify-full</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-[var(--text-secondary)]">
                    Schema
                  </Label>
                  <Input
                    value={form.schema_name}
                    onChange={(e) =>
                      setForm((f) => ({
                        ...f,
                        schema_name: e.target.value,
                      }))
                    }
                    placeholder="public"
                    className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                  />
                </div>
              </div>
            )}
            {["mysql", "postgresql", "mongodb"].includes(form.type) && (
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">最大连接数</Label>
              <Input
                type="number"
                value={form.max_open}
                onChange={(e) =>
                  setForm((f) => ({ ...f, max_open: e.target.value }))
                }
                className="w-32 border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
            </div>
            )}

            {/* Elasticsearch specific fields */}
            {form.type === "elasticsearch" && (
              <div className="space-y-3 rounded-lg border border-orange-500/20 bg-orange-500/5 p-3">
                <div className="text-xs font-medium text-orange-400">
                  Elasticsearch 配置
                </div>
                <div className="space-y-1.5">
                  <Label className="text-[var(--text-secondary)]">
                    节点地址
                  </Label>
                  <Input
                    value={form.es_urls}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, es_urls: e.target.value }))
                    }
                    placeholder="https://es1.example.com:9200, https://es2.example.com:9200"
                    className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                  />
                  {errors.es_urls && (
                    <p className="text-xs text-red-400">{errors.es_urls}</p>
                  )}
                </div>
                <div className="space-y-1.5">
                  <Label className="text-[var(--text-secondary)]">
                    认证方式
                  </Label>
                  <Select
                    value={form.es_auth_type}
                    onValueChange={(v) =>
                      setForm((f) => ({ ...f, es_auth_type: v }))
                    }
                  >
                    <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="basic">Basic Auth</SelectItem>
                      <SelectItem value="api_key">API Key</SelectItem>
                      <SelectItem value="none">无认证</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                {form.es_auth_type === "basic" && (
                  <div className="grid grid-cols-2 gap-3">
                    <div className="space-y-1.5">
                      <Label className="text-[var(--text-secondary)]">
                        用户名
                      </Label>
                      <Input
                        value={form.username}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            username: e.target.value,
                          }))
                        }
                        placeholder="Elasticsearch 用户名"
                        className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                      />
                      {errors.username && (
                        <p className="text-xs text-red-400">{errors.username}</p>
                      )}
                    </div>
                    <div className="space-y-1.5">
                      <Label className="text-[var(--text-secondary)]">
                        密码{" "}
                        {editingId && (
                          <span className="font-normal text-[var(--text-muted)]">
                            (留空不修改)
                          </span>
                        )}
                      </Label>
                      <Input
                        type="password"
                        value={form.password}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            password: e.target.value,
                          }))
                        }
                        placeholder={editingId ? "留空不修改" : "密码"}
                        className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                      />
                      {errors.password && (
                        <p className="text-xs text-red-400">{errors.password}</p>
                      )}
                    </div>
                  </div>
                )}
                {form.es_auth_type === "api_key" && (
                  <div className="space-y-1.5">
                    <Label className="text-[var(--text-secondary)]">
                      API Key
                    </Label>
                    <Input
                      type="password"
                      value={form.es_api_key}
                      onChange={(e) =>
                        setForm((f) => ({
                          ...f,
                          es_api_key: e.target.value,
                        }))
                      }
                      placeholder={editingId ? "留空不修改" : "API Key"}
                      className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                    />
                    {errors.es_api_key && (
                      <p className="text-xs text-red-400">
                        {errors.es_api_key}
                      </p>
                    )}
                  </div>
                )}
                <div className="space-y-1.5">
                  <Label className="text-[var(--text-secondary)]">
                    默认索引模式
                  </Label>
                  <Input
                    value={form.es_index_pattern}
                    onChange={(e) =>
                      setForm((f) => ({
                        ...f,
                        es_index_pattern: e.target.value,
                      }))
                    }
                    placeholder="logs-* (可选)"
                  />
                </div>
                <label className="flex items-center gap-2 text-sm text-[var(--text-secondary)]">
                  <input
                    type="checkbox"
                    checked={form.es_verify_certs}
                    onChange={(e) =>
                      setForm((f) => ({
                        ...f,
                        es_verify_certs: e.target.checked,
                      }))
                    }
                  />
                  验证 SSL 证书
                </label>
              </div>
            )}

            {activeConnectionTest.status !== "idle" && (
              <div
                className={`flex items-start gap-2 rounded-lg border px-3 py-2.5 text-sm ${
                  activeConnectionTest.status === "success"
                    ? "border-emerald-500/25 bg-emerald-500/8 text-emerald-500"
                    : activeConnectionTest.status === "error"
                      ? "border-red-500/25 bg-red-500/8 text-red-400"
                      : "border-[var(--border-default)] bg-[var(--bg-elevated)] text-[var(--text-secondary)]"
                }`}
              >
                {activeConnectionTest.status === "testing" ? (
                  <Loader2 size={16} className="mt-0.5 shrink-0 animate-spin" />
                ) : activeConnectionTest.status === "success" ? (
                  <CheckCircle2 size={16} className="mt-0.5 shrink-0" />
                ) : (
                  <XCircle size={16} className="mt-0.5 shrink-0" />
                )}
                <span className="break-all">{activeConnectionTest.message}</span>
              </div>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={handleTestConfig}
                disabled={
                  submitting || activeConnectionTest.status === "testing"
                }
                className="mr-auto gap-1.5 border-[var(--border-default)]"
              >
                {activeConnectionTest.status === "testing" ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <Plug size={14} />
                )}
                {activeConnectionTest.status === "testing"
                  ? "测试中..."
                  : "测试连接"}
              </Button>
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
                {submitting ? "验证并保存中..." : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Disable Confirm */}
      <AlertDialog
        open={!!disableTarget}
        onOpenChange={(open) => {
          if (!open) setDisableTarget(null);
        }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              确认禁用数据源
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要禁用数据源「{disableTarget?.name}
              」吗？禁用后相关查询将不可用，但历史数据和配置会保留。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDisable}
              disabled={disabling}
              className="bg-amber-600 text-white hover:bg-amber-700"
            >
              {disabling ? "禁用中..." : "确认禁用"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Permanent Delete Confirm */}
      <AlertDialog
        open={!!hardDeleteTarget}
        onOpenChange={(open) => {
          if (!open) setHardDeleteTarget(null);
        }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              永久删除数据源
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要永久删除数据源「{hardDeleteTarget?.name}
              」吗？此操作不可恢复。存在工单、审计、权限或脱敏规则引用时，系统会拒绝删除。
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
              {deleting ? "删除中..." : "永久删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// --- Main Page ---

export default function SettingsPage() {
  const location = useLocation();
  const routeTab = location.pathname.split("/")[2] as SettingsTab | undefined;
  const activeTab: SettingsTab = SETTINGS_TABS.includes(routeTab!)
    ? routeTab!
    : "datasource";
  const [user, setUser] = useState<{ id: number; username: string; role: string } | null>(null);

  useEffect(() => {
    api
      .get<{ code: number; data: { id: number; username: string; role: string } }>("/auth/me")
      .then((res) => {
        if (res.code === 0) setUser(res.data);
      })
      .catch(() => {});
  }, []);

  return (
    <div className="mx-auto w-full max-w-[1360px] page-transition">
      {activeTab === "datasource" && <DataSourceTab />}
      {activeTab === "approval-policies" && <ApprovalPoliciesTab />}
      {activeTab === "mask-rules" && <MaskRulesTab />}
      {activeTab === "sla" && <SLATab />}
      {activeTab === "ai-config" && <AIConfigTab />}
      {activeTab === "integrations" && <IntegrationsTab user={user} />}
    </div>
  );
}
