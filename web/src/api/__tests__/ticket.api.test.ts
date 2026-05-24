import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

import {
  listTickets,
  getTicket,
  createTicket,
  approveTicket,
  rejectTicket,
  cancelTicket,
  executeTicket,
} from '@/api/ticket'

function mockJson(data: unknown) {
  mockFetch.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => data,
  })
}

describe('Ticket API Functions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    localStorage.setItem('token', 'test-token')
  })

  afterEach(() => localStorage.clear())

  describe('listTickets', () => {
    it('fetches tickets with default params', async () => {
      mockJson({ data: [], page: 1, page_size: 50, total: 0 })
      const res = await listTickets()
      expect(res.data).toEqual([])
      expect(res.page).toBe(1)
    })

    it('fetches tickets with filters', async () => {
      mockJson({ data: [], page: 1, page_size: 10, total: 0 })
      const res = await listTickets({
        page: 1,
        page_size: 10,
        status: 'PENDING_APPROVAL',
        keyword: 'ALTER',
        scope: 'mine',
      })
      expect(res.page_size).toBe(10)
      const url = mockFetch.mock.calls[0][0]
      expect(url).toContain('page=1')
      expect(url).toContain('status=PENDING_APPROVAL')
      expect(url).toContain('keyword=ALTER')
      expect(url).toContain('scope=mine')
    })
  })

  describe('getTicket', () => {
    it('fetches ticket by id', async () => {
      const ticket = { id: 1, sql_content: 'SELECT 1', status: 'APPROVED' }
      mockJson({ code: 0, data: ticket })
      const res = await getTicket(1)
      expect(res.data.id).toBe(1)
      expect(res.data.status).toBe('APPROVED')
    })
  })

  describe('createTicket', () => {
    it('creates a new ticket', async () => {
      const ticket = { id: 1, status: 'SUBMITTED' }
      mockJson({ code: 0, data: ticket })
      const res = await createTicket({
        datasource_id: 10,
        database: 'testdb',
        sql: 'SELECT 1',
        db_type: 'mysql',
        change_reason: 'test',
      })
      expect(res.data.status).toBe('SUBMITTED')
      const body = JSON.parse(mockFetch.mock.calls[0][1].body)
      expect(body.datasource_id).toBe(10)
      expect(body.sql).toBe('SELECT 1')
    })
  })

  describe('approveTicket', () => {
    it('approves a ticket', async () => {
      mockJson({ code: 0, data: { id: 1, status: 'APPROVED' } })
      const res = await approveTicket(1, 'OK')
      expect(res.data.status).toBe('APPROVED')
      const body = JSON.parse(mockFetch.mock.calls[0][1].body)
      expect(body.comment).toBe('OK')
    })
  })

  describe('rejectTicket', () => {
    it('rejects a ticket', async () => {
      mockJson({ code: 0, data: { id: 1, status: 'REJECTED' } })
      const res = await rejectTicket(1, 'Too risky')
      expect(res.data.status).toBe('REJECTED')
    })
  })

  describe('cancelTicket', () => {
    it('cancels a ticket', async () => {
      mockJson({ code: 0, data: { id: 1, status: 'CANCELLED' } })
      const res = await cancelTicket(1, 'No longer needed')
      expect(res.data.status).toBe('CANCELLED')
    })
  })

  describe('executeTicket', () => {
    it('executes a ticket', async () => {
      mockJson({ code: 0, data: { id: 1, status: 'EXECUTING' } })
      const res = await executeTicket(1)
      expect(res.data.status).toBe('EXECUTING')
    })
  })
})
