import type { AgentObservabilityExecutionInitialContext, AgentObservabilityPayloadBreakdown } from '@/api'
import { ChevronDown, ChevronRight, Compass, FileText, History, Layers, Sparkles, Wrench } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AIMarkdown } from '@/components/common/ai-assistant/markdown'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { formatByteSize, keyedFlowMessages } from './agent-execution-flow'
import { ObservabilityJsonBlock } from './agent-observability-json'

export function AgentPayloadBreakdownBar({ breakdown }: { breakdown: AgentObservabilityPayloadBreakdown }) {
  const { t } = useTranslation()
  const segments = [
    { key: 'system', label: t('operationsDashboardPage.turnDetail.payload.system'), bytes: breakdown.systemBytes, barClass: 'bg-muted-foreground/60', dotClass: 'bg-muted-foreground/70' },
    { key: 'tools', label: t('operationsDashboardPage.turnDetail.payload.toolSchemas'), bytes: breakdown.toolSchemaBytes, barClass: 'bg-info-solid', dotClass: 'bg-info-solid' },
    { key: 'history', label: t('operationsDashboardPage.turnDetail.payload.history'), bytes: breakdown.historyBytes, barClass: 'bg-primary/65', dotClass: 'bg-primary/70' },
    { key: 'user', label: t('operationsDashboardPage.turnDetail.payload.userInput'), bytes: breakdown.userInputBytes, barClass: 'bg-primary', dotClass: 'bg-primary' },
    { key: 'toolResults', label: t('operationsDashboardPage.turnDetail.payload.toolResults'), bytes: breakdown.toolResultBytes, barClass: 'bg-warning-solid', dotClass: 'bg-warning-solid' },
    { key: 'output', label: t('operationsDashboardPage.turnDetail.payload.modelOutput'), bytes: breakdown.modelOutputBytes, barClass: 'bg-success-solid', dotClass: 'bg-success-solid' },
  ].filter(item => item.bytes > 0)

  const totalBytes = segments.reduce((sum, item) => sum + item.bytes, 0)
  if (totalBytes <= 0)
    return null

  return (
    <div className="grid gap-2.5 rounded-container border border-border bg-surface p-3.5 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
        <span className="inline-flex items-center gap-1.5 font-medium text-foreground">
          <Layers className="size-3.5 text-muted-foreground" />
          {t('operationsDashboardPage.turnDetail.payload.title')}
        </span>
        <span className="font-mono text-muted-foreground">
          {t('operationsDashboardPage.turnDetail.payload.total', { size: formatByteSize(totalBytes) })}
        </span>
      </div>
      <div className="flex h-2.5 overflow-hidden rounded-control bg-surface-subtle">
        {segments.map(segment => (
          <span
            key={segment.key}
            className={cn('h-full transition-all first:rounded-l-control last:rounded-r-control', segment.barClass)}
            style={{ width: `${Math.max(1.5, segment.bytes / totalBytes * 100)}%` }}
            title={`${segment.label}: ${formatByteSize(segment.bytes)} (${(segment.bytes / totalBytes * 100).toFixed(1)}%)`}
          />
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs">
        {segments.map(segment => (
          <span key={segment.key} className="inline-flex items-center gap-1.5 text-muted-foreground">
            <span className={cn('size-2 rounded-sm', segment.dotClass)} />
            <span>{segment.label}</span>
            <span className="font-mono text-foreground">
              {(segment.bytes / totalBytes * 100).toFixed(1)}
              %
            </span>
            <span className="font-mono opacity-75">
              (
              {formatByteSize(segment.bytes)}
              )
            </span>
          </span>
        ))}
      </div>
    </div>
  )
}

type InitialContextSection = 'history' | 'page' | 'system' | 'tools'

export function AgentInitialContextBanner({ context }: { context: AgentObservabilityExecutionInitialContext }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const hasHistory = context.historyMessages.length > 0 || Boolean(context.compactedSummary)
  const hasPage = Boolean(context.pageContext && Object.keys(context.pageContext).length > 0)
  const hasSystem = Boolean(context.systemInstructions)
  const hasTools = context.initialTools.length > 0

  const defaultTab: InitialContextSection = hasHistory ? 'history' : hasPage ? 'page' : hasSystem ? 'system' : 'tools'
  const [section, setSection] = useState<InitialContextSection>(defaultTab)

  if (!hasHistory && !hasPage && !hasSystem && !hasTools)
    return null

  const pageKeys = context.pageContext ? Object.keys(context.pageContext).slice(0, 3) : []

  return (
    <div className="overflow-hidden rounded-container border border-border bg-surface shadow-sm">
      <button
        aria-expanded={expanded}
        className="flex w-full flex-wrap items-center gap-2.5 px-4 py-3 text-left transition-colors hover:bg-surface-subtle/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
        type="button"
        onClick={() => setExpanded(prev => !prev)}
      >
        {expanded ? <ChevronDown className="size-4 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-4 shrink-0 text-muted-foreground" />}
        <span className="text-sm font-medium">{t('operationsDashboardPage.turnDetail.initialContext.title')}</span>
        <StatusBadge tone="neutral">{t('operationsDashboardPage.turnDetail.initialContext.onceBadge')}</StatusBadge>
        <span className="flex flex-1 flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-1 rounded-control bg-surface-subtle px-2 py-0.5">
            <History className="size-3" />
            {t('operationsDashboardPage.turnDetail.initialContext.historyTurns', { count: context.historyTurnCount })}
          </span>
          {(context.compressionOutcome === 'compressed' || context.compressionOutcome === 'reused' || Boolean(context.compactedSummary)) && (
            <StatusBadge tone="info">
              {context.compressionOutcome === 'compressed'
                ? t('operationsDashboardPage.turnDetail.initialContext.compressedNow')
                : t('operationsDashboardPage.turnDetail.initialContext.reusedSummary')}
            </StatusBadge>
          )}
          {pageKeys.length > 0 && (
            <span className="inline-flex items-center gap-1 rounded-control bg-surface-subtle px-2 py-0.5 font-mono">
              <Compass className="size-3" />
              {pageKeys.join(', ')}
            </span>
          )}
          {hasTools && (
            <span className="inline-flex items-center gap-1 rounded-control bg-surface-subtle px-2 py-0.5">
              <Wrench className="size-3" />
              {t('operationsDashboardPage.turnDetail.initialContext.initialToolCount', { count: context.initialTools.length })}
            </span>
          )}
          {hasSystem && (
            <span className="inline-flex items-center gap-1 rounded-control bg-surface-subtle px-2 py-0.5">
              <FileText className="size-3" />
              {t('operationsDashboardPage.turnDetail.initialContext.systemReady')}
            </span>
          )}
        </span>
      </button>

      {expanded && (
        <div className="grid gap-3 border-t border-border bg-surface-subtle/40 p-4">
          <div className="flex flex-wrap gap-1 rounded-control bg-surface-subtle p-1 w-fit">
            {hasHistory && (
              <Button size="sm" variant={section === 'history' ? 'secondary' : 'ghost'} onClick={() => setSection('history')}>
                <History className="size-3.5" />
                {t('operationsDashboardPage.turnDetail.initialContext.tabs.history')}
              </Button>
            )}
            {hasPage && (
              <Button size="sm" variant={section === 'page' ? 'secondary' : 'ghost'} onClick={() => setSection('page')}>
                <Compass className="size-3.5" />
                {t('operationsDashboardPage.turnDetail.initialContext.tabs.pageContext')}
              </Button>
            )}
            {hasSystem && (
              <Button size="sm" variant={section === 'system' ? 'secondary' : 'ghost'} onClick={() => setSection('system')}>
                <Sparkles className="size-3.5" />
                {t('operationsDashboardPage.turnDetail.initialContext.tabs.systemPrompt')}
              </Button>
            )}
            {hasTools && (
              <Button size="sm" variant={section === 'tools' ? 'secondary' : 'ghost'} onClick={() => setSection('tools')}>
                <Wrench className="size-3.5" />
                {t('operationsDashboardPage.turnDetail.initialContext.tabs.tools')}
              </Button>
            )}
          </div>

          {section === 'history' && (
            <div className="grid gap-2.5 max-h-80 overflow-y-auto pr-1">
              {context.compactedSummary && (
                <div className="grid gap-1.5 rounded-control border border-border bg-surface p-3">
                  <span className="text-xs font-semibold text-muted-foreground">{t('operationsDashboardPage.turnDetail.initialContext.compactedSummaryTitle')}</span>
                  <ObservabilityJsonBlock className="max-h-48" value={parseMaybeJson(context.compactedSummary)} />
                </div>
              )}
              {keyedFlowMessages(context.historyMessages).map(({ key, message: msg }) => (
                <div key={key} className="grid gap-1 rounded-control border border-border bg-surface p-3">
                  <span className="text-xs font-semibold text-muted-foreground">
                    {msg.role === 'user' ? t('operationsDashboardPage.turnDetail.roles.user') : t('operationsDashboardPage.turnDetail.roles.assistant')}
                    {msg.turnIndex !== undefined && ` · ${t('operationsDashboardPage.turnNumber', { index: msg.turnIndex + 1 })}`}
                  </span>
                  <AIMarkdown className="text-sm leading-6">{msg.text || '—'}</AIMarkdown>
                </div>
              ))}
            </div>
          )}

          {section === 'page' && context.pageContext && (
            <ObservabilityJsonBlock className="max-h-72" value={context.pageContext} />
          )}

          {section === 'system' && context.systemInstructions && (
            <div className="rounded-control border border-border bg-surface p-3 max-h-80 overflow-y-auto">
              <AIMarkdown className="text-sm leading-6">{context.systemInstructions}</AIMarkdown>
            </div>
          )}

          {section === 'tools' && (
            <div className="flex flex-wrap gap-1.5 rounded-control border border-border bg-surface p-3">
              {context.initialTools.map(toolName => (
                <span key={toolName} className="inline-flex items-center gap-1.5 rounded-control border border-border bg-surface-subtle px-2.5 py-1 text-xs">
                  <span className="font-medium text-foreground">{toolDisplayName(t, toolName)}</span>
                  <code className="font-mono text-muted-foreground">{toolName}</code>
                </span>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function parseMaybeJson(raw: string): unknown {
  try {
    return JSON.parse(raw)
  }
  catch {
    return raw
  }
}
