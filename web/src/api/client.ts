import { toast } from 'sonner'

const API_BASE = '/api'

function getToken(): string | null {
  return localStorage.getItem('token')
}

function handleUnauthorized() {
  localStorage.removeItem('token')
  window.location.href = '/login'
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: body != null ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    handleUnauthorized()
    throw new Error('Unauthorized')
  }

  if (res.status >= 500) {
    toast.error('服务器错误，请稍后重试')
    throw new Error(`Server error: ${res.status}`)
  }

  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.message || `Request failed: ${res.status}`)
  }

  return res.json()
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body: unknown) => request<T>('PUT', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
}
