import type { CSSProperties, MouseEvent, ReactNode } from 'react'
import { MoreHorizontal } from 'lucide-react'
import { useState, useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { TableFrame } from '@/components/ui/table-frame'
import { cn } from '@/lib/utils'
import { EmptyState } from './empty-state'
import { DataListSkeleton } from './loading-states'
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
  mobile?: 'hidden' | 'visible'
  mobileActions?: 'collapse' | 'inline'
  sticky?: 'left' | 'right'
  width?: DataListColumnWidth
  render: (item: T) => ReactNode
}

interface DataListProps<T> {
  items: T[]
  columns: DataListColumn<T>[]
  rowKey: (item: T) => string
  title?: ReactNode
  toolbar?: ReactNode
  emptyTitle: string
  emptyActions?: ReactNode
  emptyDescription?: ReactNode
  emptyIcon?: ReactNode
  emptyMode?: 'actionable' | 'filtered'
  loading?: boolean
  constrainedHeight?: boolean
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
    surface === 'cell' && (sticky === 'right' ? 'right-0 border-l-0' : 'left-0 border-r-0'),
    surface === 'header' && (sticky === 'right' ? 'right-0' : 'left-0'),
    surface === 'header' ? 'z-30 [background:var(--data-list-header-surface)]' : 'z-20 bg-card group-hover:[background:var(--data-list-row-hover)]',
  )
}

