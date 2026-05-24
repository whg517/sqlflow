import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

// --- Mocks ---

const mockNavigate = vi.fn()
vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useSearchParams: () => [new URLSearchParams(), vi.fn()],
  }
})

// Mock api client
const mockApiGet = vi.fn()
vi.mock('@/api/client', () => ({
  api: {
    get: (...args: unknown[]) => mockApiGet(...args),
    post: vi.fn(),
    put: vi.fn(),
    del: vi.fn(),
  },
}))

// Mock toast
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

// Mock listTickets
const mockListTickets = vi.fn()
vi.mock('@/api/ticket', () => ({
  listTickets: (...args: unknown[]) => mockListTickets(...args),
  getStatusLabel: (status: string) => {
    const map: Record<string, string> = {
      PENDING_APPROVAL: '待审批', APPROVED: '已通过', REJECTED: '已拒绝',
      CANCELLED: '已取消', DONE: '已完成', SUBMITTED: '已提交',
    }
    return map[status] ?? status
  },
  getStatusColor: () => 'bg-blue-500/20 text-blue-400',
  getRiskLabel: (level: string) => {
    const map: Record<string, string> = { low: '低风险', medium: '中风险', high: '高风险' }
    return map[level] ?? level
  },
  getRiskColor: () => 'bg-emerald-500/20 text-emerald-400',
  getRiskDot: () => 'bg-emerald-400',
  formatTime: () => '05-23 18:00',
}))

// Mock TicketDetailDrawer
vi.mock('@/pages/Ticket/components/TicketDetailDrawer', () => ({
  default: ({
    open,
    ticketId,
  }: {
    open: boolean
    ticketId: number | null
    onOpenChange: (v: boolean) => void
    userRole: string
    userId: number
    onActionComplete: () => void
  }) =>
    open ? <div data-testid="ticket-detail-drawer">Ticket #{ticketId}</div> : null,
}))

import { TooltipProvider } from '@/components/ui/tooltip'

import TicketPage from '@/pages/Ticket'

// --- Fixtures ---

const mockTickets = [
  {
    id: 1,
    submitter_id: 1,
    submitter_name: 'Alice',
    datasource_id: 10,
    database: 'testdb',
    sql_content: 'SELECT * FROM users WHERE id = 1',
    sql_summary: 'SELECT * FROM users...',
    db_type: 'mysql',
    change_reason: 'Query users',
    status: 'PENDING_APPROVAL',
    risk_level: 'low',
    ai_review_result: '',
    reviewer_id: 0,
    reviewer_name: '',
    review_comment: '',
    executed_at: null,
    created_at: '2026-05-23T10:00:00Z',
    updated_at: '2026-05-23T10:00:00Z',
  },
  {
    id: 2,
    submitter_id: 2,
    submitter_name: 'Bob',
    datasource_id: 11,
    database: 'production',
    sql_content: 'ALTER TABLE orders ADD COLUMN status VARCHAR(50)',
    sql_summary: 'ALTER TABLE orders...',
    db_type: 'mysql',
    change_reason: 'Add status column',
    status: 'APPROVED',
    risk_level: 'medium',
    ai_review_result: '',
    reviewer_id: 1,
    reviewer_name: 'Alice',
    review_comment: 'OK',
    executed_at: null,
    created_at: '2026-05-23T11:00:00Z',
    updated_at: '2026-05-23T12:00:00Z',
  },
  {
    id: 3,
    submitter_id: 1,
    submitter_name: 'Alice',
    datasource_id: 10,
    database: 'testdb',
    sql_content: 'DELETE FROM temp_data WHERE created_at < NOW()',
    sql_summary: 'DELETE FROM temp_data...',
    db_type: 'mysql',
    change_reason: 'Cleanup',
    status: 'REJECTED',
    risk_level: 'high',
    ai_review_result: '',
    reviewer_id: 2,
    reviewer_name: 'Bob',
    review_comment: 'Too risky',
    executed_at: null,
    created_at: '2026-05-23T09:00:00Z',
    updated_at: '2026-05-23T09:30:00Z',
  },
]

function setupDefaultMocks(tickets = mockTickets, total = tickets.length) {
  // /auth/me
  mockApiGet.mockImplementation((url: string) => {
    if (url.includes('/auth/me')) {
      return Promise.resolve({ code: 0, data: { id: 1, username: 'admin', role: 'admin' } })
    }
    if (url.includes('/datasources')) {
      return Promise.resolve({
        code: 0,
        data: [
          { id: 10, name: 'MySQL Test', type: 'mysql' },
          { id: 11, name: 'MySQL Prod', type: 'mysql' },
        ],
      })
    }
    return Promise.resolve({})
  })

  // listTickets
  mockListTickets.mockResolvedValue({
    data: tickets,
    page: 1,
    page_size: 50,
    total,
  })
}

function renderTicketPage() {
  return render(
    <MemoryRouter initialEntries={['/tickets']}>
      <TooltipProvider>
        <Routes>
          <Route path="/tickets" element={<TicketPage />} />
        </Routes>
      </TooltipProvider>
    </MemoryRouter>,
  )
}

