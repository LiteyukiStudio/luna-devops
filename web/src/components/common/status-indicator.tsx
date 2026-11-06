import type { ReactNode } from 'react'
import type { StatusIndicatorState } from '@/components/common/status-tone'
import { statusIndicatorTone } from '@/components/common/status-tone'
import { cn } from '@/lib/utils'

/**
 * 运行、任务、连接等状态的统一表达：状态图形 + 文字 + 可选副文本（如 “2/2”）。
 * 离散标签（角色、范围、阶段结果）继续使用 StatusBadge。图形只做辅助，状态含义始终由 label 文本表达。
 */
export function StatusIndicator({ className, detail, label, state }: {
  className?: string
  detail?: ReactNode
  label: ReactNode
  state: StatusIndicatorState
}) {
  return (
    <span className={cn('inline-flex min-w-0 max-w-full items-center gap-1.5 text-sm', className)} data-slot="status-indicator" data-state={state} data-tone={statusIndicatorTone(state)}>
      <StatusGlyph state={state} />
      <span className="min-w-0 truncate text-foreground">{label}</span>
      {detail !== undefined && detail !== null && detail !== '' && (
        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{detail}</span>
      )}
    </span>
  )
}

const glyphToneClassName: Record<StatusIndicatorState, string> = {
  canceled: 'text-muted-foreground',
  failure: 'text-danger-solid',
  info: 'text-info-solid',
  neutral: 'text-muted-foreground',
  queued: 'text-warning-solid',
  running: 'text-warning-solid',
  success: 'text-success-solid',
  warning: 'text-warning-solid',
}

const RING_RADIUS = 6.25
const RING_STROKE = 1.5

/**
 * 只有图形的状态指示（尺寸随字号，默认 1em），始终 aria-hidden；调用方必须在相邻文字或 sr-only 文本中表达状态含义。
 * 所有图形都画在 16×16 视窗内：进行中只旋转视窗内的圆弧，动画不会溢出元素自身，任何容器都不会裁切；
 * 开启“减少动态效果”时停止旋转，保留静态圆弧和中心点。
 */
export function StatusGlyph({ className, state }: { className?: string, state: StatusIndicatorState }) {
  return (
    <svg
      aria-hidden="true"
      className={cn('size-[1em] shrink-0', glyphToneClassName[state], className)}
      data-slot="status-glyph"
      data-state={state}
      fill="none"
      focusable="false"
      viewBox="0 0 16 16"
    >
      <GlyphShape state={state} />
    </svg>
  )
}

function GlyphShape({ state }: { state: StatusIndicatorState }) {
  switch (state) {
    case 'running':
      return (
        <>
          <circle cx="8" cy="8" r={RING_RADIUS} stroke="currentColor" strokeOpacity="0.25" strokeWidth={RING_STROKE} />
          <circle
            className="origin-center [transform-box:fill-box] motion-safe:animate-spin"
            cx="8"
            cy="8"
            data-slot="status-glyph-spinner"
            r={RING_RADIUS}
            stroke="currentColor"
            strokeDasharray="11 40"
            strokeLinecap="round"
            strokeWidth={RING_STROKE}
          />
          <circle cx="8" cy="8" fill="currentColor" r="2.75" />
        </>
      )
    case 'queued':
      return <circle cx="8" cy="8" r={RING_RADIUS} stroke="currentColor" strokeDasharray="2.6 2.31" strokeWidth={RING_STROKE} />
    case 'success':
      return (
        <>
          <circle cx="8" cy="8" fill="currentColor" r="8" />
          <path d="M4.75 8.25 7 10.5l4.25-4.75" stroke="white" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.75" />
        </>
      )
    case 'failure':
      return (
        <>
          <circle cx="8" cy="8" fill="currentColor" r="8" />
          <path d="m5.5 5.5 5 5m0-5-5 5" stroke="white" strokeLinecap="round" strokeWidth="1.75" />
        </>
      )
    case 'warning':
      return (
        <>
          <circle cx="8" cy="8" r={RING_RADIUS} stroke="currentColor" strokeWidth={RING_STROKE} />
          <path d="M8 4.75v3.75" stroke="currentColor" strokeLinecap="round" strokeWidth="1.75" />
          <circle cx="8" cy="11.1" fill="currentColor" r="1" />
        </>
      )
    case 'canceled':
      return (
        <>
          <circle cx="8" cy="8" r={RING_RADIUS} stroke="currentColor" strokeWidth={RING_STROKE} />
          <path d="m3.75 12.25 8.5-8.5" stroke="currentColor" strokeWidth={RING_STROKE} />
        </>
      )
    case 'info':
    case 'neutral':
      return <circle cx="8" cy="8" fill="currentColor" r="3.5" />
  }
}
