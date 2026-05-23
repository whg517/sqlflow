import { api } from './client'

export interface DashboardStats {
  pending_tickets: number
  recent_queries_7d: number
  active_datasources: number
  total_users: number
  sensitive_tables?: number
}

export async function getDashboardStats(): Promise<{ code: number; data: DashboardStats }> {
  return api.get('/dashboard/stats')
}