const columnWidthProfiles: Record<DataListColumnWidth, { max?: number, min: number }> = {
  actions: { min: 0 },
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

function columnCellClassName(column: DataListColumn<unknown>) {
  if (inferredColumnWidth(column) !== 'actions')
    return undefined

  // 操作列由实际按钮内容撑开；公共层覆盖页面遗留固定宽度，避免小屏主信息被挤压。
  return 'w-px min-w-0 px-2 whitespace-nowrap sm:px-4'
}

const mobileActionMediaQuery = '(max-width: 47.999rem)'

function subscribeMobileActionViewport(onStoreChange: () => void) {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function')
    return () => undefined

  const mediaQuery = window.matchMedia(mobileActionMediaQuery)
  mediaQuery.addEventListener('change', onStoreChange)
  return () => mediaQuery.removeEventListener('change', onStoreChange)
}

function mobileActionViewportSnapshot() {
  return typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia(mobileActionMediaQuery).matches
}

function MobileActionMenu({ children, label }: { children: ReactNode, label: string }) {
  const [open, setOpen] = useState(false)
  const closeAfterAction = (event: MouseEvent<HTMLDivElement>) => {
    const target = event.target
    if (!(target instanceof Element))
      return
    const action = target.closest('button:not(:disabled), a[href], [role="menuitem"]:not([aria-disabled="true"])')
    if (action && action.getAttribute('aria-haspopup') !== 'dialog')
      setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button aria-label={label} size="icon" variant="ghost">
          <MoreHorizontal className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-max min-w-40 max-w-[calc(100vw-2rem)] p-1.5">
        <div
          className="flex flex-col items-stretch gap-1 [&>div]:flex-col [&>div]:items-stretch [&>div]:gap-1 [&_a]:w-full [&_a]:justify-start [&_button]:w-full [&_button]:justify-start"
          data-slot="data-list-mobile-actions"
          onClick={closeAfterAction}
        >
          {children}
        </div>
      </PopoverContent>
    </Popover>
  )
}

/**
 * 管理台列表和表格的统一展示组件。
 * 用于资源列表、用户列表、凭据列表等需要列、空状态和分页的场景；布局型页面或少量指标卡片不应套用它。
 * 列宽采用“内容自适应 + 画像上限”策略：浏览器先按每列最宽内容分配宽度，未填满容器时由 min-w-full 均摊剩余空间；
 * 超过容器时按列画像限制最大宽度，让次要列先收缩，主列保留更高的最小宽度；操作列在桌面按内容撑开，移动端统一收进溢出菜单。
 */
export function DataList<T>({
  items,
  columns,
  rowKey,
  title,
  toolbar,
  emptyTitle,
  emptyActions,
  emptyDescription,
  emptyIcon,
  emptyMode = 'actionable',
  loading = false,
  constrainedHeight = false,
  search,
  selection,
  pagination,
}: DataListProps<T>) {
  const { t } = useTranslation()
  const collapseMobileActions = useSyncExternalStore(
    subscribeMobileActionViewport,
    mobileActionViewportSnapshot,
    () => false,
  )
  const selectedKeySet = new Set(selection?.selectedKeys ?? [])
  const rowKeys = items.map(rowKey)
  const selectableRowKeys = selection ? items.filter(item => selection.isRowSelectable?.(item) ?? true).map(rowKey) : rowKeys
  const selectable = Boolean(selection)
  const hasTools = Boolean(title || toolbar || search || selection?.bulkActions)
  const showTableFrame = loading || items.length > 0
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
  const tableFooter = pagination && pagination.total > 0 && !loading
    ? (
        <div className="px-3 py-3 text-sm text-muted-foreground sm:px-4">
          <div className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
            <span>{pagination.pageInfoLabel}</span>
            <PaginationController
              className="w-full justify-between sm:w-auto sm:justify-center"
              hideOnSinglePage
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
      )
    : undefined

  return (
    <div
      className={cn(
        'flex w-full min-w-0 max-w-full max-h-none flex-col md:max-h-[calc(100vh-15rem)]',
        constrainedHeight && 'md:h-[calc(100vh-15rem)]',
      )}
      data-slot="data-list"
    >
      {hasTools && (
        <div
          className={cn(
            'flex shrink-0 flex-col gap-3 pb-4 sm:flex-row sm:flex-wrap sm:items-center',
            title ? 'sm:justify-between' : 'sm:justify-start',
          )}
          data-slot="data-list-tools"
        >
          <div className="min-w-0">
            {title && <h2 className="text-base font-semibold">{title}</h2>}
            {selection && selection.selectedKeys.length > 0 && (
              <p className="mt-1 text-xs text-muted-foreground">{selection.selectedLabel}</p>
            )}
          </div>
          <div className={cn(
            'flex min-w-0 flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center',
            title && 'sm:justify-end',
          )}
          >
            {search && (
              <Input
                className="h-9 w-full sm:w-64"
                placeholder={search.placeholder}
                value={search.value}
                onChange={event => search.onChange(event.target.value)}
              />
            )}
            {selection?.bulkActions}
            {toolbar}
          </div>
        </div>
      )}
      <TableFrame
        className={cn(
          'w-full flex-1',
          !hasTools && showTableFrame && 'mt-group',
        )}
        footer={tableFooter}
        framed={showTableFrame}
        scrollAreaClassName="h-full"
        scrollbars="both"
        scrollType="auto"
      >
        {loading
          ? <DataListSkeleton columns={Math.max(2, Math.min(columns.length + (selectable ? 1 : 0), 6))} />
          : items.length === 0
            ? (
                <EmptyState
                  actions={emptyActions}
                  description={emptyDescription}
                  icon={emptyIcon}
                  mode={emptyMode}
                  title={emptyTitle}
                  variant="plain"
                />
              )
            : (
                <table className="w-max min-w-full table-auto bg-transparent caption-bottom text-sm" data-slot="data-list-table">
                  <thead className="sticky top-0 z-10 [background:var(--data-list-header-surface)]">
                    <tr>
                      {selectable && (
                        <th className="h-11 w-10 px-3 py-3 text-left align-middle text-sm font-medium whitespace-nowrap text-foreground sm:px-4">
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
                            column.className,
                            'h-11 px-3 py-3 text-left align-middle text-sm font-medium whitespace-nowrap text-foreground sm:px-4',
                            stickyColumnClass(column.sticky, 'header'),
                            column.headerClassName,
                            column.mobile === 'hidden' && 'hidden md:table-cell',
                            columnCellClassName(column as DataListColumn<unknown>),
                          )}
                        >
                          <div className={columnContentClassName(column as DataListColumn<unknown>, 'header')} style={columnWidthStyle(column as DataListColumn<unknown>)}>
                            {column.header}
                          </div>
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="bg-card">
                    {items.map((item) => {
                      const itemKey = rowKey(item)
                      const rowSelectable = selection?.isRowSelectable?.(item) ?? true
                      return (
                        <tr
                          key={itemKey}
                          className="group border-t border-border transition-colors hover:[&>td]:[background:var(--data-list-row-hover)] [&>td]:transition-colors"
                        >
                          {selectable && (
                            <td className="w-10 px-3 py-3 align-middle sm:px-4">
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
                          {columns.map((column) => {
                            const content = column.render(item)
                            const collapseActions = collapseMobileActions
                              && inferredColumnWidth(column as DataListColumn<unknown>) === 'actions'
                              && column.mobileActions !== 'inline'
                            return (
                              <td
                                key={column.key}
                                className={cn(
                                  column.className,
                                  'px-3 py-3 align-middle sm:px-4',
                                  stickyColumnClass(column.sticky, 'cell'),
                                  column.cellClassName,
                                  column.mobile === 'hidden' && 'hidden md:table-cell',
                                  columnCellClassName(column as DataListColumn<unknown>),
                                )}
                              >
                                <div className={columnContentClassName(column as DataListColumn<unknown>, 'cell')} style={columnWidthStyle(column as DataListColumn<unknown>)}>
                                  {collapseActions
                                    ? <MobileActionMenu label={t('common.actions')}>{content}</MobileActionMenu>
                                    : content}
                                </div>
                              </td>
                            )
                          })}
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              )}
      </TableFrame>
    </div>
  )
}
