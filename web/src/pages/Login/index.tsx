import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Database } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { api } from '@/api/client'

function validateUsername(v: string): string | null {
  if (!v) return '请输入用户名'
  if (v.length < 3 || v.length > 32) return '用户名需 3-32 个字符'
  return null
}

function validatePassword(v: string): string | null {
  if (!v) return '请输入密码'
  if (v.length < 8 || v.length > 128) return '密码需 8-128 个字符'
  return null
}

export default function LoginPage() {
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [errors, setErrors] = useState<{ username?: string; password?: string }>({})
  const [serverError, setServerError] = useState('')
  const [loading, setLoading] = useState(false)

  function handleBlur(field: 'username' | 'password') {
    const err = field === 'username' ? validateUsername(username) : validatePassword(password)
    setErrors((prev) => ({ ...prev, [field]: err ?? undefined }))
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const uErr = validateUsername(username)
    const pErr = validatePassword(password)
    if (uErr || pErr) {
      setErrors({ username: uErr ?? undefined, password: pErr ?? undefined })
      return
    }
    setLoading(true)
    setServerError('')
    try {
      const res = await api.post<{ code: number; data: { token: string } }>(
        '/auth/login',
        { username, password },
      )
      localStorage.setItem('token', res.data.token)
      navigate('/query', { replace: true })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : ''
      setServerError(msg || '用户名或密码错误')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex h-screen items-center justify-center bg-[var(--bg-base)]">
      {/* §3.1 Login card: w-[400px] rounded-xl border border-default bg-surface shadow-xl */}
      <div className="w-[400px] rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-8 shadow-[var(--shadow-xl)]">
        {/* Brand — §3.1: Database icon 28px + "SQLFlow" text-2xl font-bold accent-primary */}
        <div className="mb-2 flex items-center justify-center gap-2 text-2xl font-bold text-[var(--accent-primary)]">
          <Database size={28} />
          <span>SQLFlow</span>
        </div>
        {/* Subtitle — §3.1: text-sm text-secondary */}
        <p className="mb-6 text-center text-sm text-[var(--text-secondary)]">
          SQL 审批管理平台
        </p>

        {/* Server error — §3.1: bg-red-500/10 text-red-400 rounded-md px-3 py-2 */}
        {serverError && (
          <div className="mb-4 rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">
            {serverError}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-3">
          {/* Username — §3.1: h-10 bg-elevated border-default */}
          <div>
            <Input
              type="text"
              placeholder="用户名"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              onBlur={() => handleBlur('username')}
              className="h-10 bg-[var(--bg-elevated)] border-[var(--border-default)]"
              autoComplete="username"
            />
            {/* Error — §3.1: text-xs text-danger, mt-1 */}
            {errors.username && (
              <p className="mt-1 text-xs text-[var(--danger)]">{errors.username}</p>
            )}
          </div>

          {/* Password — §3.1: h-10 bg-elevated border-default */}
          <div>
            <Input
              type="password"
              placeholder="密码"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onBlur={() => handleBlur('password')}
              className="h-10 bg-[var(--bg-elevated)] border-[var(--border-default)]"
              autoComplete="current-password"
            />
            {errors.password && (
              <p className="mt-1 text-xs text-[var(--danger)]">{errors.password}</p>
            )}
          </div>

          {/* Login button — §3.1: w-full bg-accent-primary text-white hover:bg-accent-hover h-10 */}
          <Button
            type="submit"
            loading={loading}
            className="mt-1 h-10 w-full bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
          >
            {loading ? '登录中...' : '登 录'}
          </Button>
        </form>

        {/* Copyright — §3.1: text-xs text-muted text-center mt-6 */}
        <p className="mt-6 text-center text-xs text-[var(--text-muted)]">
          © 2026 SQLFlow
        </p>
      </div>
    </div>
  )
}
