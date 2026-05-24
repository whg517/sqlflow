import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

import {
  listMaskRules,
  getMaskRule,
  createMaskRule,
  updateMaskRule,
  deleteMaskRule,
  listSensitiveTables,
  createSensitiveTable,
  deleteSensitiveTable,
  fetchDatasourceTables,
} from '@/api/maskRule'

function mockJson(data: unknown) {
  mockFetch.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => data,
  })
}

describe('MaskRule API Functions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    localStorage.setItem('token', 'test-token')
  })

  afterEach(() => localStorage.clear())

  describe('listMaskRules', () => {
    it('fetches mask rules with no params', async () => {
      mockJson({ data: [], page: 1, page_size: 50, total: 0 })
      const res = await listMaskRules()
      expect(res.data).toEqual([])
    })

    it('fetches mask rules with filters', async () => {
      mockJson({ data: [], page: 1, page_size: 20, total: 0 })
      await listMaskRules({
        page: 1, page_size: 20,
        datasource_id: '5', database: 'testdb', table_name: 'users',
      })
      const url = mockFetch.mock.calls[0][0]
      expect(url).toContain('datasource_id=5')
      expect(url).toContain('database=testdb')
      expect(url).toContain('table_name=users')
    })
  })

  describe('getMaskRule', () => {
    it('fetches single mask rule', async () => {
      mockJson({ code: 0, data: { id: 1, field: 'phone', mask_type: 'phone' } })
      const res = await getMaskRule(1)
      expect(res.data.field).toBe('phone')
    })
  })

  describe('createMaskRule', () => {
    it('creates a mask rule via POST', async () => {
      mockJson({ code: 0, data: { id: 1, mask_type: 'email' } })
      const res = await createMaskRule({
        datasource_id: 10, database: 'db', table_name: 'users',
        field: 'email', mask_type: 'email',
      })
      expect(res.data.mask_type).toBe('email')
      expect(mockFetch).toHaveBeenCalledWith('/api/mask-rules', expect.objectContaining({ method: 'POST' }))
    })
  })

  describe('updateMaskRule', () => {
    it('updates a mask rule via PUT', async () => {
      mockJson({ code: 0, data: { id: 1, mask_type: 'full' } })
      const res = await updateMaskRule(1, { table_name: 'users', field: 'phone', mask_type: 'full' })
      expect(res.data.mask_type).toBe('full')
      expect(mockFetch).toHaveBeenCalledWith('/api/mask-rules/1', expect.objectContaining({ method: 'PUT' }))
    })
  })

  describe('deleteMaskRule', () => {
    it('deletes a mask rule via DELETE', async () => {
      mockJson({ code: 0, data: { message: 'deleted' } })
      const res = await deleteMaskRule(1)
      expect(res.data.message).toBe('deleted')
      expect(mockFetch).toHaveBeenCalledWith('/api/mask-rules/1', expect.objectContaining({ method: 'DELETE' }))
    })
  })

  describe('listSensitiveTables', () => {
    it('fetches sensitive tables', async () => {
      mockJson({ data: [], page: 1, page_size: 50, total: 0 })
      const res = await listSensitiveTables()
      expect(res.data).toEqual([])
    })

    it('passes filter params', async () => {
      mockJson({ data: [], page: 1, page_size: 10, total: 0 })
      await listSensitiveTables({ datasource_id: '5', database: 'db' })
      const url = mockFetch.mock.calls[0][0]
      expect(url).toContain('datasource_id=5')
      expect(url).toContain('database=db')
    })
  })

  describe('createSensitiveTable', () => {
    it('creates a sensitive table entry', async () => {
      mockJson({ code: 0, data: { id: 1, sensitivity_level: 'high' } })
      const res = await createSensitiveTable({
        datasource_id: 10, database: 'db', table_name: 'users', sensitivity_level: 'high',
      })
      expect(res.data.sensitivity_level).toBe('high')
    })
  })

  describe('deleteSensitiveTable', () => {
    it('deletes a sensitive table entry', async () => {
      mockJson({ code: 0, data: { message: 'deleted' } })
      const res = await deleteSensitiveTable(1)
      expect(res.data.message).toBe('deleted')
    })
  })

  describe('fetchDatasourceTables', () => {
    it('fetches table names for a datasource', async () => {
      mockJson({ code: 0, data: ['users', 'orders', 'products'] })
      const res = await fetchDatasourceTables(10)
      expect(res.data).toEqual(['users', 'orders', 'products'])
    })
  })
})
