import type { ComponentType } from 'react'
import { ArrowDownToLine, ArrowUpFromLine, BrainCircuit, CircleHelp, Database, DatabaseZap, Gauge } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export interface AgentTokenUsage {
  inputTokens: number
  outputTokens: number
  cacheReadInputTokens?: number | null
  cacheWriteInputTokens?: number | null
  reasoningOutputTokens?: number | null
  cacheHitRate?: number | null
}

interface TokenUsageField {
  icon: ComponentType<{ className?: string }>
  key: keyof AgentTokenUsage
  labelKey: string
  valueKind?: 'percentage'
}

const summaryUsageFields: TokenUsageField[] = [
  { key: 'inputTokens', labelKey: 'inputTokens', icon: ArrowDownToLine },
  { key: 'outputTokens', labelKey: 'outputTokens', icon: ArrowUpFromLine },
]

const detailUsageFields: TokenUsageField[] = [
  ...summaryUsageFields,
  { key: 'cacheReadInputTokens', labelKey: 'cacheReadInputTokens', icon: Database },
  { key: 'cacheWriteInputTokens', labelKey: 'cacheWriteInputTokens', icon: DatabaseZap },
  { key: 'reasoningOutputTokens', labelKey: 'reasoningOutputTokens', icon: BrainCircuit },
]

const cacheHitRateField: TokenUsageField = { key: 'cacheHitRate', labelKey: 'cacheHitRate', icon: Gauge, valueKind: 'percentage' }

export function AgentTokenUsageStrip({ className, usage }: { className?: string, usage?: AgentTokenUsage | null }) {
  const { t, i18n } = useTranslation()
  return (
    <div className="@container/agent-usage">
      <dl className={cn('grid grid-cols-2 gap-px overflow-hidden rounded-container bg-border sm:grid-cols-3 @[60rem]/agent-usage:grid-cols-6', className)}>
        {[...detailUsageFields, cacheHitRateField].map(({ icon: Icon, key, labelKey, valueKind }) => (
          <div key={key} className="min-w-0 bg-surface-raised px-3 py-2.5">
            <dt className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
              <Icon className="size-3.5 shrink-0" />
              <span className="truncate" title={t(`operationsDashboardPage.${labelKey}`)}>{t(`operationsDashboardPage.${labelKey}`)}</span>
              {valueKind === 'percentage' && (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        aria-label={t('operationsDashboardPage.cacheHitRateDescription')}
                        className="inline-flex shrink-0 rounded-xs outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                        type="button"
                      >
                        <CircleHelp aria-hidden="true" className="size-3.5" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-80 leading-5" side="top">
                      {t('operationsDashboardPage.cacheHitRateDescription')}
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
            </dt>
            <dd className="mt-1 font-mono text-base font-semibold tabular-nums">{formatAgentUsageValue(usage?.[key], i18n.language, valueKind)}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

export function AgentTokenUsageInline({ className, usage, variant = 'detail' }: { className?: string, usage: AgentTokenUsage, variant?: 'summary' | 'detail' }) {
  const { t, i18n } = useTranslation()
  const fields = variant === 'summary' ? summaryUsageFields : detailUsageFields
  return (
    <dl className={cn('flex min-w-0 flex-wrap gap-x-3 gap-y-1 font-mono text-xs tabular-nums', className)}>
      {fields.map(({ key, labelKey }) => (
        <div key={key} className="flex min-w-0 items-baseline gap-1">
          <dt className="text-muted-foreground">{t(`operationsDashboardPage.${labelKey}`)}</dt>
          <dd>{formatAgentUsageValue(usage[key], i18n.language)}</dd>
        </div>
      ))}
    </dl>
  )
}

function formatAgentUsageValue(value: number | null | undefined, language: string, valueKind?: TokenUsageField['valueKind']) {
  if (typeof value !== 'number' || !Number.isFinite(value))
    return '—'
  if (valueKind === 'percentage') {
    if (value < 0 || value > 100)
      return '—'
    const formatter = new Intl.NumberFormat(language, { style: 'percent', maximumFractionDigits: 1 })
    if (value > 0 && value < 0.1)
      return `<${formatter.format(0.001)}`
    if (value > 99.9 && value < 100)
      return `>${formatter.format(0.999)}`
    return formatter.format(value / 100)
  }
  return new Intl.NumberFormat(language, { maximumFractionDigits: 0 }).format(value)
}
