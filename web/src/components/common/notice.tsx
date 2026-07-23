import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

type NoticeTone = 'danger' | 'info' | 'success' | 'warning'

/** 页面级状态提示；状态颜色固定，不随个人品牌主题变化。 */
export function Notice({ actions, children, className, icon, title, tone = 'info' }: {
  actions?: ReactNode
  children?: ReactNode
  className?: string
  icon?: ReactNode
  title: ReactNode
  tone?: NoticeTone
}) {
  return (
    <div className={cn('grid gap-3 rounded-lg border p-4', noticeToneClassName(tone), className)} data-slot="notice" role="status">
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-2">
          {icon && <span className="mt-0.5 shrink-0">{icon}</span>}
          <div className="min-w-0">
            <h2 className="font-semibold text-foreground">{title}</h2>
            {children && <div className="mt-1 text-sm text-muted-foreground">{children}</div>}
          </div>
        </div>
        {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </div>
  )
}

function noticeToneClassName(tone: NoticeTone) {
  if (tone === 'danger')
    return 'border-danger-border bg-danger-subtle'
  if (tone === 'warning')
    return 'border-warning-border bg-warning-subtle'
  if (tone === 'success')
    return 'border-success-border bg-success-subtle'
  return 'border-info-border bg-info-subtle'
}
