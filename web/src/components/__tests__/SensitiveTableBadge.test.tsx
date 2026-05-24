import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'

// Badge is needed as SensitiveTableBadge imports it
vi.mock('@/components/ui/badge', () => ({
  Badge: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <span data-testid="badge" className={className}>{children}</span>
  ),
}))

import { SensitiveTableBadge, SensitiveTableName } from '@/components/SensitiveTableBadge'

describe('SensitiveTableBadge', () => {
  describe('SensitiveTableBadge', () => {
    it('renders with high sensitivity', () => {
      render(<SensitiveTableBadge sensitivityLevel="high" />)
      const badge = screen.getByTestId('badge')
      expect(badge.className).toContain('bg-red-500/20')
      expect(badge.className).toContain('text-red-400')
    })

    it('renders with medium sensitivity', () => {
      render(<SensitiveTableBadge sensitivityLevel="medium" />)
      const badge = screen.getByTestId('badge')
      expect(badge.className).toContain('bg-yellow-500/20')
      expect(badge.className).toContain('text-yellow-400')
    })

    it('renders with low sensitivity', () => {
      render(<SensitiveTableBadge sensitivityLevel="low" />)
      const badge = screen.getByTestId('badge')
      expect(badge.className).toContain('bg-emerald-500/20')
      expect(badge.className).toContain('text-emerald-400')
    })

    it('hides label by default', () => {
      render(<SensitiveTableBadge sensitivityLevel="high" />)
      expect(screen.queryByText('高')).not.toBeInTheDocument()
    })

    it('shows label when showLabel is true', () => {
      render(<SensitiveTableBadge sensitivityLevel="high" showLabel />)
      expect(screen.getByText('高')).toBeInTheDocument()
    })

    it('uses sm size by default', () => {
      render(<SensitiveTableBadge sensitivityLevel="high" />)
      const badge = screen.getByTestId('badge')
      expect(badge.className).toContain('text-[10px]')
    })

    it('uses md size when specified', () => {
      render(<SensitiveTableBadge sensitivityLevel="high" size="md" />)
      const badge = screen.getByTestId('badge')
      expect(badge.className).toContain('text-xs')
    })

    it('falls back to medium badge for unknown level', () => {
      render(<SensitiveTableBadge sensitivityLevel={'unknown' as any} />)
      const badge = screen.getByTestId('badge')
      expect(badge.className).toContain('bg-yellow-500/20')
    })
  })

  describe('SensitiveTableName', () => {
    it('renders table name with shield icon', () => {
      render(<SensitiveTableName tableName="users" sensitivityLevel="high" />)
      expect(screen.getByText('users')).toBeInTheDocument()
    })

    it('applies high sensitivity background', () => {
      render(<SensitiveTableName tableName="users" sensitivityLevel="high" />)
      // The table name span should have a red background
      const nameEl = screen.getByText('users').closest('span')
      expect(nameEl?.className).toContain('bg-red-500/20')
    })

    it('applies medium sensitivity background', () => {
      render(<SensitiveTableName tableName="users" sensitivityLevel="medium" />)
      const nameEl = screen.getByText('users').closest('span')
      expect(nameEl?.className).toContain('bg-red-500/15')
    })

    it('applies low sensitivity background', () => {
      render(<SensitiveTableName tableName="users" sensitivityLevel="low" />)
      const nameEl = screen.getByText('users').closest('span')
      expect(nameEl?.className).toContain('bg-red-500/10')
    })
  })
})
