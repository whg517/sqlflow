import { useState, useEffect, useCallback, useRef } from 'react'
import { toast } from 'sonner'
import { Clock } from 'lucide-react'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { api } from '@/api/client'
import { executeQuery, streamAIReview, buildMongoSql, type MongoQueryBody, type AIReviewResult } from '@/api/query'
import { useQueryStore, type MongoOperation } from '@/store/queryStore'
import SqlEditor from './components/SqlEditor'
import MongoEditor from './components/MongoEditor'
import ResultTable from './components/ResultTable'
import QueryTabs from './components/QueryTabs'
import HistoryPanel from './components/HistoryPanel'
import StatusBar from './components/StatusBar'
import ResizableSplit from './components/ResizableSplit'
import AIReviewCard from './components/AIReviewCard'
import TicketSubmitSheet from './components/TicketSubmitSheet'

// --- Types ---

interface DataSourceOption {
  id: number
  name: string
  type: string
  status: string
}

interface DataSourceListResponse {
  code: number
  data: DataSourceOption[]
}

// --- Main Page ---

export default function QueryPage() {
  const tabs = useQueryStore((s) => s.tabs)
  const activeTabId = useQueryStore((s) => s.activeTabId)
  const splitRatio = useQueryStore((s) => s.splitRatio)
  const historyOpen = useQueryStore((s) => s.historyOpen)
  const setSplitRatio = useQueryStore((s) => s.setSplitRatio)
  const updateTabSql = useQueryStore((s) => s.updateTabSql)
  const updateTabDatasource = useQueryStore((s) => s.updateTabDatasource)
  const setTabResult = useQueryStore((s) => s.setTabResult)
  const setTabExecuting = useQueryStore((s) => s.setTabExecuting)
  const setHistoryOpen = useQueryStore((s) => s.setHistoryOpen)
  const updateMongoField = useQueryStore((s) => s.updateMongoField)
  const setAIReviewStatus = useQueryStore((s) => s.setAIReviewStatus)
  const setAIReviewResult = useQueryStore((s) => s.setAIReviewResult)
  const appendAIReviewContent = useQueryStore((s) => s.appendAIReviewContent)
  const setAIReviewError = useQueryStore((s) => s.setAIReviewError)
  const clearAIReview = useQueryStore((s) => s.clearAIReview)

  const [datasources, setDatasources] = useState<DataSourceOption[]>([])
  const [ticketSheetOpen, setTicketSheetOpen] = useState(false)

  const cancelReviewRef = useRef<(() => void) | null>(null)

  const activeTab = tabs.find((t) => t.id === activeTabId) ?? tabs[0]
  const isMongo = activeTab?.datasourceType === 'mongodb'

  // Load datasources
  useEffect(() => {
    api.get<DataSourceListResponse>('/datasources').then((res) => {
      const list = (res.data ?? []).filter((ds) => ds.status === 'active')
      setDatasources(list)
      // Auto-select first datasource if tab has none
      if (list.length > 0) {
        for (const tab of tabs) {
          if (!tab.datasourceId) {
            const ds = list[0]
            updateTabDatasource(tab.id, ds.id, '', ds.type)
          }
        }
      }
    }).catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Cleanup review stream on unmount
  useEffect(() => {
    return () => {
      cancelReviewRef.current?.()
    }
  }, [])

  const buildMongoQuerySql = useCallback((): string | null => {
    if (!activeTab) return null

    let filter: Record<string, unknown>
    let options: Record<string, unknown>
    try {
      filter = JSON.parse(activeTab.mongoFilter || '{}')
    } catch {
      toast.error('Filter JSON 格式错误')
      return null
    }
    try {
      options = JSON.parse(activeTab.mongoOptions || '{}')
    } catch {
      toast.error('Options JSON 格式错误')
      return null
    }

    if (!activeTab.mongoCollection.trim()) {
      toast.error('请输入集合名')
      return null
    }

    const body: MongoQueryBody = {
      collection: activeTab.mongoCollection.trim(),
      operation: activeTab.mongoOperation === 'update' ? 'update' : activeTab.mongoOperation,
    }

    if (activeTab.mongoOperation === 'aggregate') {
      body.pipeline = Array.isArray(filter) ? filter : []
    } else {
      body.filter = filter
    }

    if (options && Object.keys(options).length > 0) {
      body.options = options
    }

    return buildMongoSql(body)
  }, [activeTab])

  // Execute query directly (bypasses AI review — used for confirmed/low-risk)
  const doExecute = useCallback(async (sql: string) => {
    if (!activeTab?.datasourceId) return

    setTabExecuting(activeTab.id, true)
    try {
      const result = await executeQuery({
        datasource_id: activeTab.datasourceId,
        database: activeTab.database,
        sql,
      })
      setTabResult(activeTab.id, result)
      clearAIReview(activeTab.id)
      if (!activeTab.result) {
        setSplitRatio(0.5)
      }
    } catch (err) {
      setTabResult(activeTab.id, null, err instanceof Error ? err.message : '查询执行失败')
      toast.error(err instanceof Error ? err.message : '查询执行失败')
    }
  }, [activeTab, setTabExecuting, setTabResult, setSplitRatio, clearAIReview])

  // Start AI review + execute flow
  const handleExecute = useCallback(async () => {
    if (!activeTab?.datasourceId) return

    let sql: string
    if (isMongo) {
      if (activeTab.mongoOperation === 'update') {
        toast.error('MongoDB update 操作需要通过变更工单提交')
        return
      }
      const mongoSql = buildMongoQuerySql()
      if (!mongoSql) return
      sql = mongoSql
    } else {
      if (!activeTab.sql.trim()) return
      sql = activeTab.sql.trim()
    }

    // Cancel any previous review
    cancelReviewRef.current?.()
    cancelReviewRef.current = null

    // Clear previous review state and start reviewing
    clearAIReview(activeTab.id)
    setAIReviewStatus(activeTab.id, 'reviewing')

    const cancel = streamAIReview(
      {
        datasource_id: activeTab.datasourceId,
        database: activeTab.database,
        sql,
      },
      (event) => {
        if (event.type === 'content') {
          appendAIReviewContent(activeTab.id, String(event.data))
        } else if (event.type === 'result') {
          const result = event.data as AIReviewResult
          setAIReviewResult(activeTab.id, result)

          // Auto-execute for low risk
          if (result.decision === 'execute') {
            setTimeout(() => {
              doExecute(sql)
            }, 1000)
          }
        } else if (event.type === 'error') {
          setAIReviewError(activeTab.id, String(event.data))
        }
      },
      (err) => {
        setAIReviewError(activeTab.id, err.message)
      },
    )

    cancelReviewRef.current = cancel
  }, [activeTab, isMongo, buildMongoQuerySql, clearAIReview, setAIReviewStatus, appendAIReviewContent, setAIReviewResult, setAIReviewError, doExecute])

  // Handle confirmed execute (medium risk)
  const handleConfirmExecute = useCallback(() => {
    if (!activeTab) return
    const sql = isMongo ? buildMongoQuerySql() || '' : activeTab.sql.trim()
    if (sql) {
      doExecute(sql)
    }
  }, [activeTab, isMongo, buildMongoQuerySql, doExecute])

  // Handle auto-execute trigger from low-risk
  const handleAutoExecute = useCallback(() => {
    if (!activeTab) return
    const sql = isMongo ? buildMongoQuerySql() || '' : activeTab.sql.trim()
    if (sql) {
      doExecute(sql)
    }
  }, [activeTab, isMongo, buildMongoQuerySql, doExecute])

  // Handle submit ticket (high risk)
  const handleSubmitTicket = useCallback(() => {
    setTicketSheetOpen(true)
  }, [])

  // Handle dismiss review
  const handleDismissReview = useCallback(() => {
    cancelReviewRef.current?.()
    cancelReviewRef.current = null
    clearAIReview(activeTab.id)
  }, [activeTab, clearAIReview])

  // Handle ticket submit success
  const handleTicketSuccess = useCallback((ticketId: number) => {
    toast.success(`工单 #${ticketId} 已提交，等待审批`)
    clearAIReview(activeTab.id)
  }, [activeTab, clearAIReview])

  const currentSql = isMongo ? '' : (activeTab?.sql ?? '')

  return (
    <div className="flex h-full flex-col">
      {/* Toolbar: datasource selectors + history button */}
      <div className="flex items-center gap-2 border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-3 py-2">
        <Select
          value={activeTab?.datasourceId ? String(activeTab.datasourceId) : ''}
          onValueChange={(v) => {
            const ds = datasources.find((d) => d.id === Number(v))
            updateTabDatasource(activeTab.id, Number(v), ds?.type === 'mysql' ? activeTab.database : activeTab.database, ds?.type ?? '')
          }}
        >
          <SelectTrigger className="h-7 w-48 border-[var(--border-default)] bg-[var(--bg-elevated)] text-xs">
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

        <input
          type="text"
          value={activeTab?.database ?? ''}
          onChange={(e) => updateTabDatasource(activeTab.id, activeTab.datasourceId, e.target.value, activeTab.datasourceType)}
          placeholder={isMongo ? 'Database' : '数据库名'}
          className="h-7 w-36 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-2 text-xs text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent-primary)]"
        />

        {/* DB type indicator */}
        {isMongo && (
          <span className="rounded bg-green-500/20 px-1.5 py-0.5 text-[10px] font-medium text-green-400">
            MongoDB
          </span>
        )}

        <div className="flex-1" />

        {/* History toggle */}
        <div className="relative" data-history-panel>
          <Button
            variant="ghost"
            size="sm"
            className={`h-7 gap-1 px-2 text-xs ${historyOpen ? 'text-[var(--accent-primary)]' : 'text-[var(--text-secondary)]'}`}
            onClick={() => setHistoryOpen(!historyOpen)}
          >
            <Clock size={14} />
            历史
          </Button>
          <HistoryPanel />
        </div>
      </div>

      {/* Tabs */}
      <QueryTabs />

      {/* AI Review Card (between editor and result) */}
      <AIReviewCard
        status={activeTab?.aiReviewStatus ?? 'idle'}
        result={activeTab?.aiReviewResult ?? null}
        streamingContent={activeTab?.aiReviewContent ?? ''}
        error={activeTab?.aiReviewError ?? null}
        onConfirm={handleConfirmExecute}
        onAutoExecute={handleAutoExecute}
        onSubmitTicket={handleSubmitTicket}
        onDismiss={handleDismissReview}
      />

      {/* Main editor + result area */}
      <div className="flex-1 overflow-hidden">
        <ResizableSplit
          ratio={splitRatio}
          onRatioChange={setSplitRatio}
          top={
            isMongo ? (
              <MongoEditor
                key={`mongo-${activeTab?.id}`}
                collection={activeTab?.mongoCollection ?? ''}
                operation={activeTab?.mongoOperation ?? 'find'}
                filter={activeTab?.mongoFilter ?? '{}'}
                options={activeTab?.mongoOptions ?? '{}'}
                onCollectionChange={(v) => updateMongoField(activeTab.id, { mongoCollection: v })}
                onOperationChange={(v: MongoOperation) => updateMongoField(activeTab.id, { mongoOperation: v })}
                onFilterChange={(v) => updateMongoField(activeTab.id, { mongoFilter: v })}
                onOptionsChange={(v) => updateMongoField(activeTab.id, { mongoOptions: v })}
                onExecute={handleExecute}
              />
            ) : (
              <SqlEditor
                key={activeTab?.id}
                value={activeTab?.sql ?? ''}
                onChange={(sql) => updateTabSql(activeTab.id, sql)}
                onExecute={handleExecute}
              />
            )
          }
          bottom={
            <div className="flex h-full flex-col">
              <div className="flex-1 overflow-hidden">
                <ResultTable result={activeTab?.result ?? null} />
              </div>
              <StatusBar
                executing={activeTab?.executing ?? false}
                error={activeTab?.error ?? null}
                result={activeTab?.result ?? null}
                datasourceId={activeTab?.datasourceId ?? null}
                database={activeTab?.database ?? ''}
                sql={currentSql}
                onExecute={handleExecute}
                isMongo={isMongo}
                mongoCollection={activeTab?.mongoCollection ?? ''}
              />
            </div>
          }
        />
      </div>

      {/* Ticket submission sheet for high-risk operations */}
      <TicketSubmitSheet
        open={ticketSheetOpen}
        onOpenChange={setTicketSheetOpen}
        sql={currentSql}
        datasourceId={activeTab?.datasourceId ?? null}
        database={activeTab?.database ?? ''}
        dbType={activeTab?.datasourceType ?? ''}
        reviewResult={activeTab?.aiReviewResult ?? null}
        onSubmitSuccess={handleTicketSuccess}
      />
    </div>
  )
}
