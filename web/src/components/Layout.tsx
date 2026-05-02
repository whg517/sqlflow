import { useState } from 'react'
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
} from 'lucide-react'

const settingsSubItems = [
  { to: '/settings/datasource', label: '数据源管理', icon: Server },
  { to: '/settings/mask-rules', label: '脱敏规则', icon: EyeOff },
  { to: '/settings/ai-config', label: 'AI 配置', icon: Bot },
]

export default function Layout() {
  const location = useLocation()
  const [settingsOpen, setSettingsOpen] = useState(
    location.pathname.startsWith('/settings'),
  )
  const isSettingsActive = location.pathname.startsWith('/settings')

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

        {/* Footer */}
        <div className="border-t border-[var(--border-subtle)] p-2">
          <button
            onClick={() => {
              localStorage.removeItem('token')
              window.location.href = '/login'
            }}
            className="flex w-full items-center gap-2.5 rounded-md px-3.5 py-2.5 text-left text-sm text-[var(--text-secondary)] transition-colors hover:text-[#ef4444]"
          >
            <LogOut size={18} />
            <span>登出</span>
          </button>
        </div>
      </aside>

      {/* Main area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex h-[52px] min-h-[52px] items-center border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6">
          <span className="text-sm font-medium text-[var(--text-secondary)]">
            SQL 审批管理平台
          </span>
        </header>
        <main className="flex-1 overflow-auto bg-[var(--bg-base)]">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
