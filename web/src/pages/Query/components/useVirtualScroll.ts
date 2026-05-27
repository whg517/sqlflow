import { useState, useCallback, useRef, useEffect, useMemo } from "react";

const ROW_HEIGHT = 33; // approximate row height in px (py-1.5 + content)
const OVERSCAN = 5; // extra rows rendered above/below viewport

interface VirtualScrollState {
  startIndex: number;
  endIndex: number;
  totalHeight: number;
  offsetY: number;
  visibleRows: number;
}

/**
 * Hook to implement virtual scrolling for large datasets.
 * Returns the indices of rows to render and positioning info.
 */
export function useVirtualScroll(
  totalRows: number,
  containerRef: React.RefObject<HTMLDivElement | null>,
  rowHeight: number = ROW_HEIGHT,
  overscan: number = OVERSCAN,
) {
  const [scrollOffset, setScrollOffset] = useState(0);
  const [containerHeight, setContainerHeight] = useState(0);

  const handleScroll = useCallback(() => {
    if (containerRef.current) {
      setScrollOffset(containerRef.current.scrollTop);
    }
  }, [containerRef]);

  // Observe container height changes
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerHeight(entry.contentRect.height);
      }
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [containerRef]);

  // Attach scroll listener
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    el.addEventListener("scroll", handleScroll, { passive: true });
    return () => el.removeEventListener("scroll", handleScroll);
  }, [containerRef, handleScroll]);

  const state = useMemo<VirtualScrollState>(() => {
    const totalHeight = totalRows * rowHeight;
    const visibleRows = containerHeight > 0 ? Math.ceil(containerHeight / rowHeight) : 0;

    let startIndex = Math.floor(scrollOffset / rowHeight) - overscan;
    startIndex = Math.max(0, startIndex);

    let endIndex = startIndex + visibleRows + 2 * overscan;
    endIndex = Math.min(totalRows, endIndex);

    const offsetY = startIndex * rowHeight;

    return { startIndex, endIndex, totalHeight, offsetY, visibleRows };
  }, [totalRows, rowHeight, overscan, scrollOffset, containerHeight]);

  // Scroll to top when data changes significantly
  const scrollToTop = useCallback(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = 0;
      setScrollOffset(0);
    }
  }, [containerRef]);

  return {
    ...state,
    scrollRef: containerRef,
    scrollToTop,
    // Whether virtual scrolling should be active
    isEnabled: totalRows >= 1000,
  };
}
