import type { AgentObservabilityConversationToolCall, AgentObservabilityTrace, AgentObservabilityTraceSpan } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { Bot, Braces, ChevronDown, ChevronRight, Clock3, Database, Network, TriangleAlert, Wrench } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { ErrorState } from '@/components/common/error-state'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'
import { isAgentSpanContentAttribute } from './agent-span-content'
import { AgentTokenUsageStrip } from './agent-token-usage'
import { AgentTraceContextPanel } from './agent-trace-context-panel'
import { filterAgentTraceDisplaySpans, isAgentModelSpan, isCanonicalAgentToolSpan } from './agent-turn-timeline'

export function AgentTraceDetailSheet({ trace, onOpenChange }: {
  trace: AgentObservabilityTrace | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const detail = useQuery({
    queryKey: ['agent-observability-trace', trace?.traceId],
    queryFn: () => api.getAgentObservabilityTrace(trace!.traceId),
    enabled: Boolean(trace),
  })
  const [selectedSpanId, setSelectedSpanId] = useState<string | null>(null)
  const [collapsedSpanIdsByTrace, setCollapsedSpanIdsByTrace] = useState<Record<string, string[]>>({})
  const traceId = trace?.traceId ?? ''
  const collapsedSpanIds = useMemo(() => new Set(collapsedSpanIdsByTrace[traceId] ?? []), [collapsedSpanIdsByTrace, traceId])
  const displaySpans = useMemo(() => filterAgentTraceDisplaySpans(detail.data?.spans ?? []), [detail.data?.spans])
  const collapsibleSpanIds = useMemo(() => getCollapsibleSpanIds(displaySpans), [displaySpans])
  const spans = useMemo(() => buildSpanRows(displaySpans, collapsedSpanIds), [collapsedSpanIds, displaySpans])
  const selected = displaySpans.find(span => span.spanId === selectedSpanId) ?? displaySpans[0]
  const modelSpans = detail.data?.spans.filter(span => isAgentModelSpan(span)).length ?? 0
  const toolSpans = detail.data?.spans.filter(span => isCanonicalAgentToolSpan(span)).length ?? 0
  const traceDetail = detail.data
  const traceUsage = traceDetail && traceDetail.traceId === trace?.traceId ? traceDetail.usage : null
  const updateCollapsedSpanIds = (values: Set<string>) => {
    if (traceId)
      setCollapsedSpanIdsByTrace(current => ({ ...current, [traceId]: [...values] }))
  }
  const toggleSpan = (spanId: string) => {
    const next = new Set(collapsedSpanIds)
    if (next.has(spanId))
      next.delete(spanId)
    else
      next.add(spanId)
    updateCollapsedSpanIds(next)
  }
  const selectToolSpan = (call: AgentObservabilityConversationToolCall) => {
    const exact = detail.data?.spans.find(span => isCanonicalAgentToolSpan(span) && (span.attributes['gen_ai.tool.call.id'] === call.id || span.attributes['luna.tool_call.id'] === call.id))
    const matchingName = detail.data?.spans.find(span => isCanonicalAgentToolSpan(span) && span.attributes['gen_ai.tool.name'] === call.operationId)
    setSelectedSpanId((exact ?? matchingName)?.spanId ?? null)
  }

  return (
    <Sheet open={Boolean(trace)} onOpenChange={onOpenChange}>
      <SheetContent className="w-[min(96vw,90rem)] max-w-none gap-0 p-0 sm:max-w-none" side="right">
        <SheetHeader className="border-b border-border px-6 py-4">
          <div className="flex flex-wrap items-center gap-2 pr-10">
            <SheetTitle>{t('operationsDashboardPage.traceDetail.title')}</SheetTitle>
            {detail.data && <StatusBadge tone={detail.data.errorCount > 0 ? 'danger' : 'success'}>{detail.data.errorCount > 0 ? t('operationsDashboardPage.traceDetail.withErrors') : t('operationsDashboardPage.traceDetail.succeeded')}</StatusBadge>}
          </div>
          <SheetDescription className="font-mono text-xs">{trace?.traceId}</SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-auto p-6">
          {detail.isLoading && <ToolViewportSkeleton />}
          {detail.isError && <ErrorState title={t('operationsDashboardPage.traceDetail.loadFailed')} description={t('operationsDashboardPage.traceDetail.loadFailedDescription')} />}
          {detail.data && (
            <div className="grid gap-6">
              <AgentTokenUsageStrip usage={traceUsage} />
              <MetricGroup>
                <MetricItem icon={<Clock3 className="size-4" />} label={t('operationsDashboardPage.duration')} value={formatDuration(detail.data.durationMs)} />
                <MetricItem icon={<Network className="size-4" />} label={t('operationsDashboardPage.traceDetail.spans')} value={String(detail.data.spanCount)} />
                <MetricItem icon={<Bot className="size-4" />} label={t('operationsDashboardPage.traceDetail.modelCalls')} value={String(modelSpans)} />
                <MetricItem icon={<Wrench className="size-4" />} label={t('operationsDashboardPage.traceDetail.toolCalls')} value={String(toolSpans)} />
                <MetricItem icon={<TriangleAlert className="size-4" />} label={t('operationsDashboardPage.traceDetail.errors')} value={String(detail.data.errorCount)} tone={detail.data.errorCount ? 'danger' : 'success'} />
              </MetricGroup>
              <div className="grid gap-3">
                <div className="grid gap-1">
                  <h2 className="text-base font-semibold">{t('operationsDashboardPage.traceDetail.executionReplay')}</h2>
                  <p className="m-0 text-sm text-muted-foreground">{t('operationsDashboardPage.traceDetail.executionReplayDescription')}</p>
                </div>
                {detail.data.context
                  ? <AgentTraceContextPanel context={detail.data.context} onSelectToolCall={selectToolSpan} />
                  : <p className="m-0 rounded-container bg-surface-raised p-4 text-sm text-muted-foreground">{t('operationsDashboardPage.traceDetail.contextUnavailable')}</p>}
              </div>
              <div className="grid gap-1">
                <h2 className="text-base font-semibold">{t('operationsDashboardPage.traceDetail.spanWaterfall')}</h2>
                <p className="m-0 text-sm text-muted-foreground">{t('operationsDashboardPage.traceDetail.spanWaterfallDescription')}</p>
              </div>
              <div className="grid min-w-[56rem] gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
                <div className="overflow-hidden rounded-container bg-surface-raised">
                  <div className="flex justify-end gap-2 border-b border-border px-3 py-2">
                    <Button size="sm" variant="ghost" disabled={collapsedSpanIds.size === 0} onClick={() => updateCollapsedSpanIds(new Set())}>
                      {t('operationsDashboardPage.traceDetail.expandAll')}
                    </Button>
                    <Button size="sm" variant="ghost" disabled={collapsibleSpanIds.size === 0 || collapsedSpanIds.size === collapsibleSpanIds.size} onClick={() => updateCollapsedSpanIds(collapsibleSpanIds)}>
                      {t('operationsDashboardPage.traceDetail.collapseAll')}
                    </Button>
                  </div>
                  <div className="grid grid-cols-[minmax(18rem,42%)_1fr_5rem] gap-3 border-b border-border px-4 py-3 text-xs font-medium text-muted-foreground">
                    <span>{t('operationsDashboardPage.traceDetail.operation')}</span>
                    <span>{t('operationsDashboardPage.traceDetail.timeline')}</span>
                    <span className="text-right">{t('operationsDashboardPage.duration')}</span>
                  </div>
                  <div className="divide-y divide-border/70">
                    {spans.map(({ span, depth, hasChildren }) => (
                      <div
                        key={span.spanId}
                        className={cn('grid min-h-12 w-full grid-cols-[minmax(18rem,42%)_1fr_5rem] gap-3 px-4 py-2 text-left', selected?.spanId === span.spanId && 'bg-primary-subtle')}
                      >
                        <span className="flex min-w-0 items-center gap-1" style={{ paddingLeft: `${Math.min(depth, 8) * 14}px` }}>
                          {hasChildren
                            ? (
                                <Button
                                  className="size-7 shrink-0"
                                  size="icon"
                                  variant="ghost"
                                  aria-expanded={!collapsedSpanIds.has(span.spanId)}
                                  aria-label={t(collapsedSpanIds.has(span.spanId) ? 'operationsDashboardPage.traceDetail.expandSpan' : 'operationsDashboardPage.traceDetail.collapseSpan', { name: span.name })}
                                  onClick={() => toggleSpan(span.spanId)}
                                >
                                  {collapsedSpanIds.has(span.spanId) ? <ChevronRight className="size-4" /> : <ChevronDown className="size-4" />}
                                </Button>
                              )
                            : <span className="size-7 shrink-0" />}
                          <button className="flex min-w-0 items-center gap-2 rounded-control text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" type="button" onClick={() => setSelectedSpanId(span.spanId)}>
                            <SpanIcon span={span} />
                            <span className="min-w-0">
                              <span className="block truncate text-sm font-medium">{span.name}</span>
                              <span className="block truncate text-xs text-muted-foreground">{span.serviceName}</span>
                            </span>
                          </button>
                        </span>
                        <span className="relative h-6 overflow-hidden rounded-control bg-muted">
                          <span className={cn('absolute top-1 h-4 min-w-1 rounded-sm', spanBarClass(span))} style={barStyle(span, detail.data.durationMs)} />
                        </span>
                        <span className="self-center text-right font-mono text-xs">{formatDuration(span.durationMs)}</span>
                      </div>
                    ))}
                  </div>
                </div>
                <div className="self-start rounded-container bg-surface-raised p-4 xl:sticky xl:top-0">
                  {selected && <SpanInspector span={selected} />}
                </div>
              </div>
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function SpanInspector({ span }: { span: AgentObservabilityTraceSpan }) {
  const { t } = useTranslation()
  return (
    <div className="grid gap-4">
      <div className="grid gap-1">
        <div className="flex items-center gap-2">
          <SpanIcon span={span} />
          <h3 className="break-all text-sm font-semibold">{span.name}</h3>
        </div>
        <p className="text-xs text-muted-foreground">
          {span.serviceName}
          {' '}
          ·
          {' '}
          {span.kind || 'internal'}
        </p>
      </div>
      <div className="grid grid-cols-2 gap-3 text-sm">
        <DetailValue label={t('operationsDashboardPage.duration')} value={formatDuration(span.durationMs)} />
        <DetailValue label={t('operationsDashboardPage.status')} value={span.status} danger={span.status === 'error'} />
        <DetailValue label={t('operationsDashboardPage.traceDetail.spanId')} value={span.spanId} mono />
        <DetailValue label={t('operationsDashboardPage.traceDetail.parentSpan')} value={span.parentSpanId || '—'} mono />
      </div>
      <div className="grid gap-2">
        <p className="text-xs font-medium text-muted-foreground">{t('operationsDashboardPage.traceDetail.attributes')}</p>
        {!Object.keys(span.attributes).some(key => !isAgentSpanContentAttribute(key))
          ? <p className="text-sm text-muted-foreground">{t('operationsDashboardPage.traceDetail.noAttributes')}</p>
          : Object.entries(span.attributes).filter(([key]) => !isAgentSpanContentAttribute(key)).map(([key, value]) => (
              <div key={key} className="grid gap-1 rounded-control bg-muted p-2">
                <span className="font-mono text-[11px] text-muted-foreground">{key}</span>
                <span className="break-all text-sm">{value}</span>
              </div>
            ))}
      </div>
    </div>
  )
}

function DetailValue({ label, value, mono, danger }: { label: string, value: string, mono?: boolean, danger?: boolean }) {
  return (
    <div className="grid gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className={cn('break-all', mono && 'font-mono text-xs', danger && 'text-danger')}>{value}</span>
    </div>
  )
}

function getCollapsibleSpanIds(spans: AgentObservabilityTraceSpan[]) {
  const ids = new Set(spans.map(span => span.spanId))
  return new Set(spans.filter(span => ids.has(span.parentSpanId)).map(span => span.parentSpanId))
}

function buildSpanRows(spans: AgentObservabilityTraceSpan[], collapsedSpanIds: Set<string>) {
  const children = new Map<string, AgentObservabilityTraceSpan[]>()
  const ids = new Set(spans.map(span => span.spanId))
  for (const span of spans) {
    const parent = ids.has(span.parentSpanId) ? span.parentSpanId : ''
    children.set(parent, [...(children.get(parent) ?? []), span])
  }
  for (const values of children.values()) values.sort((a, b) => a.startOffsetMs - b.startOffsetMs)
  const result: Array<{ span: AgentObservabilityTraceSpan, depth: number, hasChildren: boolean }> = []
  const visit = (span: AgentObservabilityTraceSpan, depth: number) => {
    const nested = children.get(span.spanId) ?? []
    result.push({ span, depth, hasChildren: nested.length > 0 })
    if (collapsedSpanIds.has(span.spanId))
      return
    for (const child of nested) visit(child, depth + 1)
  }
  for (const root of children.get('') ?? []) visit(root, 0)
  return result
}

function SpanIcon({ span }: { span: AgentObservabilityTraceSpan }) {
  const className = cn('size-4 shrink-0', span.status === 'error' ? 'text-danger' : 'text-muted-foreground')
  if (isAgentModelSpan(span))
    return <Bot className={className} />
  if (isToolSpan(span))
    return <Wrench className={className} />
  if (span.name.includes('db.') || span.name.includes('postgres'))
    return <Database className={className} />
  if (span.kind === 'client' || span.kind === 'server')
    return <Network className={className} />
  return <Braces className={className} />
}

function isToolSpan(span: AgentObservabilityTraceSpan) {
  return span.name.includes('tool') || Boolean(span.attributes['gen_ai.tool.name'])
}
function spanBarClass(span: AgentObservabilityTraceSpan) {
  if (span.status === 'error')
    return 'bg-danger'
  if (isAgentModelSpan(span))
    return 'bg-primary'
  if (isToolSpan(span))
    return 'bg-warning'
  return 'bg-info'
}
function barStyle(span: AgentObservabilityTraceSpan, total: number) {
  const safeTotal = Math.max(total, 0.001)
  return { left: `${Math.min(100, span.startOffsetMs / safeTotal * 100)}%`, width: `${Math.max(0.5, Math.min(100, span.durationMs / safeTotal * 100))}%` }
}
function formatDuration(value: number) {
  return value < 1 ? `${Math.round(value * 1000)} μs` : value < 1000 ? `${value.toFixed(value < 10 ? 2 : 1)} ms` : `${(value / 1000).toFixed(2)} s`
}
