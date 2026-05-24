import { useRef, useCallback, useEffect, useState } from 'react'

interface ResizableSplitProps {
  top: React.ReactNode
  bottom: React.ReactNode
  ratio: number
  onRatioChange: (ratio: number) => void
  minTop?: number
  minBottom?: number
}

export default function ResizableSplit({
  top,
  bottom,
  ratio,
  onRatioChange,
  minTop = 120,
  minBottom = 120,
}: ResizableSplitProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [dragging, setDragging] = useState(false)

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    setDragging(true)
  }, [])

  useEffect(() => {
    if (!dragging) return

    function handleMouseMove(e: MouseEvent) {
      const container = containerRef.current
      if (!container) return
      const rect = container.getBoundingClientRect()
      const totalHeight = rect.height
      const offsetY = e.clientY - rect.top
      const newRatio = Math.max(
        minTop / totalHeight,
        Math.min(1 - minBottom / totalHeight, offsetY / totalHeight),
      )
      onRatioChange(newRatio)
    }

    function handleMouseUp() {
      setDragging(false)
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)
    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [dragging, minTop, minBottom, onRatioChange])

  const topHeight = `${ratio * 100}%`
  const bottomHeight = `${(1 - ratio) * 100}%`

  return (
    <div ref={containerRef} className="flex h-full flex-col">
      <div style={{ height: topHeight, minHeight: minTop }} className="overflow-hidden">
        {top}
      </div>
      <div
        className={`relative z-10 h-[4px] shrink-0 cursor-row-resize border-y border-[var(--border-default)] bg-[var(--bg-elevated)] transition-colors hover:bg-[var(--accent-primary)]/30 ${
          dragging ? 'bg-[var(--accent-primary)]/30' : ''
        }`}
        onMouseDown={handleMouseDown}
      />
      <div style={{ height: bottomHeight, minHeight: minBottom }} className="overflow-hidden">
        {bottom}
      </div>
    </div>
  )
}
