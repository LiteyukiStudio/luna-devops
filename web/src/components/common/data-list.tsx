import type { CSSProperties, ReactNode } from 'react'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { EmptyState } from './empty-state'
import { PaginationController } from './pagination'

type DataListColumnWidth = 'actions' | 'compact' | 'normal' | 'number' | 'primary' | 'secondary' | 'status'

export interface DataListColumn<T> {
  key: string
  header: ReactNode
  className?: string
  headerClassName?: string
  cellClassName?: string
  maxWidth?: number | string
  minWidth?: number | string
  sticky?: 'left' | 'right'
  width?: DataListColumnWidth
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
    isRowSelectable?: (item: T) => boolean
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

function stickyColumnClass(sticky: DataListColumn<unknown>['sticky'], surface: 'header' | 'cell') {
  if (!sticky)
    return undefined

  return cn(
    'sticky',
    sticky === 'right' ? 'right-0 border-l border-border' : 'left-0 border-r border-border',
    surface === 'header' ? 'z-30 bg-muted' : 'z-20 bg-card group-hover:bg-muted/40',
  )
}

const columnWidthProfiles: Record<DataListColumnWidth, { max?: number, min: number }> = {
  actions: { min: 96 },
  compact: { min: 96, max: 144 },
  normal: { min: 144, max: 288 },
  number: { min: 80, max: 128 },
  primary: { min: 224, max: 448 },
  secondary: { min: 128, max: 224 },
  status: { min: 112, max: 176 },
}

function inferredColumnWidth(column: DataListColumn<unknown>): DataListColumnWidth {
  if (column.width)
    return column.width
  const key = column.key.toLowerCase()
  if (column.sticky === 'right' || key.includes('action'))
    return 'actions'
  if (key.includes('status') || key.includes('scope') || key.includes('state') || key.includes('enabled'))
    return 'status'
  if (key.includes('count') || key.includes('size') || key.includes('total') || key.includes('amount') || key.includes('cost'))
    return 'number'
  if (key.includes('time') || key.includes('date') || key.includes('stage') || key.includes('role'))
    return 'compact'
  if (key.includes('description') || key.includes('message') || key.includes('image') || key.includes('url') || key.includes('endpoint'))
    return 'normal'
  if (key.includes('name') || key.includes('project') || key.includes('application') || key.includes('repository'))
    return 'primary'
  return 'secondary'
}

function widthValue(value: number | string | undefined, fallback?: number) {
  if (typeof value === 'number')
    return `${value}px`
  return value ?? (fallback === undefined ? undefined : `${fallback}px`)
}

function columnWidthStyle(column: DataListColumn<unknown>): CSSProperties {
  const profile = columnWidthProfiles[inferredColumnWidth(column)]
  const maxWidth = widthValue(column.maxWidth, profile.max)
  const style: CSSProperties = {
    minWidth: widthValue(column.minWidth, profile.min),
  }
  if (maxWidth)
    style.maxWidth = maxWidth
  return style
}

function columnContentClassName(column: DataListColumn<unknown>, surface: 'header' | 'cell') {
  if (inferredColumnWidth(column) === 'actions') {
    return cn(
      'max-w-none overflow-visible',
      surface === 'header' ? 'ml-auto min-w-0 truncate' : 'ml-auto w-max',
    )
  }

  return cn('min-w-0 overflow-hidden', surface === 'header' && 'truncate')
}

/**
 * 管理台列表和表格的统一展示组件。
 * 用于资源列表、用户列表、凭据列表等需要列、空状态和分页的场景；布局型页面或少量指标卡片不应套用它。
 * 列宽采用“内容自适应 + 画像上限”策略：浏览器先按每列最宽内容分配宽度，未填满容器时由 min-w-full 均摊剩余空间；
 * 超过容器时按列画像限制最大宽度，让次要列先收缩，主列保留更高的最小宽度；操作列按按钮内容撑开并交给外层滚动处理。
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
  const selectableRowKeys = selection ? items.filter(item => selection.isRowSelectable?.(item) ?? true).map(rowKey) : rowKeys
  const selectable = Boolean(selection)
  const allRowsSelected = selectableRowKeys.length > 0 && selectableRowKeys.every(key => selectedKeySet.has(key))
  const someRowsSelected = selectableRowKeys.some(key => selectedKeySet.has(key))
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
    for (const key of selectableRowKeys) {
      if (selected)
        next.add(key)
      else
        next.delete(key)
    }
    selection.onSelectionChange([...next])
  }

  return (
    <Card className={cn('flex w-full min-w-0 max-w-full max-h-none flex-col overflow-hidden p-0 md:max-h-[calc(100vh-15rem)]', variant === 'plain' && 'rounded-none border-0 bg-transparent shadow-none')}>
      {(title || search || selection?.bulkActions) && (
        <div className="flex shrink-0 flex-col gap-3 border-b border-border px-4 py-4 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
          <div className="min-w-0">
            {title && <h2 className="text-base font-semibold">{title}</h2>}
            {selection && selection.selectedKeys.length > 0 && (
              <p className="mt-1 text-xs text-muted-foreground">{selection.selectedLabel}</p>
            )}
          </div>
          <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center sm:justify-end">
            {search && (
              <Input
                className="h-9 w-full sm:w-64"
                placeholder={search.placeholder}
                value={search.value}
                onChange={event => search.onChange(event.target.value)}
              />
            )}
            {selection?.bulkActions}
          </div>
        </div>
      )}
      <div className="min-h-0 w-full min-w-0 max-w-full flex-1 overflow-auto">
        {items.length === 0
          ? <EmptyState description={emptyDescription} title={emptyTitle} variant="plain" />
          : (
              <table className="min-w-full table-auto caption-bottom text-sm" data-slot="data-list-table">
                <thead className="sticky top-0 z-10 bg-muted/95 backdrop-blur [&_tr]:border-b">
                  <tr className="border-b border-border transition-colors hover:bg-muted/40">
                    {selectable && (
                      <th className="h-10 w-10 px-4 py-3 text-left align-middle text-xs font-medium whitespace-nowrap text-muted-foreground">
                        <input
                          aria-label={selection?.selectAllLabel}
                          checked={allRowsSelected}
                          className="size-4 accent-primary"
                          disabled={selectableRowKeys.length === 0}
                          ref={(element) => {
                            if (element)
                              element.indeterminate = someRowsSelected && !allRowsSelected
                          }}
                          type="checkbox"
                          onChange={event => updateAllRowsSelection(event.target.checked)}
                        />
                      </th>
                    )}
                    {columns.map(column => (
                      <th
                        key={column.key}
                        className={cn(
                          'h-10 px-4 py-3 text-left align-middle text-xs font-medium whitespace-nowrap text-muted-foreground',
                          column.className,
                          stickyColumnClass(column.sticky, 'header'),
                          column.headerClassName,
                        )}
                      >
                        <div className={columnContentClassName(column as DataListColumn<unknown>, 'header')} style={columnWidthStyle(column as DataListColumn<unknown>)}>
                          {column.header}
                        </div>
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody className="[&_tr:last-child]:border-0">
                  {items.map((item) => {
                    const itemKey = rowKey(item)
                    const rowSelectable = selection?.isRowSelectable?.(item) ?? true
                    return (
                      <tr key={itemKey} className="group border-b border-border transition-colors hover:bg-muted/40">
                        {selectable && (
                          <td className="w-10 px-4 py-3 align-middle">
                            <input
                              aria-label={selection?.selectRowLabel(item)}
                              checked={selectedKeySet.has(itemKey)}
                              className="size-4 accent-primary"
                              disabled={!rowSelectable}
                              type="checkbox"
                              onChange={event => updateRowSelection(itemKey, event.target.checked)}
                            />
                          </td>
                        )}
                        {columns.map(column => (
                          <td
                            key={column.key}
                            className={cn(
                              'px-4 py-3 align-middle',
                              column.className,
                              stickyColumnClass(column.sticky, 'cell'),
                              column.cellClassName,
                            )}
                          >
                            <div className={columnContentClassName(column as DataListColumn<unknown>, 'cell')} style={columnWidthStyle(column as DataListColumn<unknown>)}>
                              {column.render(item)}
                            </div>
                          </td>
                        ))}
                      </tr>
                    )
                  })}
                </tbody>
              </table>
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
