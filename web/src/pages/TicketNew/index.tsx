import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft, Loader2, Send } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { api } from '@/api/client'
import { createTicket } from '@/api/ticket'

// --- Types ---

interface DataSourceOption {
  id: number
  name: string
  type: string
  status: string
}

interface DatasourceListResponse {
  code: number
  data: DataSourceOption[]
}

// --- Component ---

export default function TicketNewPage() {
  const navigate = useNavigate()

  const [datasources, setDatasources] = useState<DataSourceOption[]>([])
  const [submitting, setSubmitting] = useState(false)

  // Form state
  const [datasourceId, setDatasourceId] = useState<string>('')
  const [database, setDatabase] = useState('')
  const [sql, setSql] = useState('')
  const [changeReason, setChangeReason] = useState('')

  // Validation errors
  const [errors, setErrors] = useState<Record<string, string>>({})

  // Load datasources
  useEffect(() => {
    api.get<DatasourceListResponse>('/datasources').then((res) => {
      setDatasources((res.data ?? []).filter((ds) => ds.status === 'active'))
    }).catch(() => {})
  }, [])

  function validate(): boolean {
    const errs: Record<string, string> = {}
    if (!datasourceId) errs.datasourceId = '请选择数据源'
    if (!sql.trim()) errs.sql = '请输入 SQL'
    if (!changeReason.trim()) errs.changeReason = '请填写变更原因'
    else if (changeReason.trim().length < 10) errs.changeReason = '变更原因至少 10 个字符'
    setErrors(errs)
    return Object.keys(errs).length === 0
  }

  async function handleSubmit() {
    if (!validate()) return

    const ds = datasources.find((d) => d.id === Number(datasourceId))
    setSubmitting(true)
    try {
      const res = await createTicket({
        datasource_id: Number(datasourceId),
        database,
        sql: sql.trim(),
        db_type: ds?.type === 'mongodb' ? 'mongodb' : 'mysql',
        change_reason: changeReason.trim(),
      })
      toast.success('工单提交成功')
      navigate(`/tickets`)
      // Optionally open detail
      void res.data.id
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '提交失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center gap-3 border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6 py-3">
        <Button
          variant="ghost"
          size="sm"
          className="h-7 w-7 p-0 text-[var(--text-secondary)]"
          onClick={() => navigate('/tickets')}
        >
          <ArrowLeft size={16} />
        </Button>
        <h1 className="text-base font-semibold text-[var(--text-primary)]">提交新工单</h1>
      </div>

      {/* Form */}
      <div className="flex-1 overflow-auto">
        <div className="mx-auto max-w-[680px] space-y-6 px-6 py-6">
          {/* Datasource + Database */}
          <div className="space-y-4">
            <div>
              <Label className="mb-1.5 text-xs font-medium text-[var(--text-secondary)]">
                数据源 <span className="text-red-400">*</span>
              </Label>
              <Select value={datasourceId} onValueChange={setDatasourceId}>
                <SelectTrigger className={`h-9 w-full border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm text-[var(--text-primary)] ${errors.datasourceId ? 'border-red-500' : ''}`}>
                  <SelectValue placeholder="选择数据源" />
                </SelectTrigger>
                <SelectContent>
                  {datasources.map((ds) => (
                    <SelectItem key={ds.id} value={String(ds.id)}>
                      <span className="flex items-center gap-2">
                        <span className={`inline-block h-1.5 w-1.5 rounded-full ${ds.type === 'mysql' ? 'bg-blue-400' : 'bg-green-400'}`} />
                        {ds.name}
                        <span className="text-[var(--text-muted)]">({ds.type})</span>
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.datasourceId && <p className="mt-1 text-xs text-red-400">{errors.datasourceId}</p>}
            </div>

            <div>
              <Label className="mb-1.5 text-xs font-medium text-[var(--text-secondary)]">
                数据库名
              </Label>
              <Input
                value={database}
                onChange={(e) => setDatabase(e.target.value)}
                placeholder="输入数据库名（可选）"
                className="h-9 border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
              />
            </div>
          </div>

          <Separator className="bg-[var(--border-default)]" />

          {/* SQL Editor */}
          <div>
            <Label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
              SQL 内容 <span className="text-red-400">*</span>
            </Label>
            <Textarea
              value={sql}
              onChange={(e) => setSql(e.target.value)}
              placeholder="输入要执行的 SQL 语句..."
              className={`min-h-[200px] font-mono border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] ${errors.sql ? 'border-red-500' : ''}`}
            />
            {errors.sql && <p className="mt-1 text-xs text-red-400">{errors.sql}</p>}
          </div>

          <Separator className="bg-[var(--border-default)]" />

          {/* Change Reason */}
          <div>
            <Label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
              变更原因 <span className="text-red-400">*</span>
            </Label>
            <Textarea
              value={changeReason}
              onChange={(e) => setChangeReason(e.target.value)}
              placeholder="请说明此次变更的原因和预期影响（至少 10 个字符）..."
              className={`min-h-[100px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] ${errors.changeReason ? 'border-red-500' : ''}`}
            />
            {errors.changeReason && <p className="mt-1 text-xs text-red-400">{errors.changeReason}</p>}
            <p className="mt-1 text-xs text-[var(--text-muted)]">
              {changeReason.length}/500
            </p>
          </div>

          {/* Submit */}
          <div className="flex items-center justify-end gap-3 pt-2">
            <Button
              variant="ghost"
              size="sm"
              className="text-xs text-[var(--text-muted)]"
              onClick={() => navigate('/tickets')}
              disabled={submitting}
            >
              取消
            </Button>
            <Button
              size="sm"
              className="h-9 gap-1.5 bg-[var(--accent-primary)] px-5 text-sm text-white hover:bg-[var(--accent-hover)]"
              onClick={handleSubmit}
              disabled={submitting}
            >
              {submitting ? (
                <>
                  <Loader2 size={14} className="animate-spin" />
                  提交中...
                </>
              ) : (
                <>
                  <Send size={14} />
                  提交工单
                </>
              )}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
