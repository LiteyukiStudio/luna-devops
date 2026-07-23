import type { ReactNode } from 'react'
import type { SurfaceVariant } from './surface'
import { cn } from '@/lib/utils'
import { Surface } from './surface'

/** 页面内区块的统一标题、说明、图标和操作布局。 */
export function Section({ children, className, description, icon, title, tools, variant = 'plain' }: {
  children?: ReactNode
  className?: string
  description?: ReactNode
  icon?: ReactNode
  title: ReactNode
  tools?: ReactNode
  variant?: SurfaceVariant
}) {
  return (
    <Surface className={cn('grid gap-4', variant !== 'plain' && 'p-6', className)} variant={variant}>
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-2">
          {icon && <span className="mt-0.5 shrink-0 text-muted-foreground">{icon}</span>}
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-foreground">{title}</h2>
            {description && <div className="mt-1 text-sm text-muted-foreground">{description}</div>}
          </div>
        </div>
        {tools && <div className="flex shrink-0 flex-wrap items-center gap-2">{tools}</div>}
      </div>
      {children}
    </Surface>
  )
}
