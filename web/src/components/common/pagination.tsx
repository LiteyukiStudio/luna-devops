import type { HTMLAttributes } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { NativeSelect } from '@/components/ui/native-select'
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination'
import { cn } from '@/lib/utils'

interface PaginationControllerProps extends HTMLAttributes<HTMLDivElement> {
  total: number
  pageSize?: number
  defaultPageSize?: number
  pageSizeOptions?: number[]
  initialPage?: number
  maxButtons?: number
  onPageChange?: (page: number) => void
  onPageSizeChange?: (pageSize: number) => void
  disabled?: boolean
  hideOnSinglePage?: boolean
  reportAutoAdjust?: boolean
}

/**
 * 列表分页控制器。
 * 用于后端分页列表的页码和 pageSize 控制；无限滚动、少量固定选项或 tab 切换不应复用它。
 */
export function PaginationController({
  total,
  pageSize,
  defaultPageSize = 10,
  pageSizeOptions = [10, 20, 50, 100],
  initialPage = 1,
  maxButtons = 7,
  onPageChange,
  onPageSizeChange,
  disabled = false,
  hideOnSinglePage = false,
  reportAutoAdjust = true,
  className,
  ...rest
}: PaginationControllerProps) {
  const { t } = useTranslation()
  const effectivePageSize = pageSize ?? defaultPageSize
  const maxBtns = useMemo(() => {
    const normalized = Math.max(5, maxButtons || 7)
    return normalized % 2 === 0 ? normalized + 1 : normalized
  }, [maxButtons])
  const totalPages = useMemo(
    () => Math.max(0, Math.ceil(Math.max(0, total) / Math.max(1, effectivePageSize))),
    [effectivePageSize, total],
  )
  const [uncontrolledPage, setUncontrolledPage] = useState(() => clampPage(initialPage, totalPages))
  const currentPage = onPageChange ? initialPage : uncontrolledPage
  const effectivePage = clampPage(currentPage, totalPages)
  const normalizedPageSizeOptions = useMemo(() => {
    const options = [...new Set([...pageSizeOptions, effectivePageSize])]
      .filter(option => Number.isFinite(option) && option > 0)
      .sort((left, right) => left - right)
    return options.length > 0 ? options : [effectivePageSize]
  }, [effectivePageSize, pageSizeOptions])

  const lastReportedRef = useRef<number | null>(null)
  useEffect(() => {
    if (!onPageChange)
      return
    if (lastReportedRef.current === effectivePage)
      return
    if (!reportAutoAdjust && lastReportedRef.current !== null && currentPage > totalPages)
      return
    lastReportedRef.current = effectivePage
    onPageChange(effectivePage)
  }, [currentPage, effectivePage, onPageChange, reportAutoAdjust, totalPages])

  const handleSetPage = useCallback((page: number) => {
    if (disabled)
      return
    const nextPage = clampPage(page, totalPages)
    if (onPageChange) {
      lastReportedRef.current = nextPage
      onPageChange(nextPage)
      return
    }
    setUncontrolledPage(nextPage)
  }, [disabled, onPageChange, totalPages])

  const pages = useMemo(() => {
    if (totalPages <= maxBtns)
      return { list: range(1, totalPages), type: 'all' as const }

    const windowSize = maxBtns - 4
    let start = effectivePage - Math.floor(windowSize / 2)
    let end = start + windowSize - 1
    if (start < 3) {
      start = 3
      end = start + windowSize - 1
    }
    if (end > totalPages - 2) {
      end = totalPages - 2
      start = end - windowSize + 1
    }
    return { end, list: range(start, end), start, type: 'window' as const }
  }, [effectivePage, maxBtns, totalPages])

  if (total <= 0 || totalPages === 0)
    return null

  if (hideOnSinglePage && totalPages === 1)
    return null

  const renderPage = (page: number) => (
    <PaginationItem key={page}>
      <PaginationLink
        aria-current={page === effectivePage ? 'page' : undefined}
        aria-label={t('pagination.goToPage', { page })}
        className={cn(
          'h-9 min-w-9 rounded-md px-3 text-sm',
          page === effectivePage && 'border-theme-emphasis/35 bg-theme-emphasis/12 text-foreground shadow-none hover:bg-theme-emphasis/18 hover:text-foreground',
        )}
        isActive={page === effectivePage}
        tabIndex={disabled ? -1 : 0}
        onClick={(event) => {
          event.preventDefault()
          handleSetPage(page)
        }}
      >
        {page}
      </PaginationLink>
    </PaginationItem>
  )

  const previousDisabled = disabled || effectivePage === 1
  const nextDisabled = disabled || effectivePage === totalPages

  return (
    <div className={cn('flex flex-wrap items-center justify-center gap-3', className)} {...rest}>
      {onPageSizeChange && (
        <label className="flex shrink-0 items-center gap-2 text-sm text-muted-foreground">
          <span className="whitespace-nowrap">{t('pagination.pageSize')}</span>
          <NativeSelect
            aria-label={t('pagination.pageSizeAria')}
            className="h-9 bg-surface px-3 text-sm"
            containerClassName="w-24"
            disabled={disabled}
            value={effectivePageSize}
            onChange={(event) => {
              const nextPageSize = Number(event.target.value)
              if (Number.isNaN(nextPageSize))
                return
              onPageSizeChange(nextPageSize)
            }}
          >
            {normalizedPageSizeOptions.map(option => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </NativeSelect>
        </label>
      )}
      <Pagination className="mx-0 w-auto">
        <PaginationContent className="select-none gap-2">
          <PaginationItem>
            <PaginationPrevious
              aria-disabled={previousDisabled}
              className={cn('h-9 whitespace-nowrap text-sm text-foreground/70 hover:text-primary-text', previousDisabled && 'pointer-events-none text-muted-foreground opacity-60')}
              tabIndex={previousDisabled ? -1 : 0}
              onClick={(event) => {
                event.preventDefault()
                if (previousDisabled)
                  return
                handleSetPage(effectivePage - 1)
              }}
            />
          </PaginationItem>

          {pages.type === 'all' && pages.list.map(renderPage)}

          {pages.type === 'window' && (
            <>
              {renderPage(1)}
              {pages.start > 3
                ? (
                    <PaginationItem>
                      <PaginationEllipsis />
                    </PaginationItem>
                  )
                : renderPage(2)}

              {pages.list.map(renderPage)}

              {pages.end < totalPages - 2
                ? (
                    <PaginationItem>
                      <PaginationEllipsis />
                    </PaginationItem>
                  )
                : renderPage(totalPages - 1)}
              {renderPage(totalPages)}
            </>
          )}

          <PaginationItem>
            <PaginationNext
              aria-disabled={nextDisabled}
              className={cn('h-9 whitespace-nowrap text-sm text-foreground/70 hover:text-primary-text', nextDisabled && 'pointer-events-none text-muted-foreground opacity-60')}
              tabIndex={nextDisabled ? -1 : 0}
              onClick={(event) => {
                event.preventDefault()
                if (nextDisabled)
                  return
                handleSetPage(effectivePage + 1)
              }}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  )
}

function clampPage(page: number, totalPages: number) {
  if (Number.isNaN(page))
    return 1
  return Math.min(Math.max(1, Math.floor(page)), Math.max(1, totalPages))
}

function range(start: number, end: number) {
  const output: number[] = []
  for (let index = start; index <= end; index += 1)
    output.push(index)
  return output
}
