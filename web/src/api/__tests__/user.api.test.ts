import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

import {
  listUsers,
  createUser,
  updateUser,
  deleteUser,
  resetPassword,
} from '@/api/user'

function mockSuccess(data: any) {
  mockFetch.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({ code: 0, data }),
  })
}

describe('User API Functions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    localStorage.setItem('token', 'test-token')
  })

  afterEach(() => localStorage.clear())

  describe('listUsers', () => {
    it('fetches users with default pagination', async () => {
      mockSuccess({ users: [], total: 0 })
      const res = await listUsers()
      expect(res.data.users).toEqual([])
      expect(mockFetch).toHaveBeenCalledWith('/api/users?page=1&page_size=20', expect.any(Object))
    })

    it('fetches users with custom pagination', async () => {
      mockSuccess({ users: [{ id: 1 }], total: 1 })
      const res = await listUsers(2, 50)
      expect(res.data.users).toHaveLength(1)
      const url = mockFetch.mock.calls[0][0]
      expect(url).toContain('page=2')
      expect(url).toContain('page_size=50')
    })
  })

  describe('createUser', () => {
    it('creates a new user', async () => {
      mockSuccess({ id: 1, username: 'newuser' })
      const res = await createUser({
        username: 'newuser',
        password: 'password123',
        role: 'developer',
      })
      expect(res.data.username).toBe('newuser')
      expect(mockFetch).toHaveBeenCalledWith('/api/users', expect.objectContaining({
        method: 'POST',
      }))
    })
  })

  describe('updateUser', () => {
    it('updates a user', async () => {
      mockSuccess({ id: 1, role: 'admin' })
      const res = await updateUser(1, { role: 'admin' })
      expect(res.data.role).toBe('admin')
      expect(mockFetch).toHaveBeenCalledWith('/api/users/1', expect.objectContaining({
        method: 'PUT',
      }))
    })
  })

  describe('deleteUser', () => {
    it('deletes a user', async () => {
      mockSuccess({ message: 'deleted' })
      const res = await deleteUser(1)
      expect(mockFetch).toHaveBeenCalledWith('/api/users/1', expect.objectContaining({
        method: 'DELETE',
      }))
    })
  })

  describe('resetPassword', () => {
    it('resets user password', async () => {
      mockSuccess({ id: 1, username: 'test' })
      const res = await resetPassword(1, { password: 'newpass123' })
      expect(res.data.username).toBe('test')
      expect(mockFetch).toHaveBeenCalledWith('/api/users/1/reset-password', expect.objectContaining({
        method: 'PUT',
      }))
    })
  })
})
