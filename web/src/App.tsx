import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { Toaster } from "sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import Layout from "./components/Layout";
import AuthGuard from "./components/AuthGuard";
import ErrorPage from "./components/ErrorPage";
import DashboardPage from "./pages/Dashboard";
import QueryPage from "./pages/Query";
import TicketPage from "./pages/Ticket";
import TicketNewPage from "./pages/TicketNew";
import AuditPage from "./pages/Audit";
import UsersPage from "./pages/Users";
import PermissionsPage from "./pages/Permissions";
import SettingsPage from "./pages/Settings";
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
            <Route path="/query" element={<QueryPage />} />
            <Route path="/tickets" element={<TicketPage />} />
            <Route path="/tickets/new" element={<TicketNewPage />} />
            <Route path="/permissions" element={<PermissionsPage />} />
            <Route path="/audit" element={<AuditPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/settings/datasource" element={<SettingsPage />} />
            <Route path="/settings/mask-rules" element={<SettingsPage />} />
            <Route path="/settings/ai-config" element={<SettingsPage />} />
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
