import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import React from 'react'

import NetworkBanner from '@/components/NetworkBanner'

describe('NetworkBanner', () => {
  let originalOnline: boolean

  beforeEach(() => {
    originalOnline = navigator.onLine
  })

  afterEach(() => {
    // Restore onLine state
    Object.defineProperty(navigator, 'onLine', {
      writable: true,
      value: originalOnline,
    })
  })

  describe('when online', () => {
    it('renders nothing when online', () => {
      Object.defineProperty(navigator, 'onLine', { writable: true, value: true })
      const { container } = render(<NetworkBanner />)
      expect(container.innerHTML).toBe('')
    })
  })

  describe('when offline', () => {
    it('renders offline banner when offline', () => {
      Object.defineProperty(navigator, 'onLine', { writable: true, value: false })
      render(<NetworkBanner />)
      expect(screen.getByText('网络连接已断开，部分功能不可用')).toBeInTheDocument()
    })

    it('shows WifiOff icon', () => {
      Object.defineProperty(navigator, 'onLine', { writable: true, value: false })
      render(<NetworkBanner />)
      // The WifiOff icon is an SVG element
      const svg = document.querySelector('svg.lucide-wifi-off') || document.querySelector('svg')
      expect(svg).toBeInTheDocument()
    })

    it('disappears when coming back online', async () => {
      Object.defineProperty(navigator, 'onLine', { writable: true, value: false })
      render(<NetworkBanner />)

      expect(screen.getByText('网络连接已断开，部分功能不可用')).toBeInTheDocument()

      // Simulate coming back online
      Object.defineProperty(navigator, 'onLine', { writable: true, value: true })
      act(() => {
        window.dispatchEvent(new Event('online'))
      })

      expect(screen.queryByText('网络连接已断开，部分功能不可用')).not.toBeInTheDocument()
    })

    it('appears when going offline', async () => {
      Object.defineProperty(navigator, 'onLine', { writable: true, value: true })
      render(<NetworkBanner />)

      expect(screen.queryByText('网络连接已断开，部分功能不可用')).not.toBeInTheDocument()

      // Simulate going offline
      Object.defineProperty(navigator, 'onLine', { writable: true, value: false })
      act(() => {
        window.dispatchEvent(new Event('offline'))
      })

      expect(screen.getByText('网络连接已断开，部分功能不可用')).toBeInTheDocument()
    })
  })
})
