import type { ReactNode } from 'react'
import i18next from 'i18next'
import { ChevronDown, CircleHelp } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

interface ProgressiveSectionProps {
  children: ReactNode
  defaultOpen?: boolean
  description?: ReactNode
  hint?: ReactNode
  storageKey?: string
  summary?: ReactNode
  title: ReactNode
}

/**
 * 渐进式表单分组。
 * 用于把高级配置折叠到稳定摘要后面，避免首屏暴露过多字段；不负责表单状态和校验。
 */
export function ProgressiveSection({ children, defaultOpen = false, description, hint, storageKey, summary, title }: ProgressiveSectionProps) {
  const [open, setOpen] = useState(() => {
    if (!storageKey || typeof window === 'undefined')
      return defaultOpen
    const stored = window.localStorage.getItem(storageKey)
    return stored === null ? defaultOpen : stored === 'true'
  })

  const toggleOpen = () => {
    setOpen((value) => {
      const next = !value
      if (storageKey && typeof window !== 'undefined')
        window.localStorage.setItem(storageKey, String(next))
      return next
    })
  }

  return (
    <section className="min-w-0 rounded-lg border border-border bg-card">
      <div className="flex min-w-0 items-start gap-1 rounded-lg px-4 py-3 transition-colors hover:bg-muted/60">
        <Button
          aria-expanded={open}
          className="h-auto min-w-0 flex-1 justify-between gap-3 rounded-none p-0 text-left whitespace-normal hover:bg-transparent"
          type="button"
          variant="ghost"
          onClick={toggleOpen}
        >
          <span className="min-w-0 flex-1 text-left">
            <span className="block text-sm font-semibold break-words text-foreground">{title}</span>
            {summary && <span className="mt-1 block truncate text-xs text-muted-foreground">{summary}</span>}
            {description && open && <span className="mt-1 block text-xs break-words text-muted-foreground">{description}</span>}
          </span>
          <ChevronDown className={cn('size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
        </Button>
        {hint && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                aria-label={typeof title === 'string'
                  ? `${title} ${i18next.t('common.helpSuffix', { defaultValue: 'Help' })}`
                  : i18next.t('common.helpSuffix', { defaultValue: 'Help' })}
                className="size-7 shrink-0 text-muted-foreground hover:text-primary-text"
                size="icon"
                type="button"
                variant="ghost"
              >
                <CircleHelp className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent className="max-w-72 leading-5" side="top">
              {hint}
            </TooltipContent>
          </Tooltip>
        )}
      </div>
      {open && (
        <div className="grid min-w-0 gap-4 border-t border-border px-4 py-4 [&>*]:min-w-0">
          {children}
        </div>
      )}
    </section>
  )
}
