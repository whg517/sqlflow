import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

import {
  listComments,
  createComment,
  deleteComment,
} from '@/api/comment'

function mockSuccess(data: any) {
  mockFetch.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({ code: 0, data }),
  })
}

describe('Comment API Functions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    localStorage.setItem('token', 'test-token')
  })

  afterEach(() => localStorage.clear())

  describe('listComments', () => {
    it('fetches comments for a ticket', async () => {
      mockSuccess([])
      const res = await listComments(1)
      expect(res.data).toEqual([])
      expect(mockFetch).toHaveBeenCalledWith('/api/tickets/1/comments', expect.any(Object))
    })
  })

  describe('createComment', () => {
    it('creates a comment', async () => {
      const comment = { id: 1, content: 'Looks good', parent_id: 0 }
      mockSuccess(comment)
      const res = await createComment(1, 'Looks good')
      expect(res.data.content).toBe('Looks good')
      const body = JSON.parse(mockFetch.mock.calls[0][1].body)
      expect(body.content).toBe('Looks good')
      expect(body.parent_id).toBe(0)
    })

    it('creates a reply comment with parent_id', async () => {
      mockSuccess({ id: 2, content: 'Reply', parent_id: 1 })
      const res = await createComment(1, 'Reply', 1)
      expect(res.data.parent_id).toBe(1)
      const body = JSON.parse(mockFetch.mock.calls[0][1].body)
      expect(body.parent_id).toBe(1)
    })

    it('defaults parent_id to 0 when not provided', async () => {
      mockSuccess({ id: 3, content: 'Comment', parent_id: 0 })
      await createComment(1, 'Comment')
      const body = JSON.parse(mockFetch.mock.calls[0][1].body)
      expect(body.parent_id).toBe(0)
    })
  })

  describe('deleteComment', () => {
    it('deletes a comment', async () => {
      mockSuccess({ id: 1 })
      const res = await deleteComment(1)
      expect(res.data.id).toBe(1)
      expect(mockFetch).toHaveBeenCalledWith('/api/comments/1', expect.objectContaining({
        method: 'DELETE',
      }))
    })
  })
})
