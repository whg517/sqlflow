import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Toaster } from 'sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import Layout from './components/Layout'
import QueryPage from './pages/Query'
import TicketPage from './pages/Ticket'
import TicketNewPage from './pages/TicketNew'
import AuditPage from './pages/Audit'
import PermissionsPage from './pages/Permissions'
import SettingsPage from './pages/Settings'
import DataSourcePage from './pages/settings/DataSource'
import MaskRulesPage from './pages/settings/MaskRules'
import AIConfigPage from './pages/settings/AIConfig'
import LoginPage from './pages/Login'

function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route element={<Layout />}>
            <Route path="/query" element={<QueryPage />} />
            <Route path="/tickets" element={<TicketPage />} />
            <Route path="/tickets/new" element={<TicketNewPage />} />
            <Route path="/permissions" element={<PermissionsPage />} />
            <Route path="/audit" element={<AuditPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/settings/datasource" element={<DataSourcePage />} />
            <Route path="/settings/mask-rules" element={<MaskRulesPage />} />
            <Route path="/settings/ai-config" element={<AIConfigPage />} />
            <Route path="*" element={<Navigate to="/query" replace />} />
          </Route>
        </Routes>
        <Toaster richColors position="top-right" />
      </TooltipProvider>
    </BrowserRouter>
  )
}

export default App
