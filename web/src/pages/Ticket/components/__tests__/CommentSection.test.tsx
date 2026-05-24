import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import React from 'react'

// --- Mocks ---

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const mockListComments = vi.fn()
const mockCreateComment = vi.fn()
const mockDeleteComment = vi.fn()
vi.mock('@/api/comment', () => ({
  listComments: (...args: unknown[]) => mockListComments(...args),
  createComment: (...args: unknown[]) => mockCreateComment(...args),
  deleteComment: (...args: unknown[]) => mockDeleteComment(...args),
  formatCommentTime: () => '05-24 10:00',
}))

vi.mock('@/components/ui/separator', () => ({
  Separator: () => <hr />,
}))

vi.mock('@/components/ui/avatar', () => ({
  Avatar: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  AvatarFallback: ({ children }: { children: React.ReactNode }) => <div data-testid="avatar-fallback">{children}</div>,
}))

import CommentSection from '@/pages/Ticket/components/CommentSection'

// --- Fixtures ---

const mockComments = [
  { id: 1, order_id: 1, user_id: 10, username: 'admin', content: 'Looks good to me', parent_id: 0, created_at: '2026-05-24T10:00:00Z' },
  { id: 2, order_id: 1, user_id: 20, username: 'developer', content: 'Please add index', parent_id: 0, created_at: '2026-05-24T11:00:00Z' },
  { id: 3, order_id: 1, user_id: 10, username: 'admin', content: 'Good point', parent_id: 2, created_at: '2026-05-24T11:30:00Z' },
]

function renderCommentSection(props: Record<string, unknown> = {}) {
  const defaults = {
    orderId: 1,
    currentUserId: 10,
    currentUserRole: 'admin',
  }
  return render(<CommentSection {...defaults} {...(props as Record<string, unknown>)} />)
}

describe('CommentSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockListComments.mockResolvedValue({ data: mockComments })
  })

  // --- Rendering ---

  describe('rendering', () => {
    it('renders comment header', () => {
      renderCommentSection()
      expect(screen.getByText(/评论 \(/)).toBeInTheDocument()
    })

    it('renders comment input area', () => {
      renderCommentSection()
      expect(screen.getByPlaceholderText(/输入评论/)).toBeInTheDocument()
    })

    // CommentSection only fetches on orderId change
    // These tests verify the component can render comment data
    it('renders comments when data is available', async () => {
      mockListComments.mockResolvedValue({ data: mockComments })
      const { rerender } = renderCommentSection({ orderId: 0 })
      
      // Trigger fetch by changing orderId
      rerender(<CommentSection orderId={1} currentUserId={10} currentUserRole="admin" />)
      
      await waitFor(() => {
        expect(screen.getByText('Looks good to me')).toBeInTheDocument()
      })
    })

    it('shows usernames in comments', async () => {
      mockListComments.mockResolvedValue({ data: mockComments })
      const { rerender } = renderCommentSection({ orderId: 0 })
      rerender(<CommentSection orderId={1} currentUserId={10} currentUserRole="admin" />)
      
      await waitFor(() => {
        expect(screen.getByText('developer')).toBeInTheDocument()
      })
    })
  })

  // --- Empty state ---

  describe('empty state', () => {
    it('shows "暂无评论" when no comments', async () => {
      mockListComments.mockResolvedValue({ data: [] })
      renderCommentSection({ orderId: 99 })
      // Since orderId change triggers fetch via rAF, we need initial fetch
      // The component only fetches on orderId change, not initial mount
      // So we check the empty state text
      await waitFor(() => {
        expect(screen.getByText('暂无评论，参与讨论吧')).toBeInTheDocument()
      })
    })
  })

  // --- Submit comment ---

  describe('submit comment', () => {
    it('enables submit button when content is typed', async () => {
      mockListComments.mockResolvedValue({ data: [] })
      const { rerender } = renderCommentSection({ orderId: 0 })
      rerender(<CommentSection orderId={1} currentUserId={10} currentUserRole="admin" />)
      await waitFor(() => expect(mockListComments).toHaveBeenCalledWith(1))

      const textarea = screen.getByPlaceholderText(/输入评论/)
      await userEvent.type(textarea, 'Test comment')

      // Send button should now be enabled
      const buttons = screen.getAllByRole('button')
      const sendBtn = buttons.find(b => b.querySelector('svg.lucide-send') || b.textContent === '')
      expect(sendBtn).toBeTruthy()
      expect(sendBtn?.disabled).toBe(false)
    })

    it('disables send button when content is empty', () => {
      renderCommentSection()
      const sendBtn = document.querySelector('button[class*="accent-primary"]') as HTMLElement
      expect(sendBtn).toBeDisabled()
    })
  })

  // --- Delete comment ---

  describe('delete comment', () => {
    it('shows delete button for own comments', async () => {
      mockListComments.mockResolvedValue({ data: [mockComments[0]] }) // Only admin's comment
      const { rerender } = renderCommentSection({ orderId: 0, currentUserId: 10, currentUserRole: 'developer' })
      rerender(<CommentSection orderId={1} currentUserId={10} currentUserRole="developer" />)

      await waitFor(() => screen.getByText('Looks good to me'))
      const deleteButtons = screen.getAllByText('删除')
      expect(deleteButtons.length).toBeGreaterThanOrEqual(1)
    })

    it('shows delete button for admin on all comments', async () => {
      mockListComments.mockResolvedValue({ data: mockComments })
      const { rerender } = renderCommentSection({ orderId: 0, currentUserId: 99, currentUserRole: 'admin' })
      rerender(<CommentSection orderId={1} currentUserId={99} currentUserRole="admin" />)

      await waitFor(() => screen.getByText('Looks good to me'))
      const deleteButtons = screen.getAllByText('删除')
      expect(deleteButtons.length).toBe(3)
    })

    it('calls deleteComment on click', async () => {
      mockDeleteComment.mockResolvedValue({ code: 0 })
      mockListComments.mockResolvedValue({ data: [mockComments[0]] })
      const { toast } = await import('sonner')

      const { rerender } = renderCommentSection({ orderId: 0 })
      rerender(<CommentSection orderId={1} currentUserId={10} currentUserRole="admin" />)

      await waitFor(() => screen.getByText('Looks good to me'))

      const deleteButtons = screen.getAllByText('删除')
      await userEvent.click(deleteButtons[0])

      await waitFor(() => {
        expect(mockDeleteComment).toHaveBeenCalled()
        expect(toast.success).toHaveBeenCalledWith('评论已删除')
      })
    })
  })
})
