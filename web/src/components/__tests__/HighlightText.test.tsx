import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import HighlightText from '../HighlightText'

describe('HighlightText', () => {
  it('highlights matching keyword with <mark>', () => {
    render(<HighlightText text="SELECT * FROM users" keyword="SELECT" />)
    const mark = screen.getByText('SELECT')
    expect(mark.tagName).toBe('MARK')
  })

  it('highlights all occurrences of the keyword', () => {
    render(<HighlightText text="SELECT id, SELECT name FROM SELECT" keyword="SELECT" />)
    const marks = document.querySelectorAll('mark')
    expect(marks).toHaveLength(3)
    marks.forEach((m) => expect(m.textContent).toBe('SELECT'))
  })

  it('renders original text unchanged when no keyword matches', () => {
    render(<HighlightText text="hello world" keyword="xyz" />)
    // No <mark> elements should be rendered
    expect(document.querySelectorAll('mark')).toHaveLength(0)
    expect(screen.getByText('hello world')).toBeInTheDocument()
  })

  it('renders original text unchanged when keyword is empty', () => {
    render(<HighlightText text="hello world" keyword="" />)
    expect(document.querySelectorAll('mark')).toHaveLength(0)
    expect(screen.getByText('hello world')).toBeInTheDocument()
  })

  it('renders original text unchanged when keyword is whitespace-only', () => {
    render(<HighlightText text="hello world" keyword="   " />)
    expect(document.querySelectorAll('mark')).toHaveLength(0)
    expect(screen.getByText('hello world')).toBeInTheDocument()
  })

  it('safely handles HTML injection in keyword (no XSS)', () => {
    render(<HighlightText text="<script>alert('xss')</script>" keyword="<script>" />)
    // The highlighted text should be the literal string, not parsed as HTML
    const mark = document.querySelector('mark')
    expect(mark).toBeInTheDocument()
    expect(mark!.textContent).toBe('<script>')
    // There should be no actual script tags in the DOM
    expect(document.querySelectorAll('script')).toHaveLength(0)
  })

  it('safely handles HTML injection in text (no XSS)', () => {
    render(<HighlightText text="<img onerror=alert(1)>hello" keyword="hello" />)
    const mark = screen.getByText('hello')
    expect(mark.tagName).toBe('MARK')
    // The <img> part should be rendered as plain text, not an actual element
    expect(document.querySelectorAll('img')).toHaveLength(0)
  })

  it('truncates long text to maxLen and appends ellipsis', () => {
    const longText = 'a'.repeat(200)
    render(<HighlightText text={longText} keyword="" maxLen={50} />)
    const content = screen.getByText(/^a{50}…$/)
    expect(content).toBeInTheDocument()
  })

  it('applies case-insensitive matching', () => {
    render(<HighlightText text="select ID from users" keyword="SELECT" />)
    const mark = screen.getByText('select')
    expect(mark.tagName).toBe('MARK')
  })
})
