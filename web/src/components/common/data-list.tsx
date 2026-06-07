import type { ReactNode } from 'react'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { EmptyState } from './empty-state'
import { PaginationController } from './pagination'

export interface DataListColumn<T> {
  key: string
  header: ReactNode
  className?: string
  render: (item: T) => ReactNode
}

interface DataListProps<T> {
  items: T[]
  columns: DataListColumn<T>[]
  rowKey: (item: T) => string
  title?: ReactNode
  variant?: 'card' | 'plain'
  emptyTitle: string
  emptyDescription?: string
  search?: {
    value: string
    placeholder: string
    onChange: (value: string) => void
  }
  selection?: {
    selectedKeys: string[]
    selectAllLabel: string
    selectRowLabel: (item: T) => string
    selectedLabel: string
    bulkActions?: ReactNode
    onSelectionChange: (keys: string[]) => void
  }
  pagination?: {
    page: number
    pageSize: number
    defaultPageSize?: number
    total: number
    totalPages: number
    pageInfoLabel: string
    onPageChange: (page: number) => void
    onPageSizeChange?: (pageSize: number) => void
    pageSizeOptions?: number[]
  }
}

/**
 * 管理台列表和表格的统一展示组件。
 * 用于资源列表、用户列表、凭据列表等需要列、空状态和分页的场景；布局型页面或少量指标卡片不应套用它。
 */
export function DataList<T>({
  items,
  columns,
  rowKey,
  title,
  variant = 'card',
  emptyTitle,
  emptyDescription,
  search,
  selection,
  pagination,
}: DataListProps<T>) {
  const selectedKeySet = new Set(selection?.selectedKeys ?? [])
  const rowKeys = items.map(rowKey)
  const selectable = Boolean(selection)
  const allRowsSelected = rowKeys.length > 0 && rowKeys.every(key => selectedKeySet.has(key))
  const someRowsSelected = rowKeys.some(key => selectedKeySet.has(key))
  const updateRowSelection = (key: string, selected: boolean) => {
    if (!selection)
      return
    const next = new Set(selection.selectedKeys)
    if (selected)
      next.add(key)
    else
      next.delete(key)
    selection.onSelectionChange([...next])
  }
  const updateAllRowsSelection = (selected: boolean) => {
    if (!selection)
      return
    const next = new Set(selection.selectedKeys)
    for (const key of rowKeys) {
      if (selected)
        next.add(key)
      else
        next.delete(key)
    }
    selection.onSelectionChange([...next])
  }

  return (
    <Card className={`flex max-h-[calc(100vh-15rem)] min-h-0 flex-col overflow-hidden p-0 ${variant === 'plain' ? 'rounded-none border-0 bg-transparent shadow-none' : ''}`}>
      {(title || search || selection?.bulkActions) && (
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-4">
          <div className="min-w-0">
            {title && <h2 className="text-base font-semibold">{title}</h2>}
            {selection && selection.selectedKeys.length > 0 && (
              <p className="mt-1 text-xs text-muted-foreground">{selection.selectedLabel}</p>
            )}
          </div>
          <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
            {search && (
              <Input
                className="h-9 w-64 max-w-full"
                placeholder={search.placeholder}
                value={search.value}
                onChange={event => search.onChange(event.target.value)}
              />
            )}
            {selection?.bulkActions}
          </div>
        </div>
      )}
      <div className="min-h-0 flex-1 overflow-auto">
        {items.length === 0
          ? <EmptyState title={emptyTitle} description={emptyDescription} />
          : (
              <Table>
                <TableHeader className="sticky top-0 z-10 bg-muted/95 backdrop-blur">
                  <TableRow>
                    {selectable && (
                      <TableHead className="w-10 px-4 py-3 align-middle">
                        <input
                          aria-label={selection?.selectAllLabel}
                          checked={allRowsSelected}
                          className="size-4 accent-primary"
                          ref={(element) => {
                            if (element)
                              element.indeterminate = someRowsSelected && !allRowsSelected
                          }}
                          type="checkbox"
                          onChange={event => updateAllRowsSelection(event.target.checked)}
                        />
                      </TableHead>
                    )}
                    {columns.map(column => (
                      <TableHead key={column.key} className={column.className}>
                        {column.header}
                      </TableHead>
                    ))}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map(item => (
                    <TableRow key={rowKey(item)}>
                      {selectable && (
                        <TableCell className="w-10 px-4 py-3 align-middle">
                          <input
                            aria-label={selection?.selectRowLabel(item)}
                            checked={selectedKeySet.has(rowKey(item))}
                            className="size-4 accent-primary"
                            type="checkbox"
                            onChange={event => updateRowSelection(rowKey(item), event.target.checked)}
                          />
                        </TableCell>
                      )}
                      {columns.map(column => (
                        <TableCell key={column.key} className={column.className}>
                          {column.render(item)}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
      </div>

      {pagination && (
        <div className="shrink-0 border-t border-border px-4 py-3 text-sm text-muted-foreground">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <span>{pagination.pageInfoLabel}</span>
            <PaginationController
              initialPage={pagination.page}
              pageSize={pagination.pageSize}
              defaultPageSize={pagination.defaultPageSize}
              pageSizeOptions={pagination.pageSizeOptions}
              total={pagination.total}
              onPageChange={pagination.onPageChange}
              onPageSizeChange={pagination.onPageSizeChange}
            />
          </div>
        </div>
      )}
    </Card>
  )
}
