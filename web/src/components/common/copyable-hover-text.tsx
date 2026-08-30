import type { ReactNode } from 'react'
import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export function CopyableHoverText({
  children,
  className,
  contentClassName,
  display,
  copyLabel,
  side = 'top',
  truncate = true,
  value,
}: {
  children?: ReactNode
  className?: string
  contentClassName?: string
  copyLabel?: string
  display?: ReactNode
  side?: 'top' | 'right' | 'bottom' | 'left'
  truncate?: boolean
  value?: string
}) {
  const { t } = useTranslation()
  const content = value?.trim() ?? ''
  const visible = display ?? children ?? content

  if (!content)
    return <span className={cn('block min-w-0', truncate && 'truncate', className)}>{visible || '-'}</span>

  const copyValue = () => {
    navigator.clipboard.writeText(content)
      .then(() => toast.success(t('common.copied')))
      .catch(error => toast.error(error.message))
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            aria-label={copyLabel ?? t('common.copy')}
            className={cn('block h-auto min-w-0 justify-start rounded-sm p-0 text-left font-normal transition hover:bg-transparent hover:text-primary-text focus-visible:ring-2 focus-visible:ring-ring', truncate ? 'truncate' : 'whitespace-normal', className)}
            type="button"
            variant="ghost"
            onClick={copyValue}
          >
            {visible}
          </Button>
        </TooltipTrigger>
        <TooltipContent className={cn('flex max-w-96 items-start gap-2 break-all leading-5', contentClassName)} side={side}>
          <Button
            aria-label={copyLabel ?? t('common.copy')}
            className="mt-0.5 size-5 shrink-0 rounded-sm p-0 text-background/80 transition hover:bg-background/15 hover:text-background focus-visible:ring-background/50 [&_svg]:size-3.5"
            size="icon"
            type="button"
            variant="ghost"
            onClick={copyValue}
          >
            <Copy className="size-3.5" />
          </Button>
          <span>{content}</span>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
