import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export function UsageRing({
  ariaLabel,
  baseTone = 'primary',
  ratio,
  tooltip,
}: {
  ariaLabel: string
  baseTone?: 'info' | 'primary'
  ratio: number
  tooltip: ReactNode
}) {
  const clamped = Math.min(1, Math.max(0, ratio))
  const radius = 7
  const circumference = 2 * Math.PI * radius
  const filled = clamped * circumference
  const danger = ratio >= 1
  const warning = !danger && ratio >= 0.8

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
            <svg className="size-5 -rotate-90" viewBox="0 0 18 18" role="img">
              <circle className="text-separator-subtle" cx="9" cy="9" fill="none" r={radius} stroke="currentColor" strokeWidth="2" />
              <circle
                className={cn(
                  'transition-[stroke-dashoffset] duration-200',
                  danger ? 'text-danger' : warning ? 'text-warning' : baseTone === 'info' ? 'text-info' : 'text-primary',
                )}
                cx="9"
                cy="9"
                fill="none"
                r={radius}
                stroke="currentColor"
                strokeDasharray={circumference}
                strokeDashoffset={circumference - filled}
                strokeLinecap="round"
                strokeWidth="2"
              />
            </svg>
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">{tooltip}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
