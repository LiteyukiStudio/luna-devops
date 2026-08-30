import type { ReactNode } from 'react'
import { HoverText } from '@/components/common/hover-text'
import { StatusValueBadge } from '@/components/common/status-badge'
import { cn } from '@/lib/utils'

interface ResourceDeletionStatusProps {
  message?: string
  messagePreview?: ReactNode
  status?: string
  className?: string
  messageClassName?: string
}

/** Shared deletion progress and failure detail used by resource list summaries. */
export function ResourceDeletionStatus({ className, message, messageClassName, messagePreview, status }: ResourceDeletionStatusProps) {
  if (!status || status === 'active')
    return null

  const failureMessage = status === 'delete_failed' ? message?.trim() : ''
  return (
    <span className={cn('flex min-w-0 items-center gap-2', className)}>
      <StatusValueBadge labelKeyPrefix="apps.deleteStatuses" value={status} />
      {failureMessage && (
        <HoverText className={cn('flex-1 text-xs text-muted-foreground', messageClassName)} value={failureMessage}>
          {messagePreview}
        </HoverText>
      )}
    </span>
  )
}
