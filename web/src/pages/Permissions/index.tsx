import { useState, useCallback, type FormEvent } from "react";
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
} from "lucide-react";
import { api } from "@/api/client";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
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

const PAGE_SIZE = 20;

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
                {ROLE_OPTIONS.filter((o) => o.value !== "admin").map((o) => (
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
            {ROLE_MAP[role] ?? role}
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
                  {ROLE_OPTIONS.map((o) => (
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
              className="bg-red-600 text-white hover:bg-red-700"
            >
              {deleteLoading ? "删除中..." : "确认删除"}
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
    <div className="flex h-[calc(100%-48px)] flex-col">
      <Tabs defaultValue="users" className="flex flex-1 flex-col">
        {/* Header — clean, no border-b bg-surface */}
        <div className="flex items-center justify-between mb-4">
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
        <TabsContent value="roles" className="flex-1 p-4 overflow-auto">
          <div className="flex h-48 flex-col items-center justify-center gap-3">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-[var(--bg-elevated)]">
              <Shield size={24} className="text-[var(--text-muted)]" />
            </div>
            <div className="text-center">
              <p className="text-sm font-medium text-[var(--text-secondary)]">
                角色管理功能开发中
              </p>
              <p className="mt-1 text-xs text-[var(--text-muted)]">
                当前角色体系包含管理员、DBA、开发人员三种预设角色
              </p>
            </div>
          </div>
        </TabsContent>

        <TabsContent
          value="policies"
          className="flex-1 p-4 overflow-auto"
        >
          <div className="flex h-48 flex-col items-center justify-center gap-3">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-[var(--bg-elevated)]">
              <Shield size={24} className="text-[var(--text-muted)]" />
            </div>
            <div className="text-center">
              <p className="text-sm font-medium text-[var(--text-secondary)]">
                权限策略功能开发中
              </p>
              <p className="mt-1 text-xs text-[var(--text-muted)]">
                未来将支持基于策略的细粒度权限控制
              </p>
            </div>
          </div>
        </TabsContent>

        <TabsContent
          value="users"
          className="flex-1 overflow-auto p-4"
        >
          <UserManagementTab />
        </TabsContent>
        </div>{/* end card container */}
      </Tabs>
    </div>
  );
}
