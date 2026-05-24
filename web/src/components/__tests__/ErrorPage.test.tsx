import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

// --- Mocks ---

const mockNavigate = vi.fn()
vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => mockNavigate }
})

import ErrorPage from '@/components/ErrorPage'

describe('ErrorPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('403 error', () => {
    it('renders 403 error page', () => {
      render(<ErrorPage code={403} />)
      expect(screen.getByText('403')).toBeInTheDocument()
    })

    it('shows permission denied message', () => {
      render(<ErrorPage code={403} />)
      expect(screen.getByText('您没有访问此页面的权限')).toBeInTheDocument()
    })

    it('renders back button', () => {
      render(<ErrorPage code={403} />)
      expect(screen.getByText('返回上一页')).toBeInTheDocument()
    })

    it('navigates back when clicking return button', async () => {
      render(
        <MemoryRouter>
          <ErrorPage code={403} />
        </MemoryRouter>,
      )

      await userEvent.click(screen.getByText('返回上一页'))

      expect(mockNavigate).toHaveBeenCalledWith(-1)
    })
  })

  describe('404 error', () => {
    it('renders 404 error page', () => {
      render(<ErrorPage code={404} />)
      expect(screen.getByText('404')).toBeInTheDocument()
    })

    it('shows page not found message', () => {
      render(<ErrorPage code={404} />)
      expect(screen.getByText('页面不存在或已被移除')).toBeInTheDocument()
    })

    it('renders home button', () => {
      render(<ErrorPage code={404} />)
      expect(screen.getByText('返回首页')).toBeInTheDocument()
    })

    it('navigates to /query when clicking home button', async () => {
      render(
        <MemoryRouter>
          <ErrorPage code={404} />
        </MemoryRouter>,
      )

      await userEvent.click(screen.getByText('返回首页'))

      expect(mockNavigate).toHaveBeenCalledWith('/query')
    })
  })
})
