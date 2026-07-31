import { useState, useCallback, useEffect, type FormEvent } from "react";
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  type ColumnDef,
} from "@tanstack/react-table";
import { toast } from "sonner";
import {
  Plus,
  Pencil,
  KeyRound,
  Trash2,
  ChevronLeft,
  ChevronRight,
  Shield,
  Search,
} from "lucide-react";
import { api } from "@/api/client";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

// --- Types ---

interface UserItem {
  id: number;
  username: string;
  role: string;
  created_at: string;
}

interface ListUsersResponse {
  code: number;
  data: {
    users: UserItem[];
    total: number;
  };
}

interface ApiResponse {
  code: number;
  message: string;
  data?: unknown;
}

interface PolicyItem {
  id: number;
  ptype: string;
  sub: string;
  dom: string;
  obj: string;
  act: string;
}

interface PolicyListResponse {
  code: number;
  data: PolicyItem[];
  total: number;
}

interface DatasourceItem {
  id: number;
  name: string;
  type: string;
}

interface RoleItem {
  id: number;
  name: string;
  display_name: string;
  description: string;
  is_builtin: boolean;
  status: "active" | "disabled";
  user_count: number;
  policy_count: number;
  permissions: string[];
  created_at: string;
  updated_at: string;
}

interface RoleListResponse {
  code: number;
  data: RoleItem[];
}

// --- Constants ---

const ROLE_MAP: Record<string, string> = {
  admin: "管理员",
  dba: "DBA",
  developer: "开发人员",
};

const ROLE_OPTIONS = [
  { value: "admin", label: "管理员" },
  { value: "dba", label: "DBA" },
  { value: "developer", label: "开发人员" },
];

