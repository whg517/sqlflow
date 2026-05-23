import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { Mock } from 'vitest'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type MockMediaQueryList = {
  matches: boolean
  media: string
  onchange: ((e: MediaQueryListEvent) => void) | null
  addListener: Mock
  removeListener: Mock
  addEventListener: Mock
  removeEventListener: Mock
  dispatchEvent: Mock
}

let mediaChangeCallbacks: Map<string, (e: MediaQueryListEvent) => void>

function mockMatchMedia(prefersLight: boolean) {
  mediaChangeCallbacks = new Map()

  window.matchMedia = vi.fn().mockImplementation((query: string): MockMediaQueryList => ({
    matches: query.includes('light') ? prefersLight : false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn((_event: string, listener: () => void) => {
      if (query.includes('light')) {
        mediaChangeCallbacks.set(query, listener as (e: MediaQueryListEvent) => void)
      }
    }),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
}

function fireMediaChange(matches: boolean) {
  const cb = mediaChangeCallbacks.get('(prefers-color-scheme: light)')
  if (cb) {
    act(() => {
      cb({
        matches,
        media: '(prefers-color-scheme: light)',
      } as MediaQueryListEvent)
    })
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useTheme', () => {
  let originalMatchMedia: typeof window.matchMedia

  beforeEach(() => {
    originalMatchMedia = window.matchMedia
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
  })

  afterEach(() => {
    window.matchMedia = originalMatchMedia
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
  })

  // Each test uses describe.sequential or runs independently.
  // Since useTheme has module-level singleton state, we isolate tests
  // by working with the singleton's behavior rather than trying to reset it.
  // The first test will set the initial state; subsequent tests build on it.

  // ---------------------------------------------------------------
  // 1. Default value
  // ---------------------------------------------------------------
  it('returns a theme value (dark or light)', async () => {
    mockMatchMedia(false)
    const { useTheme } = await import('../useTheme')
    const { result } = renderHook(() => useTheme())
    expect(['dark', 'light']).toContain(result.current.theme)
  })

  it('defaults to system preference when no localStorage value exists', async () => {
    // Reset module state by invalidating cache
    vi.resetModules()
    mockMatchMedia(true)
    const { useTheme } = await import('../useTheme')
    const { result } = renderHook(() => useTheme())
    // After resetModules, currentTheme is null, so it reads system pref
    expect(result.current.theme).toBe('light')
  })

  // ---------------------------------------------------------------
  // 2. localStorage persistence
  // ---------------------------------------------------------------
  it('persists theme to localStorage after setTheme', async () => {
    vi.resetModules()
    mockMatchMedia(false)
    const { useTheme } = await import('../useTheme')
    const { result } = renderHook(() => useTheme())

    act(() => {
      result.current.setTheme('light')
    })

    expect(result.current.theme).toBe('light')
    expect(localStorage.getItem('theme')).toBe('light')
  })

  it('reads persisted theme from localStorage on fresh load', async () => {
    vi.resetModules()
    localStorage.setItem('theme', 'light')
    mockMatchMedia(false) // system prefers dark, but localStorage has 'light'
    const { useTheme } = await import('../useTheme')
    const { result } = renderHook(() => useTheme())
    // Should read from localStorage
    expect(result.current.theme).toBe('light')
  })

  // ---------------------------------------------------------------
  // 3. toggle()
  // ---------------------------------------------------------------
  it('toggle() flips between dark and light', async () => {
    vi.resetModules()
    mockMatchMedia(false)
    const { useTheme } = await import('../useTheme')
    const { result } = renderHook(() => useTheme())

    const initial = result.current.theme

    act(() => { result.current.toggle() })
    expect(result.current.theme).toBe(initial === 'dark' ? 'light' : 'dark')

    act(() => { result.current.toggle() })
    expect(result.current.theme).toBe(initial)
  })

  // ---------------------------------------------------------------
  // 4. setTheme()
  // ---------------------------------------------------------------
  it('setTheme() directly sets the theme', async () => {
    vi.resetModules()
    mockMatchMedia(false)
    const { useTheme } = await import('../useTheme')
    const { result } = renderHook(() => useTheme())

    act(() => { result.current.setTheme('light') })
    expect(result.current.theme).toBe('light')

    act(() => { result.current.setTheme('dark') })
    expect(result.current.theme).toBe('dark')
  })

  // ---------------------------------------------------------------
  // 5. data-theme attribute
  // ---------------------------------------------------------------
  it('sets data-theme attribute on document element', async () => {
    vi.resetModules()
    mockMatchMedia(false)
    const { useTheme } = await import('../useTheme')
    const { result } = renderHook(() => useTheme())

    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')

    act(() => { result.current.toggle() })
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })

  // ---------------------------------------------------------------
  // 6. prefers-color-scheme system preference
  // ---------------------------------------------------------------
  it('follows system preference change when no user choice stored', async () => {
    vi.resetModules()
    mockMatchMedia(false)
    const { useTheme } = await import('../useTheme')
    const { result } = renderHook(() => useTheme())
    expect(result.current.theme).toBe('dark')

    // Simulate system preference change to light
    fireMediaChange(true)
    expect(result.current.theme).toBe('light')
  })

  it('ignores system preference change when user has explicit choice', async () => {
    vi.resetModules()
    localStorage.setItem('theme', 'dark')
    mockMatchMedia(false)
    const { useTheme } = await import('../useTheme')
    const { result } = renderHook(() => useTheme())
    expect(result.current.theme).toBe('dark')

    // Simulate system preference change to light
    fireMediaChange(true)

    // User has explicit preference → should stay dark
    expect(result.current.theme).toBe('dark')
  })

  it('syncs theme across multiple hook instances', async () => {
    vi.resetModules()
    mockMatchMedia(false)
    const { useTheme } = await import('../useTheme')
    const { result: r1 } = renderHook(() => useTheme())
    const { result: r2 } = renderHook(() => useTheme())

    expect(r1.current.theme).toBe(r2.current.theme)

    act(() => { r1.current.toggle() })

    expect(r1.current.theme).toBe(r2.current.theme)
  })
})
