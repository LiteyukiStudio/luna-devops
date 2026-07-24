import type { ComponentProps, ReactNode } from 'react'

import { cn } from '@/lib/utils'
import { TableFrame } from './table-frame'

type TableProps = ComponentProps<'table'> & {
  footer?: ReactNode
  footerClassName?: string
  frameClassName?: string
}

function Table({
  className,
  footer,
  footerClassName,
  frameClassName,
  ...props
}: TableProps) {
  return (
    <TableFrame
      className={cn('w-full', frameClassName)}
      data-slot="table-container"
      footer={footer}
      footerClassName={footerClassName}
      scrollbars="horizontal"
      scrollType="auto"
    >
      <table className={cn('w-max min-w-full bg-transparent caption-bottom text-sm', className)} data-slot="table" {...props} />
    </TableFrame>
  )
}

function TableHeader({ className, ...props }: ComponentProps<'thead'>) {
  return <thead className={cn('[background:var(--data-list-header-surface)] [&_tr]:border-b [&_tr]:border-border', className)} data-slot="table-header" {...props} />
}

function TableBody({ className, ...props }: ComponentProps<'tbody'>) {
  return <tbody className={cn('bg-card [&_tr:last-child]:border-0', className)} data-slot="table-body" {...props} />
}

function TableRow({ className, ...props }: ComponentProps<'tr'>) {
  return (
    <tr
      className={cn('border-b border-border transition-colors hover:[background:var(--data-list-row-hover)]', className)}
      data-slot="table-row"
      {...props}
    />
  )
}

function TableHead({ className, ...props }: ComponentProps<'th'>) {
  return (
    <th
      className={cn('h-10 px-4 text-left align-middle text-xs font-medium whitespace-nowrap text-muted-foreground', className)}
      data-slot="table-head"
      {...props}
    />
  )
}

function TableCell({ className, ...props }: ComponentProps<'td'>) {
  return <td className={cn('px-4 py-3 align-middle', className)} data-slot="table-cell" {...props} />
}

export { Table, TableBody, TableCell, TableHead, TableHeader, TableRow }
