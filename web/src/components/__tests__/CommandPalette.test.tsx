import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import React from 'react'

// --- Mocks ---

// React Router
const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}))

// API: query
const mockSearchQueryHistory = vi.fn()
vi.mock('@/api/query', () => ({
  searchQueryHistory: (...args: unknown[]) => mockSearchQueryHistory(...args),
}))

// API: ticket
const mockListTickets = vi.fn()
vi.mock('@/api/ticket', () => ({
  listTickets: (...args: unknown[]) => mockListTickets(...args),
  getStatusLabel: (status: string) => status,
  getStatusColor: (status: string) => 'bg-gray-500/20 text-gray-400',
}))

// API: audit
const mockSearchAuditLogs = vi.fn()
vi.mock('@/api/audit', () => ({
  searchAuditLogs: (...args: unknown[]) => mockSearchAuditLogs(...args),
  getActionLabel: (action: string) => action,
}))

// Zustand store
const mockRestoreHistoryAsTab = vi.fn()
vi.mock('@/store/queryStore', () => ({
  useQueryStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ restoreHistoryAsTab: mockRestoreHistoryAsTab }),
}))

// Component under test
import CommandPalette from '@/components/CommandPalette'

// --- Fixtures ---

const recentQueryItem = {
  id: 1,
  user_id: 1,
  datasource_id: 10,
  database: 'testdb',
  sql_content: 'SELECT * FROM users',
  sql_summary: 'SELECT * FROM users',
  db_type: 'mysql',
  execution_time: 50,
  result_rows: 100,
  affected_rows: 0,
  created_at: new Date(Date.now() - 5 * 60000).toISOString(), // 5 min ago
}

const searchQueryItem = {
  id: 2,
  user_id: 1,
  datasource_id: 11,
  database: 'production',
  sql_content: 'SELECT * FROM orders WHERE id = 1',
  sql_summary: 'SELECT * FROM orders WHERE id = 1',
  db_type: 'mysql',
  execution_time: 30,
  result_rows: 1,
  affected_rows: 0,
  created_at: new Date().toISOString(),
}

const ticketItem = {
  id: 100,
  submitter_id: 1,
  submitter_name: 'Alice',
  datasource_id: 10,
  database: 'testdb',
  sql_content: 'ALTER TABLE users ADD COLUMN age INT',
  sql_summary: 'ALTER TABLE users ADD COLUMN age INT',
  db_type: 'mysql',
  change_reason: 'Adding age column',
  status: 'APPROVED' as const,
  risk_level: 'low',
  ai_review_result: '',
  reviewer_id: 2,
  reviewer_name: 'Bob',
  review_comment: 'OK',
  executed_at: null,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
}

const auditLogItem = {
  id: 200,
  user_id: 1,
  username: 'Alice',
  action: 'SELECT',
  datasource_id: 10,
  database: 'testdb',
  sql_content: 'SELECT * FROM products',
  sql_summary: 'SELECT * FROM products',
  result_rows: 50,
  affected_rows: 0,
  execution_time_ms: 20,
  error_message: '',
  desensitized_fields: '',
  ip_address: '127.0.0.1',
  created_at: new Date().toISOString(),
}

// --- Helpers ---

/**
 * Renders with open=false first, then re-renders with open=true to trigger
 * the open-transition effect (prevOpenRef: false → true).
 */
function renderWithOpenTransition() {
  const onOpenChange = vi.fn()

  const utils = render(<CommandPalette open={false} onOpenChange={onOpenChange} />)

  // Trigger open transition
  utils.rerender(<CommandPalette open={true} onOpenChange={onOpenChange} />)

  return { onOpenChange, ...utils }
}

// --- Tests ---

