import { useState, useEffect } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import {
  Database,
  FileText,
  ShieldCheck,
  ScrollText,
  Settings,
  LogOut,
  Server,
  EyeOff,
  Bot,
  ChevronDown,
  KeyRound,
  User,
} from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import ChangePasswordDialog from '@/components/ChangePasswordDialog'
import { api } from '@/api/client'

const settingsSubItems = [
  { to: '/settings/datasource', label: '数据源管理', icon: Server },
  { to: '/settings/mask-rules', label: '脱敏规则', icon: EyeOff },
  { to: '/settings/ai-config', label: 'AI 配置', icon: Bot },
]

interface CurrentUser {
  username: string
  role: string
}

export default function Layout() {
  const location = useLocation()
  const [settingsOpen, setSettingsOpen] = useState(
    location.pathname.startsWith('/settings'),
  )
  const isSettingsActive = location.pathname.startsWith('/settings')

  const [user, setUser] = useState<CurrentUser | null>(null)
  const [pwdOpen, setPwdOpen] = useState(false)

  useEffect(() => {
    api
      .get<{ code: number; data: CurrentUser }>('/auth/me')
      .then((res) => {
        if (res.code === 0) setUser(res.data)
      })
      .catch(() => {})
  }, [])

  const initial = user?.username ? user.username[0].toUpperCase() : 'U'

  return (
    <div className="flex h-full">
      {/* Sidebar */}
      <aside className="flex w-[200px] min-w-[200px] flex-col border-r border-[var(--border-subtle)] bg-[var(--bg-sidebar)]">
        {/* Brand */}
        <div className="flex items-center gap-2.5 border-b border-[var(--border-subtle)] px-5 py-4 text-lg font-bold text-[var(--accent-primary)]">
          <Database size={24} />
          <span>SQLFlow</span>
        </div>

        {/* Navigation */}
        <nav className="flex flex-1 flex-col gap-0.5 p-2">
          <NavLink
            to="/query"
            className={({ isActive }) =>
              `flex items-center gap-2.5 rounded-md px-3.5 py-2.5 text-sm no-underline transition-colors ${
                isActive
                  ? 'bg-[var(--accent-primary)]/10 font-medium text-[var(--accent-primary)]'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]'
              }`
            }
          >
            <Database size={18} />
            <span>查询</span>
          </NavLink>

          <NavLink
            to="/tickets"
            className={({ isActive }) =>
              `flex items-center gap-2.5 rounded-md px-3.5 py-2.5 text-sm no-underline transition-colors ${
                isActive
                  ? 'bg-[var(--accent-primary)]/10 font-medium text-[var(--accent-primary)]'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]'
              }`
            }
          >
            <FileText size={18} />
            <span>工单</span>
          </NavLink>

          <NavLink
            to="/permissions"
            className={({ isActive }) =>
              `flex items-center gap-2.5 rounded-md px-3.5 py-2.5 text-sm no-underline transition-colors ${
                isActive
                  ? 'bg-[var(--accent-primary)]/10 font-medium text-[var(--accent-primary)]'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]'
              }`
            }
          >
            <ShieldCheck size={18} />
            <span>权限</span>
          </NavLink>

          <NavLink
            to="/audit"
            className={({ isActive }) =>
              `flex items-center gap-2.5 rounded-md px-3.5 py-2.5 text-sm no-underline transition-colors ${
                isActive
                  ? 'bg-[var(--accent-primary)]/10 font-medium text-[var(--accent-primary)]'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]'
              }`
            }
          >
            <ScrollText size={18} />
            <span>审计</span>
          </NavLink>

          {/* Separator */}
          <div className="my-1 border-t border-[var(--border-subtle)]" />

          {/* Settings with submenu */}
          <button
            onClick={() => setSettingsOpen(!settingsOpen)}
            className={`flex w-full items-center gap-2.5 rounded-md px-3.5 py-2.5 text-left text-sm transition-colors ${
              isSettingsActive
                ? 'bg-[var(--accent-primary)]/10 font-medium text-[var(--accent-primary)]'
                : 'text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]'
            }`}
          >
            <Settings size={18} />
            <span className="flex-1">设置</span>
            <ChevronDown
              size={14}
              className={`transition-transform ${settingsOpen ? 'rotate-180' : ''}`}
            />
          </button>

          {settingsOpen && (
            <div className="ml-4 flex flex-col gap-0.5 border-l border-[var(--border-subtle)] pl-2">
              {settingsSubItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    `flex items-center gap-2 rounded-md px-2.5 py-1.5 text-xs no-underline transition-colors ${
                      isActive
                        ? 'font-medium text-[var(--accent-primary)]'
                        : 'text-[var(--text-tertiary)] hover:text-[var(--text-primary)]'
                    }`
                  }
                >
                  <item.icon size={14} />
                  <span>{item.label}</span>
                </NavLink>
              ))}
            </div>
          )}
        </nav>
      </aside>

      {/* Main area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex h-[52px] min-h-[52px] items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6">
          <span className="text-sm font-medium text-[var(--text-secondary)]">
            SQL 审批管理平台
          </span>

          {/* Avatar dropdown */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button className="flex items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-[var(--bg-elevated)]">
                <Avatar size="sm">
                  <AvatarFallback className="text-xs">{initial}</AvatarFallback>
                </Avatar>
                <ChevronDown size={14} className="text-[var(--text-tertiary)]" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuLabel className="flex items-center gap-2">
                <User size={14} />
                <span>{user?.username ?? '—'}</span>
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setPwdOpen(true)}>
                <KeyRound size={14} />
                修改密码
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                onClick={() => {
                  localStorage.removeItem('token')
                  window.location.href = '/login'
                }}
              >
                <LogOut size={14} />
                退出登录
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </header>
        <main className="flex-1 overflow-auto bg-[var(--bg-base)]">
          <Outlet />
        </main>
      </div>

      {/* Change password dialog */}
      <ChangePasswordDialog open={pwdOpen} onOpenChange={setPwdOpen} />
    </div>
  )
}
