import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'

// --- Mocks ---

vi.mock('@/api/maskRule', () => ({
  listSensitiveTables: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  api: {
    get: vi.fn(),
  },
}))

import { listSensitiveTables } from '@/api/maskRule'
import { useSensitiveTables } from '@/hooks/useSensitiveTables'

const mockedListSensitiveTables = vi.mocked(listSensitiveTables)

describe('useSensitiveTables', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns empty map when datasourceId is null', () => {
    const { result } = renderHook(() => useSensitiveTables(null))
    expect(result.current.sensitiveMap.size).toBe(0)
    expect(result.current.loading).toBe(false)
  })

  it('fetches sensitive tables for a datasource', async () => {
    mockedListSensitiveTables.mockResolvedValueOnce({
      data: [
        { id: 1, table_name: 'users', sensitivity_level: 'high' },
        { id: 2, table_name: 'orders', sensitivity_level: 'medium' },
      ],
      page: 1,
      page_size: 500,
      total: 2,
    })

    const { result } = renderHook(() => useSensitiveTables(1))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(mockedListSensitiveTables).toHaveBeenCalledWith({
      datasource_id: '1',
      page_size: 500,
    })
    expect(result.current.sensitiveMap.size).toBe(2)
  })

  it('isSensitive returns matching table (case-insensitive)', async () => {
    mockedListSensitiveTables.mockResolvedValueOnce({
      data: [{ id: 1, table_name: 'Users', sensitivity_level: 'high' }],
      page: 1,
      page_size: 500,
      total: 1,
    })

    const { result } = renderHook(() => useSensitiveTables(1))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    const sensitive = result.current.isSensitive('users')
    expect(sensitive).toBeDefined()
    expect(sensitive?.table_name).toBe('Users')
  })

  it('isSensitive returns undefined for non-sensitive table', async () => {
    mockedListSensitiveTables.mockResolvedValueOnce({
      data: [{ id: 1, table_name: 'users', sensitivity_level: 'high' }],
      page: 1,
      page_size: 500,
      total: 1,
    })

    const { result } = renderHook(() => useSensitiveTables(1))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(result.current.isSensitive('orders')).toBeUndefined()
  })

  it('caches results and does not re-fetch for same datasource', async () => {
    mockedListSensitiveTables.mockResolvedValue({
      data: [{ id: 1, table_name: 'users', sensitivity_level: 'high' }],
      page: 1,
      page_size: 500,
      total: 1,
    })

    const { result, rerender } = renderHook(({ dsId }) => useSensitiveTables(dsId), {
      initialProps: { dsId: 1 },
    })

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(mockedListSensitiveTables).toHaveBeenCalledTimes(1)

    // Re-render with same datasource
    rerender({ dsId: 1 })

    // Wait a tick to ensure no new calls
    await act(async () => {
      await new Promise(r => setTimeout(r, 10))
    })

    expect(mockedListSensitiveTables).toHaveBeenCalledTimes(1)
  })

  it('handles API errors gracefully', async () => {
    mockedListSensitiveTables.mockRejectedValueOnce(new Error('Network error'))

    const { result } = renderHook(() => useSensitiveTables(1))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(result.current.sensitiveMap.size).toBe(0)
  })

  it('refetch function is callable', async () => {
    mockedListSensitiveTables.mockResolvedValue({
      data: [{ id: 1, table_name: 'users', sensitivity_level: 'high' }],
      page: 1,
      page_size: 500,
      total: 1,
    })

    const { result } = renderHook(() => useSensitiveTables(1))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    // refetch is a function that can be called
    expect(typeof result.current.refetch).toBe('function')
  })
})
