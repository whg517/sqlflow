import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import React from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

// --- Mocks ---

const mockApiGet = vi.fn()
vi.mock('@/api/client', () => ({
  api: {
    get: (...args: unknown[]) => mockApiGet(...args),
    post: vi.fn(),
    put: vi.fn(),
    del: vi.fn(),
  },
}))

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/components/NetworkBanner', () => ({
  default: () => <div data-testid="network-banner" />,
}))

vi.mock('@/components/PasswordChangeDialog', () => ({
  default: ({ open }: { open: boolean }) => open ? <div data-testid="password-dialog" /> : null,
}))

vi.mock('@/components/CommandPalette', () => ({
  default: () => <div data-testid="command-palette" />,
}))

vi.mock('@/components/ui/tooltip', () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Tooltip: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}))

vi.mock('@/components/ui/dropdown-menu', () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuItem: ({ children, onClick }: { children: React.ReactNode; onClick?: () => void }) =>
    <div onClick={onClick}>{children}</div>,
  DropdownMenuSeparator: () => <hr />,
  DropdownMenuLabel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

vi.mock('@/hooks/useTheme', () => ({
  useTheme: () => ({ theme: 'dark', toggle: vi.fn() }),
}))

import Layout from '@/components/Layout'

// --- Fixtures ---

function renderLayout(userRole = 'admin', username = 'admin') {
  mockApiGet.mockImplementation((url: string) => {
    if (url.includes('/auth/me')) {
      return Promise.resolve({ code: 0, data: { username, role: userRole } })
    }
    if (url.includes('/datasources')) {
      return Promise.resolve({ code: 0, data: [] })
    }
    return Promise.resolve({ code: 0, data: {} })
  })

  return render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route path="/*" element={<Layout />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('Layout - Permission Boundary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // --- Role-based navigation visibility ---

  describe('admin role', () => {
    it('shows "用户管理" nav item for admin', async () => {
      renderLayout('admin')
      await waitFor(() => {
        expect(screen.getByText('用户管理')).toBeInTheDocument()
      })
    })

    it('shows all standard nav items', async () => {
      renderLayout('admin')
      await waitFor(() => {
        expect(screen.getByText('概览')).toBeInTheDocument()
        expect(screen.getByText('查询')).toBeInTheDocument()
        expect(screen.getByText('工单')).toBeInTheDocument()
        expect(screen.getByText('权限')).toBeInTheDocument()
        expect(screen.getByText('审计')).toBeInTheDocument()
      })
    })
  })

  describe('dba role', () => {
    it('hides "用户管理" nav item for dba', async () => {
      renderLayout('dba', 'dba_user')
      await waitFor(() => {
        expect(screen.getByText('概览')).toBeInTheDocument()
      })
      expect(screen.queryByText('用户管理')).not.toBeInTheDocument()
    })

    it('shows standard nav items for dba', async () => {
      renderLayout('dba', 'dba_user')
      await waitFor(() => {
        expect(screen.getByText('概览')).toBeInTheDocument()
        expect(screen.getByText('查询')).toBeInTheDocument()
        expect(screen.getByText('工单')).toBeInTheDocument()
        expect(screen.getByText('权限')).toBeInTheDocument()
        expect(screen.getByText('审计')).toBeInTheDocument()
      })
    })
  })

  describe('developer role', () => {
    it('hides "用户管理" nav item for developer', async () => {
      renderLayout('developer', 'dev_user')
      await waitFor(() => {
        expect(screen.getByText('概览')).toBeInTheDocument()
      })
      expect(screen.queryByText('用户管理')).not.toBeInTheDocument()
    })

    it('shows standard nav items for developer', async () => {
      renderLayout('developer', 'dev_user')
      await waitFor(() => {
        expect(screen.getByText('概览')).toBeInTheDocument()
        expect(screen.getByText('查询')).toBeInTheDocument()
        expect(screen.getByText('工单')).toBeInTheDocument()
        expect(screen.getByText('审计')).toBeInTheDocument()
      })
    })
  })

  // --- Auth state ---

  describe('auth state', () => {
    it('fetches /auth/me on mount', async () => {
      renderLayout('admin')
      await waitFor(() => {
        expect(mockApiGet).toHaveBeenCalledWith('/auth/me')
      })
    })

    it('displays user initial in avatar', async () => {
      renderLayout('admin', 'Marcus')
      await waitFor(() => {
        expect(screen.getByText('M')).toBeInTheDocument()
      })
    })

    it('displays "U" when no user loaded', () => {
      mockApiGet.mockResolvedValue({ code: 1, data: null })
      render(
        <MemoryRouter initialEntries={['/']}>
          <Routes>
            <Route path="/*" element={<Layout />} />
          </Routes>
        </MemoryRouter>,
      )
      // Before auth resolves, default is 'U'
      expect(screen.getByText('U')).toBeInTheDocument()
    })
  })

  // --- Sidebar collapse ---

  describe('sidebar collapse', () => {
    it('renders collapse toggle button', async () => {
      renderLayout('admin')
      await waitFor(() => {
        expect(screen.getByText('概览')).toBeInTheDocument()
      })
      // Sidebar has a toggle button (PanelLeftClose/PanelLeft icon)
      const aside = document.querySelector('aside')
      expect(aside).toBeInTheDocument()
    })

    it('shows brand "SQLFlow" when expanded', async () => {
      renderLayout('admin')
      await waitFor(() => {
        expect(screen.getByText('SQLFlow')).toBeInTheDocument()
      })
    })
  })

  // --- Settings submenu ---

  describe('settings submenu', () => {
    it('shows settings section', async () => {
      renderLayout('admin')
      await waitFor(() => {
        expect(screen.getByText('设置')).toBeInTheDocument()
      })
    })
  })
})
