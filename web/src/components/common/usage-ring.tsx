import type { ReactNode } from 'react'
import type { UsageRingBaseTone } from './usage-ring-utils'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { usageRingToneClass } from './usage-ring-utils'

/**
 * 统一的 SVG 圆环几何渲染：
 * - 小尺寸内联模式（`size={20}`, `strokeWidth={2.5}`, `minVisualPercent={4}`）：用于单 Pod CPU/内存与紧凑压力环；
 * - 详情大圆环模式（`size={76}`, `strokeWidth={6.5}`, `minVisualPercent={2}`）：支持中心文字区（`centerContent`）与 `requestRatio` 保底刻度线。
 */
export function UsageRingGraphic({
  baseTone = 'success',
  centerContent,
  className,
  minVisualPercent = 2,
  percent,
  requestRatio = null,
  size = 20,
  strokeWidth = 2.5,
}: {
  baseTone?: UsageRingBaseTone
  centerContent?: ReactNode
  className?: string
  minVisualPercent?: number
  percent?: number
  requestRatio?: number | null
  size?: number
  strokeWidth?: number
}) {
  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  const hasPercent = percent !== undefined && Number.isFinite(percent) && percent >= 0
  const clamped = hasPercent ? Math.min(100, Math.max(0, percent)) : 0
  // 极低非零负载保留最小可见弧度，避免 0.1% 看起来完全空环。
  const visualPct = hasPercent ? (clamped > 0 && clamped < minVisualPercent ? minVisualPercent : clamped) : 0
  const dashOffset = circumference - (visualPct / 100) * circumference
  const toneClass = usageRingToneClass(percent, baseTone)

  let markerLine: { x1: string, x2: string, y1: string, y2: string } | null = null
  if (requestRatio !== null && requestRatio > 0 && requestRatio < 1) {
    const angle = (requestRatio * 360 - 90) * (Math.PI / 180)
    const center = size / 2
    markerLine = {
      x1: (center + (radius - strokeWidth * 0.55) * Math.cos(angle)).toFixed(1),
      y1: (center + (radius - strokeWidth * 0.55) * Math.sin(angle)).toFixed(1),
      x2: (center + (radius + strokeWidth * 0.55) * Math.cos(angle)).toFixed(1),
      y2: (center + (radius + strokeWidth * 0.55) * Math.sin(angle)).toFixed(1),
    }
  }

  return (
    <span
      className={cn('relative inline-flex shrink-0 items-center justify-center', className)}
      data-slot="usage-ring"
      style={{ height: size, width: size }}
    >
      <svg aria-hidden="true" className="-rotate-90" height={size} viewBox={`0 0 ${size} ${size}`} width={size}>
        <circle
          className="text-border/75"
          cx={size / 2}
          cy={size / 2}
          fill="none"
          r={radius}
          stroke="currentColor"
          strokeWidth={strokeWidth}
        />
        {hasPercent && (
          <circle
            className={cn('transition-[stroke-dashoffset] duration-200 motion-reduce:transition-none', toneClass)}
            cx={size / 2}
            cy={size / 2}
            fill="none"
            r={radius}
            stroke="currentColor"
            strokeDasharray={circumference.toFixed(2)}
            strokeDashoffset={dashOffset.toFixed(2)}
            strokeLinecap="round"
            strokeWidth={strokeWidth}
          />
        )}
      </svg>
      {markerLine && (
        <svg aria-hidden="true" className="pointer-events-none absolute inset-0" height={size} viewBox={`0 0 ${size} ${size}`} width={size}>
          <line
            className="text-muted-foreground/70"
            stroke="currentColor"
            strokeLinecap="round"
            strokeWidth="1.5"
            x1={markerLine.x1}
            x2={markerLine.x2}
            y1={markerLine.y1}
            y2={markerLine.y2}
          />
        </svg>
      )}
      {centerContent && (
        <span className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center leading-tight">
          {centerContent}
        </span>
      )}
    </span>
  )
}

/** 带键盘聚焦与 Tooltip 说明的紧凑指标圆环按钮（用于集群压力与 AI 上下文占用）。 */
export function UsageRing({
  ariaLabel,
  baseTone = 'success',
  ratio,
  tooltip,
}: {
  ariaLabel: string
  baseTone?: UsageRingBaseTone
  ratio: number
  tooltip: ReactNode
}) {
  const percent = Number.isFinite(ratio) ? Math.max(0, ratio * 100) : undefined

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            aria-label={ariaLabel}
            className="size-7 shrink-0 rounded-full p-0 hover:bg-transparent focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-5"
            size="icon"
            type="button"
            variant="ghost"
          >
            <UsageRingGraphic
              baseTone={baseTone}
              minVisualPercent={4}
              percent={percent}
              size={20}
              strokeWidth={2.5}
            />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">{tooltip}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
