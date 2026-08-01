import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { Toaster } from "sonner";
import { TooltipProvider } from "@/shared/components/ui/tooltip";
import Layout from "@/shared/components/Layout";
import AuthGuard from "@/shared/components/AuthGuard";
import ErrorPage from "@/shared/components/ErrorPage";
import { lazyPage } from "@/shared/components/LazyLoad";

// ── 路由级懒加载：Vite 会为每个 lazy() 自动拆分独立 chunk ──────────────────

const DashboardPage = lazyPage(() => import("@/features/ops/pages/Dashboard"), {
  title: "仪表盘页面出现了问题",
});

const LoginPage = lazyPage(() => import("@/features/iam/pages/Login"), {
  title: "登录页面出现了问题",
});

const QueryPage = lazyPage(() => import("@/features/query/pages/Query"), {
  title: "查询页面出现了问题",
});

const PerformancePage = lazyPage(() => import("@/features/ops/pages/Performance"), {
  title: "性能分析页面出现了问题",
});

const TicketPage = lazyPage(() => import("@/features/ticket/pages/Ticket"), {
  title: "工单页面出现了问题",
});

const TicketNewPage = lazyPage(() => import("@/features/ticket/pages/TicketNew"), {
  title: "新建工单页面出现了问题",
});

const AuditPage = lazyPage(() => import("@/features/audit/pages/Audit"), {
  title: "审计页面出现了问题",
});

const ReportsPage = lazyPage(() => import("@/features/audit/pages/Reports"), {
  title: "报表页面出现了问题",
});

const UsersPage = lazyPage(() => import("@/features/iam/pages/Users"), {
  title: "用户管理页面出现了问题",
});

const PermissionsPage = lazyPage(() => import("@/features/security/pages/Permissions"), {
  title: "权限管理页面出现了问题",
});

const SettingsPage = lazyPage(() => import("@/features/ops/pages/Settings"), {
  title: "设置页面出现了问题",
});

const TokenPage = lazyPage(() => import("@/features/iam/pages/TokenPage"), {
  title: "Token 管理页面出现了问题",
});

const PermRequestPage = lazyPage(() => import("@/features/security/pages/PermRequestPage"), {
  title: "临时权限页面出现了问题",
});

const SQLTemplatePage = lazyPage(() => import("@/features/query/pages/SQLTemplatePage"), {
  title: "SQL 模板页面出现了问题",
});

const SharedResultPage = lazyPage(() => import("@/features/query/pages/SharedResultPage"), {
  title: "共享结果页面出现了问题",
});

// ── App ─────────────────────────────────────────────────────────────────────

function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/s/:token" element={<SharedResultPage />} />
          <Route
            element={
              <AuthGuard>
                <Layout />
              </AuthGuard>
            }
          >
            <Route path="/" element={<DashboardPage />} />
            <Route path="/query" element={<QueryPage />} />
            <Route path="/performance" element={<PerformancePage />} />
            <Route path="/tickets" element={<TicketPage />} />
            <Route path="/tickets/new" element={<TicketNewPage />} />
            <Route path="/permissions" element={<PermissionsPage />} />
            <Route path="/audit" element={<AuditPage />} />
            <Route path="/reports" element={<ReportsPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/settings/datasource" element={<SettingsPage />} />
            <Route path="/settings/mask-rules" element={<SettingsPage />} />
            <Route path="/settings/ai-config" element={<SettingsPage />} />
            <Route path="/settings/:tab" element={<SettingsPage />} />
            <Route path="/tokens" element={<TokenPage />} />
            <Route path="/permission-requests" element={<PermRequestPage />} />
            <Route path="/sql-templates" element={<SQLTemplatePage />} />
            <Route path="/403" element={<ErrorPage code={403} />} />
            <Route path="/404" element={<ErrorPage code={404} />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
        <Toaster
          richColors
          position="top-right"
          expand
          visibleToasts={5}
          gap={10}
        />
      </TooltipProvider>
    </BrowserRouter>
  );
}

export default App;
