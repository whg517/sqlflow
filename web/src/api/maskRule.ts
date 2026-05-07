import { api } from './client'

// --- Types ---

export interface MaskRule {
  id: number
  datasource_id: number
  database: string
  table_name: string
  field: string
  mask_type: string
  custom_regex?: string
  custom_template?: string
  created_at: string
  updated_at: string
}

export interface SensitiveTable {
  id: number
  datasource_id: number
  database: string
  table_name: string
  sensitivity_level: 'low' | 'medium' | 'high'
  created_at: string
  updated_at: string
}

export interface MaskRuleListResponse {
  code: number
  message: string
  data: MaskRule[]
  page: number
  page_size: number
  total: number
}

export interface SensitiveTableListResponse {
  code: number
  message: string
  data: SensitiveTable[]
  page: number
  page_size: number
  total: number
}

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export interface CreateMaskRuleRequest {
  datasource_id: number
  database: string
  table_name: string
  field: string
  mask_type: string
  custom_regex?: string
  custom_template?: string
}

export interface UpdateMaskRuleRequest {
  table_name: string
  field: string
  mask_type: string
  custom_regex?: string
  custom_template?: string
}

export interface CreateSensitiveTableRequest {
  datasource_id: number
  database: string
  table_name: string
  sensitivity_level: string
}

export interface TableInfo {
  name: string
}

// --- Mask Rules API ---

export async function listMaskRules(params?: {
  page?: number
  page_size?: number
  datasource_id?: string
  database?: string
  table_name?: string
}): Promise<MaskRuleListResponse> {
  const qs = new URLSearchParams()
  if (params?.page) qs.set('page', String(params.page))
  if (params?.page_size) qs.set('page_size', String(params.page_size))
  if (params?.datasource_id) qs.set('datasource_id', params.datasource_id)
  if (params?.database) qs.set('database', params.database)
  if (params?.table_name) qs.set('table_name', params.table_name)
  const query = qs.toString()
  return api.get<MaskRuleListResponse>(`/mask-rules${query ? `?${query}` : ''}`)
}

export async function getMaskRule(id: number): Promise<ApiResponse<MaskRule>> {
  return api.get<ApiResponse<MaskRule>>(`/mask-rules/${id}`)
}

export async function createMaskRule(req: CreateMaskRuleRequest): Promise<ApiResponse<MaskRule>> {
  return api.post<ApiResponse<MaskRule>>('/mask-rules', req)
}

export async function updateMaskRule(id: number, req: UpdateMaskRuleRequest): Promise<ApiResponse<MaskRule>> {
  return api.put<ApiResponse<MaskRule>>(`/mask-rules/${id}`, req)
}

export async function deleteMaskRule(id: number): Promise<ApiResponse<{ message: string }>> {
  return api.del<ApiResponse<{ message: string }>>(`/mask-rules/${id}`)
}

// --- Sensitive Tables API ---

export async function listSensitiveTables(params?: {
  page?: number
  page_size?: number
  datasource_id?: string
  database?: string
  table_name?: string
}): Promise<SensitiveTableListResponse> {
  const qs = new URLSearchParams()
  if (params?.page) qs.set('page', String(params.page))
  if (params?.page_size) qs.set('page_size', String(params.page_size))
  if (params?.datasource_id) qs.set('datasource_id', params.datasource_id)
  if (params?.database) qs.set('database', params.database)
  if (params?.table_name) qs.set('table_name', params.table_name)
  const query = qs.toString()
  return api.get<SensitiveTableListResponse>(`/sensitive-tables${query ? `?${query}` : ''}`)
}

export async function createSensitiveTable(req: CreateSensitiveTableRequest): Promise<ApiResponse<SensitiveTable>> {
  return api.post<ApiResponse<SensitiveTable>>('/sensitive-tables', req)
}

export async function deleteSensitiveTable(id: number): Promise<ApiResponse<{ message: string }>> {
  return api.del<ApiResponse<{ message: string }>>(`/sensitive-tables/${id}`)
}

// --- Datasource Tables API ---

export async function fetchDatasourceTables(datasourceId: number): Promise<ApiResponse<string[]>> {
  return api.get<ApiResponse<string[]>>(`/datasources/${datasourceId}/tables`)
}

// --- Mask Type Labels ---

export const MASK_TYPE_OPTIONS = [
  { value: 'phone', label: '手机号' },
  { value: 'id_card', label: '身份证' },
  { value: 'name', label: '姓名' },
  { value: 'email', label: '邮箱' },
  { value: 'bank_card', label: '银行卡' },
  { value: 'address', label: '地址' },
  { value: 'full', label: '全掩码' },
  { value: 'custom', label: '自定义正则' },
] as const

export function getMaskTypeLabel(type: string): string {
  return MASK_TYPE_OPTIONS.find((o) => o.value === type)?.label ?? type
}

export const SENSITIVITY_OPTIONS = [
  { value: 'low', label: '低' },
  { value: 'medium', label: '中' },
  { value: 'high', label: '高' },
] as const

export function getSensitivityLabel(level: string): string {
  return SENSITIVITY_OPTIONS.find((o) => o.value === level)?.label ?? level
}

export const SENSITIVITY_BADGE: Record<string, { label: string; cls: string }> = {
  low: { label: '低', cls: 'bg-emerald-500/20 text-emerald-400' },
  medium: { label: '中', cls: 'bg-yellow-500/20 text-yellow-400' },
  high: { label: '高', cls: 'bg-red-500/20 text-red-400' },
}
