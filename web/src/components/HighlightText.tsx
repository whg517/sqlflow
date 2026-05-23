import { useMemo } from 'react'

interface HighlightTextProps {
  text: string
  keyword: string
  className?: string
  maxLen?: number
}

/**
 * Highlights occurrences of `keyword` in `text` using <mark> tags.
 * Truncates text to `maxLen` characters if provided.
 */
export default function HighlightText({ text, keyword, className, maxLen = 120 }: HighlightTextProps) {
  const parts = useMemo(() => {
    if (!keyword.trim()) {
      const display = text.length > maxLen ? text.slice(0, maxLen) + '…' : text
      return [{ text: display, highlight: false }]
    }

    const escaped = keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const regex = new RegExp(`(${escaped})`, 'gi')
    const display = text.length > maxLen ? text.slice(0, maxLen) + '…' : text
    const segments = display.split(regex)

    return segments
      .filter((s) => s.length > 0)
      .map((s) => ({
        text: s,
        highlight: regex.test(s),
      }))
  }, [text, keyword, maxLen])

  return (
    <span className={className}>
      {parts.map((part, i) =>
        part.highlight ? (
          <mark key={i} className="rounded-sm bg-yellow-500/30 text-inherit">
            {part.text}
          </mark>
        ) : (
          <span key={i}>{part.text}</span>
        ),
      )}
    </span>
  )
}
