import { useMemo, useEffect, useState } from 'react'
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  flexRender,
  type ColumnDef,
  type SortingState,
} from '@tanstack/react-table'
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
} from '@/components/ui/table'
import { ChevronUp, ChevronDown, ChevronsUpDown, Lock } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import type { QueryResult } from '@/api/query'

interface ResultTableProps {
  result: QueryResult | null
  pageSize?: number
}

const PAGE_SIZE_OPTIONS = [50, 100, 200] as const

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

export default function ResultTable({ result, pageSize: initialPageSize = 50 }: ResultTableProps) {
  const [sorting, setSorting] = useState<SortingState>([])
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(initialPageSize)

  // Reset page and sorting when result changes
  useEffect(() => {
    setPage(0)
    setSorting([])
    setPageSize(initialPageSize)
  }, [result, initialPageSize])

  const columns = useMemo<ColumnDef<Record<string, unknown>>[]>(() => {
    if (!result?.columns) return []
    return result.columns.map((col) => ({
      accessorKey: col,
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
      cell: ({ getValue }) => <CellValue value={getValue()} />,
      size: 150,
    }))
  }, [result])

  const data = useMemo(() => result?.rows ?? [], [result])

  const table = useReactTable({
    data,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  })

  const totalPages = Math.ceil(data.length / pageSize)
  const paginatedRows = table.getRowModel().rows.slice(
    page * pageSize,
    (page + 1) * pageSize,
  )

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
      <div className="flex-1 overflow-auto">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((hg) => (
              <TableRow key={hg.id} className="border-[var(--border-default)] hover:bg-transparent">
                {hg.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    className="cursor-pointer whitespace-nowrap text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                    style={{ width: header.getSize() }}
                    onClick={header.column.getToggleSortingHandler()}
                  >
                    <div className="flex items-center gap-1">
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
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {paginatedRows.map((row) => (
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
            ))}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      {data.length > 0 && (
        <div className="flex items-center justify-between border-t border-[var(--border-default)] px-3 py-2 text-xs text-[var(--text-secondary)]">
          <div className="flex items-center gap-2">
            <span>
              共 {data.length} 行{totalPages > 1 ? `，第 ${page + 1}/${totalPages} 页` : ''}
            </span>
            <select
              value={pageSize}
              onChange={(e) => {
                setPageSize(Number(e.target.value))
                setPage(0)
              }}
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
                onClick={() => setPage(0)}
              >
                首页
              </button>
              <button
                className="rounded px-2 py-0.5 hover:bg-[var(--bg-elevated)] disabled:opacity-30"
                disabled={page === 0}
                onClick={() => setPage((p) => p - 1)}
              >
                上一页
              </button>
              <button
                className="rounded px-2 py-0.5 hover:bg-[var(--bg-elevated)] disabled:opacity-30"
                disabled={page >= totalPages - 1}
                onClick={() => setPage((p) => p + 1)}
              >
                下一页
              </button>
              <button
                className="rounded px-2 py-0.5 hover:bg-[var(--bg-elevated)] disabled:opacity-30"
                disabled={page >= totalPages - 1}
                onClick={() => setPage(totalPages - 1)}
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
