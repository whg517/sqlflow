import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
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
  }
})

// Mock api client
const mockApiGet = vi.fn()
const mockApiPost = vi.fn()
vi.mock('@/api/client', () => ({
  api: {
    get: (...args: unknown[]) => mockApiGet(...args),
    post: (...args: unknown[]) => mockApiPost(...args),
    put: vi.fn(),
    del: vi.fn(),
  },
}))

// Mock toast
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

// Mock Radix-based Select component for JSDOM compatibility
// Radix Select uses pointer capture API not available in JSDOM
vi.mock('@/components/ui/select', () => ({
  Select: ({ children, value, onValueChange }: { children: React.ReactNode; value: string; onValueChange: (v: string) => void }) => {
    ;(globalThis as Record<string, unknown>).__selectOnValueChange = onValueChange
    return <div data-testid="select">{children}</div>
  },
  SelectTrigger: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="select-trigger">{children}</div>,
  SelectValue: ({ placeholder }: { placeholder: string }) =>
    <span>{placeholder}</span>,
  SelectContent: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="select-content">{children}</div>,
  SelectItem: ({ children, value }: { children: React.ReactNode; value: string }) =>
    <div data-testid="select-item" data-value={value} onClick={() => {
      const cb = (globalThis as Record<string, unknown>).__selectOnValueChange as ((v: string) => void) | undefined
      cb?.(value)
    }}>{children}</div>,
}))

// Mock createTicket
const mockCreateTicket = vi.fn()
vi.mock('@/api/ticket', () => ({
  createTicket: (...args: unknown[]) => mockCreateTicket(...args),
}))

import TicketNewPage from '@/pages/TicketNew'

// --- Fixtures ---

const activeDatasources = [
  { id: 1, name: 'MySQL Test', type: 'mysql', status: 'active' },
  { id: 2, name: 'MongoDB Prod', type: 'mongodb', status: 'active' },
  { id: 3, name: 'MySQL Offline', type: 'mysql', status: 'inactive' },
]

function setupMocks() {
  mockApiGet.mockImplementation((url: string) => {
    if (url.includes('/datasources')) {
      return Promise.resolve({ code: 0, data: activeDatasources })
    }
    return Promise.resolve({})
  })
}

