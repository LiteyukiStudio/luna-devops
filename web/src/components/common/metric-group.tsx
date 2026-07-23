import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { cn } from '@/lib/utils'

type MetricTone = 'danger' | 'neutral' | 'success' | 'warning'

/** 概览页指标的统一分组，使用共享边界代替多层独立卡片。 */
export function MetricGroup({ children, className }: { children: ReactNode, className?: string }) {
  return (
    <div className={cn('grid gap-px overflow-hidden rounded-lg border border-border bg-border sm:grid-cols-2 xl:grid-cols-4', className)} data-slot="metric-group">
      {children}
    </div>
  )
}

/** 指标项只表达标签、值和语义权重，不负责业务查询。 */
export function MetricItem({ emphasis = true, href, icon, label, meta, tone = 'neutral', value }: {
  emphasis?: boolean
  href?: string
  icon?: ReactNode
  label: ReactNode
  meta?: ReactNode
  tone?: MetricTone
  value: ReactNode
}) {
  const content = (
    <>
      <div className="flex items-center gap-2 text-sm text-muted-foreground transition-colors group-hover:text-primary-text">
        {icon}
        <span className="truncate">{label}</span>
      </div>
      <div className="mt-2 flex min-w-0 items-end justify-between gap-3">
        <span className={cn('text-2xl font-semibold tabular-nums', !emphasis && 'text-muted-foreground', metricToneClassName(tone))}>{value}</span>
        {meta && <span className="min-w-0 truncate text-xs text-muted-foreground">{meta}</span>}
      </div>
    </>
  )
  const className = 'group min-w-0 bg-surface-raised p-4 transition-colors hover:bg-surface-subtle'

  return href
    ? <Link className={className} data-slot="metric-item" to={href}>{content}</Link>
    : <div className={className} data-slot="metric-item">{content}</div>
}

function metricToneClassName(tone: MetricTone) {
  if (tone === 'danger')
    return 'text-danger'
  if (tone === 'warning')
    return 'text-warning'
  if (tone === 'success')
    return 'text-success'
  return undefined
}
