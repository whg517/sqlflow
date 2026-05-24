import { useState, useCallback, useMemo, type FormEvent } from "react";
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
  Ban,
  Search,
  ChevronLeft,
  ChevronRight,
  Users as UsersIcon,
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
  listUsers,
  createUser,
  updateUser,
  deleteUser,
  resetPassword,
  ROLE_LABEL_MAP,
  ROLE_BADGE_CLASS,
  ROLE_OPTIONS,
  type User,
} from "@/api/user";

// --- Constants ---

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
  if (v.length < 8) return "密码至少 8 个字符";
  if (!/[a-zA-Z]/.test(v) || !/[0-9]/.test(v))
    return "密码必须包含至少一个字母和一个数字";
  return null;
}

// --- Main Component ---

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [inited, setInited] = useState(false);
  const [keyword, setKeyword] = useState("");

  // Create dialog
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState({
    username: "",
    password: "",
    role: "developer",
  });
  const [createErrors, setCreateErrors] = useState<Record<string, string>>({});
  const [createLoading, setCreateLoading] = useState(false);

  // Edit dialog
  const [editOpen, setEditOpen] = useState(false);
  const [editUser, setEditUser] = useState<User | null>(null);
  const [editForm, setEditForm] = useState({ username: "", role: "" });
  const [editErrors, setEditErrors] = useState<Record<string, string>>({});
  const [editLoading, setEditLoading] = useState(false);

  // Reset password dialog
  const [resetTarget, setResetTarget] = useState<User | null>(null);
  const [resetPwd, setResetPwd] = useState("");
  const [resetPwdError, setResetPwdError] = useState("");
  const [resetLoading, setResetLoading] = useState(false);

  // Disable dialog
  const [disableTarget, setDisableTarget] = useState<User | null>(null);
  const [disableLoading, setDisableLoading] = useState(false);

  // --- Fetch ---

  const fetchUsers = useCallback(async (p: number) => {
    setLoading(true);
    try {
      const res = await listUsers(p, PAGE_SIZE);
      setUsers(res.data?.users ?? []);
      setTotal(res.data?.total ?? 0);
      setPage(p);
      setInited(true);
    } catch {
      toast.error("获取用户列表失败");
    } finally {
      setLoading(false);
    }
  }, []);

  if (!inited && !loading) {
    fetchUsers(1);
  }

  // --- Filtered users ---

  const filteredUsers = useMemo(() => {
    if (!keyword.trim()) return users;
    const kw = keyword.trim().toLowerCase();
    return users.filter((u) => u.username.toLowerCase().includes(kw));
  }, [users, keyword]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  // --- Create User ---

  function openCreateDialog() {
    setCreateForm({ username: "", password: "", role: "developer" });
    setCreateErrors({});
    setCreateOpen(true);
  }

  async function handleCreateSubmit(e: FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    const uErr = validateUsername(createForm.username);
    const pErr = validatePassword(createForm.password);
    if (uErr) errs.username = uErr;
    if (pErr) errs.password = pErr;
    if (!createForm.role) errs.role = "请选择角色";
    if (Object.keys(errs).length) {
      setCreateErrors(errs);
      return;
    }
    setCreateLoading(true);
    try {
      await createUser(createForm);
      toast.success("用户创建成功");
      setCreateOpen(false);
      fetchUsers(1);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "创建用户失败");
    } finally {
      setCreateLoading(false);
    }
  }

  // --- Edit User ---

  function openEditDialog(user: User) {
    setEditUser(user);
    setEditForm({ username: user.username, role: user.role });
    setEditErrors({});
    setEditOpen(true);
  }

  async function handleEditSubmit(e: FormEvent) {
    e.preventDefault();
    if (!editUser) return;
    const errs: Record<string, string> = {};
    const uErr = validateUsername(editForm.username);
    if (uErr) errs.username = uErr;
    if (!editForm.role) errs.role = "请选择角色";
    if (Object.keys(errs).length) {
      setEditErrors(errs);
      return;
    }
    setEditLoading(true);
    try {
      await updateUser(editUser.id, {
        username: editForm.username,
        role: editForm.role,
      });
      toast.success("用户更新成功");
      setEditOpen(false);
      fetchUsers(page);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新用户失败");
    } finally {
      setEditLoading(false);
    }
  }

  // --- Reset Password ---

  async function handleResetPassword() {
    if (!resetTarget) return;
    const err = validatePassword(resetPwd);
    if (err) {
      setResetPwdError(err);
      return;
    }
    setResetLoading(true);
    try {
      await resetPassword(resetTarget.id, { password: resetPwd });
      toast.success("密码重置成功");
      setResetTarget(null);
      setResetPwd("");
      setResetPwdError("");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "重置密码失败");
    } finally {
      setResetLoading(false);
    }
  }

  // --- Disable User ---

  async function handleDisable() {
    if (!disableTarget) return;
    setDisableLoading(true);
    try {
      await deleteUser(disableTarget.id);
      toast.success("用户已禁用");
      setDisableTarget(null);
      fetchUsers(page);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "禁用用户失败");
    } finally {
      setDisableLoading(false);
    }
  }

  // --- TanStack Table ---

  const columns = useMemo<ColumnDef<User>[]>(
    () => [
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
        cell: ({ getValue }) => {
          const role = getValue() as string;
          return (
            <Badge
              className={
                ROLE_BADGE_CLASS[role] ?? "bg-muted text-muted-foreground"
              }
            >
              {ROLE_LABEL_MAP[role] ?? role}
            </Badge>
          );
        },
      },
      {
        accessorKey: "created_at",
        header: "创建时间",
        cell: ({ getValue }) => {
          const v = getValue() as string;
          if (!v) return <span className="text-[var(--text-muted)]">-</span>;
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
            return <span className="text-[var(--text-secondary)]">{v}</span>;
          }
        },
      },
      {
        accessorKey: "status",
        header: "状态",
        cell: ({ getValue }) => {
          const status = getValue() as string;
          const active = status !== "disabled";
          return (
            <Badge
              className={
                active
                  ? "bg-emerald-500/20 text-emerald-400"
                  : "bg-muted text-muted-foreground"
              }
            >
              {active ? "活跃" : "已禁用"}
            </Badge>
          );
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
                onClick={() => openEditDialog(user)}
                title={isAdmin ? "不能编辑管理员" : "编辑"}
              >
                <Pencil size={13} />
                编辑
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                onClick={() => {
                  setResetTarget(user);
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
                disabled={isAdmin || user.status === "disabled"}
                onClick={() => setDisableTarget(user)}
                title={
                  isAdmin
                    ? "不能禁用管理员"
                    : user.status === "disabled"
                      ? "已禁用"
                      : "禁用用户"
                }
              >
                <Ban size={13} />
                禁用
              </Button>
            </div>
          );
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [page],
  );

  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    data: filteredUsers,
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    pageCount: totalPages,
  });

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-4">
        <div className="flex items-center gap-2.5">
          <UsersIcon size={18} className="text-[var(--accent-primary)]" />
          <h1 className="text-xl font-semibold text-[var(--text-primary)]">
            用户管理
          </h1>
          {total > 0 && (
            <span className="rounded-full bg-[var(--bg-elevated)] px-2 py-0.5 text-xs text-[var(--text-muted)]">
              {total}
            </span>
          )}
        </div>
        <Button
          onClick={openCreateDialog}
          size="sm"
          className="gap-1 bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
        >
          <Plus size={14} />
          新建用户
        </Button>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto p-6 bg-[var(--bg-base)]">
        <div className="space-y-4">
          {/* Search + Count */}
          <div className="flex items-center justify-between">
            <div className="relative w-64">
              <Search
                size={14}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]"
              />
              <Input
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                placeholder="搜索用户名..."
                className="h-8 pl-8 text-sm border-[var(--border-default)] bg-[var(--bg-elevated)] text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
              />
            </div>
            <p className="text-xs text-[var(--text-muted)]">
              共 {total} 个用户
            </p>
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
                    <TableCell colSpan={columns.length} className="h-24">
                      <div className="space-y-3 px-4">
                        {[1, 2, 3].map((i) => (
                          <div key={i} className="flex items-center gap-4">
                            <div className="h-3 w-20 rounded bg-[var(--bg-elevated)] animate-pulse" />
                            <div className="h-5 w-14 rounded-full bg-[var(--bg-elevated)] animate-pulse" />
                            <div className="h-3 w-24 rounded bg-[var(--bg-elevated)] animate-pulse" />
                            <div className="ml-auto h-3 w-16 rounded bg-[var(--bg-elevated)] animate-pulse" />
                          </div>
                        ))}
                      </div>
                    </TableCell>
                  </TableRow>
                ) : !filteredUsers.length ? (
                  <TableRow>
                    <TableCell colSpan={columns.length} className="h-32">
                      <div className="flex flex-col items-center gap-2 page-transition">
                        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--bg-elevated)] empty-state-icon">
                          <UsersIcon
                            size={20}
                            className="text-[var(--text-muted)]"
                          />
                        </div>
                        <span className="text-sm text-[var(--text-muted)]">
                          {keyword ? "没有匹配的用户" : "暂无用户数据"}
                        </span>
                      </div>
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
        </div>
      </div>

      {/* Create User Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-[var(--text-primary)]">
              新建用户
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={handleCreateSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">用户名</Label>
              <Input
                value={createForm.username}
                onChange={(e) =>
                  setCreateForm((f) => ({ ...f, username: e.target.value }))
                }
                onBlur={() =>
                  setCreateErrors((e) => ({
                    ...e,
                    username: validateUsername(createForm.username) ?? "",
                  }))
                }
                placeholder="3-32 个字符，字母数字下划线"
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
              {createErrors.username && (
                <p className="text-xs text-red-400">{createErrors.username}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">密码</Label>
              <Input
                type="password"
                value={createForm.password}
                onChange={(e) =>
                  setCreateForm((f) => ({ ...f, password: e.target.value }))
                }
                onBlur={() =>
                  setCreateErrors((e) => ({
                    ...e,
                    password: validatePassword(createForm.password) ?? "",
                  }))
                }
                placeholder="至少 8 个字符，需含字母和数字"
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
              {createErrors.password && (
                <p className="text-xs text-red-400">{createErrors.password}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">角色</Label>
              <Select
                value={createForm.role}
                onValueChange={(v) => setCreateForm((f) => ({ ...f, role: v }))}
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
              {createErrors.role && (
                <p className="text-xs text-red-400">{createErrors.role}</p>
              )}
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                className="border-[var(--border-default)]"
                onClick={() => setCreateOpen(false)}
              >
                取消
              </Button>
              <Button
                type="submit"
                disabled={createLoading}
                className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
              >
                {createLoading ? "创建中..." : "创建"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Edit User Dialog */}
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-[var(--text-primary)]">
              编辑用户 — {editUser?.username}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={handleEditSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">用户名</Label>
              <Input
                value={editForm.username}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, username: e.target.value }))
                }
                onBlur={() =>
                  setEditErrors((e) => ({
                    ...e,
                    username: validateUsername(editForm.username) ?? "",
                  }))
                }
                placeholder="3-32 个字符，字母数字下划线"
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
              {editErrors.username && (
                <p className="text-xs text-red-400">{editErrors.username}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">角色</Label>
              <Select
                value={editForm.role}
                onValueChange={(v) => setEditForm((f) => ({ ...f, role: v }))}
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
              {editErrors.role && (
                <p className="text-xs text-red-400">{editErrors.role}</p>
              )}
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                className="border-[var(--border-default)]"
                onClick={() => setEditOpen(false)}
              >
                取消
              </Button>
              <Button
                type="submit"
                disabled={editLoading}
                className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
              >
                {editLoading ? "保存中..." : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Reset Password Dialog */}
      <AlertDialog
        open={!!resetTarget}
        onOpenChange={(open) => {
          if (!open) setResetTarget(null);
        }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              重置密码 — {resetTarget?.username}
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              请输入新密码，至少 8 个字符且包含字母和数字。
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

      {/* Disable Confirm Dialog */}
      <AlertDialog
        open={!!disableTarget}
        onOpenChange={(open) => {
          if (!open) setDisableTarget(null);
        }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              确认禁用用户
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要禁用用户「{disableTarget?.username}
              」吗？禁用后该用户将无法登录。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDisable}
              disabled={disableLoading}
              className="bg-red-600 text-white hover:bg-red-700"
            >
              {disableLoading ? "禁用中..." : "确认禁用"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
