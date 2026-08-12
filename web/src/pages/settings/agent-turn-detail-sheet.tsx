import type { AgentTurnTimelineFilter, AgentTurnTimelineKind } from './agent-turn-timeline'
import type { AgentObservabilityTraceSpan, AgentObservabilityTurn } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { Bot, Braces, ChevronDown, ChevronRight, CircleDot, Clock3, Database, MessageSquareText, Network, TriangleAlert, UserRound, Wrench } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { CopyableCodeBlock } from '@/components/common/ai-assistant/copyable-code-block'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { agentSpanContentSections, agentSpanMessages, formatSpanJSON } from './agent-span-content'
import { agentTurnTimelineKind, filterAgentTurnTimelineSpans } from './agent-turn-timeline'

export function AgentTurnDetailSheet({ turn, onOpenChange }: {
  turn: AgentObservabilityTurn | null
  onOpenChange: (open: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const [filter, setFilter] = useState<AgentTurnTimelineFilter>('all')
  const [hideExternalServices, setHideExternalServices] = useState(false)
  const [expandedSpanIds, setExpandedSpanIds] = useState<string[]>([])
  const detail = useQuery({
    queryKey: ['agent-observability-turn-trace', turn?.id, turn?.traceId],
    queryFn: () => api.getAgentObservabilityTrace(turn!.traceId),
    enabled: Boolean(turn?.traceId),
    retry: 1,
  })
  const spans = useMemo(() => filterAgentTurnTimelineSpans(detail.data?.spans ?? [], filter, hideExternalServices), [detail.data?.spans, filter, hideExternalServices])
  const lastModelSpanId = useMemo(() => [...(detail.data?.spans ?? [])].reverse().find(span => agentTurnTimelineKind(span) === 'model')?.spanId, [detail.data?.spans])
  const toggleSpan = (spanId: string) => setExpandedSpanIds(current => current.includes(spanId) ? current.filter(id => id !== spanId) : [...current, spanId])
  const owner = turn?.user.name || turn?.user.email || '—'

  return (
    <Sheet open={Boolean(turn)} onOpenChange={onOpenChange}>
      <SheetContent className="w-full max-w-none gap-0 p-0 sm:max-w-none lg:w-2/3" side="right">
        <SheetHeader className="border-b border-border px-6 py-4">
          <div className="flex flex-wrap items-center gap-2 pr-10">
            <SheetTitle>{turn ? t('operationsDashboardPage.turnDetail.title', { index: turn.turnIndex + 1 }) : t('operationsDashboardPage.turnDetail.titleFallback')}</SheetTitle>
            {turn && <StatusBadge tone={runStatusTone(turn.status)}>{t(`operationsDashboardPage.runStatus.${turn.status}`, { defaultValue: turn.status })}</StatusBadge>}
          </div>
          <SheetDescription className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span className="min-w-0 truncate font-medium text-foreground">{turn?.conversationTitle || t('operationsDashboardPage.turnDetail.untitledConversation')}</span>
            <span className="inline-flex items-center gap-1">
              <UserRound className="size-3.5" />
              {owner}
            </span>
            {turn && <span>{formatDateTime(turn.createdAt, i18n.language)}</span>}
          </SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-auto p-6">
          {turn && (
            <div className="grid gap-6">
              <MetricGroup className="grid-cols-2 sm:grid-cols-3 xl:grid-cols-6">
                <MetricItem icon={<Clock3 className="size-4" />} label={t('operationsDashboardPage.duration')} value={turn.durationMs > 0 ? formatDuration(turn.durationMs) : '—'} />
                <MetricItem icon={<MessageSquareText className="size-4" />} label={t('operationsDashboardPage.inputTokens')} value={formatNumber(turn.inputTokens)} />
                <MetricItem icon={<Bot className="size-4" />} label={t('operationsDashboardPage.outputTokens')} value={formatNumber(turn.outputTokens)} />
                <MetricItem icon={<Wrench className="size-4" />} label={t('operationsDashboardPage.toolCalls')} value={formatNumber(turn.toolCallCount)} />
                <MetricItem icon={<Network className="size-4" />} label={t('operationsDashboardPage.turnDetail.spans')} value={formatNumber(detail.data?.spanCount)} />
                <MetricItem icon={<TriangleAlert className="size-4" />} label={t('operationsDashboardPage.turnDetail.errorSpans')} value={formatNumber(detail.data?.errorCount)} tone={(detail.data?.errorCount ?? 0) > 0 ? 'danger' : 'success'} />
              </MetricGroup>

              <div className="grid gap-2 rounded-container bg-surface-raised p-4">
                <p className="m-0 text-xs font-medium text-muted-foreground">{t('operationsDashboardPage.userMessage')}</p>
                <p className="m-0 whitespace-pre-wrap break-words text-sm">{turn.userMessage || '—'}</p>
                {turn.assistantMessage && (
                  <>
                    <p className="m-0 mt-2 text-xs font-medium text-muted-foreground">{t('operationsDashboardPage.assistantMessage')}</p>
                    <p className="m-0 line-clamp-4 whitespace-pre-wrap break-words text-sm">{turn.assistantMessage}</p>
                  </>
                )}
              </div>

              <section className="grid gap-4">
                <div className="grid items-start gap-3 lg:grid-cols-[minmax(0,1fr)_auto]">
                  <div className="grid gap-1">
                    <h2 className="text-base font-semibold">{t('operationsDashboardPage.turnDetail.timeline')}</h2>
                    <p className="m-0 text-sm text-muted-foreground">{t('operationsDashboardPage.turnDetail.timelineDescription')}</p>
                  </div>
                  <div className="flex flex-wrap items-center gap-3 lg:justify-self-end">
                    <label className="flex cursor-pointer items-center gap-2 text-sm text-muted-foreground">
                      <Switch aria-label={t('operationsDashboardPage.turnDetail.hideExternalServices')} checked={hideExternalServices} onCheckedChange={setHideExternalServices} />
                      <span>{t('operationsDashboardPage.turnDetail.hideExternalServices')}</span>
                    </label>
                    <div className="flex flex-wrap gap-1 rounded-control bg-muted p-1">
                      {(['all', 'model', 'tool', 'error'] as const).map(value => (
                        <Button key={value} aria-pressed={filter === value} size="sm" variant={filter === value ? 'secondary' : 'ghost'} onClick={() => setFilter(value)}>
                          {t(`operationsDashboardPage.turnDetail.filters.${value}`)}
                        </Button>
                      ))}
                    </div>
                  </div>
                </div>

                {!turn.traceId && (
                  <EmptyState
                    description={t('operationsDashboardPage.turnDetail.tracePendingDescription')}
                    title={t('operationsDashboardPage.turnDetail.tracePending')}
                    variant="plain"
                  />
                )}
                {turn.traceId && detail.isLoading && <ToolViewportSkeleton />}
                {turn.traceId && detail.isError && (
                  <div className="grid gap-3">
                    <ErrorState
                      description={t('operationsDashboardPage.turnDetail.loadFailedDescription')}
                      title={t('operationsDashboardPage.turnDetail.loadFailed')}
                    />
                    <div><Button variant="outline" onClick={() => void detail.refetch()}>{t('common.retry')}</Button></div>
                  </div>
                )}
                {detail.data && spans.length === 0 && (
                  <EmptyState
                    description={t('operationsDashboardPage.turnDetail.noMatchingStepsDescription')}
                    title={t('operationsDashboardPage.turnDetail.noMatchingSteps')}
                    variant="plain"
                  />
                )}
                {detail.data && spans.length > 0 && (
                  <ol className="grid" aria-label={t('operationsDashboardPage.turnDetail.timeline')}>
                    {spans.map((span, index) => (
                      <TimelineStep
                        key={span.spanId}
                        expanded={expandedSpanIds.includes(span.spanId)}
                        isLast={index === spans.length - 1}
                        span={span}
                        assistantMessage={span.spanId === lastModelSpanId ? turn.assistantMessage : undefined}
                        userMessage={span.name === 'agent.turn.accept' ? turn.userMessage : undefined}
                        onToggle={() => toggleSpan(span.spanId)}
                      />
                    ))}
                  </ol>
                )}
              </section>
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function TimelineStep({ span, expanded, isLast, userMessage, assistantMessage, onToggle }: {
  span: AgentObservabilityTraceSpan
  expanded: boolean
  isLast: boolean
  userMessage?: string
  assistantMessage?: string
  onToggle: () => void
}) {
  const { t, i18n } = useTranslation()
  const kind = agentTurnTimelineKind(span)
  const attributes = Object.entries(span.attributes)
  const contentSections = agentSpanContentSections(span)
  return (
    <li className="grid grid-cols-[4.5rem_1.5rem_minmax(0,1fr)] gap-3">
      <time className="grid gap-0.5 pt-4 text-right font-mono text-xs text-muted-foreground">
        <span>{formatSpanTime(span.startTimeUnixNano, i18n.language)}</span>
        <span className="text-[10px]">
          +
          {formatDuration(span.startOffsetMs)}
        </span>
      </time>
      <span className="relative flex justify-center">
        {!isLast && <span className="absolute inset-y-0 top-6 w-px bg-border" />}
        <span className={cn('relative mt-4 grid size-5 place-items-center rounded-full ring-4 ring-background', timelineDotClass(span, kind))}>
          <CircleDot className="size-3" />
        </span>
      </span>
      <div className="mb-4 overflow-hidden rounded-container bg-surface-raised">
        <button className="flex w-full items-center gap-3 p-4 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring" type="button" onClick={onToggle}>
          <TimelineIcon kind={kind} status={span.status} />
          <span className="min-w-0 flex-1">
            <span className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{timelineTitle(span, kind, t)}</span>
              {span.status === 'error' && <StatusBadge tone="danger">{t('operationsDashboardPage.turnDetail.failedStep')}</StatusBadge>}
            </span>
            <span className="mt-1 block truncate font-mono text-xs text-muted-foreground">
              {span.name}
              {' '}
              ·
              {' '}
              {span.serviceName}
            </span>
          </span>
          <span className="shrink-0 font-mono text-xs text-muted-foreground">{formatDuration(span.durationMs)}</span>
          {expanded ? <ChevronDown className="size-4 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-4 shrink-0 text-muted-foreground" />}
        </button>
        {expanded && (
          <div className="grid gap-3 border-t border-border px-4 py-3">
            {userMessage && <TimelineContent label={t('operationsDashboardPage.userMessage')} value={userMessage} />}
            {assistantMessage && <TimelineContent label={t('operationsDashboardPage.assistantMessage')} value={assistantMessage} />}
            {contentSections.map(section => (
              <SpanContentSection key={section.id} kind={section.kind} value={section.value} />
            ))}
            {(kind === 'model' || kind === 'tool') && contentSections.length === 0 && (
              <p className="m-0 rounded-control bg-info-subtle px-3 py-2 text-xs text-info">
                {t('operationsDashboardPage.turnDetail.contentCaptureUnavailable')}
              </p>
            )}
            <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
              <DetailValue label={t('operationsDashboardPage.status')} value={span.status} />
              <DetailValue label={t('operationsDashboardPage.duration')} value={formatDuration(span.durationMs)} />
              <DetailValue label={t('operationsDashboardPage.turnDetail.spanId')} value={span.spanId} mono />
              <DetailValue label={t('operationsDashboardPage.turnDetail.parentSpan')} value={span.parentSpanId || '—'} mono />
            </div>
            {attributes.length > 0 && (
              <div className="grid gap-2">
                <p className="m-0 text-xs font-medium text-muted-foreground">{t('operationsDashboardPage.turnDetail.attributes')}</p>
                <div className="grid gap-2 sm:grid-cols-2">
                  {attributes.map(([key, value]) => (
                    <div key={key} className="grid min-w-0 gap-1 rounded-control bg-muted p-2">
                      <span className="truncate font-mono text-[11px] text-muted-foreground">{key}</span>
                      <span className="break-all text-xs">{value}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
            <details className="group overflow-hidden rounded-control bg-muted">
              <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-xs font-medium outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                <ChevronRight className="size-3.5 text-muted-foreground transition-transform group-open:rotate-90" />
                {t('operationsDashboardPage.turnDetail.rawJson')}
              </summary>
              <div className="border-t border-border p-3">
                <CopyableCodeBlock className="max-h-96" value={formatSpanJSON(span.raw)}><code>{formatSpanJSON(span.raw)}</code></CopyableCodeBlock>
              </div>
            </details>
          </div>
        )}
      </div>
    </li>
  )
}

function SpanContentSection({ kind, value }: { kind: 'modelInput' | 'modelOutput' | 'modelError' | 'toolArguments' | 'toolResult', value: unknown }) {
  const { t } = useTranslation()
  const messages = kind === 'modelInput' ? agentSpanMessages(value) : []
  return (
    <section className="grid min-w-0 gap-2">
      <h3 className="text-xs font-medium text-muted-foreground">{t(`operationsDashboardPage.turnDetail.content.${kind}`)}</h3>
      {messages.length > 0
        ? (
            <div className="grid gap-2">
              {messages.map(message => (
                <div key={message.id} className="grid min-w-0 gap-1 rounded-control bg-muted p-3">
                  <span className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                    {t(`operationsDashboardPage.turnDetail.roles.${message.role}`, { defaultValue: message.role })}
                  </span>
                  <CopyableCodeBlock className="max-h-80" value={displayContent(message.content)}><code>{displayContent(message.content)}</code></CopyableCodeBlock>
                </div>
              ))}
              <details className="group overflow-hidden rounded-control bg-muted">
                <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-xs font-medium outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                  <ChevronRight className="size-3.5 text-muted-foreground transition-transform group-open:rotate-90" />
                  {t('operationsDashboardPage.turnDetail.fullModelInput')}
                </summary>
                <div className="border-t border-border p-3">
                  <CopyableCodeBlock className="max-h-96" value={formatSpanJSON(value)}><code>{formatSpanJSON(value)}</code></CopyableCodeBlock>
                </div>
              </details>
            </div>
          )
        : <CopyableCodeBlock className="max-h-96" value={formatSpanJSON(value)}><code>{formatSpanJSON(value)}</code></CopyableCodeBlock>}
    </section>
  )
}

function displayContent(value: unknown) {
  return typeof value === 'string' ? value : formatSpanJSON(value)
}

function TimelineContent({ label, value }: { label: string, value: string }) {
  return (
    <div className="grid gap-1 rounded-control bg-muted p-3">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <span className="whitespace-pre-wrap break-words text-sm">{value}</span>
    </div>
  )
}

function DetailValue({ label, value, mono }: { label: string, value: string, mono?: boolean }) {
  return (
    <div className="grid min-w-0 gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className={cn('break-all', mono && 'font-mono text-xs')}>{value}</span>
    </div>
  )
}

function TimelineIcon({ kind, status }: { kind: AgentTurnTimelineKind, status: string }) {
  const className = cn('size-4 shrink-0', status === 'error' ? 'text-danger' : 'text-muted-foreground')
  if (kind === 'turn')
    return <MessageSquareText className={className} />
  if (kind === 'agent' || kind === 'model')
    return <Bot className={className} />
  if (kind === 'tool')
    return <Wrench className={className} />
  if (kind === 'storage')
    return <Database className={className} />
  if (kind === 'external')
    return <Network className={className} />
  return <Braces className={className} />
}

function timelineTitle(span: AgentObservabilityTraceSpan, kind: AgentTurnTimelineKind, t: (key: string, options?: Record<string, unknown>) => string) {
  if (kind === 'turn')
    return t('operationsDashboardPage.turnDetail.steps.turnAccepted')
  if (kind === 'agent')
    return t('operationsDashboardPage.turnDetail.steps.agentStarted')
  if (kind === 'model')
    return t('operationsDashboardPage.turnDetail.steps.modelCalled')
  if (kind === 'tool')
    return t('operationsDashboardPage.turnDetail.steps.toolCalled', { name: span.attributes['gen_ai.tool.name'] || span.name })
  if (kind === 'storage')
    return t('operationsDashboardPage.turnDetail.steps.persistence')
  if (kind === 'external')
    return t('operationsDashboardPage.turnDetail.steps.externalRequest')
  return t('operationsDashboardPage.turnDetail.steps.internalOperation')
}

function timelineDotClass(span: AgentObservabilityTraceSpan, kind: AgentTurnTimelineKind) {
  if (span.status === 'error')
    return 'bg-danger-subtle text-danger'
  if (kind === 'model' || kind === 'agent')
    return 'bg-primary-subtle text-primary'
  if (kind === 'tool')
    return 'bg-warning-subtle text-warning'
  return 'bg-info-subtle text-info'
}

function runStatusTone(status: string): 'success' | 'danger' | 'warning' | 'neutral' {
  if (status === 'completed')
    return 'success'
  if (status === 'failed' || status === 'canceled' || status === 'expired')
    return 'danger'
  if (status.startsWith('waiting_'))
    return 'warning'
  return 'neutral'
}

function formatDateTime(value: string, language: string) {
  return new Intl.DateTimeFormat(language, { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value))
}

function formatNumber(value: number | undefined) {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value ?? 0)
}

function formatSpanTime(value: string, language: string) {
  try {
    const milliseconds = Number(BigInt(value) / 1_000_000n)
    return new Intl.DateTimeFormat(language, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(milliseconds))
  }
  catch {
    return '—'
  }
}

function formatDuration(value: number) {
  if (!Number.isFinite(value))
    return '—'
  if (value < 1)
    return `${Math.round(value * 1000)} μs`
  if (value < 1000)
    return `${value.toFixed(value < 10 ? 2 : 1)} ms`
  if (value < 60_000)
    return `${(value / 1000).toFixed(value < 10_000 ? 2 : 1)} s`
  return `${Math.floor(value / 60_000)}m ${Math.round(value % 60_000 / 1000)}s`
}
