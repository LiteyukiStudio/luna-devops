import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { cn } from '@/lib/utils'

type MetricTone = 'danger' | 'info' | 'neutral' | 'success' | 'warning'
type MetricSurface = 'neutral' | 'tinted'

/** 概览页指标的统一分组，外层依靠表面与圆角分层，指标之间保留共享分隔线。 */
export function MetricGroup({ children, className }: { children: ReactNode, className?: string }) {
  return (
    <div className={cn('grid gap-px overflow-hidden rounded-container bg-border sm:grid-cols-2 xl:grid-cols-4', className)} data-slot="metric-group">
      {children}
    </div>
  )
}

/** 指标项只表达标签、值和语义权重，不负责业务查询。 */
export function MetricItem({ emphasis = true, href, icon, label, meta, surface = 'tinted', tone = 'neutral', value }: {
  emphasis?: boolean
  href?: string
  icon?: ReactNode
  label: ReactNode
  meta?: ReactNode
  surface?: MetricSurface
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
  const className = cn(
    'group min-w-0 p-4 ring-1 ring-inset transition-colors',
    surface === 'neutral' ? metricToneSurfaceClassName('neutral') : metricToneSurfaceClassName(tone),
  )

  return href
    ? <Link className={className} data-slot="metric-item" data-surface={surface} data-tone={tone} to={href}>{content}</Link>
    : <div className={className} data-slot="metric-item" data-surface={surface} data-tone={tone}>{content}</div>
}

function metricToneClassName(tone: MetricTone) {
  if (tone === 'danger')
    return 'text-danger'
  if (tone === 'warning')
    return 'text-warning'
  if (tone === 'success')
    return 'text-success'
  if (tone === 'info')
    return 'text-info'
  return undefined
}

function metricToneSurfaceClassName(tone: MetricTone) {
  if (tone === 'danger')
    return 'bg-danger-subtle/45 ring-danger-border/45 hover:bg-danger-subtle/65'
  if (tone === 'warning')
    return 'bg-warning-subtle/45 ring-warning-border/45 hover:bg-warning-subtle/65'
  if (tone === 'success')
    return 'bg-success-subtle/35 ring-success-border/35 hover:bg-success-subtle/55'
  if (tone === 'info')
    return 'bg-info-subtle/35 ring-info-border/35 hover:bg-info-subtle/55'
  return 'bg-surface-raised ring-transparent hover:bg-surface-subtle'
}
