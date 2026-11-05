import type { ReactNode } from 'react'
import type { StatusTone } from '@/components/common/status-tone'
import { cn } from '@/lib/utils'

/**
 * 运行、健康等连续状态的统一表达：圆点 + 文字 + 可选副文本（如 “2/2”）。
 * 用于列表和头部的运行状态；离散标签（角色、范围、阶段结果）继续使用 StatusBadge。
 * 颜色只做辅助，状态含义始终由 label 文本表达。
 */
export function StatusDot({ className, detail, label, tone }: {
  className?: string
  detail?: ReactNode
  label: ReactNode
  tone: StatusTone
}) {
  return (
    <span className={cn('inline-flex min-w-0 max-w-full items-center gap-1.5 text-sm', className)} data-slot="status-dot" data-tone={tone}>
      <span aria-hidden="true" className={cn('size-2 shrink-0 rounded-full', statusDotToneClassName(tone))} data-slot="status-dot-indicator" />
      <span className="min-w-0 truncate text-foreground">{label}</span>
      {detail !== undefined && detail !== null && detail !== '' && (
        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{detail}</span>
      )}
    </span>
  )
}

function statusDotToneClassName(tone: StatusTone) {
  switch (tone) {
    case 'success':
      return 'bg-success'
    case 'warning':
      return 'bg-warning'
    case 'danger':
      return 'bg-danger'
    case 'info':
      return 'bg-info'
    case 'neutral':
      return 'bg-muted-foreground'
  }
}
