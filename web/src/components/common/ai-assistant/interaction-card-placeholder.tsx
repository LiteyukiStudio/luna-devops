import type { AIToolDisplayResult } from '@/api'
import { LayoutTemplate, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { AIMarkdown } from './markdown'

export function AIInteractionCardPlaceholder({ arguments: rawArguments, result }: { arguments: Record<string, unknown>, result?: AIToolDisplayResult }) {
  const { t } = useTranslation()
  const preparation = rawArguments as {
    title?: unknown
    description?: unknown
  }
  const title = typeof preparation.title === 'string' ? preparation.title : t('aiAssistant.cards.preparing')
  const description = typeof preparation.description === 'string' ? preparation.description : undefined
  const repairing = Boolean(result?.attempt && result.attempt > 0)

  return (
    <section aria-label={title} aria-live="polite" className="min-w-0 overflow-hidden rounded-container border border-border bg-surface shadow-sm" data-ai-card-preparing role="status">
      <div className="grid gap-4 p-3.5">
        <div className="flex min-w-0 items-start gap-3">
          <span className="grid size-9 shrink-0 place-items-center rounded-control bg-primary-subtle text-primary-text">
            <LayoutTemplate className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <LoaderCircle aria-hidden="true" className="size-3 animate-spin motion-reduce:animate-none" />
              {repairing
                ? t('aiAssistant.cards.repairingEyebrow', { attempt: result?.attempt, max: result?.maxAttempts })
                : t('aiAssistant.cards.preparingEyebrow')}
            </p>
            <AIMarkdown className="mt-1 text-sm font-semibold leading-5">{title}</AIMarkdown>
            {description && <AIMarkdown className="mt-1 text-xs leading-5 text-muted-foreground">{description}</AIMarkdown>}
          </div>
        </div>
        <div aria-hidden="true" className="grid gap-2">
          <Skeleton className="h-2.5 w-[72%]" />
          <Skeleton className="h-2.5 w-[90%]" />
          <Skeleton className="h-2.5 w-[54%]" />
        </div>
      </div>
    </section>
  )
}
