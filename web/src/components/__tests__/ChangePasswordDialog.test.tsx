import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import React from 'react'
import { MemoryRouter } from 'react-router-dom'

// --- Mocks ---

// Mock fetch globally
const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

// Mock toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

// Component under test
import ChangePasswordDialog from '@/components/ChangePasswordDialog'

// --- Helpers ---

function renderDialog(open = true) {
  const onOpenChange = vi.fn()
  const utils = render(
    <MemoryRouter>
      <ChangePasswordDialog open={open} onOpenChange={onOpenChange} />
    </MemoryRouter>,
  )
  return { onOpenChange, ...utils }
}

// --- Tests ---

describe('ChangePasswordDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetch.mockReset()
  })

  // --- Rendering ---

  describe('rendering', () => {
    it('renders nothing when open is false', () => {
      renderDialog(false)
      expect(screen.queryByText('修改密码')).not.toBeInTheDocument()
    })

    it('renders dialog title when open', () => {
      renderDialog(true)
      expect(screen.getByText('修改密码')).toBeInTheDocument()
    })

    it('renders description text', () => {
      renderDialog(true)
      expect(screen.getByText('请输入当前密码并设置新密码')).toBeInTheDocument()
    })

    it('renders three password input fields', () => {
      renderDialog(true)
      expect(screen.getByPlaceholderText('请输入当前密码')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('8-128 字符，需包含字母和数字')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('再次输入新密码')).toBeInTheDocument()
    })

    it('renders cancel and submit buttons', () => {
      renderDialog(true)
      expect(screen.getByText('取消')).toBeInTheDocument()
      expect(screen.getByText('保存修改')).toBeInTheDocument()
    })
  })

  // --- Validation ---

  describe('validation', () => {
    it('shows error for empty old password on submit', async () => {
      renderDialog(true)

      await userEvent.click(screen.getByText('保存修改'))

      expect(screen.getByText('请输入当前密码')).toBeInTheDocument()
    })

    it('shows error for empty new password on submit', async () => {
      renderDialog(true)
      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'oldpass1')

      await userEvent.click(screen.getByText('保存修改'))

      expect(screen.getByText('请输入新密码')).toBeInTheDocument()
    })

    it('shows error for empty confirm password on submit', async () => {
      renderDialog(true)
      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'oldpass1')
      await userEvent.type(screen.getByPlaceholderText('8-128 字符，需包含字母和数字'), 'newpass12')

      await userEvent.click(screen.getByText('保存修改'))

      expect(screen.getByText('请确认新密码')).toBeInTheDocument()
    })

    it('shows error for mismatched confirm password', async () => {
      renderDialog(true)
      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'oldpass1')
      await userEvent.type(screen.getByPlaceholderText('8-128 字符，需包含字母和数字'), 'newpass12')
      await userEvent.type(screen.getByPlaceholderText('再次输入新密码'), 'different1')

      await userEvent.click(screen.getByText('保存修改'))

      expect(screen.getByText('两次输入的密码不一致')).toBeInTheDocument()
    })

    it('shows error for new password too short', async () => {
      renderDialog(true)
      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'oldpass1')
      await userEvent.type(screen.getByPlaceholderText('8-128 字符，需包含字母和数字'), 'short')
      await userEvent.type(screen.getByPlaceholderText('再次输入新密码'), 'short')

      await userEvent.click(screen.getByText('保存修改'))

      expect(screen.getByText('密码长度至少 8 个字符')).toBeInTheDocument()
    })

    it('shows error for new password without letters', async () => {
      renderDialog(true)
      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'oldpass1')
      await userEvent.type(screen.getByPlaceholderText('8-128 字符，需包含字母和数字'), '12345678')
      await userEvent.type(screen.getByPlaceholderText('再次输入新密码'), '12345678')

      await userEvent.click(screen.getByText('保存修改'))

      expect(screen.getByText('密码必须包含至少一个字母')).toBeInTheDocument()
    })

    it('shows error for new password without numbers', async () => {
      renderDialog(true)
      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'oldpass1')
      await userEvent.type(screen.getByPlaceholderText('8-128 字符，需包含字母和数字'), 'longpassword')
      await userEvent.type(screen.getByPlaceholderText('再次输入新密码'), 'longpassword')

      await userEvent.click(screen.getByText('保存修改'))

      expect(screen.getByText('密码必须包含至少一个数字')).toBeInTheDocument()
    })
  })

  // --- Successful submit ---

  describe('successful submission', () => {
    it('calls API with correct body and closes dialog on success', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0 }),
      })

      const { onOpenChange } = renderDialog(true)

      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'oldpass1')
      await userEvent.type(screen.getByPlaceholderText('8-128 字符，需包含字母和数字'), 'newpass12')
      await userEvent.type(screen.getByPlaceholderText('再次输入新密码'), 'newpass12')

      await userEvent.click(screen.getByText('保存修改'))

      await act(async () => {
        await vi.waitFor(() => {
          expect(mockFetch).toHaveBeenCalledWith('/api/auth/password', {
            method: 'PUT',
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({ old_password: 'oldpass1', new_password: 'newpass12' }),
          })
        })
      })

      expect(onOpenChange).toHaveBeenCalledWith(false)
    })

    it('shows toast success on successful password change', async () => {
      const { toast } = await import('sonner')
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ code: 0 }),
      })

      renderDialog(true)

      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'oldpass1')
      await userEvent.type(screen.getByPlaceholderText('8-128 字符，需包含字母和数字'), 'newpass12')
      await userEvent.type(screen.getByPlaceholderText('再次输入新密码'), 'newpass12')

      await userEvent.click(screen.getByText('保存修改'))

      await act(async () => {
        await vi.waitFor(() => {
          expect(toast.success).toHaveBeenCalledWith('密码修改成功')
        })
      })
    })
  })

  // --- Failed submit ---

  describe('failed submission', () => {
    it('shows error message on API failure', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: async () => ({ message: '当前密码不正确' }),
      })

      renderDialog(true)

      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'wrongpass1')
      await userEvent.type(screen.getByPlaceholderText('8-128 字符，需包含字母和数字'), 'newpass12')
      await userEvent.type(screen.getByPlaceholderText('再次输入新密码'), 'newpass12')

      await userEvent.click(screen.getByText('保存修改'))

      await act(async () => {
        await vi.waitFor(() => {
          expect(screen.getByText('当前密码不正确')).toBeInTheDocument()
        })
      })
    })
  })

  // --- Cancel ---

  describe('cancel', () => {
    it('closes dialog when clicking cancel button', async () => {
      const { onOpenChange } = renderDialog(true)

      await userEvent.click(screen.getByText('取消'))

      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })

  // --- Loading state ---

  describe('loading state', () => {
    it('disables buttons during submission', async () => {
      mockFetch.mockReturnValue(new Promise(() => {}))

      renderDialog(true)

      await userEvent.type(screen.getByPlaceholderText('请输入当前密码'), 'oldpass1')
      await userEvent.type(screen.getByPlaceholderText('8-128 字符，需包含字母和数字'), 'newpass12')
      await userEvent.type(screen.getByPlaceholderText('再次输入新密码'), 'newpass12')

      await userEvent.click(screen.getByText('保存修改'))

      await act(async () => {
        await vi.waitFor(() => {
          expect(screen.getByText('保存中...')).toBeInTheDocument()
        })
      })
    })
  })
})
