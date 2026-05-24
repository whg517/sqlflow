import { useState, useEffect, useCallback } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
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
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import ChangePasswordDialog from "@/components/ChangePasswordDialog";
import CommandPalette from "@/components/CommandPalette";
import NetworkBanner from "@/components/NetworkBanner";
import { useTheme } from "@/hooks/useTheme";
import { api } from "@/api/client";

const settingsSubItems = [
  { to: "/settings/datasource", label: "数据源管理", icon: Server },
  { to: "/settings/mask-rules", label: "脱敏规则", icon: EyeOff },
  { to: "/settings/ai-config", label: "AI 配置", icon: Bot },
];

interface CurrentUser {
  username: string;
  role: string;
}

// --- NavItem defined outside Layout to avoid react-hooks/static-components error ---

interface NavItemProps {
  to: string;
  icon: React.ElementType;
  label: string;
  collapsed: boolean;
  navLinkClass: (isActive: boolean) => string;
}

function NavItem({
  to,
  icon: Icon,
  label,
  collapsed,
  navLinkClass,
}: NavItemProps) {
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
  );
}

export default function Layout() {
  const location = useLocation();
  const [settingsOpen, setSettingsOpen] = useState(
    location.pathname.startsWith("/settings"),
  );
  const isSettingsActive = location.pathname.startsWith("/settings");

  const { theme, toggle } = useTheme();
  const [user, setUser] = useState<CurrentUser | null>(null);
  const [pwdOpen, setPwdOpen] = useState(false);
  const [manuallyCollapsed, setManuallyCollapsed] = useState(() => {
    return localStorage.getItem("sidebar-collapsed") === "true";
  });
  const [autoCollapsed, setAutoCollapsed] = useState(false);
  const [cmdOpen, setCmdOpen] = useState(false);

  // Auto-collapse at 1024-1279px per §6.2 responsive strategy
  useEffect(() => {
    const mql = window.matchMedia(
      "(min-width: 1024px) and (max-width: 1279px)",
    );
    const handler = (e: MediaQueryListEvent | MediaQueryList) => {
      setAutoCollapsed(e.matches);
    };
    handler(mql);
    mql.addEventListener("change", handler);
    return () => mql.removeEventListener("change", handler);
  }, []);

  const collapsed = autoCollapsed || manuallyCollapsed;

  useEffect(() => {
    api
      .get<{ code: number; data: CurrentUser }>("/auth/me")
      .then((res) => {
        if (res.code === 0) setUser(res.data);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    localStorage.setItem("sidebar-collapsed", String(manuallyCollapsed));
  }, [manuallyCollapsed]);

  const initial = user?.username ? user.username[0].toUpperCase() : "U";

  const toggleCollapse = useCallback(() => setManuallyCollapsed((c) => !c), []);

  const sidebarWidth = collapsed
    ? "w-[56px] min-w-[56px]"
    : "w-[220px] min-w-[220px]";

  // §2.2 Navigation item interaction — clean indicator style
  const navLinkClass = (isActive: boolean) =>
    `flex items-center rounded-md no-underline transition-colors ${
      collapsed ? "justify-center px-0 py-2.5" : "gap-3 px-4 py-2.5 text-sm"
    } ${
      isActive
        ? "font-medium text-[var(--accent-primary)] bg-[var(--accent-muted)] border-l-2 border-[var(--accent-primary)]"
        : "text-[var(--text-secondary)] hover:bg-[var(--bg-surface)] hover:text-[var(--text-primary)] border-l-2 border-transparent"
    }`;

  const settingsButtonClass = () =>
    `flex w-full items-center rounded-md text-left transition-colors ${
      collapsed ? "justify-center px-0 py-2.5" : "gap-3 px-4 py-2.5 text-sm"
    } ${
      isSettingsActive
        ? "font-medium text-[var(--accent-primary)] bg-[var(--accent-muted)] border-l-2 border-[var(--accent-primary)]"
        : "text-[var(--text-secondary)] hover:bg-[var(--bg-surface)] hover:text-[var(--text-primary)] border-l-2 border-transparent"
    }`;

  return (
    <div className="flex h-full">
      {/* Network banner */}
      <NetworkBanner />

      {/* Sidebar — §2.1/2.2 */}
      <aside
        className={`flex ${sidebarWidth} flex-col border-r border-[var(--border-subtle)] bg-[var(--bg-sidebar)] transition-[width,min-width] duration-300 ease-in-out`}
      >
        {/* Brand — §2.2: height 56px, bottom 1px border-subtle */}
        <div className="flex h-[56px] min-h-[56px] items-center gap-2.5 border-b border-[var(--border-subtle)] px-4 text-lg font-bold text-[var(--accent-primary)]">
          <Database size={24} className="shrink-0" />
          {!collapsed && <span>SQLFlow</span>}
        </div>

        {/* Navigation */}
        <nav
          aria-label="Main navigation"
          className="flex flex-1 flex-col gap-2 p-3 pl-1"
        >
          <NavItem
            to="/"
            icon={LayoutDashboard}
            label="概览"
            collapsed={collapsed}
            navLinkClass={navLinkClass}
          />
          <NavItem
            to="/query"
            icon={Database}
            label="查询"
            collapsed={collapsed}
            navLinkClass={navLinkClass}
          />
          <NavItem
            to="/tickets"
            icon={FileText}
            label="工单"
            collapsed={collapsed}
            navLinkClass={navLinkClass}
          />
          <NavItem
            to="/permissions"
            icon={ShieldCheck}
            label="权限"
            collapsed={collapsed}
            navLinkClass={navLinkClass}
          />
          <NavItem
            to="/audit"
            icon={ScrollText}
            label="审计"
            collapsed={collapsed}
            navLinkClass={navLinkClass}
          />
          {user?.role === "admin" && (
            <NavItem
              to="/users"
              icon={User}
              label="用户管理"
              collapsed={collapsed}
              navLinkClass={navLinkClass}
            />
          )}

          {/* Separator */}
          <div className="my-3 border-t border-[var(--border-subtle)]" />

          {/* Settings with submenu — §2.2 */}
          {collapsed ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  onClick={() => setSettingsOpen(!settingsOpen)}
                  className={settingsButtonClass()}
                >
                  <Settings size={18} />
                </button>
              </TooltipTrigger>
              <TooltipContent side="right" className="text-xs">
                设置
              </TooltipContent>
            </Tooltip>
          ) : (
            <button
              onClick={() => setSettingsOpen(!settingsOpen)}
              className={settingsButtonClass()}
            >
              <Settings size={18} />
              <span className="flex-1">设置</span>
              <ChevronDown
                size={14}
                className={`transition-transform duration-200 ${settingsOpen ? "rotate-180" : ""}`}
              />
            </button>
          )}

          {/* Settings submenu — §2.2: ml-4, border-l border-subtle */}
          {settingsOpen && !collapsed && (
            <div className="ml-5 flex flex-col gap-1.5 border-l border-[var(--border-subtle)] pl-3 pt-1">
              {settingsSubItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    `flex items-center gap-2.5 rounded-md px-3 py-2 text-xs no-underline transition-colors ${
                      isActive
                        ? "font-medium text-[var(--accent-primary)]"
                        : "text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
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

        {/* Collapse toggle — §2.2: border-t, icon swap */}
        <div className="border-t border-[var(--border-subtle)] p-3 pt-2">
          <button
            onClick={toggleCollapse}
            className="flex w-full items-center justify-center rounded-md px-3 py-2 text-sm text-[var(--text-tertiary)] transition-colors hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]"
          >
            {collapsed ? <PanelLeft size={18} /> : <PanelLeftClose size={18} />}
          </button>
        </div>
      </aside>

      {/* Main area — §2.3 */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar — §2.3: h-52px, border-b border-default, bg-surface */}
        <header className="flex h-[52px] min-h-[52px] items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-surface)] px-6">
          {/* Command palette trigger — §2.3 */}
          <button
            onClick={() => setCmdOpen(true)}
            className="flex h-8 items-center gap-2 rounded-md border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 py-1.5 text-xs text-[var(--text-tertiary)] transition-colors hover:border-[var(--accent-primary)] hover:text-[var(--text-secondary)]"
          >
            <Search size={14} />
            <span>搜索...</span>
            <kbd className="ml-2 rounded border border-[var(--border-default)] px-1.5 py-0.5 font-mono text-[10px]">
              ⌘K
            </kbd>
          </button>

          {/* Avatar dropdown — §2.3 */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button className="flex items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-[var(--bg-elevated)]">
                <Avatar size="sm">
                  <AvatarFallback className="text-xs">{initial}</AvatarFallback>
                </Avatar>
                <ChevronDown
                  size={14}
                  className="text-[var(--text-tertiary)]"
                />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuLabel className="flex items-center gap-2">
                <User size={14} />
                <span>{user?.username ?? "—"}</span>
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setPwdOpen(true)}>
                <KeyRound size={14} />
                修改密码
              </DropdownMenuItem>
              <DropdownMenuItem onClick={toggle}>
                {theme === "dark" ? <Sun size={14} /> : <Moon size={14} />}
                {theme === "dark" ? "浅色模式" : "深色模式"}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onClick={() => {
                  localStorage.removeItem("token");
                  window.location.href = "/login";
                }}
              >
                <LogOut size={14} />
                退出登录
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </header>
        <main className="flex-1 overflow-auto bg-[var(--bg-base)] p-6 page-transition">
          <Outlet />
        </main>
      </div>

      {/* Dialogs */}
      <ChangePasswordDialog open={pwdOpen} onOpenChange={setPwdOpen} />
      <CommandPalette open={cmdOpen} onOpenChange={setCmdOpen} />
    </div>
  );
}
