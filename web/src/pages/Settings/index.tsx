import { useState, useCallback, type FormEvent } from 'react'
import { toast } from 'sonner'
import {
  Plus, Pencil, Trash2, Database, ShieldCheck, Brain,
  Plug, Loader2,
} from 'lucide-react'
import { api } from '@/api/client'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
} from '@/components/ui/table'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from '@/components/ui/dialog'
import {
  AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogTitle,
  AlertDialogDescription, AlertDialogFooter, AlertDialogCancel, AlertDialogAction,
} from '@/components/ui/alert-dialog'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import MaskRulesTab from './MaskRulesTab'
import AIConfigTab from './AIConfigTab'

// --- Types ---

interface DataSourceItem {
  id: number
  name: string
  type: string
  host: string
  port: number
  username: string
  database: string
  max_open: number
  status: string
  created_at: string
}

interface DataSourceListResponse {
  code: number
  data: DataSourceItem[]
}

interface TestConnectionResponse {
  code: number
  data: { message: string; success: boolean }
}

interface ApiResponse {
  code: number
  message: string
}

type SettingsTab = 'datasource' | 'mask-rules' | 'ai-config'

// --- Constants ---

const NAV_ITEMS: { key: SettingsTab; label: string; icon: typeof Database }[] = [
  { key: 'datasource', label: '数据源', icon: Database },
  { key: 'mask-rules', label: '脱敏规则', icon: ShieldCheck },
  { key: 'ai-config', label: 'AI 配置', icon: Brain },
]

const TYPE_BADGE: Record<string, { label: string; cls: string }> = {
  mysql: { label: 'MySQL', cls: 'bg-blue-500/20 text-blue-400' },
  mongodb: { label: 'MongoDB', cls: 'bg-green-500/20 text-green-400' },
}

const STATUS_BADGE: Record<string, { label: string; cls: string }> = {
  active: { label: '正常', cls: 'bg-emerald-500/20 text-emerald-400' },
  disabled: { label: '已禁用', cls: 'bg-gray-500/20 text-gray-400' },
  error: { label: '异常', cls: 'bg-red-500/20 text-red-400' },
}

const DEFAULT_BADGE = { label: '未知', cls: 'bg-gray-500/20 text-gray-400' }

// --- Validators ---

function validateName(v: string): string | null {
  if (!v.trim()) return '请输入名称'
  if (v.trim().length < 2 || v.trim().length > 50) return '名称需 2-50 个字符'
  return null
}

function validateHost(v: string): string | null {
  if (!v.trim()) return '请输入主机地址'
  return null
}

function validatePort(v: string): string | null {
  const n = Number(v)
  if (!v || isNaN(n)) return '请输入端口号'
  if (n < 1 || n > 65535 || !Number.isInteger(n)) return '端口范围 1-65535'
  return null
}

// --- DataSource Tab ---

function DataSourceTab() {
  const [sources, setSources] = useState<DataSourceItem[]>([])
  const [loading, setLoading] = useState(false)
  const [inited, setInited] = useState(false)

  // Dialog
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState({
    name: '', type: 'mysql', host: '', port: '3306',
    username: '', password: '', database: '', max_open: '10',
  })
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)

  // Test
  const [testingId, setTestingId] = useState<number | null>(null)

  // Delete
  const [deleteTarget, setDeleteTarget] = useState<DataSourceItem | null>(null)
  const [deleting, setDeleting] = useState(false)

  const fetchSources = useCallback(async () => {
    setLoading(true)
    try {
      const res = await api.get<DataSourceListResponse>('/datasources')
      setSources(res.data ?? [])
      setInited(true)
    } catch {
      toast.error('获取数据源列表失败')
    } finally {
      setLoading(false)
    }
  }, [])

  if (!inited && !loading) fetchSources()

  // --- Handlers ---

  function openAdd() {
    setEditingId(null)
    setForm({
      name: '', type: 'mysql', host: '', port: '3306',
      username: '', password: '', database: '', max_open: '10',
    })
    setErrors({})
    setDialogOpen(true)
  }

  function openEdit(ds: DataSourceItem) {
    setEditingId(ds.id)
    setForm({
      name: ds.name, type: ds.type, host: ds.host,
      port: String(ds.port), username: ds.username,
      password: '', database: ds.database || '', max_open: String(ds.max_open || 10),
    })
    setErrors({})
    setDialogOpen(true)
  }

  function validate(): boolean {
    const errs: Record<string, string> = {}
    const n = validateName(form.name); if (n) errs.name = n
    const h = validateHost(form.host); if (h) errs.host = h
    const p = validatePort(form.port); if (p) errs.port = p
    if (!form.username.trim()) errs.username = '请输入用户名'
    if (!editingId && !form.password) errs.password = '请输入密码'
    setErrors(errs)
    return Object.keys(errs).length === 0
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!validate()) return
    setSubmitting(true)
    const body = {
      name: form.name.trim(),
      type: form.type,
      host: form.host.trim(),
      port: Number(form.port),
      username: form.username.trim(),
      ...(form.password ? { password: form.password } : {}),
      database: form.database.trim(),
      max_open: Number(form.max_open) || 10,
    }
    try {
      if (editingId) {
        await api.put<ApiResponse>(`/datasources/${editingId}`, body)
        toast.success('数据源更新成功')
      } else {
        await api.post<ApiResponse>('/datasources', body)
        toast.success('数据源添加成功')
      }
      setDialogOpen(false)
      fetchSources()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleTest(id: number) {
    setTestingId(id)
    try {
      const res = await api.post<TestConnectionResponse>(`/datasources/${id}/test`, {})
      if (res.data.success) {
        toast.success(res.data.message || '连接测试成功')
      } else {
        toast.error(res.data.message || '连接测试失败')
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '连接测试失败')
    } finally {
      setTestingId(null)
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await api.del<ApiResponse>(`/datasources/${deleteTarget.id}`)
      toast.success('数据源已禁用')
      setDeleteTarget(null)
      fetchSources()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setDeleting(false)
    }
  }

  // --- Render ---

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-[var(--text-primary)]">数据源配置</h2>
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
        <Table>
          <TableHeader>
            <TableRow className="border-[var(--border-default)] hover:bg-transparent">
              <TableHead className="text-[var(--text-secondary)]">名称</TableHead>
              <TableHead className="text-[var(--text-secondary)]">类型</TableHead>
              <TableHead className="text-[var(--text-secondary)]">地址</TableHead>
              <TableHead className="text-[var(--text-secondary)]">数据库</TableHead>
              <TableHead className="text-[var(--text-secondary)]">状态</TableHead>
              <TableHead className="text-[var(--text-secondary)]">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && !sources.length ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-[var(--text-muted)]">
                  加载中...
                </TableCell>
              </TableRow>
            ) : !sources.length ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-[var(--text-muted)]">
                  暂无数据源，点击上方按钮添加
                </TableCell>
              </TableRow>
            ) : (
              sources.map((ds) => {
                const tb = TYPE_BADGE[ds.type] ?? { label: ds.type, cls: DEFAULT_BADGE.cls }
                const sb = STATUS_BADGE[ds.status] ?? DEFAULT_BADGE
                return (
                  <TableRow key={ds.id} className="border-[var(--border-default)]">
                    <TableCell>
                      <span className="font-medium text-[var(--text-primary)]">{ds.name}</span>
                    </TableCell>
                    <TableCell>
                      <Badge className={tb.cls}>{tb.label}</Badge>
                    </TableCell>
                    <TableCell>
                      <span className="text-[var(--text-secondary)]">{ds.host}:{ds.port}</span>
                    </TableCell>
                    <TableCell>
                      <span className="text-[var(--text-secondary)]">{ds.database || '—'}</span>
                    </TableCell>
                    <TableCell>
                      <Badge className={sb.cls}>{sb.label}</Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Button
                          variant="ghost" size="sm"
                          className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                          onClick={() => openEdit(ds)}
                        >
                          <Pencil size={13} />
                          编辑
                        </Button>
                        <Button
                          variant="ghost" size="sm"
                          className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                          disabled={testingId === ds.id}
                          onClick={() => handleTest(ds.id)}
                        >
                          {testingId === ds.id
                            ? <Loader2 size={13} className="animate-spin" />
                            : <Plug size={13} />}
                          测试
                        </Button>
                        <Button
                          variant="ghost" size="sm"
                          className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-red-400"
                          onClick={() => setDeleteTarget(ds)}
                        >
                          <Trash2 size={13} />
                          禁用
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      {/* Add / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-[var(--text-primary)]">
              {editingId ? '编辑数据源' : '添加数据源'}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">名称</Label>
                <Input
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  placeholder="2-50 个字符"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.name && <p className="text-xs text-red-400">{errors.name}</p>}
              </div>
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">类型</Label>
                <Select
                  value={form.type}
                  onValueChange={(v) => setForm((f) => ({ ...f, type: v }))}
                >
                  <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="mysql">MySQL</SelectItem>
                    <SelectItem value="mongodb">MongoDB</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">主机</Label>
                <Input
                  value={form.host}
                  onChange={(e) => setForm((f) => ({ ...f, host: e.target.value }))}
                  placeholder="IP 或域名"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.host && <p className="text-xs text-red-400">{errors.host}</p>}
              </div>
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">端口</Label>
                <Input
                  type="number"
                  value={form.port}
                  onChange={(e) => setForm((f) => ({ ...f, port: e.target.value }))}
                  placeholder="1-65535"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.port && <p className="text-xs text-red-400">{errors.port}</p>}
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">用户名</Label>
                <Input
                  value={form.username}
                  onChange={(e) => setForm((f) => ({ ...f, username: e.target.value }))}
                  placeholder="数据库用户名"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.username && <p className="text-xs text-red-400">{errors.username}</p>}
              </div>
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">
                  密码{' '}
                  {editingId && (
                    <span className="font-normal text-[var(--text-muted)]">(留空不修改)</span>
                  )}
                </Label>
                <Input
                  type="password"
                  value={form.password}
                  onChange={(e) => setForm((f) => ({ ...f, password: e.target.value }))}
                  placeholder={editingId ? '留空不修改' : '数据库密码'}
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
                {errors.password && <p className="text-xs text-red-400">{errors.password}</p>}
              </div>
            </div>
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">默认数据库</Label>
              <Input
                value={form.database}
                onChange={(e) => setForm((f) => ({ ...f, database: e.target.value }))}
                placeholder="数据库名（可选）"
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">最大连接数</Label>
              <Input
                type="number"
                value={form.max_open}
                onChange={(e) => setForm((f) => ({ ...f, max_open: e.target.value }))}
                className="w-32 border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
            </div>
            <DialogFooter>
              <Button
                type="button" variant="outline"
                className="border-[var(--border-default)]"
                onClick={() => setDialogOpen(false)}
              >
                取消
              </Button>
              <Button
                type="submit" disabled={submitting}
                className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
              >
                {submitting ? '保存中...' : '保存'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete (Disable) Confirm */}
      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              确认禁用数据源
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要禁用数据源「{deleteTarget?.name}」吗？禁用后相关查询将不可用。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete} disabled={deleting}
              className="bg-red-600 text-white hover:bg-red-700"
            >
              {deleting ? '禁用中...' : '确认禁用'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// --- Main Page ---

export default function SettingsPage() {
  const [activeTab, setActiveTab] = useState<SettingsTab>('datasource')

  return (
    <div className="flex h-full">
      {/* Left sidebar */}
      <nav className="w-44 shrink-0 border-r border-[var(--border-default)] bg-[var(--bg-surface)] p-3">
        <h1 className="mb-4 px-3 text-xl font-semibold text-[var(--text-primary)]">设置</h1>
        <div className="space-y-1">
          {NAV_ITEMS.map(({ key, label, icon: Icon }) => (
            <button
              key={key}
              onClick={() => setActiveTab(key)}
              className={cn(
                'flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
                activeTab === key
                  ? 'bg-[var(--accent-primary)]/15 text-[var(--accent-primary)]'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]',
              )}
            >
              <Icon size={16} />
              {label}
            </button>
          ))}
        </div>
      </nav>

      {/* Content */}
      <div className="flex-1 overflow-auto p-6">
        {activeTab === 'datasource' && <DataSourceTab />}
        {activeTab === 'mask-rules' && <MaskRulesTab />}
        {activeTab === 'ai-config' && <AIConfigTab />}
      </div>
    </div>
  )
}
