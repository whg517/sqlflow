import { useState, useEffect, useCallback } from 'react'
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
  PanelLeftClose,
  PanelLeft,
  Search,
  LayoutDashboard,
  Sun,
  Moon,
} from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import ChangePasswordDialog from '@/components/ChangePasswordDialog'
import CommandPalette from '@/components/CommandPalette'
import NetworkBanner from '@/components/NetworkBanner'
import { useTheme } from '@/hooks/useTheme'
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

// --- NavItem must be defined outside Layout to avoid react-hooks/static-components error ---

interface NavItemProps {
  to: string
  icon: React.ElementType
  label: string
  collapsed: boolean
  navLinkClass: (isActive: boolean) => string
}

function NavItem({ to, icon: Icon, label, collapsed, navLinkClass }: NavItemProps) {
  return collapsed ? (
    <Tooltip>
      <TooltipTrigger asChild>
        <NavLink to={to} className={({ isActive }) => navLinkClass(isActive)}>
          <Icon size={18} />
        </NavLink>
      </TooltipTrigger>
      <TooltipContent side="right" className="text-xs">
        {label}
      </TooltipContent>
    </Tooltip>
  ) : (
    <NavLink to={to} className={({ isActive }) => navLinkClass(isActive)}>
      <Icon size={18} />
      <span>{label}</span>
    </NavLink>
  )
}

export default function Layout() {
  const location = useLocation()
  const [settingsOpen, setSettingsOpen] = useState(
    location.pathname.startsWith('/settings'),
  )
  const isSettingsActive = location.pathname.startsWith('/settings')

  const { theme, toggle } = useTheme()
  const [user, setUser] = useState<CurrentUser | null>(null)
  const [pwdOpen, setPwdOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(() => {
    return localStorage.getItem('sidebar-collapsed') === 'true'
  })
  const [cmdOpen, setCmdOpen] = useState(false)

  useEffect(() => {
    api
      .get<{ code: number; data: CurrentUser }>('/auth/me')
      .then((res) => {
        if (res.code === 0) setUser(res.data)
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    localStorage.setItem('sidebar-collapsed', String(collapsed))
  }, [collapsed])

  const initial = user?.username ? user.username[0].toUpperCase() : 'U'

  const toggleCollapse = useCallback(() => setCollapsed((c) => !c), [])

  const sidebarWidth = collapsed ? 'w-[56px] min-w-[56px]' : 'w-[200px] min-w-[200px]'

  const navLinkClass = (isActive: boolean) =>
    `flex items-center rounded-md no-underline transition-colors ${
      collapsed ? 'justify-center px-0 py-2.5' : 'gap-2.5 px-3.5 py-2.5 text-sm'
    } ${
      isActive
        ? 'bg-[var(--accent-primary)]/10 font-medium text-[var(--accent-primary)]'
        : 'text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]'
    }`

  const settingsButtonClass = () =>
    `flex w-full items-center rounded-md text-left transition-colors ${
      collapsed ? 'justify-center px-0 py-2.5' : 'gap-2.5 px-3.5 py-2.5 text-sm'
    } ${
      isSettingsActive
        ? 'bg-[var(--accent-primary)]/10 font-medium text-[var(--accent-primary)]'
        : 'text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]'
    }`



  return (
    <div className="flex h-full">
      {/* Network banner */}
      <NetworkBanner />

      {/* Sidebar */}
      <aside
        className={`flex ${sidebarWidth} flex-col border-r border-[var(--border-subtle)] bg-[var(--bg-sidebar)] transition-all duration-200`}
      >
        {/* Brand */}
        <div className="flex items-center gap-2.5 border-b border-[var(--border-subtle)] px-4 py-4 text-lg font-bold text-[var(--accent-primary)]">
          <Database size={24} className="shrink-0" />
          {!collapsed && <span>SQLFlow</span>}
        </div>

        {/* Navigation */}
        <nav className="flex flex-1 flex-col gap-0.5 p-2">
          <NavItem to="/" icon={LayoutDashboard} label="概览" collapsed={collapsed} navLinkClass={navLinkClass} />
          <NavItem to="/query" icon={Database} label="查询" collapsed={collapsed} navLinkClass={navLinkClass} />
          <NavItem to="/tickets" icon={FileText} label="工单" collapsed={collapsed} navLinkClass={navLinkClass} />
          <NavItem to="/permissions" icon={ShieldCheck} label="权限" collapsed={collapsed} navLinkClass={navLinkClass} />
          <NavItem to="/audit" icon={ScrollText} label="审计" collapsed={collapsed} navLinkClass={navLinkClass} />
          {user?.role === 'admin' && (
            <NavItem to="/users" icon={User} label="用户管理" collapsed={collapsed} navLinkClass={navLinkClass} />
          )}

          {/* Separator */}
          <div className="my-1 border-t border-[var(--border-subtle)]" />

          {/* Settings with submenu */}
          {collapsed ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button onClick={() => setSettingsOpen(!settingsOpen)} className={settingsButtonClass()}>
                  <Settings size={18} />
                </button>
              </TooltipTrigger>
              <TooltipContent side="right" className="text-xs">
                设置
              </TooltipContent>
            </Tooltip>
          ) : (
            <button onClick={() => setSettingsOpen(!settingsOpen)} className={settingsButtonClass()}>
              <Settings size={18} />
              <span className="flex-1">设置</span>
              <ChevronDown
                size={14}
                className={`transition-transform ${settingsOpen ? 'rotate-180' : ''}`}
              />
            </button>
          )}

          {settingsOpen && !collapsed && (
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

        {/* Collapse toggle */}
        <div className="border-t border-[var(--border-subtle)] p-2">
          <button
            onClick={toggleCollapse}
            className="flex w-full items-center justify-center rounded-md px-3 py-2 text-sm text-[var(--text-tertiary)] transition-colors hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]"
          >
            {collapsed ? <PanelLeft size={18} /> : <PanelLeftClose size={18} />}
          </button>
        </div>
      </aside>

      {/* Main area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex h-[52px] min-h-[52px] items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6">
          <button
            onClick={() => setCmdOpen(true)}
            className="flex items-center gap-2 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 py-1.5 text-xs text-[var(--text-tertiary)] transition-colors hover:border-[var(--accent-primary)] hover:text-[var(--text-secondary)]"
          >
            <Search size={14} />
            <span>搜索...</span>
            <kbd className="ml-2 rounded border border-[var(--border-default)] px-1.5 py-0.5 font-mono text-[10px]">
              ⌘K
            </kbd>
          </button>

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
              <DropdownMenuItem onClick={toggle}>
                {theme === 'dark' ? <Sun size={14} /> : <Moon size={14} />}
                {theme === 'dark' ? '浅色模式' : '深色模式'}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
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

      {/* Dialogs */}
      <ChangePasswordDialog open={pwdOpen} onOpenChange={setPwdOpen} />
      <CommandPalette open={cmdOpen} onOpenChange={setCmdOpen} />
    </div>
  )
}