describe('CommandPalette', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.clearAllMocks()

    // Default: search APIs succeed with empty results
    mockSearchQueryHistory.mockImplementation((kw: string) =>
      Promise.resolve({
        data: [],
        page: 1,
        page_size: 5,
        total: 0,
      })
    )
    mockListTickets.mockResolvedValue({
      data: [],
      page: 1,
      page_size: 5,
      total: 0,
    })
    mockSearchAuditLogs.mockResolvedValue({
      data: [],
      page: 1,
      page_size: 5,
      total: 0,
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // --- Render: open/close ---

  describe('rendering', () => {
    it('renders nothing when open is false', () => {
      render(<CommandPalette open={false} onOpenChange={vi.fn()} />)
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })

    it('renders dialog when open is true', () => {
      renderWithOpenTransition()
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })

    it('shows title "全局搜索"', () => {
      renderWithOpenTransition()
      expect(screen.getByText('全局搜索')).toBeInTheDocument()
    })

    it('shows search input placeholder', () => {
      renderWithOpenTransition()
      expect(
        screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')
      ).toBeInTheDocument()
    })
  })

  // --- Open transition: reset + fetch recent ---

  describe('dialog open transition', () => {
    it('fetches recent queries when dialog opens', async () => {
      renderWithOpenTransition()

      await waitFor(() => {
        expect(mockSearchQueryHistory).toHaveBeenCalledWith('', 1, 5)
      })
    })

    it('shows recent query history when available', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) => {
        if (kw === '') {
          return Promise.resolve({
            data: [recentQueryItem],
            page: 1,
            page_size: 5,
            total: 1,
          })
        }
        return Promise.resolve({ data: [], page: 1, page_size: 5, total: 0 })
      })

      renderWithOpenTransition()

      await waitFor(() => {
        expect(screen.getByText('最近查询')).toBeInTheDocument()
      })
      // The sql_summary is displayed directly (no keyword highlight when no keyword)
      expect(screen.getByText('SELECT * FROM users')).toBeInTheDocument()
      expect(screen.getByText('testdb')).toBeInTheDocument()
    })

    it('shows page navigation groups', async () => {
      renderWithOpenTransition()
      await waitFor(() => {
        expect(mockSearchQueryHistory).toHaveBeenCalledWith('', 1, 5)
      })

      expect(screen.getByText('查询')).toBeInTheDocument()
      expect(screen.getByText('变更工单')).toBeInTheDocument()
      expect(screen.getByText('权限管理')).toBeInTheDocument()
      expect(screen.getByText('审计日志')).toBeInTheDocument()
      expect(screen.getByText('数据源管理')).toBeInTheDocument()
      expect(screen.getByText('脱敏规则')).toBeInTheDocument()
      expect(screen.getByText('AI 配置')).toBeInTheDocument()
    })

    it('silently handles fetch recent queries error', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) => {
        if (kw === '') {
          return Promise.reject(new Error('Network error'))
        }
        return Promise.resolve({ data: [], page: 1, page_size: 5, total: 0 })
      })

      renderWithOpenTransition()

      await waitFor(() => {
        expect(mockSearchQueryHistory).toHaveBeenCalledWith('', 1, 5)
      })

      // Page groups should still render
      expect(screen.getByText('查询')).toBeInTheDocument()
    })
  })

  // --- Search input + 300ms debounce ---

  describe('search debounce', () => {
    it('does not trigger search immediately on typing', async () => {
      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'test')

      // Should not have been called yet (before debounce)
      expect(mockSearchQueryHistory).not.toHaveBeenCalledWith('test', 1, 5)
      expect(mockListTickets).not.toHaveBeenCalled()
      expect(mockSearchAuditLogs).not.toHaveBeenCalled()
    })

    it('triggers search after 300ms debounce', async () => {
      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'test')

      // Advance timers past debounce
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(mockSearchQueryHistory).toHaveBeenCalledWith('test', 1, 5)
        expect(mockListTickets).toHaveBeenCalledWith({
          keyword: 'test',
          page: 1,
          page_size: 5,
        })
        expect(mockSearchAuditLogs).toHaveBeenCalledWith('test', 5)
      })
    })

    it('resets search state when input is cleared', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) =>
        Promise.resolve({
          data: kw ? [searchQueryItem] : [],
          page: 1,
          page_size: 5,
          total: kw ? 1 : 0,
        })
      )

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      // Type something and trigger search
      await userEvent.type(input, 'test')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })
      await waitFor(() => {
        expect(screen.getByText('查询历史')).toBeInTheDocument()
      })

      // Clear input
      await userEvent.clear(input)
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      // Should show page groups again (empty state without keyword)
      expect(screen.getByText('查询')).toBeInTheDocument()
    })

    it('debounces correctly when typing fast', async () => {
      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'a')
      await act(() => vi.advanceTimersByTime(100))

      await userEvent.type(input, 'b')
      await act(() => vi.advanceTimersByTime(100))

      await userEvent.type(input, 'c')
      await act(() => vi.advanceTimersByTime(100))

      // Search should NOT have been called yet
      expect(mockSearchQueryHistory).not.toHaveBeenCalledWith('abc', 1, 5)

      // Now advance past 300ms from last keystroke
      await act(async () => {
        vi.advanceTimersByTime(250)
      })

      await waitFor(() => {
        expect(mockSearchQueryHistory).toHaveBeenCalledWith('abc', 1, 5)
      })
    })
  })

  // --- 3 API call verification ---

  describe('search API calls', () => {
    it('calls all 3 search APIs with correct params', async () => {
      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'orders')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(mockSearchQueryHistory).toHaveBeenCalledWith('orders', 1, 5)
        expect(mockListTickets).toHaveBeenCalledWith({
          keyword: 'orders',
          page: 1,
          page_size: 5,
        })
        expect(mockSearchAuditLogs).toHaveBeenCalledWith('orders', 5)
      })
    })
  })

  // --- Search results rendering ---

  describe('search results', () => {
    it('renders query history results with highlighted text', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) =>
        kw
          ? Promise.resolve({
              data: [searchQueryItem],
              page: 1,
              page_size: 5,
              total: 1,
            })
          : Promise.resolve({ data: [], page: 1, page_size: 5, total: 0 })
      )

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'orders')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('查询历史')).toBeInTheDocument()
      })
      // HighlightText splits text; use getAllByText for the keyword mark
      expect(screen.getByText('orders', { selector: 'mark' })).toBeInTheDocument()
      expect(screen.getByText('production')).toBeInTheDocument()
    })

    it('renders ticket results with status label', async () => {
      mockListTickets.mockResolvedValue({
        data: [ticketItem],
        page: 1,
        page_size: 5,
        total: 1,
      })

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'ALTER')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('工单')).toBeInTheDocument()
      })
      // Status label rendered (mocked to return raw status)
      expect(screen.getByText('APPROVED')).toBeInTheDocument()
    })

    it('renders audit log results with action label', async () => {
      mockSearchAuditLogs.mockResolvedValue({
        data: [auditLogItem],
        page: 1,
        page_size: 5,
        total: 1,
      })

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'products')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('审计日志')).toBeInTheDocument()
      })
      expect(screen.getByText('SELECT')).toBeInTheDocument()
    })

    it('renders mixed results from all 3 categories', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) =>
        kw
          ? Promise.resolve({
              data: [searchQueryItem],
              page: 1,
              page_size: 5,
              total: 1,
            })
          : Promise.resolve({ data: [], page: 1, page_size: 5, total: 0 })
      )
      mockListTickets.mockResolvedValue({
        data: [ticketItem],
        page: 1,
        page_size: 5,
        total: 1,
      })
      mockSearchAuditLogs.mockResolvedValue({
        data: [auditLogItem],
        page: 1,
        page_size: 5,
        total: 1,
      })

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'test')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('查询历史')).toBeInTheDocument()
        expect(screen.getByText('工单')).toBeInTheDocument()
        expect(screen.getByText('审计日志')).toBeInTheDocument()
      })
    })
  })

  // --- Empty state (no results) ---

  describe('empty state', () => {
    it('shows "未找到结果" when keyword matches nothing', async () => {
      // All APIs return empty results (already default mock)
      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'nonexistent')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('未找到结果')).toBeInTheDocument()
      })
    })
  })

  // --- Loading state ---

  describe('loading state', () => {
    it('shows loading indicator while search is in progress', async () => {
      // Make APIs hang (never resolve)
      mockSearchQueryHistory.mockReturnValue(new Promise(() => {}))
      mockListTickets.mockReturnValue(new Promise(() => {}))
      mockSearchAuditLogs.mockReturnValue(new Promise(() => {}))

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'test')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('搜索中...')).toBeInTheDocument()
      })
    })
  })

  // --- API failure degradation ---

  describe('API failure handling (Promise.allSettled)', () => {
    it('shows partial error when 1 API fails', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) =>
        kw ? Promise.reject(new Error('Query API down')) : Promise.resolve({ data: [], page: 1, page_size: 5, total: 0 })
      )
      mockListTickets.mockResolvedValue({
        data: [ticketItem],
        page: 1,
        page_size: 5,
        total: 1,
      })
      mockSearchAuditLogs.mockResolvedValue({
        data: [auditLogItem],
        page: 1,
        page_size: 5,
        total: 1,
      })

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'test')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        // Partial error message shown (amber warning)
        expect(screen.getByText(/查询历史: Query API down/)).toBeInTheDocument()
      })

      // Successful results still render
      expect(screen.getByText('工单')).toBeInTheDocument()
      expect(screen.getByText('审计日志')).toBeInTheDocument()
    })

    it('shows "搜索失败" when all 3 APIs fail', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) =>
        kw ? Promise.reject(new Error('fail')) : Promise.resolve({ data: [], page: 1, page_size: 5, total: 0 })
      )
      mockListTickets.mockRejectedValue(new Error('fail'))
      mockSearchAuditLogs.mockRejectedValue(new Error('fail'))

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'test')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('搜索失败，请稍后重试')).toBeInTheDocument()
      })
    })
  })

  // --- Empty input: recent query history ---

  describe('empty input / recent queries', () => {
    it('shows recent query history from initial fetch', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) => {
        if (kw === '') {
          return Promise.resolve({
            data: [recentQueryItem],
            page: 1,
            page_size: 5,
            total: 1,
          })
        }
        return Promise.resolve({ data: [], page: 1, page_size: 5, total: 0 })
      })

      renderWithOpenTransition()

      await waitFor(() => {
        expect(screen.getByText('最近查询')).toBeInTheDocument()
      })
    })

    it('does not show "最近查询" group when no recent queries', async () => {
      // Default mock returns empty data for '' keyword
      renderWithOpenTransition()

      await waitFor(() => {
        expect(mockSearchQueryHistory).toHaveBeenCalledWith('', 1, 5)
      })

      expect(screen.queryByText('最近查询')).not.toBeInTheDocument()
    })
  })

  // --- Navigation / jump logic ---

  describe('navigation after selecting results', () => {
    it('navigates to page when clicking page shortcut', async () => {
      const { onOpenChange } = renderWithOpenTransition()

      await waitFor(() => {
        expect(screen.getByText('查询')).toBeInTheDocument()
      })

      // Click "查询" page link
      await userEvent.click(screen.getByText('查询'))

      expect(mockNavigate).toHaveBeenCalledWith('/query')
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })

    it('navigates to settings page', async () => {
      const { onOpenChange } = renderWithOpenTransition()

      await waitFor(() => {
        expect(screen.getByText('数据源管理')).toBeInTheDocument()
      })

      await userEvent.click(screen.getByText('数据源管理'))

      expect(mockNavigate).toHaveBeenCalledWith('/settings/datasource')
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })

    it('navigates to ticket detail when clicking ticket result', async () => {
      mockListTickets.mockResolvedValue({
        data: [ticketItem],
        page: 1,
        page_size: 5,
        total: 1,
      })

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'ALTER')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('工单')).toBeInTheDocument()
      })

      // Find the command-item containing the ticket text and click it
      const ticketItemEl = screen
        .getAllByRole('option')
        .find((el) => el.textContent?.includes('ALTER TABLE'))
      expect(ticketItemEl).toBeDefined()
      await userEvent.click(ticketItemEl!)

      expect(mockNavigate).toHaveBeenCalledWith('/tickets?id=100')
    })

    it('navigates to audit page with highlight when clicking audit result', async () => {
      mockSearchAuditLogs.mockResolvedValue({
        data: [auditLogItem],
        page: 1,
        page_size: 5,
        total: 1,
      })

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'products')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('审计日志')).toBeInTheDocument()
      })

      const auditItemEl = screen
        .getAllByRole('option')
        .find((el) => el.textContent?.includes('SELECT * FROM products'))
      expect(auditItemEl).toBeDefined()
      await userEvent.click(auditItemEl!)

      expect(mockNavigate).toHaveBeenCalledWith(
        '/audit?highlight=' + encodeURIComponent('products')
      )
    })

    it('calls restoreHistoryAsTab when clicking query result', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) =>
        kw
          ? Promise.resolve({
              data: [searchQueryItem],
              page: 1,
              page_size: 5,
              total: 1,
            })
          : Promise.resolve({ data: [], page: 1, page_size: 5, total: 0 })
      )

      renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'orders')
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      await waitFor(() => {
        expect(screen.getByText('查询历史')).toBeInTheDocument()
      })

      // Find the query result command-item by looking for its data-value attribute
      const queryItemEl = screen
        .getAllByRole('option')
        .find((el) => el.getAttribute('data-value')?.includes('query-2'))
      expect(queryItemEl).toBeDefined()
      await userEvent.click(queryItemEl!)

      expect(mockNavigate).toHaveBeenCalledWith('/query')

      // restoreHistoryAsTab is called inside a setTimeout(50ms)
      await act(async () => {
        vi.advanceTimersByTime(100)
      })

      expect(mockRestoreHistoryAsTab).toHaveBeenCalledWith(
        'SELECT * FROM orders WHERE id = 1',
        11,
        'production'
      )
    })

    it('calls restoreHistoryAsTab when clicking recent query', async () => {
      mockSearchQueryHistory.mockImplementation((kw: string) => {
        if (kw === '') {
          return Promise.resolve({
            data: [recentQueryItem],
            page: 1,
            page_size: 5,
            total: 1,
          })
        }
        return Promise.resolve({ data: [], page: 1, page_size: 5, total: 0 })
      })

      renderWithOpenTransition()

      await waitFor(() => {
        expect(screen.getByText('最近查询')).toBeInTheDocument()
      })

      // Find the recent query item by its data-value
      const recentItemEl = screen
        .getAllByRole('option')
        .find((el) => el.getAttribute('data-value')?.includes('recent-1'))
      expect(recentItemEl).toBeDefined()
      await userEvent.click(recentItemEl!)

      expect(mockNavigate).toHaveBeenCalledWith('/query')

      await act(async () => {
        vi.advanceTimersByTime(100)
      })

      expect(mockRestoreHistoryAsTab).toHaveBeenCalledWith(
        'SELECT * FROM users',
        10,
        'testdb'
      )
    })
  })

  // --- Keyboard shortcut ---

  describe('keyboard shortcut', () => {
    it('registers Ctrl+K listener and toggles dialog open', () => {
      const onOpenChange = vi.fn()
      render(<CommandPalette open={false} onOpenChange={onOpenChange} />)

      act(() => {
        document.dispatchEvent(
          new KeyboardEvent('keydown', {
            key: 'k',
            ctrlKey: true,
            bubbles: true,
          })
        )
      })

      // When open=false, calls onOpenChange(!false) = onOpenChange(true)
      expect(onOpenChange).toHaveBeenCalledWith(true)
    })

    it('calls onOpenChange(false) when Ctrl+K pressed and dialog is open', () => {
      const onOpenChange = vi.fn()
      render(<CommandPalette open={true} onOpenChange={onOpenChange} />)

      act(() => {
        document.dispatchEvent(
          new KeyboardEvent('keydown', {
            key: 'k',
            ctrlKey: true,
            bubbles: true,
          })
        )
      })

      // When open=true, calls onOpenChange(!true) = onOpenChange(false)
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })

  // --- Cleanup ---

  describe('cleanup', () => {
    it('clears debounce timer on unmount', async () => {
      const { unmount } = renderWithOpenTransition()
      const input = screen.getByPlaceholderText('搜索页面、查询历史、工单或审计日志...')

      await userEvent.type(input, 'test')

      // Unmount before debounce fires
      unmount()

      // Advance timers - should not throw
      await act(async () => {
        vi.advanceTimersByTime(350)
      })

      // Search should not have been called
      expect(mockSearchQueryHistory).not.toHaveBeenCalledWith('test', 1, 5)
    })
  })
})
