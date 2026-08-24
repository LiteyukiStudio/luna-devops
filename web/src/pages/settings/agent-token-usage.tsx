import type { ComponentType } from 'react'
import { ArrowDownToLine, ArrowUpFromLine, BrainCircuit, Database, DatabaseZap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

export interface AgentTokenUsage {
  inputTokens: number
  outputTokens: number
  cacheReadInputTokens?: number | null
  cacheWriteInputTokens?: number | null
  reasoningOutputTokens?: number | null
}

interface TokenUsageField {
  icon: ComponentType<{ className?: string }>
  key: keyof AgentTokenUsage
  labelKey: string
}

const tokenUsageFields: TokenUsageField[] = [
  { key: 'inputTokens', labelKey: 'inputTokens', icon: ArrowDownToLine },
  { key: 'outputTokens', labelKey: 'outputTokens', icon: ArrowUpFromLine },
  { key: 'cacheReadInputTokens', labelKey: 'cacheReadInputTokens', icon: Database },
  { key: 'cacheWriteInputTokens', labelKey: 'cacheWriteInputTokens', icon: DatabaseZap },
  { key: 'reasoningOutputTokens', labelKey: 'reasoningOutputTokens', icon: BrainCircuit },
]

export function AgentTokenUsageStrip({ className, usage }: { className?: string, usage: AgentTokenUsage }) {
  const { t, i18n } = useTranslation()
  return (
    <dl className={cn('grid grid-cols-2 gap-px overflow-hidden rounded-container bg-border sm:grid-cols-5', className)}>
      {tokenUsageFields.map(({ icon: Icon, key, labelKey }) => (
        <div key={key} className="min-w-0 bg-surface-raised px-3 py-2.5">
          <dt className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
            <Icon className="size-3.5 shrink-0" />
            <span className="truncate" title={t(`operationsDashboardPage.${labelKey}`)}>{t(`operationsDashboardPage.${labelKey}`)}</span>
          </dt>
          <dd className="mt-1 font-mono text-base font-semibold tabular-nums">{formatAgentTokenCount(usage[key], i18n.language)}</dd>
        </div>
      ))}
    </dl>
  )
}

export function AgentTokenUsageInline({ className, usage }: { className?: string, usage: AgentTokenUsage }) {
  const { t, i18n } = useTranslation()
  return (
    <dl className={cn('flex min-w-0 flex-wrap gap-x-3 gap-y-1 font-mono text-xs tabular-nums', className)}>
      {tokenUsageFields.map(({ key, labelKey }) => (
        <div key={key} className="flex min-w-0 items-baseline gap-1">
          <dt className="text-muted-foreground">{t(`operationsDashboardPage.${labelKey}`)}</dt>
          <dd>{formatAgentTokenCount(usage[key], i18n.language)}</dd>
        </div>
      ))}
    </dl>
  )
}

function formatAgentTokenCount(value: number | null | undefined, language: string) {
  return typeof value === 'number' && Number.isFinite(value)
    ? new Intl.NumberFormat(language, { maximumFractionDigits: 0 }).format(value)
    : '—'
}
