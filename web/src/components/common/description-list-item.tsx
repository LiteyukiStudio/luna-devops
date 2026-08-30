import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface DescriptionListItemProps {
  className?: string
  danger?: boolean
  emptyFallback?: ReactNode
  label: ReactNode
  mono?: boolean
  value?: ReactNode
}

/**
 * 描述列表中的单个“标签—值”条目，必须放在 `dl` 内使用。
 */
export function DescriptionListItem({ className, danger = false, emptyFallback, label, mono = false, value }: DescriptionListItemProps) {
  const displayedValue = value === null || value === undefined || value === '' ? emptyFallback : value

  return (
    <div className={cn('grid min-w-0 gap-1', className)} data-slot="description-list-item">
      <dt className="text-xs text-muted-foreground" data-slot="description-list-label">{label}</dt>
      <dd
        className={cn(
          'min-w-0 text-sm text-foreground',
          mono && 'font-mono text-xs',
          danger && 'text-danger',
        )}
        data-slot="description-list-value"
      >
        {displayedValue}
      </dd>
    </div>
  )
}
