import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

// --- Mocks ---

const mockToggle = vi.fn()
vi.mock('@/hooks/useTheme', () => ({
  useTheme: () => ({ theme: 'dark', toggle: mockToggle }),
}))

const mockApiGet = vi.fn()
vi.mock('@/api/client', () => ({
  api: {
    get: (...args: unknown[]) => mockApiGet(...args),
    post: vi.fn(),
    put: vi.fn(),
    del: vi.fn(),
  },
}))

vi.mock('@/components/ChangePasswordDialog', () => ({
  default: ({ open }: { open: boolean }) =>
    open ? <div data-testid="change-password-dialog">ChangePasswordDialog</div> : null,
}))

vi.mock('@/components/CommandPalette', () => ({
  default: ({ open }: { open: boolean }) =>
    open ? <div data-testid="command-palette">CommandPalette</div> : null,
}))

vi.mock('@/components/NetworkBanner', () => ({
  default: () => <div data-testid="network-banner" />,
}))

import { TooltipProvider } from '@/components/ui/tooltip'

import Layout from '@/components/Layout'

function renderLayout(initialPath = '/') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <TooltipProvider>
        <Routes>
          <Route element={<Layout />}>
            <Route index element={<div data-testid="outlet-page">Home</div>} />
            <Route path="query" element={<div data-testid="outlet-page">Query</div>} />
            <Route path="tickets" element={<div data-testid="outlet-page">Tickets</div>} />
          </Route>
        </Routes>
      </TooltipProvider>
    </MemoryRouter>,
  )
}

const defaultUser = { username: 'admin', role: 'admin' }

describe('Layout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    localStorage.setItem('token', 'test-token')
    mockApiGet.mockResolvedValue({ code: 0, data: defaultUser })
  })

  afterEach(() => localStorage.clear())

  // --- Sidebar navigation ---

  describe('sidebar navigation', () => {
    it('renders the SQLFlow brand logo and text', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('SQLFlow')).toBeInTheDocument()
      })
    })

    it('renders main navigation menu items', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('概览')).toBeInTheDocument()
        expect(screen.getByText('查询')).toBeInTheDocument()
        expect(screen.getByText('工单')).toBeInTheDocument()
        expect(screen.getByText('权限')).toBeInTheDocument()
        expect(screen.getByText('审计')).toBeInTheDocument()
      })
    })

    it('renders user management nav item for admin user', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('用户管理')).toBeInTheDocument()
      })
    })

    it('does not render user management nav item for non-admin user', async () => {
      mockApiGet.mockResolvedValue({ code: 0, data: { username: 'devuser', role: 'developer' } })
      renderLayout()
      await waitFor(() => {
        expect(screen.queryByText('用户管理')).not.toBeInTheDocument()
      })
    })

    it('renders settings nav item with sub-menu', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('设置')).toBeInTheDocument()
      })
    })
  })

  // --- Top header ---

  describe('top header', () => {
    it('renders the search button with placeholder text', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('搜索...')).toBeInTheDocument()
      })
    })

    it('renders user avatar with initial letter from username', async () => {
      renderLayout()
      await waitFor(() => {
        // The initial 'A' from 'admin' is rendered in the AvatarFallback
        expect(screen.getByText('A')).toBeInTheDocument()
      })
    })
  })

  // --- Sidebar collapse ---

  describe('sidebar collapse', () => {
    it('renders collapse toggle button', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('SQLFlow')).toBeInTheDocument()
      })
      // The sidebar should have a collapse toggle button
      const aside = document.querySelector('aside')!
      const buttons = aside.querySelectorAll('button')
      expect(buttons.length).toBeGreaterThan(0)
    })

    it('toggles collapsed state on button click', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('SQLFlow')).toBeInTheDocument()
      })

      // Find the collapse button (last button in sidebar)
      const aside = document.querySelector('aside')!
      const buttons = aside.querySelectorAll('button')
      const collapseButton = buttons[buttons.length - 1] as HTMLElement

      await userEvent.click(collapseButton)

      // After collapsing, SQLFlow text should be hidden
      await waitFor(() => {
        expect(screen.queryByText('SQLFlow')).not.toBeInTheDocument()
      })
    })

    it('persists collapsed state to localStorage', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('SQLFlow')).toBeInTheDocument()
      })

      const aside = document.querySelector('aside')!
      const buttons = aside.querySelectorAll('button')
      const collapseButton = buttons[buttons.length - 1] as HTMLElement

      await userEvent.click(collapseButton)

      expect(localStorage.getItem('sidebar-collapsed')).toBe('true')
    })
  })

  // --- Settings sub-menu ---

  describe('settings sub-menu', () => {
    it('opens settings sub-menu when clicking settings button', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('设置')).toBeInTheDocument()
      })

      await userEvent.click(screen.getByText('设置'))

      await waitFor(() => {
        expect(screen.getByText('数据源管理')).toBeInTheDocument()
        expect(screen.getByText('脱敏规则')).toBeInTheDocument()
        expect(screen.getByText('AI 配置')).toBeInTheDocument()
      })
    })

    it('auto-opens settings when on a settings path', async () => {
      render(
        <MemoryRouter initialEntries={['/settings/datasource']}>
          <TooltipProvider>
            <Routes>
              <Route element={<Layout />}>
                <Route path="settings/*" element={<div data-testid="outlet">Settings</div>} />
              </Route>
            </Routes>
          </TooltipProvider>
        </MemoryRouter>,
      )

      await waitFor(() => {
        expect(screen.getByText('数据源管理')).toBeInTheDocument()
      })
    })
  })

  // --- User dropdown ---

  describe('user dropdown', () => {
    it('displays username in dropdown after API loads', async () => {
      mockApiGet.mockResolvedValue({ code: 0, data: { username: 'testuser', role: 'admin' } })
      renderLayout()
      await waitFor(() => {
        const trigger = document.querySelector('[data-slot="dropdown-menu-trigger"]') as HTMLElement
        expect(trigger).toBeInTheDocument()
      })

      // Open the dropdown
      const trigger = document.querySelector('[data-slot="dropdown-menu-trigger"]') as HTMLElement
      await userEvent.click(trigger)

      await waitFor(() => {
        expect(screen.getByText('testuser')).toBeInTheDocument()
      })
    })

    it('shows default dash when user data fails to load', async () => {
      mockApiGet.mockResolvedValue({ code: 1, data: null })
      renderLayout()
      await waitFor(() => {
        const trigger = document.querySelector('[data-slot="dropdown-menu-trigger"]') as HTMLElement
        expect(trigger).toBeInTheDocument()
      })

      // Open the dropdown
      const trigger = document.querySelector('[data-slot="dropdown-menu-trigger"]') as HTMLElement
      await userEvent.click(trigger)

      await waitFor(() => {
        expect(screen.getByText('—')).toBeInTheDocument()
      })
    })
  })

  // --- API call ---

  describe('data fetching', () => {
    it('fetches current user info on mount', async () => {
      renderLayout()
      await waitFor(() => {
        expect(mockApiGet).toHaveBeenCalledWith('/auth/me')
      })
    })

    it('handles API error gracefully', async () => {
      mockApiGet.mockRejectedValue(new Error('Network error'))
      renderLayout()
      await waitFor(() => {
        expect(mockApiGet).toHaveBeenCalledWith('/auth/me')
      })
    })
  })

  // --- Logout ---

  describe('logout', () => {
    it('renders avatar dropdown trigger in header', async () => {
      renderLayout()
      await waitFor(() => {
        const trigger = document.querySelector('[data-slot="dropdown-menu-trigger"]') as HTMLElement
        expect(trigger).toBeInTheDocument()
      })
    })
  })

  // --- Theme toggle ---

  describe('theme toggle', () => {
    it('renders theme toggle option when dropdown is opened', async () => {
      renderLayout()
      await waitFor(() => {
        const trigger = document.querySelector('[data-slot="dropdown-menu-trigger"]') as HTMLElement
        expect(trigger).toBeInTheDocument()
      })

      // Open the dropdown
      const trigger = document.querySelector('[data-slot="dropdown-menu-trigger"]') as HTMLElement
      await userEvent.click(trigger)

      await waitFor(() => {
        expect(screen.getByText('浅色模式')).toBeInTheDocument()
      })
    })
  })

  // --- Command palette trigger ---

  describe('command palette', () => {
    it('renders command palette trigger button in header', async () => {
      renderLayout()
      await waitFor(() => {
        expect(screen.getByText('搜索...')).toBeInTheDocument()
      })
    })
  })
})
