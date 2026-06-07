import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

function Empty({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      className={cn('flex min-h-32 flex-col items-start justify-center rounded-lg border border-border bg-surface p-4 text-sm text-muted-foreground', className)}
      data-slot="empty"
      {...props}
    />
  )
}

function EmptyHeader({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('grid gap-1', className)} data-slot="empty-header" {...props} />
}

function EmptyTitle({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('font-medium text-foreground', className)} data-slot="empty-title" {...props} />
}

function EmptyDescription({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('text-sm text-muted-foreground', className)} data-slot="empty-description" {...props} />
}

function EmptyContent({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('mt-3', className)} data-slot="empty-content" {...props} />
}

export { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle }
