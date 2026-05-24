import { useState, useCallback, useEffect, type FormEvent } from 'react'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, ShieldCheck, ShieldAlert, FileWarning } from 'lucide-react'
import { api } from '@/api/client'
import {
  listMaskRules,
  createMaskRule,
  updateMaskRule,
  deleteMaskRule,
  listSensitiveTables,
  createSensitiveTable,
  deleteSensitiveTable,
  fetchDatasourceTables,
  getMaskTypeLabel,
  MASK_TYPE_OPTIONS,
  SENSITIVITY_OPTIONS,
  SENSITIVITY_BADGE,
  type MaskRule,
  type SensitiveTable,
} from '@/api/maskRule'
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

// --- Types ---

interface DataSourceItem {
  id: number
  name: string
  type: string
  host: string
  port: number
  database: string
}

interface DataSourceListResponse {
  code: number
  data: DataSourceItem[]
}

type MaskSubTab = 'sensitive-tables' | 'field-rules'

// --- Shared: Datasource Fetch ---

function useDatasources() {
  const [datasources, setDatasources] = useState<DataSourceItem[]>([])
  const [loading, setLoading] = useState(false)

  const fetchDatasources = useCallback(async () => {
    setLoading(true)
    try {
      const res = await api.get<DataSourceListResponse>('/datasources')
      setDatasources(res.data ?? [])
    } catch {
      toast.error('获取数据源列表失败')
    } finally {
      setLoading(false)
    }
  }, [])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => { fetchDatasources() }, [fetchDatasources])
  /* eslint-enable react-hooks/set-state-in-effect */

  return { datasources, loading }
}

// --- Sensitive Tables Tab ---

function SensitiveTablesTab() {
  const { datasources } = useDatasources()
  const [tables, setTables] = useState<SensitiveTable[]>([])
  const [, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [filterDs, setFilterDs] = useState('')

  // Add dialog
  const [dialogOpen, setDialogOpen] = useState(false)
  const [form, setForm] = useState({
    datasource_id: '' as string,
    table_name: '',
    sensitivity_level: 'medium',
  })
  const [submitting, setSubmitting] = useState(false)
  const [errors, setErrors] = useState<Record<string, string>>({})

  // Datasource tables for dropdown
  const [dsTables, setDsTables] = useState<string[]>([])

  // Delete
  const [deleteTarget, setDeleteTarget] = useState<SensitiveTable | null>(null)
  const [deleting, setDeleting] = useState(false)

  const fetchTables = useCallback(async (p = 1) => {
    setLoading(true)
    try {
      const params: Record<string, string> = { page: String(p), page_size: '50' }
      if (filterDs) params.datasource_id = filterDs
      const res = await listSensitiveTables(params)
      setTables(res.data ?? [])
      setTotal(res.total)
      setPage(p)
    } catch {
      toast.error('获取敏感表列表失败')
    } finally {
      setLoading(false)
    }
  }, [filterDs])

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { fetchTables(1) }, [fetchTables])

  // Load tables for selected datasource in form
  useEffect(() => {
    if (!form.datasource_id) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDsTables([])
      return
    }
    const dsId = Number(form.datasource_id)
    fetchDatasourceTables(dsId)
      .then((res) => setDsTables(res.data ?? []))
      .catch(() => setDsTables([]))
  }, [form.datasource_id])

  function validate(): boolean {
    const errs: Record<string, string> = {}
    if (!form.datasource_id) errs.datasource_id = '请选择数据源'
    if (!form.table_name.trim()) errs.table_name = '请输入表名'
    setErrors(errs)
    return Object.keys(errs).length === 0
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!validate()) return
    setSubmitting(true)
    try {
      await createSensitiveTable({
        datasource_id: Number(form.datasource_id),
        database: '',
        table_name: form.table_name.trim(),
        sensitivity_level: form.sensitivity_level,
      })
      toast.success('敏感表标记成功')
      setDialogOpen(false)
      setForm({ datasource_id: '', table_name: '', sensitivity_level: 'medium' })
      fetchTables(1)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await deleteSensitiveTable(deleteTarget.id)
      toast.success('已取消敏感表标记')
      setDeleteTarget(null)
      fetchTables(page)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setDeleting(false)
    }
  }

  function getDatasourceName(id: number): string {
    const ds = datasources.find((d) => d.id === id)
    return ds ? `${ds.name} (${ds.type})` : `#${id}`
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-[var(--text-primary)]">敏感表标记</h2>
        <Button
          onClick={() => {
            setForm({ datasource_id: '', table_name: '', sensitivity_level: 'medium' })
            setErrors({})
            setDialogOpen(true)
          }}
          size="sm"
          className="gap-1 bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
        >
          <Plus size={14} />
          标记敏感表
        </Button>
      </div>

      {/* Filter */}
      <div className="flex items-center gap-3">
        <Label className="text-sm text-[var(--text-secondary)]">数据源筛选</Label>
        <Select value={filterDs} onValueChange={setFilterDs}>
          <SelectTrigger className="w-48 border-[var(--border-default)] bg-[var(--bg-elevated)]">
            <SelectValue placeholder="全部数据源" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">全部数据源</SelectItem>
            {datasources.map((ds) => (
              <SelectItem key={ds.id} value={String(ds.id)}>
                {ds.name} ({ds.type})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Table */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)]">
        <Table>
          <TableHeader>
            <TableRow className="border-[var(--border-default)] hover:bg-transparent">
              <TableHead className="text-[var(--text-secondary)]">数据源</TableHead>
              <TableHead className="text-[var(--text-secondary)]">表名</TableHead>
              <TableHead className="text-[var(--text-secondary)]">敏感</TableHead>
              <TableHead className="text-[var(--text-secondary)]">敏感等级</TableHead>
              <TableHead className="text-[var(--text-secondary)]">标记时间</TableHead>
              <TableHead className="text-[var(--text-secondary)]">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && !tables.length ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-[var(--text-muted)]">
                  加载中...
                </TableCell>
              </TableRow>
            ) : !tables.length ? (
              <TableRow>
                <TableCell colSpan={6} className="h-32">
                  <div className="flex flex-col items-center gap-2">
                    <ShieldAlert size={20} className="text-[var(--text-muted)]" />
                    <span className="text-sm text-[var(--text-muted)]">
                      暂无敏感表标记
                    </span>
                    <span className="text-xs text-[var(--text-muted)]">
                      点击上方「标记敏感表」按钮添加
                    </span>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              tables.map((t) => {
                const badge = SENSITIVITY_BADGE[t.sensitivity_level] ?? SENSITIVITY_BADGE.medium
                return (
                  <TableRow key={t.id} className="border-[var(--border-default)]">
                    <TableCell>
                      <span className="text-[var(--text-secondary)]">{getDatasourceName(t.datasource_id)}</span>
                    </TableCell>
                    <TableCell>
                      <span className="inline-flex items-center gap-1.5">
                        <ShieldAlert size={13} className="shrink-0 text-red-400" />
                        <span className="rounded bg-red-500/15 px-1.5 py-0.5 font-medium text-[var(--text-primary)]">{t.table_name}</span>
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="inline-flex items-center gap-1">
                        <ShieldAlert size={12} className="text-red-400" />
                        <span className="text-xs font-medium text-red-400">是</span>
                      </span>
                    </TableCell>
                    <TableCell>
                      <Badge className={badge.cls}>{badge.label}</Badge>
                    </TableCell>
                    <TableCell>
                      <span className="text-[var(--text-muted)]">{new Date(t.created_at).toLocaleString('zh-CN')}</span>
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost" size="sm"
                        className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-red-400"
                        onClick={() => setDeleteTarget(t)}
                      >
                        <Trash2 size={13} />
                        取消标记
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      {/* Add Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-[var(--text-primary)]">标记敏感表</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">数据源</Label>
              <Select
                value={form.datasource_id}
                onValueChange={(v) => setForm((f) => ({ ...f, datasource_id: v, table_name: '' }))}
              >
                <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                  <SelectValue placeholder="选择数据源" />
                </SelectTrigger>
                <SelectContent>
                  {datasources.map((ds) => (
                    <SelectItem key={ds.id} value={String(ds.id)}>
                      {ds.name} ({ds.type})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.datasource_id && <p className="text-xs text-red-400">{errors.datasource_id}</p>}
            </div>

            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">表名</Label>
              {dsTables.length > 0 ? (
                <Select
                  value={form.table_name}
                  onValueChange={(v) => setForm((f) => ({ ...f, table_name: v }))}
                >
                  <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                    <SelectValue placeholder="选择表" />
                  </SelectTrigger>
                  <SelectContent>
                    {dsTables.map((t) => (
                      <SelectItem key={t} value={t}>{t}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <>
                  <Input
                    value={form.table_name}
                    onChange={(e) => setForm((f) => ({ ...f, table_name: e.target.value }))}
                    placeholder="输入表名"
                    className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                  />
                  {form.datasource_id && (
                    <p className="text-xs text-[var(--text-muted)]">未获取到表列表，请手动输入表名</p>
                  )}
                </>
              )}
              {errors.table_name && <p className="text-xs text-red-400">{errors.table_name}</p>}
            </div>

            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">敏感等级</Label>
              <Select
                value={form.sensitivity_level}
                onValueChange={(v) => setForm((f) => ({ ...f, sensitivity_level: v }))}
              >
                <SelectTrigger className="w-40 border-[var(--border-default)] bg-[var(--bg-elevated)]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SENSITIVITY_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
                {submitting ? '提交中...' : '确认标记'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm */}
      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              取消敏感表标记
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要取消表「{deleteTarget?.table_name}」的敏感标记吗？取消后该表的查询将不再触发脱敏规则。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete} disabled={deleting}
              className="bg-red-600 text-white hover:bg-red-700"
            >
              {deleting ? '删除中...' : '确认取消'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// --- Field Mask Rules Tab ---

function FieldRulesTab() {
  const { datasources } = useDatasources()
  const [rules, setRules] = useState<MaskRule[]>([])
  const [, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [filterDs, setFilterDs] = useState('')

  // Add/Edit dialog
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState({
    datasource_id: '' as string,
    table_name: '',
    field: '',
    mask_type: 'phone',
    custom_regex: '',
    custom_template: '',
  })
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)

  // Datasource tables for dropdown
  const [dsTables, setDsTables] = useState<string[]>([])

  // Delete
  const [deleteTarget, setDeleteTarget] = useState<MaskRule | null>(null)
  const [deleting, setDeleting] = useState(false)

  const fetchRules = useCallback(async (p = 1) => {
    setLoading(true)
    try {
      const params: Record<string, string> = { page: String(p), page_size: '50' }
      if (filterDs) params.datasource_id = filterDs
      const res = await listMaskRules(params)
      setRules(res.data ?? [])
      setTotal(res.total)
      setPage(p)
    } catch {
      toast.error('获取脱敏规则列表失败')
    } finally {
      setLoading(false)
    }
  }, [filterDs])

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { fetchRules(1) }, [fetchRules])

  // Load tables for selected datasource in form
  useEffect(() => {
    if (!form.datasource_id) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDsTables([])
      return
    }
    const dsId = Number(form.datasource_id)
    fetchDatasourceTables(dsId)
      .then((res) => setDsTables(res.data ?? []))
      .catch(() => setDsTables([]))
  }, [form.datasource_id])

  function validate(): boolean {
    const errs: Record<string, string> = {}
    if (!editingId && !form.datasource_id) errs.datasource_id = '请选择数据源'
    if (!form.table_name.trim()) errs.table_name = '请输入表名'
    if (!form.field.trim()) errs.field = '请输入字段名'
    if (!form.mask_type) errs.mask_type = '请选择脱敏类型'
    if (form.mask_type === 'custom' && !form.custom_regex.trim()) {
      errs.custom_regex = '自定义正则类型必须提供正则表达式'
    }
    setErrors(errs)
    return Object.keys(errs).length === 0
  }

  function openAdd() {
    setEditingId(null)
    setForm({
      datasource_id: '', table_name: '', field: '',
      mask_type: 'phone', custom_regex: '', custom_template: '',
    })
    setErrors({})
    setDialogOpen(true)
  }

  function openEdit(rule: MaskRule) {
    setEditingId(rule.id)
    setForm({
      datasource_id: String(rule.datasource_id),
      table_name: rule.table_name,
      field: rule.field,
      mask_type: rule.mask_type,
      custom_regex: rule.custom_regex ?? '',
      custom_template: rule.custom_template ?? '',
    })
    setErrors({})
    setDialogOpen(true)
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!validate()) return
    setSubmitting(true)
    try {
      if (editingId) {
        await updateMaskRule(editingId, {
          table_name: form.table_name.trim(),
          field: form.field.trim(),
          mask_type: form.mask_type,
          custom_regex: form.mask_type === 'custom' ? form.custom_regex : '',
          custom_template: form.mask_type === 'custom' ? form.custom_template : '',
        })
        toast.success('脱敏规则更新成功')
      } else {
        await createMaskRule({
          datasource_id: Number(form.datasource_id),
          database: '',
          table_name: form.table_name.trim(),
          field: form.field.trim(),
          mask_type: form.mask_type,
          custom_regex: form.mask_type === 'custom' ? form.custom_regex : '',
          custom_template: form.mask_type === 'custom' ? form.custom_template : '',
        })
        toast.success('脱敏规则添加成功')
      }
      setDialogOpen(false)
      fetchRules(editingId ? page : 1)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await deleteMaskRule(deleteTarget.id)
      toast.success('脱敏规则已删除')
      setDeleteTarget(null)
      fetchRules(page)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setDeleting(false)
    }
  }

  function getDatasourceName(id: number): string {
    const ds = datasources.find((d) => d.id === id)
    return ds ? `${ds.name} (${ds.type})` : `#${id}`
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-[var(--text-primary)]">字段脱敏规则</h2>
        <Button
          onClick={openAdd}
          size="sm"
          className="gap-1 bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
        >
          <Plus size={14} />
          添加脱敏规则
        </Button>
      </div>

      {/* Filter */}
      <div className="flex items-center gap-3">
        <Label className="text-sm text-[var(--text-secondary)]">数据源筛选</Label>
        <Select value={filterDs} onValueChange={setFilterDs}>
          <SelectTrigger className="w-48 border-[var(--border-default)] bg-[var(--bg-elevated)]">
            <SelectValue placeholder="全部数据源" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">全部数据源</SelectItem>
            {datasources.map((ds) => (
              <SelectItem key={ds.id} value={String(ds.id)}>
                {ds.name} ({ds.type})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Table */}
      <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)]">
        <Table>
          <TableHeader>
            <TableRow className="border-[var(--border-default)] hover:bg-transparent">
              <TableHead className="text-[var(--text-secondary)]">数据源</TableHead>
              <TableHead className="text-[var(--text-secondary)]">表名</TableHead>
              <TableHead className="text-[var(--text-secondary)]">字段名</TableHead>
              <TableHead className="text-[var(--text-secondary)]">脱敏类型</TableHead>
              <TableHead className="text-[var(--text-secondary)]">自定义正则</TableHead>
              <TableHead className="text-[var(--text-secondary)]">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && !rules.length ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-[var(--text-muted)]">
                  加载中...
                </TableCell>
              </TableRow>
            ) : !rules.length ? (
              <TableRow>
                <TableCell colSpan={6} className="h-32">
                  <div className="flex flex-col items-center gap-2">
                    <FileWarning size={20} className="text-[var(--text-muted)]" />
                    <span className="text-sm text-[var(--text-muted)]">
                      暂无脱敏规则
                    </span>
                    <span className="text-xs text-[var(--text-muted)]">
                      点击上方「添加脱敏规则」按钮配置
                    </span>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              rules.map((r) => (
                <TableRow key={r.id} className="border-[var(--border-default)]">
                  <TableCell>
                    <span className="text-[var(--text-secondary)]">{getDatasourceName(r.datasource_id)}</span>
                  </TableCell>
                  <TableCell>
                    <span className="font-medium text-[var(--text-primary)]">{r.table_name}</span>
                  </TableCell>
                  <TableCell>
                    <span className="font-mono text-[var(--text-primary)]">{r.field}</span>
                  </TableCell>
                  <TableCell>
                    <Badge className="bg-violet-500/20 text-violet-400">{getMaskTypeLabel(r.mask_type)}</Badge>
                  </TableCell>
                  <TableCell>
                    {r.mask_type === 'custom' && r.custom_regex ? (
                      <span className="max-w-48 truncate font-mono text-xs text-[var(--text-muted)]" title={r.custom_regex}>
                        {r.custom_regex}
                      </span>
                    ) : (
                      <span className="text-[var(--text-muted)]">—</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost" size="sm"
                        className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                        onClick={() => openEdit(r)}
                      >
                        <Pencil size={13} />
                        编辑
                      </Button>
                      <Button
                        variant="ghost" size="sm"
                        className="h-7 gap-1 text-xs text-[var(--text-secondary)] hover:text-red-400"
                        onClick={() => setDeleteTarget(r)}
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
              {editingId ? '编辑脱敏规则' : '添加脱敏规则'}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            {/* Datasource (only for create) */}
            {!editingId && (
              <div className="space-y-1.5">
                <Label className="text-[var(--text-secondary)]">数据源</Label>
                <Select
                  value={form.datasource_id}
                  onValueChange={(v) => setForm((f) => ({ ...f, datasource_id: v, table_name: '' }))}
                >
                  <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                    <SelectValue placeholder="选择数据源" />
                  </SelectTrigger>
                  <SelectContent>
                    {datasources.map((ds) => (
                      <SelectItem key={ds.id} value={String(ds.id)}>
                        {ds.name} ({ds.type})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {errors.datasource_id && <p className="text-xs text-red-400">{errors.datasource_id}</p>}
              </div>
            )}

            {/* Table name */}
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">表名</Label>
              {!editingId && dsTables.length > 0 ? (
                <Select
                  value={form.table_name}
                  onValueChange={(v) => setForm((f) => ({ ...f, table_name: v }))}
                >
                  <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                    <SelectValue placeholder="选择表" />
                  </SelectTrigger>
                  <SelectContent>
                    {dsTables.map((t) => (
                      <SelectItem key={t} value={t}>{t}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  value={form.table_name}
                  onChange={(e) => setForm((f) => ({ ...f, table_name: e.target.value }))}
                  placeholder="输入表名"
                  className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
                />
              )}
              {errors.table_name && <p className="text-xs text-red-400">{errors.table_name}</p>}
            </div>

            {/* Field name */}
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">字段名</Label>
              <Input
                value={form.field}
                onChange={(e) => setForm((f) => ({ ...f, field: e.target.value }))}
                placeholder="输入字段名"
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
              {errors.field && <p className="text-xs text-red-400">{errors.field}</p>}
            </div>

            {/* Mask type */}
            <div className="space-y-1.5">
              <Label className="text-[var(--text-secondary)]">脱敏类型</Label>
              <Select
                value={form.mask_type}
                onValueChange={(v) => setForm((f) => ({ ...f, mask_type: v }))}
              >
                <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                  <SelectValue placeholder="选择脱敏类型" />
                </SelectTrigger>
                <SelectContent>
                  {MASK_TYPE_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.mask_type && <p className="text-xs text-red-400">{errors.mask_type}</p>}
            </div>

            {/* Custom regex fields (only when mask_type === 'custom') */}
            {form.mask_type === 'custom' && (
              <>
                <div className="space-y-1.5">
                  <Label className="text-[var(--text-secondary)]">正则表达式</Label>
                  <Input
                    value={form.custom_regex}
                    onChange={(e) => setForm((f) => ({ ...f, custom_regex: e.target.value }))}
                    placeholder="例如: (\d{3})\d{4}(\d{4})"
                    className="font-mono border-[var(--border-default)] bg-[var(--bg-elevated)]"
                  />
                  {errors.custom_regex && <p className="text-xs text-red-400">{errors.custom_regex}</p>}
                  <p className="text-xs text-[var(--text-muted)]">用于匹配需要脱敏的内容的正则表达式</p>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-[var(--text-secondary)]">替换模板</Label>
                  <Input
                    value={form.custom_template}
                    onChange={(e) => setForm((f) => ({ ...f, custom_template: e.target.value }))}
                    placeholder="例如: $1****$2（留空则全掩码）"
                    className="font-mono border-[var(--border-default)] bg-[var(--bg-elevated)]"
                  />
                  <p className="text-xs text-[var(--text-muted)]">
                    使用 $1, $2... 引用正则中的捕获组，留空则用星号替换全部
                  </p>
                </div>
              </>
            )}

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

      {/* Delete Confirm */}
      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
      >
        <AlertDialogContent className="border-[var(--border-default)] bg-[var(--bg-surface)]">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-[var(--text-primary)]">
              删除脱敏规则
            </AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--text-secondary)]">
              确定要删除表「{deleteTarget?.table_name}」字段「{deleteTarget?.field}」的脱敏规则吗？删除后该字段将不再进行脱敏。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-[var(--border-default)]">取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete} disabled={deleting}
              className="bg-red-600 text-white hover:bg-red-700"
            >
              {deleting ? '删除中...' : '确认删除'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// --- Main MaskRulesTab ---

export default function MaskRulesTab() {
  const [subTab, setSubTab] = useState<MaskSubTab>('sensitive-tables')

  return (
    <div className="space-y-4">
      {/* Sub-tabs */}
      <div className="flex items-center gap-1 rounded-lg bg-[var(--bg-elevated)] p-1 w-fit">
        <button
          onClick={() => setSubTab('sensitive-tables')}
          className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors duration-150 ${
            subTab === 'sensitive-tables'
              ? 'bg-[var(--accent-primary)] text-white shadow-sm'
              : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-hover)]'
          }`}
        >
          <ShieldAlert size={14} />
          敏感表标记
        </button>
        <button
          onClick={() => setSubTab('field-rules')}
          className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors duration-150 ${
            subTab === 'field-rules'
              ? 'bg-[var(--accent-primary)] text-white shadow-sm'
              : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-hover)]'
          }`}
        >
          <ShieldCheck size={14} />
          字段规则
        </button>
      </div>

      {/* Content */}
      {subTab === 'sensitive-tables' && <SensitiveTablesTab />}
      {subTab === 'field-rules' && <FieldRulesTab />}
    </div>
  )
}