function useRoles() {
  const [roles, setRoles] = useState<RoleItem[]>([]);
  const [rolesLoading, setRolesLoading] = useState(true);

  const fetchRoles = useCallback(async () => {
    setRolesLoading(true);
    try {
      const res = await api.get<RoleListResponse>("/roles");
      setRoles(res.data ?? []);
    } catch {
      setRoles([]);
    } finally {
      setRolesLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchRoles();
  }, [fetchRoles]);

  return { roles, rolesLoading, fetchRoles };
}

const PAGE_SIZE = 20;

const POLICY_ACTIONS = [
  { value: "metadata:view", label: "查看表和字段结构" },
  { value: "select", label: "查询数据" },
  { value: "export", label: "导出数据" },
  { value: "unmask", label: "查看字段明文" },
  { value: "insert", label: "新增数据" },
  { value: "update", label: "修改数据" },
  { value: "delete", label: "删除数据" },
  { value: "ddl", label: "执行 DDL" },
] as const;

const PLATFORM_PERMISSIONS = [
  {
    value: "users:manage",
    label: "用户管理",
    description: "创建用户、调整角色和重置密码",
  },
  {
    value: "rbac:manage",
    label: "角色与权限",
    description: "管理角色定义和数据对象授权策略",
  },
  {
    value: "datasources:manage",
    label: "数据源管理",
    description: "登记、修改、测试和禁用数据源",
  },
  {
    value: "security:manage",
    label: "数据安全",
    description: "管理敏感表和字段脱敏规则",
  },
  {
    value: "audit:view",
    label: "审计查看",
    description: "查看审计日志与治理报表",
  },
  {
    value: "settings:manage",
    label: "系统设置",
    description: "管理通知、审批、SLA 和集成配置",
  },
] as const;

function actionLabel(action: string): string {
  return POLICY_ACTIONS.find((item) => item.value === action)?.label ?? action;
}

// --- Validators ---

function validateUsername(v: string): string | null {
  if (!v) return "请输入用户名";
  if (v.length < 3 || v.length > 32) return "用户名需 3-32 个字符";
  if (!/^[a-zA-Z0-9_]+$/.test(v)) return "用户名只能包含字母、数字和下划线";
  return null;
}

function validatePassword(v: string): string | null {
  if (!v) return "请输入密码";
  if (v.length < 8 || v.length > 128) return "密码需 8-128 个字符";
  if (!/[a-zA-Z]/.test(v) || !/[0-9]/.test(v))
    return "密码必须包含至少一个字母和一个数字";
  return null;
}

// --- User Management Tab ---

function UserManagementTab() {
  const { roles } = useRoles();
  const roleOptions =
    roles.length > 0
      ? roles
          .filter((role) => role.status === "active")
          .map((role) => ({ value: role.name, label: role.display_name }))
      : ROLE_OPTIONS;
  const roleMap = Object.fromEntries(
    roles.map((role) => [role.name, role.display_name]),
  );
  const [users, setUsers] = useState<UserItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [inited, setInited] = useState(false);

  // Add user dialog
  const [addOpen, setAddOpen] = useState(false);
  const [addForm, setAddForm] = useState({
    username: "",
    password: "",
    role: "developer",
  });
  const [addErrors, setAddErrors] = useState<Record<string, string>>({});
  const [addLoading, setAddLoading] = useState(false);

  // Edit role
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editRole, setEditRole] = useState("");
  const [editLoading, setEditLoading] = useState(false);

  // Reset password
  const [resetUser, setResetUser] = useState<UserItem | null>(null);
  const [resetPwd, setResetPwd] = useState("");
  const [resetPwdError, setResetPwdError] = useState("");
  const [resetLoading, setResetLoading] = useState(false);

  // Delete
  const [deleteUser, setDeleteUser] = useState<UserItem | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const fetchUsers = useCallback(async (p: number) => {
    setLoading(true);
    try {
      const res = await api.get<ListUsersResponse>(
        `/users?page=${p}&page_size=${PAGE_SIZE}`,
      );
      setUsers(res.data.users ?? []);
      setTotal(res.data.total ?? 0);
      setPage(p);
      setInited(true);
    } catch {
      toast.error("获取用户列表失败");
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial fetch
  if (!inited && !loading) {
    fetchUsers(1);
  }

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  // --- Add User ---

  function openAddDialog() {
    setAddForm({ username: "", password: "", role: "developer" });
    setAddErrors({});
    setAddOpen(true);
  }

  async function handleAddSubmit(e: FormEvent) {
    e.preventDefault();
    const uErr = validateUsername(addForm.username);
    const pErr = validatePassword(addForm.password);
    const errs: Record<string, string> = {};
    if (uErr) errs.username = uErr;
    if (pErr) errs.password = pErr;
    if (!addForm.role) errs.role = "请选择角色";
    if (Object.keys(errs).length) {
      setAddErrors(errs);
      return;
    }
    setAddLoading(true);
    try {
      await api.post<ApiResponse>("/users", addForm);
      toast.success("用户添加成功");
      setAddOpen(false);
      fetchUsers(1);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "添加用户失败");
    } finally {
      setAddLoading(false);
    }
  }

  // --- Edit Role ---

  async function handleEditRole(userId: number, newRole: string) {
    setEditLoading(true);
    try {
      await api.put<ApiResponse>(`/users/${userId}`, { role: newRole });
      toast.success("角色更新成功");
      setEditingId(null);
      fetchUsers(page);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新角色失败");
    } finally {
      setEditLoading(false);
    }
  }

  // --- Reset Password ---

  async function handleResetPassword() {
    if (!resetUser) return;
    const err = validatePassword(resetPwd);
    if (err) {
      setResetPwdError(err);
      return;
    }
    setResetLoading(true);
    try {
      await api.put<ApiResponse>(`/users/${resetUser.id}/reset-password`, {
        password: resetPwd,
      });
      toast.success("密码重置成功");
      setResetUser(null);
      setResetPwd("");
      setResetPwdError("");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "重置密码失败");
    } finally {
      setResetLoading(false);
    }
  }

  // --- Delete User ---

  async function handleDelete() {
    if (!deleteUser) return;
    setDeleteLoading(true);
    try {
      await api.del<ApiResponse>(`/users/${deleteUser.id}`);
      toast.success("用户已删除");
      setDeleteUser(null);
      fetchUsers(page);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除用户失败");
    } finally {
      setDeleteLoading(false);
    }
  }

  // --- TanStack Table ---

  const columns: ColumnDef<UserItem>[] = [
    {
      accessorKey: "username",
      header: "用户名",
      cell: ({ getValue }) => (
        <span className="block max-w-[120px] truncate font-medium text-[var(--text-primary)]">
          {getValue() as string}
        </span>
      ),
    },
    {
      accessorKey: "role",
      header: "角色",
      cell: ({ row }) => {
        const role = row.original.role;
        const isAdmin = role === "admin";
        const isEditing = editingId === row.original.id;

        if (isEditing) {
          return (
            <Select
              value={editRole}
              onValueChange={(v) => {
                setEditRole(v);
                handleEditRole(row.original.id, v);
              }}
              disabled={editLoading}
            >
              <SelectTrigger className="h-8 w-32 text-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {roleOptions.filter((o) => o.value !== "admin").map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          );
        }

        return (
          <Badge
            variant={isAdmin ? "default" : "secondary"}
            className={
              isAdmin
                ? "bg-[var(--accent-primary)]/20 text-[var(--accent-primary)]"
                : "bg-[var(--bg-elevated)] text-[var(--text-secondary)]"
            }
          >
            {roleMap[role] ?? ROLE_MAP[role] ?? role}
          </Badge>
        );
      },
    },
    {
      accessorKey: "created_at",
      header: "创建时间",
      cell: ({ getValue }) => {
        const v = getValue() as string;
        if (!v) return "-";
        try {
          const d = new Date(v);
          return (
            <span className="text-[var(--text-secondary)]">
              {d.toLocaleDateString("zh-CN", {
                month: "2-digit",
                day: "2-digit",
              })}{" "}
              {d.toLocaleTimeString("zh-CN", {
                hour: "2-digit",
                minute: "2-digit",
              })}
            </span>
          );
        } catch {
          return v;
        }
      },
    },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }) => {
        const user = row.original;
        const isAdmin = user.role === "admin";

        return (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
              disabled={isAdmin}
              onClick={() => {
                setEditingId(user.id);
                setEditRole(user.role);
              }}
              title={isAdmin ? "不能编辑管理员角色" : "编辑角色"}
            >
              <Pencil size={13} />
              编辑
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
              onClick={() => {
                setResetUser(user);
                setResetPwd("");
                setResetPwdError("");
              }}
            >
              <KeyRound size={13} />
              重置密码
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-red-400"
              disabled={isAdmin}
              onClick={() => setDeleteUser(user)}
              title={isAdmin ? "不能删除管理员" : "删除用户"}
            >
              <Trash2 size={13} />
              删除
            </Button>
          </div>
        );
      },
    },
  ];

  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    data: users,
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    pageCount: totalPages,
  });

  return (
    <div className="space-y-4">
      {/* Header with Add button */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-[var(--text-secondary)]">
          共 {total} 个用户
        </p>
        <Button
          onClick={openAddDialog}
          size="sm"
          className="gap-1 bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
        >
          <Plus size={14} />
          添加用户
        </Button>
      </div>

      {/* Table */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)]">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((hg) => (
              <TableRow
                key={hg.id}
                className="border-[var(--border-default)] hover:bg-transparent"
              >
                {hg.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    className="text-[var(--text-secondary)]"
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(
                          header.column.columnDef.header,
                          header.getContext(),
                        )}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {loading && !users.length ? (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className="h-24 text-center text-[var(--text-muted)]"
                >
                  加载中...
                </TableCell>
              </TableRow>
            ) : !users.length ? (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className="h-24 text-center text-[var(--text-muted)]"
                >
                  暂无用户数据
                </TableCell>
              </TableRow>
            ) : (
              table.getRowModel().rows.map((row) => (
                <TableRow
                  key={row.id}
                  className="border-[var(--border-default)]"
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext(),
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <p className="text-xs text-[var(--text-muted)]">
            第 {page}/{totalPages} 页
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="h-7 w-7 p-0 border-[var(--border-default)]"
              disabled={page <= 1 || loading}
              onClick={() => fetchUsers(page - 1)}
            >
              <ChevronLeft size={14} />
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="h-7 w-7 p-0 border-[var(--border-default)]"
              disabled={page >= totalPages || loading}
              onClick={() => fetchUsers(page + 1)}
            >
              <ChevronRight size={14} />
            </Button>
          </div>
        </div>
      )}

      {/* Add User Dialog */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-[var(--text-primary)]">
              添加用户
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={handleAddSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">用户名</Label>
              <Input
                value={addForm.username}
                onChange={(e) =>
                  setAddForm((f) => ({ ...f, username: e.target.value }))
                }
                onBlur={() =>
                  setAddErrors((e) => ({
                    ...e,
                    username: validateUsername(addForm.username) ?? "",
                  }))
                }
                placeholder="3-32 个字符，字母数字下划线"
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
              {addErrors.username && (
                <p className="text-xs text-red-400">{addErrors.username}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">密码</Label>
              <Input
                type="password"
                value={addForm.password}
                onChange={(e) =>
                  setAddForm((f) => ({ ...f, password: e.target.value }))
                }
                onBlur={() =>
                  setAddErrors((e) => ({
                    ...e,
                    password: validatePassword(addForm.password) ?? "",
                  }))
                }
                placeholder="8-128 个字符，需含字母和数字"
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
              {addErrors.password && (
                <p className="text-xs text-red-400">{addErrors.password}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">角色</Label>
              <Select
                value={addForm.role}
                onValueChange={(v) => setAddForm((f) => ({ ...f, role: v }))}
              >
                <SelectTrigger className="w-full border-[var(--border-default)] bg-[var(--bg-elevated)]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {roleOptions.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {addErrors.role && (
                <p className="text-xs text-red-400">{addErrors.role}</p>
              )}
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                className="border-[var(--border-default)]"
                onClick={() => setAddOpen(false)}
              >
                取消
              </Button>
              <Button
                type="submit"
                disabled={addLoading}
                className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
              >
                {addLoading ? "添加中..." : "添加"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Reset Password Dialog */}
      <AlertDialog
        open={!!resetUser}
        onOpenChange={(open) => {
          if (!open) setResetUser(null);
        }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              重置密码 — {resetUser?.username}
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              请输入新密码，密码需 8-128 个字符且包含字母和数字。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-1.5">
            <Input
              type="password"
              value={resetPwd}
              onChange={(e) => {
                setResetPwd(e.target.value);
                setResetPwdError("");
              }}
              placeholder="输入新密码"
              className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
            />
            {resetPwdError && (
              <p className="text-xs text-red-400">{resetPwdError}</p>
            )}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleResetPassword}
              disabled={resetLoading}
              className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
            >
              {resetLoading ? "重置中..." : "确认重置"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete Confirm Dialog */}
      <AlertDialog
        open={!!deleteUser}
        onOpenChange={(open) => {
          if (!open) setDeleteUser(null);
        }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              确认删除用户
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要删除用户「{deleteUser?.username}」吗？此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              disabled={deleteLoading}
              className="bg-[var(--danger)] text-white hover:bg-[var(--danger)]/80"
            >
              {deleteLoading ? "删除中..." : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// --- Policy Management Tab ---

function PolicyManagementTab() {
  const { roles } = useRoles();
  const roleOptions =
    roles.length > 0
      ? roles
          .filter((role) => role.status === "active")
          .map((role) => ({ value: role.name, label: role.display_name }))
      : ROLE_OPTIONS;
  const [policies, setPolicies] = useState<PolicyItem[]>([]);
  const [datasources, setDatasources] = useState<DatasourceItem[]>([]);
  const [users, setUsers] = useState<UserItem[]>([]);
  const [tables, setTables] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<PolicyItem | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [tableSearch, setTableSearch] = useState("");
  const [form, setForm] = useState({
    sub: "developer",
    datasourceId: "",
    objects: ["*"] as string[],
    actions: ["metadata:view"] as string[],
  });

  const fetchPolicies = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.get<PolicyListResponse>(
        "/policies?page=1&page_size=200",
      );
      setPolicies(res.data ?? []);
    } catch {
      toast.error("获取权限策略失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchPolicies();
    Promise.all([
      api.get<{ code: number; data: DatasourceItem[] }>("/datasources"),
      api.get<ListUsersResponse>("/users?page=1&page_size=200"),
    ])
      .then(([dsRes, userRes]) => {
        setDatasources(dsRes.data ?? []);
        setUsers(userRes.data.users ?? []);
      })
      .catch(() => toast.error("获取策略候选资源失败"));
  }, [fetchPolicies]);

  useEffect(() => {
    if (!form.datasourceId) {
      setTables([]);
      return;
    }
    api
      .get<{ code: number; data: string[] }>(
        `/datasources/${form.datasourceId}/tables`,
      )
      .then((res) => setTables(res.data ?? []))
      .catch(() => setTables([]));
  }, [form.datasourceId]);

  function openAddPolicy() {
    setForm({
      sub: "developer",
      datasourceId: datasources[0] ? String(datasources[0].id) : "",
      objects: ["*"],
      actions: ["metadata:view"],
    });
    setTableSearch("");
    setDialogOpen(true);
  }

  async function handleAddPolicy(e: FormEvent) {
    e.preventDefault();
    if (
      !form.sub ||
      !form.datasourceId ||
      form.objects.length === 0 ||
      form.actions.length === 0
    ) {
      toast.error("请选择授权主体、数据源、至少一个对象和权限");
      return;
    }

    const domain = `ds_${form.datasourceId}`;
    const existing = new Set(
      policies.map(
        (policy) =>
          `${policy.sub}\u0000${policy.dom}\u0000${policy.obj}\u0000${policy.act}`,
      ),
    );
    const rules = form.objects.flatMap((obj) =>
      form.actions
        .filter(
          (act) =>
            !existing.has(
              `${form.sub}\u0000${domain}\u0000${obj}\u0000${act}`,
            ),
        )
        .map((act) => ({ sub: form.sub, dom: domain, obj, act })),
    );

    if (rules.length === 0) {
      toast.info("所选权限均已存在，无需重复添加");
      return;
    }

    setSubmitting(true);
    try {
      const results = await Promise.allSettled(
        rules.map((rule) => api.post("/policies", rule)),
      );
      const succeeded = results.filter(
        (result) => result.status === "fulfilled",
      ).length;
      const failed = results.length - succeeded;

      if (succeeded > 0) {
        toast.success(`已添加 ${succeeded} 条权限规则`);
        setDialogOpen(false);
      }
      if (failed > 0) {
        toast.warning(`${failed} 条规则添加失败，请检查后重试`);
      }
      await fetchPolicies();
    } catch {
      toast.error("批量添加权限失败");
    } finally {
      setSubmitting(false);
    }
  }

  const filteredTables = tables.filter((table) =>
    table.toLowerCase().includes(tableSearch.trim().toLowerCase()),
  );

  function toggleObject(object: string, checked: boolean) {
    setForm((current) => {
      if (object === "*") {
        return {
          ...current,
          objects: checked ? ["*"] : [],
        };
      }
      return {
        ...current,
        objects: checked
          ? [
              ...new Set([
                ...current.objects.filter((item) => item !== "*"),
                object,
              ]),
            ]
          : current.objects.filter((item) => item !== object),
      };
    });
  }

  function toggleAction(action: string, checked: boolean) {
    setForm((current) => ({
      ...current,
      actions: checked
        ? [...new Set([...current.actions, action])]
        : current.actions.filter((item) => item !== action),
    }));
  }

  async function handleDeletePolicy() {
    if (!deleteTarget) return;
    try {
      await api.del(`/policies/${deleteTarget.id}`);
      toast.success("权限策略已删除");
      setDeleteTarget(null);
      await fetchPolicies();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除策略失败");
    }
  }

  function datasourceName(domain: string): string {
    if (domain === "*") return "全部数据源";
    const id = Number(domain.replace(/^ds_/, ""));
    return datasources.find((item) => item.id === id)?.name ?? domain;
  }

  function subjectName(subject: string): string {
    if (subject.startsWith("user:")) {
      const id = Number(subject.slice(5));
      return users.find((item) => item.id === id)?.username ?? subject;
    }
    return (
      roles.find((role) => role.name === subject)?.display_name ??
      ROLE_MAP[subject] ??
      subject
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-base font-semibold text-[var(--text-primary)]">
            数据对象权限
          </h2>
          <p className="mt-1 text-sm text-[var(--text-muted)]">
            表可见性由“查看表和字段结构”或“查询数据”决定；查询权限不自动授予导出或查看明文。
          </p>
        </div>
        <Button onClick={openAddPolicy} size="sm" className="gap-1">
          <Plus size={14} />
          添加策略
        </Button>
      </div>

      <div className="table-responsive rounded-lg border border-[var(--border-default)]">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>主体</TableHead>
              <TableHead>数据源</TableHead>
              <TableHead>表 / 集合 / 索引</TableHead>
              <TableHead>权限</TableHead>
              <TableHead className="w-20">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={5} className="h-24 text-center">
                  加载中…
                </TableCell>
              </TableRow>
            ) : policies.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="h-24 text-center">
                  暂无权限策略
                </TableCell>
              </TableRow>
            ) : (
              policies.map((policy) => (
                <TableRow key={policy.id}>
                  <TableCell>{subjectName(policy.sub)}</TableCell>
                  <TableCell>{datasourceName(policy.dom)}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {policy.obj === "*" ? "全部对象" : policy.obj}
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary">{actionLabel(policy.act)}</Badge>
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-red-400 hover:text-red-300"
                      onClick={() => setDeleteTarget(policy)}
                      aria-label={`删除 ${subjectName(policy.sub)} ${policy.obj} ${policy.act} 策略`}
                    >
                      <Trash2 size={14} />
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto border-[var(--border-default)] bg-[var(--bg-surface)]">
          <DialogHeader>
            <DialogTitle>批量添加数据对象权限</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleAddPolicy} className="space-y-4">
            <div className="space-y-1.5">
              <Label>授权主体</Label>
              <Select
                value={form.sub}
                onValueChange={(value) =>
                  setForm((current) => ({ ...current, sub: value }))
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {roleOptions.map((role) => (
                    <SelectItem key={role.value} value={role.value}>
                      角色 · {role.label}
                    </SelectItem>
                  ))}
                  {users.map((user) => (
                    <SelectItem key={user.id} value={`user:${user.id}`}>
                      用户 · {user.username}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1.5">
              <Label>数据源</Label>
              <Select
                value={form.datasourceId}
                onValueChange={(value) =>
                  setForm((current) => ({
                    ...current,
                    datasourceId: value,
                    objects: ["*"],
                  }))
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="选择数据源" />
                </SelectTrigger>
                <SelectContent>
                  {datasources.map((datasource) => (
                    <SelectItem
                      key={datasource.id}
                      value={String(datasource.id)}
                    >
                      {datasource.name} · {datasource.type}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>表 / 集合 / 索引</Label>
                <span className="text-xs text-[var(--text-muted)]">
                  {form.objects.includes("*")
                    ? "已选 ALL"
                    : `已选 ${form.objects.length} 个`}
                </span>
              </div>
              <div className="rounded-lg border border-[var(--border-default)]">
                <div className="flex items-center gap-2 border-b border-[var(--border-default)] px-3">
                  <Search size={14} className="text-[var(--text-muted)]" />
                  <Input
                    value={tableSearch}
                    onChange={(event) => setTableSearch(event.target.value)}
                    placeholder="搜索对象名称"
                    className="border-0 px-0 shadow-none focus-visible:ring-0"
                  />
                </div>
                <div className="flex items-center justify-between border-b border-[var(--border-default)] px-3 py-2 text-xs">
                  <button
                    type="button"
                    className="text-[var(--accent-primary)] hover:underline"
                    onClick={() =>
                      setForm((current) => ({
                        ...current,
                        objects: [...new Set(filteredTables)],
                      }))
                    }
                  >
                    全选当前结果
                  </button>
                  <button
                    type="button"
                    className="text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                    onClick={() =>
                      setForm((current) => ({
                        ...current,
                        objects: [],
                      }))
                    }
                  >
                    清空
                  </button>
                </div>
                <div className="max-h-48 overflow-y-auto p-2">
                  {!tableSearch.trim() && (
                    <label className="flex cursor-pointer items-center gap-2 rounded bg-[var(--bg-subtle)] px-2 py-2 text-sm hover:bg-[var(--bg-hover)]">
                      <Checkbox
                        checked={form.objects.includes("*")}
                        onCheckedChange={(checked) =>
                          toggleObject("*", checked)
                        }
                      />
                      <span className="font-mono text-xs font-semibold">
                        ALL
                      </span>
                      <span className="text-xs text-[var(--text-muted)]">
                        全部对象，包含未来新增对象
                      </span>
                    </label>
                  )}
                  {filteredTables.length === 0 ? (
                    <p className="py-6 text-center text-sm text-[var(--text-muted)]">
                      {form.datasourceId
                        ? "没有匹配的对象"
                        : "请先选择数据源"}
                    </p>
                  ) : (
                    filteredTables.map((table) => (
                      <label
                        key={table}
                        className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-[var(--bg-hover)]"
                      >
                        <Checkbox
                          checked={form.objects.includes(table)}
                          onCheckedChange={(checked) =>
                            toggleObject(table, checked)
                          }
                        />
                        <span className="font-mono text-xs">{table}</span>
                      </label>
                    ))
                  )}
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>权限动作</Label>
                <span className="text-xs text-[var(--text-muted)]">
                  已选 {form.actions.length} 项
                </span>
              </div>
              <div className="grid grid-cols-2 gap-2 rounded-lg border border-[var(--border-default)] p-3">
                {POLICY_ACTIONS.map((action) => (
                  <label
                    key={action.value}
                    className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-[var(--bg-hover)]"
                  >
                    <Checkbox
                      checked={form.actions.includes(action.value)}
                      onCheckedChange={(checked) =>
                        toggleAction(action.value, checked)
                      }
                    />
                    {action.label}
                  </label>
                ))}
              </div>
              {form.actions.includes("unmask") && (
                <p className="text-xs text-amber-400">
                  该权限会返回字段原文，应仅授予确有业务需要的主体。
                </p>
              )}
            </div>

            <div className="rounded-lg bg-[var(--bg-subtle)] px-3 py-2 text-xs text-[var(--text-secondary)]">
              将生成{" "}
              <strong className="text-[var(--text-primary)]">
                {form.objects.length * form.actions.length}
              </strong>{" "}
              条规则；已有的相同规则会自动跳过。
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialogOpen(false)}
              >
                取消
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting ? "保存中…" : "批量保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除权限策略</AlertDialogTitle>
            <AlertDialogDescription>
              删除后会立即影响后续请求。确认删除
              {deleteTarget
                ? `“${subjectName(deleteTarget.sub)} / ${deleteTarget.obj} / ${actionLabel(deleteTarget.act)}”`
                : ""}
              吗？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDeletePolicy}
              className="bg-[var(--danger)] text-white"
            >
              确认删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function RoleManagementTab() {
  const { roles, rolesLoading, fetchRoles } = useRoles();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingRole, setEditingRole] = useState<RoleItem | null>(null);
  const [deleteRole, setDeleteRole] = useState<RoleItem | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [form, setForm] = useState({
    name: "",
    display_name: "",
    description: "",
    status: "active",
    permissions: [] as string[],
  });
  const [errors, setErrors] = useState<Record<string, string>>({});

  function openCreate() {
    setEditingRole(null);
    setForm({
      name: "",
      display_name: "",
      description: "",
      status: "active",
      permissions: [],
    });
    setErrors({});
    setDialogOpen(true);
  }

  function openEdit(role: RoleItem) {
    setEditingRole(role);
    setForm({
      name: role.name,
      display_name: role.display_name,
      description: role.description,
      status: role.status,
      permissions: role.permissions ?? [],
    });
    setErrors({});
    setDialogOpen(true);
  }

  function validateRoleForm() {
    const nextErrors: Record<string, string> = {};
    if (!editingRole && !/^[a-z][a-z0-9_]{1,31}$/.test(form.name)) {
      nextErrors.name =
        "需以小写字母开头，只能包含小写字母、数字和下划线（2-32 位）";
    }
    if (!form.display_name.trim()) {
      nextErrors.display_name = "请输入角色显示名称";
    }
    setErrors(nextErrors);
    return Object.keys(nextErrors).length === 0;
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!validateRoleForm()) return;
    setSubmitting(true);
    try {
      if (editingRole) {
        await api.put(`/roles/${editingRole.name}`, {
          display_name: form.display_name.trim(),
          description: form.description.trim(),
          status: form.status,
          permissions: form.permissions,
        });
        toast.success("角色已更新");
      } else {
        await api.post("/roles", {
          name: form.name.trim(),
          display_name: form.display_name.trim(),
          description: form.description.trim(),
          permissions: form.permissions,
        });
        toast.success("角色已创建");
      }
      setDialogOpen(false);
      await fetchRoles();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存角色失败");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDeleteRole() {
    if (!deleteRole) return;
    setDeleting(true);
    try {
      await api.del(`/roles/${deleteRole.name}`);
      toast.success("角色已删除");
      setDeleteRole(null);
      await fetchRoles();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除角色失败");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-base font-semibold text-[var(--text-primary)]">
            角色管理
          </h2>
          <p className="mt-1 text-sm text-[var(--text-muted)]">
            角色用于聚合数据对象权限；内置角色不可停用或删除，在用角色需先迁移成员。
          </p>
        </div>
        <Button onClick={openCreate} size="sm" className="gap-1">
          <Plus size={14} />
          新建角色
        </Button>
      </div>

      <div className="table-responsive rounded-lg border border-[var(--border-default)]">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>角色</TableHead>
              <TableHead>标识</TableHead>
              <TableHead>成员</TableHead>
              <TableHead>策略</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="w-36">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rolesLoading ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center">
                  加载中…
                </TableCell>
              </TableRow>
            ) : roles.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center">
                  暂无角色
                </TableCell>
              </TableRow>
            ) : (
              roles.map((role) => (
                <TableRow key={role.name}>
                  <TableCell>
                    <div>
                      <div className="flex items-center gap-2 font-medium text-[var(--text-primary)]">
                        {role.display_name}
                        {role.is_builtin && (
                          <Badge variant="secondary">内置</Badge>
                        )}
                      </div>
                      <p className="mt-1 max-w-md truncate text-xs text-[var(--text-muted)]">
                        {role.description || "暂无描述"}
                      </p>
                    </div>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{role.name}</TableCell>
                  <TableCell>{role.user_count}</TableCell>
                  <TableCell>{role.policy_count}</TableCell>
                  <TableCell>
                    <Badge
                      className={
                        role.status === "active"
                          ? "bg-emerald-500/20 text-emerald-400"
                          : "bg-muted text-[var(--text-muted)]"
                      }
                    >
                      {role.status === "active" ? "启用" : "已停用"}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 gap-1 text-xs"
                        onClick={() => openEdit(role)}
                      >
                        <Pencil size={13} />
                        编辑
                      </Button>
                      {!role.is_builtin && (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 text-red-400 hover:text-red-300"
                          onClick={() => setDeleteRole(role)}
                          disabled={role.user_count > 0}
                          title={
                            role.user_count > 0
                              ? "请先迁移该角色下的用户"
                              : "删除角色"
                          }
                        >
                          <Trash2 size={13} />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <DialogHeader>
            <DialogTitle>{editingRole ? "编辑角色" : "新建角色"}</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label>角色标识</Label>
              <Input
                value={form.name}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
                disabled={Boolean(editingRole)}
                placeholder="例如 data_analyst"
              />
              {errors.name && (
                <p className="text-xs text-red-400">{errors.name}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label>显示名称</Label>
              <Input
                value={form.display_name}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    display_name: event.target.value,
                  }))
                }
                placeholder="例如 数据分析师"
              />
              {errors.display_name && (
                <p className="text-xs text-red-400">
                  {errors.display_name}
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label>描述</Label>
              <Input
                value={form.description}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    description: event.target.value,
                  }))
                }
                placeholder="说明该角色的职责和授权边界"
              />
            </div>
            {!editingRole?.is_builtin && (
              <div className="space-y-2">
                <Label>平台管理权限</Label>
                <div className="grid gap-2 rounded-lg border border-[var(--border-default)] p-3">
                  {PLATFORM_PERMISSIONS.map((permission) => {
                    const checked = form.permissions.includes(permission.value);
                    return (
                      <label
                        key={permission.value}
                        className="flex cursor-pointer items-start gap-3 rounded-md p-2 hover:bg-[var(--bg-elevated)]"
                      >
                        <input
                          type="checkbox"
                          className="mt-1"
                          checked={checked}
                          onChange={() =>
                            setForm((current) => ({
                              ...current,
                              permissions: checked
                                ? current.permissions.filter(
                                    (item) => item !== permission.value,
                                  )
                                : [...current.permissions, permission.value],
                            }))
                          }
                        />
                        <span>
                          <span className="block text-sm font-medium text-[var(--text-primary)]">
                            {permission.label}
                          </span>
                          <span className="block text-xs text-[var(--text-muted)]">
                            {permission.description}
                          </span>
                        </span>
                      </label>
                    );
                  })}
                </div>
              </div>
            )}
            {editingRole && (
              <div className="space-y-1.5">
                <Label>状态</Label>
                <Select
                  value={form.status}
                  onValueChange={(status) =>
                    setForm((current) => ({ ...current, status }))
                  }
                  disabled={editingRole.is_builtin}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="active">启用</SelectItem>
                    <SelectItem
                      value="disabled"
                      disabled={editingRole.user_count > 0}
                    >
                      停用
                    </SelectItem>
                  </SelectContent>
                </Select>
                {editingRole.user_count > 0 && !editingRole.is_builtin && (
                  <p className="text-xs text-amber-400">
                    该角色仍有成员，迁移成员后才能停用。
                  </p>
                )}
              </div>
            )}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialogOpen(false)}
              >
                取消
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting ? "保存中…" : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={Boolean(deleteRole)}
        onOpenChange={(open) => !open && setDeleteRole(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除角色</AlertDialogTitle>
            <AlertDialogDescription>
              确定删除“{deleteRole?.display_name}”吗？该角色关联的数据权限策略也会一并删除。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDeleteRole}
              disabled={deleting}
              className="bg-red-600 text-white hover:bg-red-500"
            >
              {deleting ? "删除中…" : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// --- Main Page ---

export default function PermissionsPage() {
  return (
    <div className="flex h-full flex-col">
      <Tabs defaultValue="roles" className="flex flex-1 flex-col">
        {/* Header — clean, no border-b bg-surface */}
        <div className="flex items-center justify-between mb-5">
          <div className="flex items-center gap-2.5">
            <Shield size={18} className="text-[var(--accent-primary)]" />
            <h1 className="text-xl font-semibold text-[var(--text-primary)]">
              权限管理
            </h1>
          </div>
          <TabsList className="bg-[var(--bg-elevated)]">
            <TabsTrigger value="roles">角色管理</TabsTrigger>
            <TabsTrigger value="policies">权限策略</TabsTrigger>
            <TabsTrigger value="users">用户管理</TabsTrigger>
          </TabsList>
        </div>

        {/* Tab content — card wrapper */}
        <div className="flex-1 overflow-hidden rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] flex flex-col">
        <TabsContent value="roles" className="flex-1 p-5 overflow-auto">
          <RoleManagementTab />
        </TabsContent>

        <TabsContent
          value="policies"
          className="flex-1 p-5 overflow-auto"
        >
          <PolicyManagementTab />
        </TabsContent>

        <TabsContent
          value="users"
          className="flex-1 overflow-auto p-5"
        >
          <UserManagementTab />
        </TabsContent>
        </div>{/* end card container */}
      </Tabs>
    </div>
  );
}
