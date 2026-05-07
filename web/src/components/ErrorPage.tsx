import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { ShieldX, FileQuestion } from 'lucide-react'

interface ErrorPageProps {
  code: 403 | 404
}

export default function ErrorPage({ code }: ErrorPageProps) {
  const navigate = useNavigate()

  if (code === 403) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 bg-[var(--bg-base)]">
        <ShieldX size={64} className="text-[var(--risk-high)]" />
        <h1 className="text-2xl font-bold text-[var(--text-primary)]">403</h1>
        <p className="text-sm text-[var(--text-secondary)]">
          您没有访问此页面的权限
        </p>
        <Button variant="outline" onClick={() => navigate(-1)}>
          返回上一页
        </Button>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 bg-[var(--bg-base)]">
      <FileQuestion size={64} className="text-[var(--text-muted)]" />
      <h1 className="text-2xl font-bold text-[var(--text-primary)]">404</h1>
      <p className="text-sm text-[var(--text-secondary)]">
        页面不存在或已被移除
      </p>
      <Button variant="outline" onClick={() => navigate('/query')}>
        返回首页
      </Button>
    </div>
  )
}
