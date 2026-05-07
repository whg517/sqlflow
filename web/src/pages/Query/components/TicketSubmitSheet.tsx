import { useState } from 'react'
import { Loader2, Send } from 'lucide-react'
import { toast } from 'sonner'
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription, SheetFooter,
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/api/client'
import type { AIReviewResult } from '@/api/query'

// --- Props ---

interface TicketSubmitSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  sql: string
  datasourceId: number | null
  database: string
  dbType: string
  reviewResult: AIReviewResult | null
  onSubmitSuccess: (ticketId: number) => void
}

// --- Component ---

export default function TicketSubmitSheet({
  open,
  onOpenChange,
  sql,
  datasourceId,
  database,
  dbType,
  reviewResult,
  onSubmitSuccess,
}: TicketSubmitSheetProps) {
  const [reason, setReason] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit() {
    if (!reason.trim()) {
      toast.error('请填写变更原因')
      return
    }
    if (!datasourceId) {
      toast.error('数据源不存在')
      return
    }

    setSubmitting(true)
    try {
      const res = await api.post<{ code: number; message: string; data: { id: number } }>(
        '/tickets',
        {
          datasource_id: datasourceId,
          database,
          sql,
          db_type: dbType || 'mysql',
          change_reason: reason.trim(),
          risk_level: reviewResult?.risk_level || 'medium',
          ai_review_result: reviewResult ? JSON.stringify(reviewResult) : '',
        },
      )
      toast.success('工单提交成功')
      setReason('')
      onOpenChange(false)
      onSubmitSuccess(res.data.id)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '提交失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-[420px] border-[var(--border-default)] bg-[var(--bg-surface)] sm:max-w-[420px]"
      >
        <SheetHeader>
          <SheetTitle className="text-[var(--text-primary)]">提交变更工单</SheetTitle>
          <SheetDescription className="text-[var(--text-secondary)]">
            高风险操作需通过工单审批流程，请填写变更原因
          </SheetDescription>
        </SheetHeader>

        <div className="flex-1 space-y-4 overflow-y-auto px-4">
          {/* SQL preview */}
          <div>
            <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
              SQL 内容
            </label>
            <pre className="max-h-32 overflow-auto rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] p-2 text-xs text-[var(--text-muted)]">
              {sql}
            </pre>
          </div>

          {/* Risk level */}
          {reviewResult && (
            <div>
              <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
                风险等级
              </label>
              <span className={`text-xs font-medium ${
                reviewResult.risk_level === 'high' ? 'text-red-400' :
                reviewResult.risk_level === 'medium' ? 'text-yellow-400' : 'text-green-400'
              }`}>
                {reviewResult.risk_level === 'high' ? '高风险' :
                 reviewResult.risk_level === 'medium' ? '中风险' : '低风险'}
                {' '}({reviewResult.risk_score})
              </span>
            </div>
          )}

          {/* Change reason */}
          <div>
            <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
              变更原因 <span className="text-red-400">*</span>
            </label>
            <Textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="请说明此次变更的原因和预期影响..."
              className="min-h-[100px] border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)]"
            />
          </div>

          {/* AI review summary */}
          {reviewResult?.summary && (
            <div>
              <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
                AI 评审摘要
              </label>
              <p className="text-xs text-[var(--text-muted)]">{reviewResult.summary}</p>
            </div>
          )}

          {reviewResult?.suggestions && reviewResult.suggestions.length > 0 && (
            <div>
              <label className="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]">
                建议
              </label>
              <ul className="space-y-1 pl-4">
                {reviewResult.suggestions.map((s, i) => (
                  <li key={i} className="text-xs text-[var(--text-muted)] list-disc">{s}</li>
                ))}
              </ul>
            </div>
          )}
        </div>

        <SheetFooter>
          <Button
            variant="ghost"
            size="sm"
            className="text-xs text-[var(--text-muted)]"
            onClick={() => onOpenChange(false)}
            disabled={submitting}
          >
            取消
          </Button>
          <Button
            size="sm"
            className="h-8 gap-1.5 bg-[var(--accent-primary)] px-4 text-xs text-white hover:bg-[var(--accent-hover)]"
            onClick={handleSubmit}
            disabled={submitting || !reason.trim()}
          >
            {submitting ? (
              <>
                <Loader2 size={12} className="animate-spin" />
                提交中...
              </>
            ) : (
              <>
                <Send size={12} />
                提交工单
              </>
            )}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
