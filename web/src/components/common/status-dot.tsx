import type { ReactNode } from 'react'
import type { StatusTone } from '@/components/common/status-tone'
import { cn } from '@/lib/utils'

/**
 * 运行、健康等连续状态的统一表达：圆点 + 文字 + 可选副文本（如 “2/2”）。
 * 用于列表和头部的运行状态；离散标签（角色、范围、阶段结果）继续使用 StatusBadge。
 * 颜色只做辅助，状态含义始终由 label 文本表达。
 */
export function StatusDot({ className, detail, label, pulse = false, tone }: {
  className?: string
  detail?: ReactNode
  label: ReactNode
  /** 进行中的状态（构建、发布、连接中等）显示呼吸光圈；系统开启“减少动态效果”时只保留静态圆点。 */
  pulse?: boolean
  tone: StatusTone
}) {
  const toneClassName = statusDotToneClassName(tone)
  return (
    <span className={cn('inline-flex min-w-0 max-w-full items-center gap-1.5 text-sm', className)} data-pulse={pulse || undefined} data-slot="status-dot" data-tone={tone}>
      <span aria-hidden="true" className={cn('relative size-2 shrink-0 rounded-full', toneClassName)} data-slot="status-dot-indicator">
        {pulse && <span className={cn('absolute inset-0 rounded-full opacity-60 motion-safe:animate-status-pulse motion-reduce:hidden', toneClassName)} />}
      </span>
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
      return 'bg-success-solid'
    case 'warning':
      return 'bg-warning-solid'
    case 'danger':
      return 'bg-danger-solid'
    case 'info':
      return 'bg-info-solid'
    case 'neutral':
      return 'bg-muted-foreground'
  }
}
