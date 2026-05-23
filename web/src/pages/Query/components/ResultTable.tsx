"use no memo";

import { useMemo, useEffect, useState, useCallback } from 'react'
import {
  getSortedRowModel,
  getFilteredRowModel,
  flexRender,
  type ColumnDef,
  type SortingState,
  type ColumnFiltersState,
} from '@tanstack/react-table'
import { createTable, getCoreRowModel } from '@/lib/tanstack-table'
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
} from '@/components/ui/table'
import { ChevronUp, ChevronDown, ChevronsUpDown, Lock, Filter, FilterX, Search } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Popover, PopoverTrigger, PopoverContent } from '@/components/ui/popover'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import type { QueryResult } from '@/api/query'

interface ResultTableProps {
  result: QueryResult | null
  pageSize?: number
}

const PAGE_SIZE_OPTIONS = [50, 100, 200] as const

type FilterOperator = 'contains' | 'notContains' | 'eq' | 'notEq'

const FILTER_OPERATORS: { value: FilterOperator; label: string }[] = [
  { value: 'contains', label: '包含' },
  { value: 'notContains', label: '不包含' },
  { value: 'eq', label: '等于' },
  { value: 'notEq', label: '不等于' },
]

function applyFilterOperator(rowValue: unknown, filterValue: string, operator: FilterOperator): boolean {
  const cellStr = rowValue === null || rowValue === undefined ? 'NULL' : String(rowValue)
  const filterStr = filterValue.trim()

  switch (operator) {
    case 'contains':
      return cellStr.toLowerCase().includes(filterStr.toLowerCase())
    case 'notContains':
      return !cellStr.toLowerCase().includes(filterStr.toLowerCase())
    case 'eq':
      return cellStr === filterStr
    case 'notEq':
      return cellStr !== filterStr
    default:
      return true
  }
}

