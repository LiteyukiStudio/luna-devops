import type { ComponentProps, ReactNode } from 'react'
import { cn } from '@/lib/utils'

/**
 * 列表页的统一搜索、筛选、排序和主操作容器。
 * children 放查询工具，actions 放页面主操作；小屏自动分行，桌面保持主操作靠右。
 */
export function PageToolbar({ actions, children, className, ...props }: ComponentProps<'div'> & { actions?: ReactNode }) {
  return (
    <div
      className={cn('flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between', className)}
      data-slot="page-toolbar"
      {...props}
    >
      <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
        {children}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2 sm:justify-end">{actions}</div>}
    </div>
  )
}
