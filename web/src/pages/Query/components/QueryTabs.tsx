import { X, Plus } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useQueryStore } from '@/store/queryStore'

export default function QueryTabs() {
  const tabs = useQueryStore((s) => s.tabs)
  const activeTabId = useQueryStore((s) => s.activeTabId)
  const setActiveTab = useQueryStore((s) => s.setActiveTab)
  const addTab = useQueryStore((s) => s.addTab)
  const removeTab = useQueryStore((s) => s.removeTab)

  return (
    <div className="flex items-center border-b border-[var(--border-default)] bg-[var(--bg-surface)]">
      <div className="flex flex-1 items-center overflow-x-auto">
        {tabs.map((tab) => (
          <div
            key={tab.id}
            className={`group flex shrink-0 cursor-pointer items-center gap-1.5 border-r border-[var(--border-default)] px-3 py-1.5 text-xs transition-colors ${
              tab.id === activeTabId
                ? 'border-b-2 border-b-[var(--accent-primary)] bg-[var(--bg-base)] font-medium text-[var(--text-primary)]'
                : 'text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)]/50 hover:text-[var(--text-primary)]'
            }`}
            onClick={() => setActiveTab(tab.id)}
          >
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="max-w-[120px] truncate">{tab.title}</span>
              </TooltipTrigger>
              {tab.sql.trim().length > 20 && (
                <TooltipContent side="bottom" className="max-w-[400px] whitespace-pre-wrap font-mono text-xs">
                  {tab.sql.trim().substring(0, 200)}
                </TooltipContent>
              )}
            </Tooltip>
            {tab.dirty && (
              <span className="h-1.5 w-1.5 rounded-full bg-[var(--accent-primary)]" />
            )}
            {tabs.length > 1 && (
              <button
                className="ml-0.5 rounded p-0.5 opacity-0 transition-opacity hover:bg-[var(--bg-elevated)] group-hover:opacity-100"
                onClick={(e) => {
                  e.stopPropagation()
                  removeTab(tab.id)
                }}
              >
                <X size={10} />
              </button>
            )}
          </div>
        ))}
      </div>
      <button
        className="flex shrink-0 items-center justify-center px-2 py-1.5 text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]"
        onClick={addTab}
        title="新建查询"
      >
        <Plus size={14} />
      </button>
    </div>
  )
}
