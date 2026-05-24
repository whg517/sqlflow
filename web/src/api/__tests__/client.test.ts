import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// --- Mocks ---

// Mock toast before importing client
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

import { api } from '@/api/client'

describe('API Client', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    delete (window as any).location
    ;(window as any).location = { href: '' }
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  describe('api.get', () => {
    it('makes GET request with correct URL', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0, data: [] }),
      })

      const result = await api.get('/test')

      expect(mockFetch).toHaveBeenCalledWith('/api/test', {
        method: 'GET',
        headers: { 'Content-Type': 'application/json' },
      })
      expect(result).toEqual({ code: 0, data: [] })
    })

    it('includes Authorization header when token exists', async () => {
      localStorage.setItem('token', 'test-jwt')

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0 }),
      })

      await api.get('/test')

      expect(mockFetch).toHaveBeenCalledWith('/api/test', {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
          Authorization: 'Bearer test-jwt',
        },
      })
    })

    it('does not include Authorization header when no token', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0 }),
      })

      await api.get('/test')

      const headers = mockFetch.mock.calls[0][1].headers
      expect(headers).not.toHaveProperty('Authorization')
    })
  })

  describe('api.post', () => {
    it('makes POST request with body', async () => {
      const body = { name: 'test' }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0, data: { id: 1 } }),
      })

      const result = await api.post('/items', body)

      expect(mockFetch).toHaveBeenCalledWith('/api/items', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      expect(result).toEqual({ code: 0, data: { id: 1 } })
    })
  })

  describe('api.put', () => {
    it('makes PUT request with body', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0 }),
      })

      await api.put('/items/1', { name: 'updated' })

      expect(mockFetch).toHaveBeenCalledWith('/api/items/1', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: 'updated' }),
      })
    })
  })

  describe('api.del', () => {
    it('makes DELETE request', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0 }),
      })

      await api.del('/items/1')

      expect(mockFetch).toHaveBeenCalledWith('/api/items/1', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
      })
    })
  })

  describe('error handling', () => {
    it('redirects to /login on 401 and removes token', async () => {
      localStorage.setItem('token', 'expired-token')

      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({ message: 'Unauthorized' }),
      })

      await expect(api.get('/protected')).rejects.toThrow('Unauthorized')

      expect(localStorage.getItem('token')).toBeNull()
      expect(window.location.href).toBe('/login')
    })

    it('redirects to /403 on 403', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 403,
        json: async () => ({ message: 'Forbidden' }),
      })

      await expect(api.get('/admin')).rejects.toThrow('Forbidden')

      expect(window.location.href).toBe('/403')
    })

    it('shows server error toast on 500', async () => {
      const { toast } = await import('sonner')

      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: async () => ({ message: 'Internal Server Error' }),
      })

      await expect(api.get('/error')).rejects.toThrow('Server error: 500')

      expect(toast.error).toHaveBeenCalledWith('服务器错误，请稍后重试')
    })

    it('throws error with message from response for other errors', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: async () => ({ message: 'Validation failed' }),
      })

      await expect(api.get('/validate')).rejects.toThrow('Validation failed')
    })

    it('throws generic error when response JSON fails', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: () => Promise.reject(new Error('Invalid JSON')),
      })

      await expect(api.get('/bad')).rejects.toThrow('Request failed: 400')
    })
  })
})
