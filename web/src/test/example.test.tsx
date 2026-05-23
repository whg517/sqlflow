import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'

describe('test setup', () => {
  it('vitest works', () => {
    expect(1 + 1).toBe(2)
  })

  it('react testing library works', () => {
    render(<div data-testid="hello">Hello</div>)
    expect(screen.getByTestId('hello')).toHaveTextContent('Hello')
  })
})
