import type { PrepareInteractionCardsInput } from '@luna-devops/ai-interaction-card-contract'
import type { AIToolDisplayResult } from '@/api'
import { LayoutTemplate, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AIMarkdown } from './markdown'

export function AIInteractionCardPlaceholder({ arguments: rawArguments, result }: { arguments: Record<string, unknown>, result?: AIToolDisplayResult }) {
  const { t } = useTranslation()
  const preparation = rawArguments as unknown as PrepareInteractionCardsInput & { generationId?: string }
  const title = typeof preparation.title === 'string' ? preparation.title : t('aiAssistant.cards.preparing')
  const description = typeof preparation.description === 'string' ? preparation.description : undefined
  const repairing = Boolean(result?.attempt && result.attempt > 0)

  return (
    <section aria-label={title} aria-live="polite" className="relative min-h-32 min-w-0 overflow-hidden rounded-container bg-surface" data-ai-card-preparing role="status">
      <div aria-hidden="true" className="ai-card-generation-flow absolute inset-0 motion-reduce:hidden" />
      <div aria-hidden="true" className="absolute inset-0 bg-surface/65 backdrop-blur-[1px]" />
      <div className="relative grid min-h-32 content-between gap-6 p-3">
        <div className="flex min-w-0 items-start gap-2.5">
          <span className="grid size-9 shrink-0 place-items-center rounded-control bg-primary-subtle text-primary-text">
            <LayoutTemplate className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <p className="text-[10px] font-medium uppercase tracking-wide text-primary-text">
              {repairing
                ? t('aiAssistant.cards.repairingEyebrow', { attempt: result?.attempt, max: result?.maxAttempts })
                : t('aiAssistant.cards.preparingEyebrow')}
            </p>
            <AIMarkdown className="mt-0.5 text-[13px] font-semibold leading-5">{title}</AIMarkdown>
            {description && <AIMarkdown className="mt-1 text-[11px] leading-4 text-muted-foreground">{description}</AIMarkdown>}
          </div>
          <Sparkles aria-hidden="true" className="size-4 shrink-0 animate-pulse text-primary-text motion-reduce:animate-none" />
        </div>
        <div aria-hidden="true" className="grid gap-1.5">
          <span className="h-2.5 w-[72%] rounded-full bg-primary-subtle/80" />
          <span className="h-2.5 w-[90%] rounded-full bg-surface-inset/90" />
          <span className="h-2.5 w-[54%] rounded-full bg-primary-subtle/60" />
        </div>
      </div>
    </section>
  )
}
