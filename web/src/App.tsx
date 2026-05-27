import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { Toaster } from "sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import Layout from "./components/Layout";
import AuthGuard from "./components/AuthGuard";
import ErrorBoundary from "./components/ErrorBoundary";
import ErrorPage from "./components/ErrorPage";
import DashboardPage from "./pages/Dashboard";
import QueryPage from "./pages/Query";
import PerformancePage from "./pages/Performance";
import TicketPage from "./pages/Ticket";
import TicketNewPage from "./pages/TicketNew";
import AuditPage from "./pages/Audit";
import ReportsPage from "./pages/Reports";
import UsersPage from "./pages/Users";
import PermissionsPage from "./pages/Permissions";
import SettingsPage from "./pages/Settings";
import TokenPage from "./pages/TokenPage";
import LoginPage from "./pages/Login";

function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            element={
              <AuthGuard>
                <Layout />
              </AuthGuard>
            }
          >
            <Route path="/" element={<DashboardPage />} />
            <Route
              path="/query"
              element={
                <ErrorBoundary title="查询页面出现了问题">
                  <QueryPage />
                </ErrorBoundary>
              }
            />
            <Route
              path="/performance"
              element={
                <ErrorBoundary title="性能分析页面出现了问题">
                  <PerformancePage />
                </ErrorBoundary>
              }
            />
            <Route
              path="/tickets"
              element={
                <ErrorBoundary title="工单页面出现了问题">
                  <TicketPage />
                </ErrorBoundary>
              }
            />
            <Route path="/tickets/new" element={<TicketNewPage />} />
            <Route path="/permissions" element={<PermissionsPage />} />
            <Route
              path="/audit"
              element={
                <ErrorBoundary title="审计页面出现了问题">
                  <AuditPage />
                </ErrorBoundary>
              }
            />
            <Route
              path="/reports"
              element={
                <ErrorBoundary title="报表页面出现了问题">
                  <ReportsPage />
                </ErrorBoundary>
              }
            />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/settings/datasource" element={<SettingsPage />} />
            <Route path="/settings/mask-rules" element={<SettingsPage />} />
            <Route path="/settings/ai-config" element={<SettingsPage />} />
            <Route
              path="/tokens"
              element={
                <ErrorBoundary title="Token 管理页面出现了问题">
                  <TokenPage />
                </ErrorBoundary>
              }
            />
            <Route path="/403" element={<ErrorPage code={403} />} />
            <Route path="/404" element={<ErrorPage code={404} />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
        <Toaster richColors position="top-right" />
      </TooltipProvider>
    </BrowserRouter>
  );
}

export default App;
