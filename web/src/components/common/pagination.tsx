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
    () => Math.max(1, Math.ceil(total / Math.max(1, effectivePageSize))),
    [effectivePageSize, total],
  )
  const [currentPage, setCurrentPage] = useState(() => clampPage(initialPage, totalPages))

  const effectivePage = clampPage(currentPage, totalPages)
  const normalizedPageSizeOptions = useMemo(() => {
    const options = [...new Set([...pageSizeOptions, effectivePageSize])]
      .filter(option => Number.isFinite(option) && option > 0)
      .sort((left, right) => left - right)
    return options.length > 0 ? options : [effectivePageSize]
  }, [effectivePageSize, pageSizeOptions])

  const lastReportedRef = useRef<number | null>(null)
  useEffect(() => {
    setCurrentPage(clampPage(initialPage, totalPages))
  }, [initialPage, totalPages])

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
    setCurrentPage(() => clampPage(page, totalPages))
  }, [disabled, totalPages])

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

  if (hideOnSinglePage && totalPages === 1)
    return null

  const renderPage = (page: number) => (
    <PaginationItem key={page}>
      <PaginationLink
        aria-current={page === currentPage ? 'page' : undefined}
        aria-label={t('pagination.goToPage', { page })}
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
    <div className={cn('flex flex-wrap items-center justify-end gap-3', className)} {...rest}>
      {onPageSizeChange && (
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <span>{t('pagination.pageSize')}</span>
          <NativeSelect
            aria-label={t('pagination.pageSizeAria')}
            className="h-8 w-[84px] bg-surface px-2 text-sm"
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
        <PaginationContent className="select-none">
          <PaginationItem>
            <PaginationPrevious
              aria-disabled={previousDisabled}
              tabIndex={previousDisabled ? -1 : 0}
              onClick={(event) => {
                if (previousDisabled)
                  return
                event.preventDefault()
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
              tabIndex={nextDisabled ? -1 : 0}
              onClick={(event) => {
                if (nextDisabled)
                  return
                event.preventDefault()
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
