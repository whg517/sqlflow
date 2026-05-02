import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Database } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

export default function LoginPage() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true)
    // TODO: call login API
    setTimeout(() => {
      setLoading(false)
      navigate('/query')
    }, 500)
  }

  return (
    <div className="flex h-full items-center justify-center bg-[var(--bg-base)]">
      <div className="w-[360px] rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-10 text-center shadow-lg">
        <div className="mb-8 flex items-center justify-center gap-2 text-2xl font-bold text-[var(--accent-primary)]">
          <Database size={28} />
          <span>SQLFlow</span>
        </div>
        <p className="mb-6 text-sm text-[var(--text-secondary)]">SQL 审批管理平台</p>
        <form onSubmit={handleSubmit} className="flex flex-col gap-3">
          <Input
            type="text"
            placeholder="用户名"
            className="bg-[var(--bg-elevated)] border-[var(--border-default)]"
          />
          <Input
            type="password"
            placeholder="密码"
            className="bg-[var(--bg-elevated)] border-[var(--border-default)]"
          />
          <Button type="submit" disabled={loading} className="mt-1">
            {loading ? '登录中...' : '登 录'}
          </Button>
        </form>
        <p className="mt-6 text-xs text-[var(--text-muted)]">&copy; 2026 SQLFlow</p>
      </div>
    </div>
  )
}
