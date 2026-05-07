import { api } from './client'

// --- Types ---

export interface QueryExecuteRequest {
  datasource_id: number
  database: string
  sql: string
}

export interface QueryResult {
  columns: string[]
  rows: Record<string, unknown>[]
  total: number
  execution_time_ms: number
  affected_rows: number
  desensitized: boolean
  desensitized_fields: string[]
  warnings: string[]
}

export interface QueryExecuteResponse {
  code: number
  message: string
  data: QueryResult
}

export interface QueryExportRequest {
  datasource_id: number
  database: string
  sql: string
  format: 'csv' | 'json'
}

export interface QueryHistoryItem {
  id: number
  user_id: number
  datasource_id: number
  database: string
  sql_content: string
  sql_summary: string
  db_type: string
  execution_time: number
  result_rows: number
  affected_rows: number
  created_at: string
}

export interface QueryHistoryResponse {
  code: number
  message: string
  data: QueryHistoryItem[]
  page: number
  page_size: number
  total: number
}

export interface ApiResponse {
  code: number
  message: string
}

// --- AI Review Types ---

export type ReviewDecision = 'execute' | 'confirm' | 'ticket' | 'blocked' | 'fallback'

export interface AIReviewResult {
  risk_level: 'low' | 'medium' | 'high'
  risk_score: number
  decision: ReviewDecision
  summary: string
  suggestions: string[]
  impact_analysis: string
  rollback_sql: string
  warnings: string[]
  review_source: string
  reviewed_at: string
  expires_at: string
  model_used: string
}

export interface AIReviewSSEEvent {
  type: 'content' | 'result' | 'error' | 'done'
  data: unknown
}

// --- MongoDB Types ---

export interface MongoQueryBody {
  collection: string
  operation: 'find' | 'aggregate' | 'update'
  filter?: Record<string, unknown>
  pipeline?: unknown[]
  options?: Record<string, unknown>
}

// --- API Functions ---

export async function executeQuery(req: QueryExecuteRequest): Promise<QueryResult> {
  const res = await api.post<QueryExecuteResponse>('/query/execute', req)
  return res.data
}

/**
 * streamAIReview opens an SSE connection to the AI review endpoint.
 * Calls onEvent for each SSE event received, returns a cancel function.
 */
export function streamAIReview(
  req: QueryExecuteRequest,
  onEvent: (event: AIReviewSSEEvent) => void,
  onError: (err: Error) => void,
): () => void {
  const token = localStorage.getItem('token')
  const controller = new AbortController()

  fetch('/api/query/review', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(req),
    signal: controller.signal,
  })
    .then(async (res) => {
      if (res.status === 401) {
        localStorage.removeItem('token')
        window.location.href = '/login'
        return
      }
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        onError(new Error(body.message || `Review failed: ${res.status}`))
        return
      }

      const reader = res.body?.getReader()
      if (!reader) {
        onError(new Error('No response body'))
        return
      }

      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        let currentEvent = ''
        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEvent = line.slice(7).trim()
          } else if (line.startsWith('data: ')) {
            const dataStr = line.slice(6)
            let data: unknown = dataStr
            try {
              data = JSON.parse(dataStr)
            } catch {
              // keep raw string for content events
            }
            if (currentEvent) {
              onEvent({ type: currentEvent as AIReviewSSEEvent['type'], data })
            }
          }
        }
      }
    })
    .catch((err) => {
      if (err.name !== 'AbortError') {
        onError(err)
      }
    })

  return () => controller.abort()
}

export function buildMongoSql(body: MongoQueryBody): string {
  return JSON.stringify(body, null, 2)
}

export async function exportQuery(req: QueryExportRequest): Promise<void> {
  const token = localStorage.getItem('token')
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const res = await fetch('/api/query/export', {
    method: 'POST',
    headers,
    body: JSON.stringify(req),
  })

  if (res.status === 401) {
    localStorage.removeItem('token')
    window.location.href = '/login'
    throw new Error('Unauthorized')
  }

  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.message || `Export failed: ${res.status}`)
  }

  const disposition = res.headers.get('Content-Disposition') || ''
  const match = disposition.match(/filename=(.+)/)
  const filename = match ? match[1] : `export.${req.format}`

  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

export async function fetchHistory(page = 1, pageSize = 50): Promise<QueryHistoryResponse> {
  return api.get<QueryHistoryResponse>(`/query/history?page=${page}&page_size=${pageSize}`)
}

export async function deleteHistory(id: number): Promise<ApiResponse> {
  return api.del<ApiResponse>(`/query/history/${id}`)
}

export async function clearHistory(): Promise<ApiResponse> {
  return api.del<ApiResponse>('/query/history')
}
