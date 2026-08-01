import { useCallback, useEffect, useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import {
  BarChart3,
  Bot,
  ChevronDown,
  Clock,
  Code2,
  Database,
  FileText,
  Gauge,
  Key,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Moon,
  PanelLeft,
  PanelLeftClose,
  ScrollText,
  Search,
  Server,
  Settings,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Sun,
  User,
  Webhook,
  EyeOff,
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/shared/components/ui/tooltip";
import { Avatar, AvatarFallback } from "@/shared/components/ui/avatar";
import ChangePasswordDialog from "@/shared/components/ChangePasswordDialog";
import CommandPalette from "@/shared/components/CommandPalette";
import NetworkBanner from "@/shared/components/NetworkBanner";
import { useTheme } from "@/shared/hooks/useTheme";
import { api } from "@/shared/api/client";

interface CurrentUser {
  username: string;
  role: string;
  permissions?: string[];
}

const SIDEBAR_LAYOUT_VERSION = "3";
const SIDEBAR_LAYOUT_VERSION_KEY = "sidebar-layout-version";

function getInitialSidebarState() {
  if (
    localStorage.getItem(SIDEBAR_LAYOUT_VERSION_KEY) !== SIDEBAR_LAYOUT_VERSION
  ) {
    return false;
  }
  return localStorage.getItem("sidebar-collapsed") === "true";
}

interface NavigationItem {
  to: string;
  label: string;
  icon: React.ElementType;
}

const workspaceItems: NavigationItem[] = [
  { to: "/", label: "概览", icon: LayoutDashboard },
  { to: "/query", label: "查询", icon: Database },
  { to: "/tickets", label: "工单", icon: FileText },
];

const securityItems: NavigationItem[] = [
  { to: "/permissions", label: "权限", icon: ShieldCheck },
  { to: "/permission-requests", label: "临时权限", icon: ShieldAlert },
];

const governanceItems: NavigationItem[] = [
  { to: "/audit", label: "审计", icon: ScrollText },
  { to: "/reports", label: "报表", icon: BarChart3 },
  { to: "/performance", label: "性能分析", icon: Gauge },
];

const assetItems: NavigationItem[] = [
  { to: "/sql-templates", label: "SQL 模板", icon: Code2 },
];

const policyItems: NavigationItem[] = [
  { to: "/settings/approval-policies", label: "审批策略", icon: ShieldCheck },
  { to: "/settings/mask-rules", label: "脱敏规则", icon: EyeOff },
];

const systemItems: NavigationItem[] = [
  { to: "/settings/datasource", label: "数据源配置", icon: Server },
];

const settingsSubItems: NavigationItem[] = [
  { to: "/settings/sla", label: "SLA 告警", icon: Clock },
  { to: "/settings/integrations", label: "系统集成", icon: Webhook },
  { to: "/settings/ai-config", label: "AI 配置", icon: Bot },
];

function isSystemSettingsPath(pathname: string) {
  return settingsSubItems.some(({ to }) => pathname.startsWith(to));
}

const pageTitles: Array<[string, string]> = [
  ["/settings/datasource", "数据源配置"],
  ["/settings/approval-policies", "审批策略"],
  ["/settings/mask-rules", "脱敏规则"],
  ["/settings/sla", "SLA 告警"],
  ["/settings/integrations", "系统集成"],
  ["/settings/ai-config", "AI 配置"],
  ["/permission-requests", "临时权限"],
  ["/sql-templates", "SQL 模板"],
  ["/performance", "性能分析"],
  ["/permissions", "权限管理"],
  ["/reports", "审计报表"],
  ["/tickets/new", "新建工单"],
  ["/tickets", "工单中心"],
  ["/query", "SQL 工作台"],
  ["/audit", "审计日志"],
  ["/users", "用户管理"],
  ["/tokens", "API Token"],
  ["/", "概览"],
];

function getPageTitle(pathname: string) {
  return (
    pageTitles.find(([path]) =>
      path === "/" ? pathname === "/" : pathname.startsWith(path),
    )?.[1] ?? "工作台"
  );
}

interface NavItemProps extends NavigationItem {
  collapsed: boolean;
}

function NavItem({ to, icon: Icon, label, collapsed }: NavItemProps) {
  const content = (
    <NavLink
      to={to}
      aria-label={collapsed ? label : undefined}
      className={({ isActive }) =>
        `group relative flex h-10 items-center rounded-lg no-underline transition-all duration-150 ${
          collapsed ? "justify-center px-0" : "gap-3 px-3"
        } ${
          isActive
            ? "bg-[var(--sidebar-active-bg)] text-[var(--sidebar-active-text)] shadow-[inset_0_0_0_1px_var(--sidebar-active-border)]"
            : "text-[var(--sidebar-text-secondary)] hover:bg-[var(--sidebar-hover)] hover:text-[var(--sidebar-text-primary)]"
        }`
      }
    >
      {({ isActive }) => (
        <>
          <span
            className={`flex size-7 shrink-0 items-center justify-center rounded-md transition-colors ${
              isActive
                ? "bg-[var(--sidebar-icon-active-bg)]"
                : "group-hover:bg-[var(--sidebar-icon-hover-bg)]"
            }`}
          >
            <Icon size={17} strokeWidth={isActive ? 2.2 : 1.8} />
          </span>
          {!collapsed && (
            <span className="min-w-0 truncate text-[13px] font-medium">
              {label}
            </span>
          )}
          {isActive && !collapsed && (
            <span className="ml-auto size-1.5 rounded-full bg-[var(--accent-primary)] shadow-[0_0_0_3px_var(--accent-muted)]" />
          )}
        </>
      )}
    </NavLink>
  );

  if (!collapsed) return content;

  return (
    <Tooltip>
      <TooltipTrigger asChild>{content}</TooltipTrigger>
      <TooltipContent side="right" sideOffset={10} className="text-xs">
        {label}
      </TooltipContent>
    </Tooltip>
  );
}

function NavGroup({
  label,
  items,
  collapsed,
}: {
  label: string;
  items: NavigationItem[];
  collapsed: boolean;
}) {
  return (
    <section className="space-y-1">
      {collapsed ? (
        <div className="mx-auto my-2 h-px w-7 bg-[var(--sidebar-border)]" />
      ) : (
        <div className="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-[var(--sidebar-text-muted)]">
          {label}
        </div>
      )}
      {items.map((item) => (
        <NavItem key={item.to} {...item} collapsed={collapsed} />
      ))}
    </section>
  );
}

export default function Layout() {
  const location = useLocation();
  const [settingsOpen, setSettingsOpen] = useState(
    isSystemSettingsPath(location.pathname),
  );
  const { theme, toggle } = useTheme();
  const [user, setUser] = useState<CurrentUser | null>(null);
  const [pwdOpen, setPwdOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(getInitialSidebarState);
  const [cmdOpen, setCmdOpen] = useState(false);

  const isSettingsActive = isSystemSettingsPath(location.pathname);
  const showSettingsSubmenu = settingsOpen || isSettingsActive;
  const currentPageTitle = getPageTitle(location.pathname);
  const initial = user?.username ? user.username[0].toUpperCase() : "U";

  useEffect(() => {
    api
      .get<{ code: number; data: CurrentUser }>("/auth/me")
      .then((res) => {
        if (res.code === 0) setUser(res.data);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    localStorage.setItem("sidebar-collapsed", String(collapsed));
    localStorage.setItem(SIDEBAR_LAYOUT_VERSION_KEY, SIDEBAR_LAYOUT_VERSION);
  }, [collapsed]);

  const toggleCollapse = useCallback(() => setCollapsed((value) => !value), []);

  const complianceItems: NavigationItem[] = [
    ...securityItems,
    ...(user?.role === "admin" || user?.permissions?.includes("users:manage")
      ? [{ to: "/users", label: "用户管理", icon: User }]
      : []),
    { to: "/tokens", label: "API Token", icon: Key },
    ...policyItems,
  ];

  return (
    <div className="flex h-full min-w-[900px] bg-[var(--bg-base)]">
      <NetworkBanner />

      <aside
        className={`relative z-20 flex shrink-0 flex-col border-r border-[var(--sidebar-border)] bg-[var(--bg-sidebar)] transition-[width] duration-200 ease-out ${
          collapsed ? "w-[72px]" : "w-[244px]"
        }`}
      >
        <div
          className={`flex h-16 min-h-16 items-center border-b border-[var(--sidebar-border)] ${
            collapsed ? "justify-center px-3" : "gap-3 px-4"
          }`}
        >
          <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-[var(--accent-primary)] to-[var(--accent-secondary)] text-white shadow-[0_8px_24px_var(--accent-shadow)]">
            <Database size={19} strokeWidth={2.2} />
          </div>
          {!collapsed && (
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="text-[15px] font-semibold tracking-tight text-[var(--sidebar-text-primary)]">
                  SQLFlow
                </span>
                <span className="rounded-full border border-[var(--sidebar-border)] bg-[var(--sidebar-badge-bg)] px-1.5 py-0.5 text-[8px] font-semibold uppercase tracking-wider text-[var(--sidebar-text-muted)]">
                  Ops
                </span>
              </div>
              <div className="mt-0.5 truncate text-[10px] text-[var(--sidebar-text-muted)]">
                数据库安全工作台
              </div>
            </div>
          )}
        </div>

        <nav
          aria-label="Main navigation"
          className={`flex-1 space-y-3 overflow-y-auto py-3 ${
            collapsed ? "px-2.5" : "px-3"
          }`}
        >
          <NavGroup
            label="工作台"
            items={workspaceItems}
            collapsed={collapsed}
          />
          <NavGroup
            label="安全与合规"
            items={complianceItems}
            collapsed={collapsed}
          />
          <NavGroup
            label="治理"
            items={governanceItems}
            collapsed={collapsed}
          />
          <NavGroup
            label="知识资产"
            items={assetItems}
            collapsed={collapsed}
          />

          <section className="space-y-1">
            {collapsed ? (
              <div className="mx-auto my-2 h-px w-7 bg-[var(--sidebar-border)]" />
            ) : (
              <div className="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-[var(--sidebar-text-muted)]">
                系统管理
              </div>
            )}

            {systemItems.map((item) => (
              <NavItem key={item.to} {...item} collapsed={collapsed} />
            ))}

            {collapsed ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    aria-label="设置"
                    onClick={() => {
                      setCollapsed(false);
                      setSettingsOpen(true);
                    }}
                    className={`flex h-10 w-full items-center justify-center rounded-lg transition-colors ${
                      isSettingsActive
                        ? "bg-[var(--sidebar-active-bg)] text-[var(--sidebar-active-text)]"
                        : "text-[var(--sidebar-text-secondary)] hover:bg-[var(--sidebar-hover)] hover:text-[var(--sidebar-text-primary)]"
                    }`}
                  >
                    <Settings size={17} />
                  </button>
                </TooltipTrigger>
                <TooltipContent
                  side="right"
                  sideOffset={10}
                  className="text-xs"
                >
                  设置
                </TooltipContent>
              </Tooltip>
            ) : (
              <button
                onClick={() => setSettingsOpen((value) => !value)}
                className={`flex h-10 w-full items-center gap-3 rounded-lg px-3 text-left transition-colors ${
                  isSettingsActive
                    ? "bg-[var(--sidebar-active-bg)] text-[var(--sidebar-active-text)]"
                    : "text-[var(--sidebar-text-secondary)] hover:bg-[var(--sidebar-hover)] hover:text-[var(--sidebar-text-primary)]"
                }`}
              >
                <span className="flex size-7 items-center justify-center rounded-md">
                  <Settings size={17} />
                </span>
                <span className="flex-1 text-[13px] font-medium">设置</span>
                <ChevronDown
                  size={14}
                  className={`transition-transform ${
                    showSettingsSubmenu ? "rotate-180" : ""
                  }`}
                />
              </button>
            )}

            {showSettingsSubmenu && !collapsed && (
              <div className="ml-[26px] space-y-0.5 border-l border-[var(--sidebar-border)] py-1 pl-4">
                {settingsSubItems.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    className={({ isActive }) =>
                      `flex items-center gap-2.5 rounded-md px-2 py-2 text-xs no-underline transition-colors ${
                        isActive
                          ? "font-medium text-[var(--sidebar-active-text)]"
                          : "text-[var(--sidebar-text-muted)] hover:text-[var(--sidebar-text-primary)]"
                      }`
                    }
                  >
                    <item.icon size={14} />
                    <span>{item.label}</span>
                  </NavLink>
                ))}
              </div>
            )}
          </section>
        </nav>

        <div className="border-t border-[var(--sidebar-border)] p-3">
          {!collapsed && (
            <div className="mb-2 flex items-center gap-2 rounded-lg border border-[var(--sidebar-border)] bg-[var(--sidebar-status-bg)] px-3 py-2">
              <span className="relative flex size-2">
                <span className="absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-30" />
                <span className="relative inline-flex size-2 rounded-full bg-emerald-400" />
              </span>
              <span className="text-[11px] text-[var(--sidebar-text-muted)]">
                服务运行正常
              </span>
            </div>
          )}
          <button
            aria-label={collapsed ? "展开导航" : "收起导航"}
            onClick={toggleCollapse}
            className={`flex h-9 w-full items-center rounded-lg text-[var(--sidebar-text-muted)] transition-colors hover:bg-[var(--sidebar-hover)] hover:text-[var(--sidebar-text-primary)] ${
              collapsed ? "justify-center" : "gap-3 px-3"
            }`}
          >
            {collapsed ? <PanelLeft size={17} /> : <PanelLeftClose size={17} />}
            {!collapsed && <span className="text-xs">收起导航</span>}
          </button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <header className="flex h-16 min-h-16 items-center gap-5 border-b border-[var(--border-subtle)] bg-[var(--topbar-bg)] px-6 backdrop-blur-xl">
          <div className="hidden min-w-[150px] xl:block">
            <div className="text-[10px] font-semibold uppercase tracking-[0.16em] text-[var(--text-muted)]">
              SQLFlow Workspace
            </div>
            <div className="mt-0.5 text-sm font-semibold text-[var(--text-primary)]">
              当前页面 · {currentPageTitle}
            </div>
          </div>

          <button
            onClick={() => setCmdOpen(true)}
            className="group flex h-9 w-full max-w-[420px] items-center gap-2.5 rounded-lg border border-[var(--border-default)] bg-[var(--bg-elevated)] px-3 text-sm text-[var(--text-muted)] shadow-[var(--shadow-xs)] transition-all hover:border-[var(--border-hover)] hover:bg-[var(--bg-surface)]"
          >
            <Search size={15} />
            <span className="flex-1 text-left text-xs">搜索...</span>
            <kbd className="rounded-md border border-[var(--border-default)] bg-[var(--bg-surface)] px-1.5 py-0.5 font-mono text-[9px] text-[var(--text-tertiary)]">
              ⌘ K
            </kbd>
          </button>

          <div className="ml-auto flex items-center gap-2">
            <div className="hidden items-center gap-1.5 rounded-full border border-[var(--border-default)] bg-[var(--bg-elevated)] px-2.5 py-1.5 text-[10px] font-medium text-[var(--text-secondary)] 2xl:flex">
              <Sparkles size={11} className="text-[var(--accent-primary)]" />
              安全工作区
            </div>

            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button className="flex h-10 items-center gap-2.5 rounded-lg px-2 transition-colors hover:bg-[var(--bg-elevated)]">
                  <Avatar size="sm">
                    <AvatarFallback className="bg-[var(--accent-muted)] text-xs font-semibold text-[var(--accent-primary)]">
                      {initial}
                    </AvatarFallback>
                  </Avatar>
                  <div className="hidden text-left 2xl:block">
                    <div className="max-w-24 truncate text-xs font-medium text-[var(--text-primary)]">
                      你好，{user?.username ?? "用户"}
                    </div>
                    <div className="text-[9px] uppercase tracking-wide text-[var(--text-muted)]">
                      {user?.role ?? "member"}
                    </div>
                  </div>
                  <ChevronDown size={13} className="text-[var(--text-muted)]" />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-52">
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
                    localStorage.removeItem("refresh_token");
                    window.location.href = "/login";
                  }}
                >
                  <LogOut size={14} />
                  退出登录
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        <main className="app-canvas flex-1 overflow-auto p-6 lg:p-8">
          <Outlet />
        </main>
      </div>

      <ChangePasswordDialog open={pwdOpen} onOpenChange={setPwdOpen} />
      <CommandPalette open={cmdOpen} onOpenChange={setCmdOpen} />
    </div>
  );
}
