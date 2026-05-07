import { useState } from 'react'
import { Loader2, Shield, ShieldAlert, ShieldCheck, ChevronDown, ChevronUp, Sparkles } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { AIReviewResult, ReviewDecision } from '@/api/query'

// --- Risk config ---

const riskConfig: Record<string, {
  label: string
  color: string
  bgColor: string
  borderColor: string
  icon: typeof ShieldCheck
  badgeClass: string
}> = {
  low: {
    label: '低风险',
    color: 'text-green-400',
    bgColor: 'bg-green-500/10',
    borderColor: 'border-green-500/30',
    icon: ShieldCheck,
    badgeClass: 'bg-green-500/20 text-green-400 border-green-500/30',
  },
  medium: {
    label: '中风险',
    color: 'text-yellow-400',
    bgColor: 'bg-yellow-500/10',
    borderColor: 'border-yellow-500/30',
    icon: Shield,
    badgeClass: 'bg-yellow-500/20 text-yellow-400 border-yellow-500/30',
  },
  high: {
    label: '高风险',
    color: 'text-red-400',
    bgColor: 'bg-red-500/10',
    borderColor: 'border-red-500/30',
    icon: ShieldAlert,
    badgeClass: 'bg-red-500/20 text-red-400 border-red-500/30',
  },
}

// --- Props ---

interface AIReviewCardProps {
  status: 'idle' | 'reviewing' | 'done' | 'error'
  result: AIReviewResult | null
  streamingContent: string
  error: string | null
  onConfirm: () => void
  onAutoExecute: () => void
  onSubmitTicket: () => void
  onDismiss: () => void
}

// --- Component ---

export default function AIReviewCard({
  status,
  result,
  streamingContent,
  error,
  onConfirm,
  onAutoExecute,
  onSubmitTicket,
  onDismiss,
}: AIReviewCardProps) {
  const [suggestionsOpen, setSuggestionsOpen] = useState(false)

  if (status === 'idle') return null

  // Reviewing state — pulsing animation with streaming content
  if (status === 'reviewing') {
    return (
      <div className="border-b border-[var(--border-default)] bg-[var(--bg-surface)]">
        <div className="flex items-center gap-2 px-4 py-2.5">
          <div className="relative">
            <Sparkles size={16} className="animate-pulse text-[var(--accent-primary)]" />
          </div>
          <span className="text-xs font-medium text-[var(--text-primary)]">AI 评审中...</span>
          <Loader2 size={12} className="animate-spin text-[var(--text-muted)]" />
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            取消
          </Button>
        </div>
        {streamingContent && (
          <div className="border-t border-[var(--border-default)] px-4 py-2">
            <ScrollArea className="max-h-24">
              <pre className="whitespace-pre-wrap text-xs text-[var(--text-muted)] font-mono">
                {streamingContent}
              </pre>
            </ScrollArea>
          </div>
        )}
      </div>
    )
  }

  // Error state
  if (status === 'error') {
    return (
      <div className="border-b border-[var(--border-default)] bg-red-500/5">
        <div className="flex items-center gap-2 px-4 py-2.5">
          <ShieldAlert size={16} className="text-red-400" />
          <span className="text-xs text-red-400">
            {error || 'AI 评审失败'}
          </span>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            关闭
          </Button>
        </div>
      </div>
    )
  }

  // Done state — show result card
  if (!result) return null

  const risk = riskConfig[result.risk_level] || riskConfig.medium
  const RiskIcon = risk.icon

  return (
    <div className={`border-b ${risk.borderColor} ${risk.bgColor}`}>
      {/* Header */}
      <div className="flex items-center gap-2 px-4 py-2.5">
        <RiskIcon size={16} className={risk.color} />
        <span className={`text-xs font-medium ${risk.color}`}>
          AI 评审完成
        </span>
        <Badge variant="outline" className={`text-[10px] ${risk.badgeClass}`}>
          {risk.label} ({result.risk_score})
        </Badge>
        {result.review_source !== 'ai' && (
          <Badge variant="outline" className="text-[10px] bg-[var(--bg-elevated)] text-[var(--text-muted)] border-[var(--border-default)]">
            {result.review_source === 'static' ? '静态规则' : '降级模式'}
          </Badge>
        )}
        <div className="flex-1" />
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-2 text-xs text-[var(--text-muted)]"
          onClick={onDismiss}
        >
          关闭
        </Button>
      </div>

      {/* Summary */}
      <div className="px-4 pb-2">
        <p className="text-xs text-[var(--text-secondary)]">{result.summary}</p>
      </div>

      {/* Suggestions (collapsible) */}
      {result.suggestions && result.suggestions.length > 0 && (
        <div className="px-4 pb-2">
          <button
            className="flex items-center gap-1 text-xs text-[var(--accent-primary)] hover:underline"
            onClick={() => setSuggestionsOpen(!suggestionsOpen)}
          >
            {suggestionsOpen ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
            {result.suggestions.length} 条建议
          </button>
          {suggestionsOpen && (
            <ul className="mt-1.5 space-y-1 pl-4">
              {result.suggestions.map((s, i) => (
                <li key={i} className="text-xs text-[var(--text-secondary)] list-disc">
                  {s}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {/* Impact analysis */}
      {result.impact_analysis && (
        <div className="px-4 pb-2">
          <span className="text-xs font-medium text-[var(--text-secondary)]">影响分析: </span>
          <span className="text-xs text-[var(--text-muted)]">{result.impact_analysis}</span>
        </div>
      )}

      {/* Warnings */}
      {result.warnings && result.warnings.length > 0 && (
        <div className="px-4 pb-2">
          {result.warnings.map((w, i) => (
            <div key={i} className="text-xs text-yellow-400">
              ⚠ {w}
            </div>
          ))}
        </div>
      )}

      <Separator className="bg-[var(--border-default)]" />

      {/* Action buttons based on decision */}
      <div className="flex items-center gap-2 px-4 py-2">
        <ActionButtons
          decision={result.decision}
          riskLevel={result.risk_level}
          onConfirm={onConfirm}
          onAutoExecute={onAutoExecute}
          onSubmitTicket={onSubmitTicket}
          onDismiss={onDismiss}
        />
      </div>
    </div>
  )
}

// --- Action Buttons Sub-component ---

function ActionButtons({
  decision,
  riskLevel,
  onConfirm,
  onAutoExecute,
  onSubmitTicket,
  onDismiss,
}: {
  decision: ReviewDecision
  riskLevel: string
  onConfirm: () => void
  onAutoExecute: () => void
  onSubmitTicket: () => void
  onDismiss: () => void
}) {
  switch (decision) {
    case 'execute':
      return (
        <>
          <span className="text-xs text-green-400">
            低风险查询，即将自动执行...
          </span>
          <div className="flex-1" />
          <Button
            size="sm"
            className="h-6 gap-1 bg-green-600 px-3 text-xs text-white hover:bg-green-700"
            onClick={onAutoExecute}
          >
            立即执行
          </Button>
        </>
      )
    case 'confirm':
      return (
        <>
          <span className="text-xs text-yellow-400">
            {riskLevel === 'high'
              ? '高风险查询，请确认后执行'
              : '需要确认后执行'}
          </span>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            取消
          </Button>
          <Button
            size="sm"
            className="h-6 gap-1 bg-[var(--accent-primary)] px-3 text-xs text-white hover:bg-[var(--accent-hover)]"
            onClick={onConfirm}
          >
            确认执行
          </Button>
        </>
      )
    case 'ticket':
      return (
        <>
          <span className="text-xs text-red-400">
            高风险操作，需提交工单审批
          </span>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            取消
          </Button>
          <Button
            size="sm"
            className="h-6 gap-1 bg-red-600 px-3 text-xs text-white hover:bg-red-700"
            onClick={onSubmitTicket}
          >
            提交工单
          </Button>
        </>
      )
    case 'blocked':
      return (
        <>
          <span className="text-xs text-red-400">
            操作被安全规则拦截，禁止执行
          </span>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            关闭
          </Button>
        </>
      )
    default:
      return (
        <>
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs text-[var(--text-muted)]"
            onClick={onDismiss}
          >
            关闭
          </Button>
        </>
      )
  }
}
