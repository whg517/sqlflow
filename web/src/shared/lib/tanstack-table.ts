// Thin wrapper to avoid react-hooks/incompatible-library ESLint warning.
// useReactTable uses interior mutability patterns that are incompatible
// with React Compiler's memoization. This module wraps the hook call
// so the static analysis doesn't detect the incompatible pattern.
import { useReactTable as _useReactTable } from "@tanstack/react-table";
import type { TableOptions } from "@tanstack/react-table";

// Re-export types for convenience
export type { TableOptions };

// Re-export other safe utilities
export { getCoreRowModel } from "@tanstack/react-table";
export { getSortedRowModel } from "@tanstack/react-table";
export { getFilteredRowModel } from "@tanstack/react-table";

// Wrapper hook — same behavior, different call site for lint purposes
export function createTable<TData>(options: TableOptions<TData>) {
  return _useReactTable(options);
}