// Column filter popover content
function ColumnFilterPopover({
  columnId,
  currentFilter,
  onApply,
  onReset,
}: {
  columnId: string
  currentFilter: string | undefined
  onApply: (value: string, operator: FilterOperator) => void
  onReset: () => void
}) {
  const [operator, setOperator] = useState<FilterOperator>('contains')
  const [inputValue, setInputValue] = useState(currentFilter ?? '')

  // Sync input when currentFilter changes externally (controlled prop sync)
  useEffect(() => {
    queueMicrotask(() => setInputValue(currentFilter ?? ''))
  }, [currentFilter])

  const handleApply = () => {
    if (inputValue.trim()) {
      onApply(inputValue, operator)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleApply()
    }
  }

  return (
    <PopoverContent
      className="w-64 p-3"
      side="bottom"
      align="start"
      onInteractOutside={(e) => {
        // Prevent closing on select dropdown interaction
        const target = e.target as HTMLElement
        if (target.closest('[data-radix-select-viewport]') || target.closest('[data-slot="select-trigger"]')) {
          e.preventDefault()
        }
      }}
    >
      <div className="flex flex-col gap-2.5">
        <div className="text-xs font-medium text-[var(--text-secondary)]">
          筛选: {columnId}
        </div>

        <Select value={operator} onValueChange={(v) => setOperator(v as FilterOperator)}>
          <SelectTrigger className="h-7 text-xs" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {FILTER_OPERATORS.map((op) => (
              <SelectItem key={op.value} value={op.value} className="text-xs">
                {op.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className="relative">
          <Search size={13} className="absolute left-2 top-1/2 -translate-y-1/2 text-[var(--text-muted)]" />
          <Input
            className="h-7 pl-7 text-xs"
            placeholder="输入筛选值..."
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={handleKeyDown}
            autoFocus
          />
        </div>

        <div className="flex items-center justify-end gap-1.5 pt-0.5">
          <Button
            variant="ghost"
            size="xs"
            onClick={onReset}
            className="text-[var(--text-muted)]"
          >
            重置
          </Button>
          <Button size="xs" onClick={handleApply}>
            确认
          </Button>
        </div>
      </div>
    </PopoverContent>
  )
}

function CellValue({ value }: { value: unknown }) {
  const str = value === null || value === undefined
    ? 'NULL'
    : String(value)

  if (str.length <= 80) {
    return <span className={value === null ? 'italic text-[var(--text-muted)]' : ''}>{str}</span>
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="cursor-default truncate block max-w-[300px]">{str}</span>
      </TooltipTrigger>
      <TooltipContent side="bottom" className="max-w-[400px] whitespace-pre-wrap break-all">
        {str}
      </TooltipContent>
    </Tooltip>
  )
}

// Inner component that owns useReactTable — isolates TanStack Table's
// incompatible-library API to this component boundary.
function ResultTableInner({
  result,
  sorting,
  onSortingChange,
  columnFilters,
  onColumnFiltersChange,
  page,
  onPageChange,
  pageSize,
  onPageSizeChange,
}: {
  result: QueryResult | null
  sorting: SortingState
  onSortingChange: React.Dispatch<React.SetStateAction<SortingState>>
  columnFilters: ColumnFiltersState
  onColumnFiltersChange: React.Dispatch<React.SetStateAction<ColumnFiltersState>>
  page: number
  onPageChange: (page: number) => void
  pageSize: number
  onPageSizeChange: (size: number) => void
}) {
  const columns = useMemo<ColumnDef<Record<string, unknown>>[]>(() => {
    if (!result?.columns) return []
    return result.columns.map((col) => ({
      accessorKey: col,
      filterFn: 'textFilter',
      header: () => {
        const isDesensitized = result.desensitized_fields?.includes(col)
        return (
          <div className="flex items-center gap-1">
            <span>{col}</span>
            {isDesensitized && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Lock size={12} className="text-[var(--warning)]" />
                </TooltipTrigger>
                <TooltipContent>该字段已脱敏</TooltipContent>
              </Tooltip>
            )}
          </div>
        )
      },
      cell: ({ getValue }: { getValue: () => unknown }) => <CellValue value={getValue()} />,
      size: 150,
    }))
  }, [result])

  const data = useMemo(() => result?.rows ?? [], [result])

  // Build a custom filterFns map that handles our operators
  const filterFns = useMemo(() => ({
    textFilter: (row: { getValue: (id: string) => unknown }, _columnId: string, filterValue: { value: string; operator: FilterOperator }) => {
      const rowValue = row.getValue(_columnId)
      return applyFilterOperator(rowValue, filterValue.value, filterValue.operator)
    },
  }), [])

  const table = createTable({
    data,
    columns,
    state: { sorting, columnFilters },
    onSortingChange,
    onColumnFiltersChange,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    filterFns,
  })

  const totalPages = Math.ceil(table.getFilteredRowModel().rows.length / pageSize)
  const paginatedRows = table.getFilteredRowModel().rows.slice(
    page * pageSize,
    (page + 1) * pageSize,
  )

  const totalFilteredCount = table.getFilteredRowModel().rows.length
  const totalOriginalCount = data.length
  const hasActiveFilters = columnFilters.length > 0

  // Filter handlers (must be called before any conditional returns per rules-of-hooks)
  const handleFilterApply = useCallback((columnId: string, value: string, operator: FilterOperator) => {
    onColumnFiltersChange((prev) => {
      const existing = prev.findIndex((f) => f.id === columnId)
      const newFilter = { id: columnId, value: { value, operator } }
      if (existing >= 0) {
        const next = [...prev]
        next[existing] = newFilter
        return next
      }
      return [...prev, newFilter]
    })
    onPageChange(0)
  }, [onColumnFiltersChange, onPageChange])

  const handleFilterReset = useCallback((columnId: string) => {
    onColumnFiltersChange((prev) => prev.filter((f) => f.id !== columnId))
    onPageChange(0)
  }, [onColumnFiltersChange, onPageChange])

  const handleClearAllFilters = useCallback(() => {
    onColumnFiltersChange([])
    onPageChange(0)
  }, [onColumnFiltersChange, onPageChange])

  if (!result) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <p className="text-[var(--text-muted)]">执行查询以查看结果</p>
        </div>
      </div>
    )
  }

  if (result.rows.length === 0) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <p className="text-[var(--text-muted)]">未查询到数据</p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      {/* Active filters bar */}
      {hasActiveFilters && (
        <div className="flex items-center gap-1.5 border-b border-[var(--border-default)] px-3 py-1.5 text-xs">
          <Filter size={12} className="text-[var(--color-primary)]" />
          <span className="text-[var(--text-secondary)]">已筛选:</span>
          {columnFilters.map((f) => (
            <span
              key={f.id}
              className="inline-flex items-center gap-1 rounded bg-[var(--bg-elevated)] px-1.5 py-0.5 text-[var(--text-secondary)]"
            >
              <span className="font-medium text-[var(--text-primary)]">{f.id}</span>
              <span>
                {FILTER_OPERATORS.find((op) => op.value === (f.value as { operator: FilterOperator }).operator)?.label}
              </span>
              <span className="max-w-[120px] truncate">&quot;{(f.value as { value: string }).value}&quot;</span>
              <button
                className="ml-0.5 text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                onClick={() => handleFilterReset(f.id)}
              >
                <FilterX size={11} />
              </button>
            </span>
          ))}
          <button
            className="ml-1 text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            onClick={handleClearAllFilters}
          >
            清除全部
          </button>
        </div>
      )}

      <div className="flex-1 overflow-auto">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((hg) => (
              <TableRow key={hg.id} className="border-[var(--border-default)] hover:bg-transparent">
                {hg.headers.map((header) => {
                  const columnId = header.column.id
                  const filterEntry = columnFilters.find((f) => f.id === columnId)
                  const isFilterActive = !!filterEntry

                  return (
                    <TableHead
                      key={header.id}
                      className="whitespace-nowrap text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                      style={{ width: header.getSize() }}
                    >
                      <div className="flex items-center gap-1">
                        {/* Column name + sort (clickable) */}
                        <div
                          className="flex items-center gap-1 cursor-pointer"
                          onClick={header.column.getToggleSortingHandler()}
                        >
                          {flexRender(header.column.columnDef.header, header.getContext())}
                          <span className="ml-1 inline-flex">
                            {{
                              asc: <ChevronUp size={12} />,
                              desc: <ChevronDown size={12} />,
                            }[header.column.getIsSorted() as string] ?? (
                              <ChevronsUpDown size={12} className="opacity-30" />
                            )}
                          </span>
                        </div>

                        {/* Filter icon + popover */}
                        <Popover>
                          <PopoverTrigger asChild>
                            <button
                              className="ml-0.5 rounded p-0.5 hover:bg-[var(--bg-elevated)]"
                              onClick={(e) => e.stopPropagation()}
                            >
                              <Filter
                                size={12}
                                className={isFilterActive
                                  ? 'text-[var(--color-primary)]'
                                  : 'opacity-30 hover:opacity-60'
                                }
                              />
                            </button>
                          </PopoverTrigger>
                          <ColumnFilterPopover
                            columnId={columnId}
                            currentFilter={isFilterActive ? (filterEntry!.value as { value: string }).value : undefined}
                            onApply={(value, operator) => handleFilterApply(columnId, value, operator)}
                            onReset={() => handleFilterReset(columnId)}
                          />
                        </Popover>
                      </div>
                    </TableHead>
                  )
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {paginatedRows.length > 0 ? (
              paginatedRows.map((row) => (
                <TableRow
                  key={row.id}
                  className="border-[var(--border-default)] text-xs hover:bg-[var(--bg-elevated)]/50"
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id} className="py-1.5 font-mono text-xs">
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  colSpan={table.getHeaderGroups()[0].headers.length}
                  className="h-32 text-center text-[var(--text-muted)]"
                >
                  <div className="flex flex-col items-center gap-1">
                    <Search size={24} className="opacity-30" />
                    <span>无匹配数据</span>
                    {hasActiveFilters && (
                      <button
                        className="text-xs text-[var(--color-primary)] hover:underline"
                        onClick={handleClearAllFilters}
                      >
                        清除筛选条件
                      </button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      {data.length > 0 && (
        <div className="flex items-center justify-between border-t border-[var(--border-default)] px-3 py-2 text-xs text-[var(--text-secondary)]">
          <div className="flex items-center gap-2">
            <span>
              共 {totalOriginalCount} 行
              {hasActiveFilters && (
                <span>（筛选后 {totalFilteredCount} 行）</span>
              )}
              {totalPages > 1 ? `，第 ${page + 1}/${totalPages} 页` : ''}
            </span>
            <select
              value={pageSize}
              onChange={(e) => onPageSizeChange(Number(e.target.value))}
              className="rounded border border-[var(--border-default)] bg-[var(--bg-elevated)] px-1 py-0.5 text-xs text-[var(--text-primary)]"
            >
              {PAGE_SIZE_OPTIONS.map((size) => (
                <option key={size} value={size}>{size} 行/页</option>
              ))}
            </select>
          </div>
          {totalPages > 1 && (
            <div className="flex items-center gap-1">
              <button
                className="rounded px-2 py-0.5 hover:bg-[var(--bg-elevated)] disabled:opacity-30"
                disabled={page === 0}
                onClick={() => onPageChange(0)}
              >
                首页
              </button>
              <button
                className="rounded px-2 py-0.5 hover:bg-[var(--bg-elevated)] disabled:opacity-30"
                disabled={page === 0}
                onClick={() => onPageChange(page - 1)}
              >
                上一页
              </button>
              <button
                className="rounded px-2 py-0.5 hover:bg-[var(--bg-elevated)] disabled:opacity-30"
                disabled={page >= totalPages - 1}
                onClick={() => onPageChange(page + 1)}
              >
                下一页
              </button>
              <button
                className="rounded px-2 py-0.5 hover:bg-[var(--bg-elevated)] disabled:opacity-30"
                disabled={page >= totalPages - 1}
                onClick={() => onPageChange(totalPages - 1)}
              >
                末页
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export default function ResultTable({ result, pageSize: initialPageSize = 50 }: ResultTableProps) {
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(initialPageSize)

  // Reset page, sorting, and filters when result changes
  useEffect(() => {
    const id = requestAnimationFrame(() => {
      setPage(0)
      setSorting([])
      setColumnFilters([])
      setPageSize(initialPageSize)
    })
    return () => cancelAnimationFrame(id)
  }, [result, initialPageSize])

  const handlePageSizeChange = useCallback((newSize: number) => {
    setPageSize(newSize)
    setPage(0)
  }, [])

  return (
    <ResultTableInner
      result={result}
      sorting={sorting}
      onSortingChange={setSorting}
      columnFilters={columnFilters}
      onColumnFiltersChange={setColumnFilters}
      page={page}
      onPageChange={setPage}
      pageSize={pageSize}
      onPageSizeChange={handlePageSizeChange}
    />
  )
}
