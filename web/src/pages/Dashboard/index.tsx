import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { FileText, Database, Server, Users, ShieldAlert, Loader2 } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { getDashboardStats, type DashboardStats } from '@/api/dashboard'
import { listSensitiveTables } from '@/api/maskRule'

/* §3.2: stat card config — icon color + bg matching spec */
const statCards = [
  {
    key: 'pending_tickets' as const,
    label: '待审批工单',
    icon: FileText,
    color: 'text-blue-500',
    bg: 'bg-blue-500/10',
    link: '/tickets?status=PENDING_APPROVAL',
  },
  {
    key: 'recent_queries_7d' as const,
    label: '近 7 天查询',
    icon: Database,
    color: 'text-green-500',
    bg: 'bg-green-500/10',
  },
  {
    key: 'active_datasources' as const,
    label: '活跃数据源',
    icon: Server,
    color: 'text-purple-500',
    bg: 'bg-purple-500/10',
  },
  {
    key: 'total_users' as const,
    label: '系统用户数',
    icon: Users,
    color: 'text-orange-500',
    bg: 'bg-orange-500/10',
  },
  {
    key: 'sensitive_tables' as const,
    label: '敏感表',
    icon: ShieldAlert,
    color: 'text-red-500',
    bg: 'bg-red-500/10',
    link: '/settings/mask-rules',
  },
]

export default function DashboardPage() {
  const navigate = useNavigate()
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [sensitiveCount, setSensitiveCount] = useState(0)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    getDashboardStats()
      .then((res) => {
        if (res.code === 0) setStats(res.data)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    listSensitiveTables({ page_size: 1 })
      .then((res) => setSensitiveCount(res.total ?? 0))
      .catch(() => {})
  }, [])

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-[var(--text-muted)]" />
      </div>
    )
  }

  return (
    /* §3.2: centered content max-width 960px */
    <div className="mx-auto max-w-[960px] space-y-6 p-6">
      <h1 className="text-xl font-semibold text-[var(--text-primary)]">概览</h1>

      {/* §3.2: grid grid-cols-2 gap-4 */}
      <div className="grid grid-cols-2 gap-4">
        {statCards.map((card) => {
          const value = card.key === 'sensitive_tables'
            ? sensitiveCount
            : (stats?.[card.key] ?? 0)
          const Icon = card.icon
          const content = (
            /* §3.2: Card rounded-lg hover:shadow-md transition-shadow cursor-pointer */
            <Card className="cursor-pointer transition-shadow duration-150 hover:shadow-[var(--shadow-md)]">
              <CardContent className="flex items-center gap-4 py-4">
                {/* §3.2: 40x40 rounded-lg icon with bg */}
                <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${card.bg}`}>
                  <Icon size={20} className={card.color} />
                </div>
                <div>
                  {/* §3.2: text-2xl font-bold text-primary */}
                  <div className="text-2xl font-bold text-[var(--text-primary)]">{value}</div>
                  {/* §3.2: text-sm text-secondary */}
                  <div className="text-sm text-[var(--text-secondary)]">{card.label}</div>
                </div>
              </CardContent>
            </Card>
          )

          if (card.link) {
            return (
              <div key={card.key} onClick={() => navigate(card.link!)} className="block">
                {content}
              </div>
            )
          }
          return <div key={card.key}>{content}</div>
        })}
      </div>
    </div>
  )
}
