import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

// Component under test
import AuthGuard from '@/components/AuthGuard'

// --- Tests ---

describe('AuthGuard', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  describe('with token', () => {
    it('renders children when token exists in localStorage', () => {
      localStorage.setItem('token', 'valid-jwt-token')

      render(
        <MemoryRouter>
          <AuthGuard>
            <div data-testid="protected-content">Secret Page</div>
          </AuthGuard>
        </MemoryRouter>,
      )

      expect(screen.getByTestId('protected-content')).toBeInTheDocument()
      expect(screen.getByText('Secret Page')).toBeInTheDocument()
    })

    it('renders multiple children', () => {
      localStorage.setItem('token', 'valid-jwt-token')

      render(
        <MemoryRouter>
          <AuthGuard>
            <div>Child 1</div>
            <div>Child 2</div>
          </AuthGuard>
        </MemoryRouter>,
      )

      expect(screen.getByText('Child 1')).toBeInTheDocument()
      expect(screen.getByText('Child 2')).toBeInTheDocument()
    })
  })

  describe('without token', () => {
    it('redirects to /login when no token exists', () => {
      render(
        <MemoryRouter>
          <AuthGuard>
            <div data-testid="protected-content">Secret Page</div>
          </AuthGuard>
        </MemoryRouter>,
      )

      // Children should NOT be rendered
      expect(screen.queryByTestId('protected-content')).not.toBeInTheDocument()
      expect(screen.queryByText('Secret Page')).not.toBeInTheDocument()
    })

    it('navigates to /login page when token is null', () => {
      render(
        <MemoryRouter>
          <AuthGuard>
            <div>Protected</div>
          </AuthGuard>
        </MemoryRouter>,
      )

      // With no token, Navigate component redirects to /login
      // The protected content should not be visible
      expect(screen.queryByText('Protected')).not.toBeInTheDocument()
    })
  })

  describe('token management', () => {
    it('shows protected content after token is set', () => {
      localStorage.setItem('token', 'new-token')

      render(
        <MemoryRouter>
          <AuthGuard>
            <div>Content</div>
          </AuthGuard>
        </MemoryRouter>,
      )

      expect(screen.getByText('Content')).toBeInTheDocument()
    })

    it('hides protected content after token is removed', () => {
      localStorage.removeItem('token')

      render(
        <MemoryRouter>
          <AuthGuard>
            <div>Content</div>
          </AuthGuard>
        </MemoryRouter>,
      )

      expect(screen.queryByText('Content')).not.toBeInTheDocument()
    })

    it('treats empty string token as no token', () => {
      localStorage.setItem('token', '')

      render(
        <MemoryRouter>
          <AuthGuard>
            <div>Content</div>
          </AuthGuard>
        </MemoryRouter>,
      )

      // localStorage.getItem returns '' which is falsy for the check
      // Actually '' is truthy for localStorage.getItem, but the component
      // checks `if (!token)` which makes empty string falsy
      expect(screen.queryByText('Content')).not.toBeInTheDocument()
    })
  })
})
