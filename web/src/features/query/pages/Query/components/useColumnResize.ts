import { useState, useCallback, useRef, useEffect } from "react";

const STORAGE_KEY_PREFIX = "sql-platform:col-widths:";

interface ColumnResizeState {
  widths: Record<string, number>;
  isResizing: boolean;
  resizingColId: string | null;
}

/**
 * Hook to manage column resize with drag handles.
 * Persists widths to localStorage keyed by a table identifier.
 */
export function useColumnResize(
  tableKey: string,
  defaultWidths: Record<string, number>,
) {
  const storageKey = STORAGE_KEY_PREFIX + tableKey;

  const loadWidths = useCallback((): Record<string, number> => {
    try {
      const saved = localStorage.getItem(storageKey);
      if (saved) {
        return JSON.parse(saved);
      }
    } catch {
      // ignore parse errors
    }
    return {};
  }, [storageKey]);

  const saveWidths = useCallback(
    (widths: Record<string, number>) => {
      try {
        localStorage.setItem(storageKey, JSON.stringify(widths));
      } catch {
        // ignore quota errors
      }
    },
    [storageKey],
  );

  const [state, setState] = useState<ColumnResizeState>(() => ({
    widths: { ...defaultWidths, ...loadWidths() },
    isResizing: false,
    resizingColId: null,
  }));

  // Persist on width change (debounced)
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => {
    if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);
    saveTimeoutRef.current = setTimeout(() => {
      saveWidths(state.widths);
    }, 300);
    return () => {
      if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);
    };
  }, [state.widths, saveWidths]);

  const startXRef = useRef(0);
  const startWidthRef = useRef(0);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent, colId: string) => {
      e.preventDefault();
      e.stopPropagation();
      startXRef.current = e.clientX;
      startWidthRef.current = state.widths[colId] ?? 150;
      setState((prev) => ({ ...prev, isResizing: true, resizingColId: colId }));
    },
    [state.widths],
  );

  const handleMouseMove = useCallback(
    (e: MouseEvent) => {
      if (!state.isResizing || !state.resizingColId) return;
      const delta = e.clientX - startXRef.current;
      const newWidth = Math.max(60, startWidthRef.current + delta);
      setState((prev) => ({
        ...prev,
        widths: {
          ...prev.widths,
          [prev.resizingColId!]: newWidth,
        },
      }));
    },
    [state.isResizing, state.resizingColId],
  );

  const handleMouseUp = useCallback(() => {
    setState((prev) => ({ ...prev, isResizing: false, resizingColId: null }));
  }, []);

  // Global mouse listeners for drag
  useEffect(() => {
    if (state.isResizing) {
      document.addEventListener("mousemove", handleMouseMove);
      document.addEventListener("mouseup", handleMouseUp);
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
      return () => {
        document.removeEventListener("mousemove", handleMouseMove);
        document.removeEventListener("mouseup", handleMouseUp);
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      };
    }
  }, [state.isResizing, handleMouseMove, handleMouseUp]);

  // Reset widths to defaults
  const resetWidths = useCallback(() => {
    setState((prev) => ({ ...prev, widths: { ...defaultWidths } }));
    try {
      localStorage.removeItem(storageKey);
    } catch {
      // ignore
    }
  }, [defaultWidths, storageKey]);

  return {
    widths: state.widths,
    isResizing: state.isResizing,
    resizingColId: state.resizingColId,
    handleMouseDown,
    resetWidths,
  };
}