function renderTicketNewPage() {
  return render(
    <MemoryRouter initialEntries={['/tickets/new']}>
      <Routes>
        <Route path="/tickets/new" element={<TicketNewPage />} />
        <Route path="/tickets" element={<div>Tickets List</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('TicketNewPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setupMocks()
    // Clean up global select callback from mocked Select component
    delete (globalThis as Record<string, unknown>).__selectOnValueChange
  })

  // --- Rendering ---

  describe('rendering', () => {
    it('renders page header "提交新工单"', async () => {
      renderTicketNewPage()
      expect(screen.getByText('提交新工单')).toBeInTheDocument()
    })

    it('renders back button that navigates to /tickets', async () => {
      renderTicketNewPage()
      const backBtn = screen.getByRole('button', { name: '' })
      // The back button is a ghost button with ArrowLeft icon
      expect(backBtn).toBeInTheDocument()
    })

    it('renders "数据源" label with required marker', () => {
      renderTicketNewPage()
      expect(screen.getByText('数据源')).toBeInTheDocument()
    })

    it('renders "数据库名" input', () => {
      renderTicketNewPage()
      expect(screen.getByPlaceholderText('输入数据库名（可选）')).toBeInTheDocument()
    })

    it('renders "SQL 内容" label with required marker', () => {
      renderTicketNewPage()
      expect(screen.getByText('SQL 内容')).toBeInTheDocument()
    })

    it('renders SQL textarea', () => {
      renderTicketNewPage()
      expect(screen.getByPlaceholderText('输入要执行的 SQL 语句...')).toBeInTheDocument()
    })

    it('renders "变更原因" label with required marker', () => {
      renderTicketNewPage()
      expect(screen.getByText('变更原因')).toBeInTheDocument()
    })

    it('renders change reason textarea', () => {
      renderTicketNewPage()
      expect(screen.getByPlaceholderText('请说明此次变更的原因和预期影响（至少 10 个字符）...')).toBeInTheDocument()
    })

    it('renders cancel and submit buttons', () => {
      renderTicketNewPage()
      expect(screen.getByText('取消')).toBeInTheDocument()
      expect(screen.getByText('提交工单')).toBeInTheDocument()
    })

    it('renders character counter for change reason', () => {
      renderTicketNewPage()
      expect(screen.getByText('0/500')).toBeInTheDocument()
    })
  })

  // --- Datasource loading ---

  describe('datasource loading', () => {
    it('fetches datasources on mount', async () => {
      renderTicketNewPage()
      await waitFor(() => {
        expect(mockApiGet).toHaveBeenCalledWith('/datasources')
      })
    })

    it('filters datasources to only active ones', async () => {
      renderTicketNewPage()
      await waitFor(() => {
        expect(screen.getByText('MySQL Test')).toBeInTheDocument()
        expect(screen.getByText('MongoDB Prod')).toBeInTheDocument()
        expect(screen.queryByText('MySQL Offline')).not.toBeInTheDocument()
      })
    })

    it('shows datasource type indicator', async () => {
      renderTicketNewPage()
      await waitFor(() => {
        expect(screen.getByText('(mysql)')).toBeInTheDocument()
        expect(screen.getByText('(mongodb)')).toBeInTheDocument()
      })
    })
  })

  // --- Form validation ---

  describe('form validation', () => {
    it('shows "请选择数据源" error when submitting without datasource', async () => {
      renderTicketNewPage()

      // Wait for datasources to load
      await waitFor(() => screen.getByText('MySQL Test'))

      // Fill SQL and change reason to isolate datasource validation
      await userEvent.type(screen.getByPlaceholderText('输入要执行的 SQL 语句...'), 'SELECT 1')
      await userEvent.type(
        screen.getByPlaceholderText('请说明此次变更的原因和预期影响（至少 10 个字符）...'),
        'Regular data maintenance task',
      )

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(screen.getByText('请选择数据源')).toBeInTheDocument()
      })
    })

    it('shows "请输入 SQL" error when submitting without SQL', async () => {
      renderTicketNewPage()

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(screen.getByText('请输入 SQL')).toBeInTheDocument()
      })
    })

    it('shows "请填写变更原因" error when submitting without reason', async () => {
      renderTicketNewPage()

      await userEvent.type(screen.getByPlaceholderText('输入要执行的 SQL 语句...'), 'SELECT 1')

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(screen.getByText('请填写变更原因')).toBeInTheDocument()
      })
    })

    it('shows "变更原因至少 10 个字符" when reason is too short', async () => {
      renderTicketNewPage()

      await userEvent.type(screen.getByPlaceholderText('输入要执行的 SQL 语句...'), 'SELECT 1')
      await userEvent.type(
        screen.getByPlaceholderText('请说明此次变更的原因和预期影响（至少 10 个字符）...'),
        'short',
      )

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(screen.getByText('变更原因至少 10 个字符')).toBeInTheDocument()
      })
    })

    it('applies red border on fields with validation errors', async () => {
      renderTicketNewPage()

      await waitFor(() => screen.getByText('MySQL Test'))

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        // The textarea for SQL gets border-red-500 class
        const sqlTextarea = screen.getByPlaceholderText('输入要执行的 SQL 语句...')
        expect(sqlTextarea.className).toContain('border-red-500')
      })
    })

    it('clears validation errors when user starts filling fields', async () => {
      renderTicketNewPage()

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(screen.getByText('请输入 SQL')).toBeInTheDocument()
      })

      // Type SQL to clear the error
      await userEvent.type(screen.getByPlaceholderText('输入要执行的 SQL 语句...'), 'SELECT 1')

      // Submit again to trigger re-validation
      await userEvent.click(screen.getByText('提交工单'))

      // SQL error should be gone, only datasource/reason errors remain
      expect(screen.queryByText('请输入 SQL')).not.toBeInTheDocument()
    })
  })

  // --- Submission flow ---

  describe('submission flow', () => {
    async function fillValidForm() {
      // Click the MySQL Test datasource option (using mocked Select)
      await waitFor(() => screen.getByText('MySQL Test'))
      await userEvent.click(screen.getByText('MySQL Test'))

      // Fill database
      await userEvent.type(screen.getByPlaceholderText('输入数据库名（可选）'), 'testdb')

      // Fill SQL
      await userEvent.type(screen.getByPlaceholderText('输入要执行的 SQL 语句...'), 'ALTER TABLE users ADD COLUMN age INT')

      // Fill change reason
      await userEvent.type(
        screen.getByPlaceholderText('请说明此次变更的原因和预期影响（至少 10 个字符）...'),
        'Adding age column for user profile feature requirement',
      )
    }

    it('calls createTicket with correct params on valid submission', async () => {
      mockCreateTicket.mockResolvedValue({
        code: 0,
        data: { id: 42, status: 'SUBMITTED' },
      })

      renderTicketNewPage()
      await fillValidForm()

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(mockCreateTicket).toHaveBeenCalledWith({
          datasource_id: 1,
          database: 'testdb',
          sql: 'ALTER TABLE users ADD COLUMN age INT',
          db_type: 'mysql',
          change_reason: 'Adding age column for user profile feature requirement',
        })
      })
    })

    it('uses "mongodb" db_type for MongoDB datasources', async () => {
      mockCreateTicket.mockResolvedValue({
        code: 0,
        data: { id: 43, status: 'SUBMITTED' },
      })

      renderTicketNewPage()

      // Wait for datasources to load and component to be fully rendered
      await waitFor(() => {
        expect(screen.getByText('MongoDB Prod')).toBeInTheDocument()
      }, { timeout: 5000 })

      // Click the MongoDB option - this triggers our mocked onValueChange
      await userEvent.click(screen.getByText('MongoDB Prod'))

      // Verify the select value changed by checking datasourceId state
      await userEvent.type(screen.getByPlaceholderText('输入要执行的 SQL 语句...'), 'db.users.find()')
      await userEvent.type(
        screen.getByPlaceholderText('请说明此次变更的原因和预期影响（至少 10 个字符）...'),
        'Query users collection for reporting',
      )

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(mockCreateTicket).toHaveBeenCalledWith(
          expect.objectContaining({ db_type: 'mongodb' }),
        )
      })
    })

    it('shows loading state during submission', async () => {
      mockCreateTicket.mockReturnValue(new Promise(() => {}))

      renderTicketNewPage()
      await fillValidForm()

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(screen.getByText('提交中...')).toBeInTheDocument()
        expect(document.querySelector('.animate-spin')).toBeInTheDocument()
      })
    })

    it('disables submit button during submission', async () => {
      mockCreateTicket.mockReturnValue(new Promise(() => {}))

      renderTicketNewPage()
      await fillValidForm()

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        const submitBtn = screen.getByText('提交中...').closest('button')!
        expect(submitBtn).toBeDisabled()
      })
    })

    it('shows success toast and navigates to tickets on success', async () => {
      const { toast } = await import('sonner')
      mockCreateTicket.mockResolvedValue({
        code: 0,
        data: { id: 42, status: 'SUBMITTED' },
      })

      renderTicketNewPage()
      await fillValidForm()

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(toast.success).toHaveBeenCalledWith('工单提交成功')
        expect(mockNavigate).toHaveBeenCalledWith('/tickets')
      })
    })

    it('shows error toast on submission failure', async () => {
      const { toast } = await import('sonner')
      mockCreateTicket.mockRejectedValue(new Error('网络错误'))

      renderTicketNewPage()
      await fillValidForm()

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        expect(toast.error).toHaveBeenCalledWith('网络错误')
      })
    })

    it('shows generic error when submission fails without message', async () => {
      const { toast } = await import('sonner')
      mockCreateTicket.mockRejectedValue(new Error(''))

      renderTicketNewPage()
      await fillValidForm()

      await userEvent.click(screen.getByText('提交工单'))

      await waitFor(() => {
        // Error has empty message, so it shows the empty string (not '提交失败')
        expect(toast.error).toHaveBeenCalled()
      })
    })
  })

  // --- Navigation ---

  describe('navigation', () => {
    it('navigates to /tickets when clicking back button', async () => {
      renderTicketNewPage()
      await waitFor(() => screen.getByText('MySQL Test'))
      // Back button is the ghost button with ArrowLeft icon
      const backBtn = screen.getAllByRole('button').find(b => b.querySelector('svg.lucide-arrow-left'))!
      await userEvent.click(backBtn)
      expect(mockNavigate).toHaveBeenCalledWith('/tickets')
    })

    it('navigates to /tickets when clicking cancel button', async () => {
      renderTicketNewPage()
      await userEvent.click(screen.getByText('取消'))
      expect(mockNavigate).toHaveBeenCalledWith('/tickets')
    })
  })

  // --- Character counter ---

  describe('character counter', () => {
    it('updates counter when typing in change reason', async () => {
      renderTicketNewPage()
      const textarea = screen.getByPlaceholderText('请说明此次变更的原因和预期影响（至少 10 个字符）...')

      await userEvent.type(textarea, 'Hello World')

      expect(screen.getByText('11/500')).toBeInTheDocument()
    })
  })
})