describe('TicketPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setupDefaultMocks()
  })

  // --- Rendering ---

  describe('rendering', () => {
    it('renders page header "变更工单"', () => {
      renderTicketPage()
      expect(screen.getByText('变更工单')).toBeInTheDocument()
    })

    it('renders "提交新工单" button', () => {
      renderTicketPage()
      expect(screen.getByText('提交新工单')).toBeInTheDocument()
    })

    it('renders status tabs', () => {
      renderTicketPage()
      expect(screen.getByText('全部')).toBeInTheDocument()
      expect(screen.getByText('待审批')).toBeInTheDocument()
      expect(screen.getByText('已通过')).toBeInTheDocument()
      expect(screen.getByText('已拒绝')).toBeInTheDocument()
      expect(screen.getByText('已取消')).toBeInTheDocument()
      expect(screen.getByText('已执行')).toBeInTheDocument()
    })

    it('renders "我提交的" filter button', () => {
      renderTicketPage()
      expect(screen.getByText('我提交的')).toBeInTheDocument()
    })

    it('renders "待我审批" filter button for admin user', async () => {
      renderTicketPage()
      await waitFor(() => {
        expect(screen.getByText('待我审批')).toBeInTheDocument()
      })
    })

    it('renders search input', () => {
      renderTicketPage()
      expect(screen.getByPlaceholderText('搜索 SQL 内容...')).toBeInTheDocument()
    })
  })

  // --- Ticket list ---

  describe('ticket list', () => {
    it('fetches and displays tickets', async () => {
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getByText('#1')).toBeInTheDocument()
        expect(screen.getByText('#2')).toBeInTheDocument()
        expect(screen.getByText('#3')).toBeInTheDocument()
      })
    })

    it('displays ticket SQL summary', async () => {
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getByText('SELECT * FROM users...')).toBeInTheDocument()
        expect(screen.getByText('ALTER TABLE orders...')).toBeInTheDocument()
      })
    })

    it('displays ticket database names', async () => {
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getAllByText('testdb').length).toBeGreaterThan(0)
        expect(screen.getAllByText('production').length).toBeGreaterThan(0)
      })
    })

    it('displays risk level labels', async () => {
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getByText('低风险')).toBeInTheDocument()
        expect(screen.getByText('中风险')).toBeInTheDocument()
        expect(screen.getByText('高风险')).toBeInTheDocument()
      })
    })

    it('displays status labels', async () => {
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getByText('待审批')).toBeInTheDocument()
        expect(screen.getByText('已通过')).toBeInTheDocument()
        expect(screen.getByText('已拒绝')).toBeInTheDocument()
      })
    })
  })

  // --- Empty state ---

  describe('empty state', () => {
    it('shows empty state when no tickets', async () => {
      setupDefaultMocks([], 0)
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getByText('暂无变更工单')).toBeInTheDocument()
      })
    })
  })

  // --- Loading state ---

  describe('loading state', () => {
    it('shows loading skeleton while fetching', async () => {
      mockListTickets.mockReturnValue(new Promise(() => {}))
      renderTicketPage()

      await waitFor(() => {
        expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
      })
    })
  })

  // --- Search functionality ---

  describe('keyword search', () => {
    it('renders search input and allows typing', async () => {
      renderTicketPage()
      const input = screen.getByPlaceholderText('搜索 SQL 内容...')
      await userEvent.type(input, 'SELECT')
      expect(input).toHaveValue('SELECT')
    })

    it('triggers search on Enter key', async () => {
      renderTicketPage()
      const input = screen.getByPlaceholderText('搜索 SQL 内容...')

      await userEvent.type(input, 'ALTER{Enter}')

      await waitFor(() => {
        expect(mockListTickets).toHaveBeenCalledWith(
          expect.objectContaining({ keyword: 'ALTER', page: 1 }),
        )
      })
    })
  })

  // --- Tab switching ---

  describe('tab switching', () => {
    it('calls API with status filter when clicking a tab', async () => {
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getByText('待审批')).toBeInTheDocument()
      })

      await userEvent.click(screen.getByText('待审批'))

      await waitFor(() => {
        expect(mockListTickets).toHaveBeenCalledWith(
          expect.objectContaining({ status: 'PENDING_APPROVAL', page: 1 }),
        )
      })
    })
  })

  // --- Row click ---

  describe('row interaction', () => {
    it('opens detail drawer when clicking a ticket row', async () => {
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getByText('#1')).toBeInTheDocument()
      })

      const row = screen.getByText('#1').closest('tr')!
      await userEvent.click(row)

      await waitFor(() => {
        expect(screen.getByTestId('ticket-detail-drawer')).toBeInTheDocument()
        expect(screen.getByText('Ticket #1')).toBeInTheDocument()
      })
    })
  })

  // --- Navigate to new ticket ---

  describe('create ticket', () => {
    it('navigates to /tickets/new when clicking create button', async () => {
      renderTicketPage()
      await userEvent.click(screen.getByText('提交新工单'))
      expect(mockNavigate).toHaveBeenCalledWith('/tickets/new')
    })
  })

  // --- Scope filters ---

  describe('scope filters', () => {
    it('toggles "我提交的" filter on click', async () => {
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getByText('#1')).toBeInTheDocument()
      })

      await userEvent.click(screen.getByText('我提交的'))

      await waitFor(() => {
        expect(mockListTickets).toHaveBeenCalledWith(
          expect.objectContaining({ scope: 'mine', page: 1 }),
        )
      })
    })
  })

  // --- Pagination ---

  describe('pagination', () => {
    it('renders pagination when there are multiple pages', async () => {
      const manyTickets = Array.from({ length: 55 }, (_, i) => ({
        ...mockTickets[0],
        id: i + 1,
      }))
      setupDefaultMocks(manyTickets, 55)
      renderTicketPage()

      await waitFor(() => {
        expect(screen.getByText(/共 55 条/)).toBeInTheDocument()
        expect(screen.getByText(/第 1\/2 页/)).toBeInTheDocument()
      })
    })
  })
})
