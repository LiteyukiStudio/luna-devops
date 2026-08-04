import type { AgentObservabilityTrace, AgentObservabilityTraceSpan } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { Bot, Braces, Clock3, Database, Network, TriangleAlert, Wrench } from 'lucide-react'
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
  const spans = useMemo(() => buildSpanRows(detail.data?.spans ?? []), [detail.data?.spans])
  const selected = detail.data?.spans.find(span => span.spanId === selectedSpanId) ?? detail.data?.spans[0]
  const modelSpans = detail.data?.spans.filter(span => isModelSpan(span)).length ?? 0
  const toolSpans = detail.data?.spans.filter(span => isToolSpan(span)).length ?? 0

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
              <MetricGroup>
                <MetricItem icon={<Clock3 className="size-4" />} label={t('operationsDashboardPage.duration')} value={formatDuration(detail.data.durationMs)} />
                <MetricItem icon={<Network className="size-4" />} label={t('operationsDashboardPage.traceDetail.spans')} value={String(detail.data.spanCount)} />
                <MetricItem icon={<Bot className="size-4" />} label={t('operationsDashboardPage.traceDetail.modelCalls')} value={String(modelSpans)} />
                <MetricItem icon={<Wrench className="size-4" />} label={t('operationsDashboardPage.traceDetail.toolCalls')} value={String(toolSpans)} />
                <MetricItem icon={<TriangleAlert className="size-4" />} label={t('operationsDashboardPage.traceDetail.errors')} value={String(detail.data.errorCount)} tone={detail.data.errorCount ? 'danger' : 'success'} />
              </MetricGroup>
              <div className="grid min-w-[56rem] gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
                <div className="overflow-hidden rounded-container bg-surface-raised">
                  <div className="grid grid-cols-[minmax(18rem,42%)_1fr_5rem] gap-3 border-b border-border px-4 py-3 text-xs font-medium text-muted-foreground">
                    <span>{t('operationsDashboardPage.traceDetail.operation')}</span>
                    <span>{t('operationsDashboardPage.traceDetail.timeline')}</span>
                    <span className="text-right">{t('operationsDashboardPage.duration')}</span>
                  </div>
                  <div className="divide-y divide-border/70">
                    {spans.map(({ span, depth }) => (
                      <Button
                        key={span.spanId}
                        className={cn('grid h-auto w-full grid-cols-[minmax(18rem,42%)_1fr_5rem] gap-3 rounded-none px-4 py-2 text-left font-normal', selected?.spanId === span.spanId && 'bg-primary-subtle')}
                        variant="ghost"
                        onClick={() => setSelectedSpanId(span.spanId)}
                      >
                        <span className="flex min-w-0 items-center gap-2" style={{ paddingLeft: `${Math.min(depth, 8) * 14}px` }}>
                          <SpanIcon span={span} />
                          <span className="min-w-0">
                            <span className="block truncate text-sm font-medium">{span.name}</span>
                            <span className="block truncate text-xs text-muted-foreground">{span.serviceName}</span>
                          </span>
                        </span>
                        <span className="relative h-6 overflow-hidden rounded-control bg-muted">
                          <span className={cn('absolute top-1 h-4 min-w-1 rounded-sm', spanBarClass(span))} style={barStyle(span, detail.data.durationMs)} />
                        </span>
                        <span className="self-center text-right font-mono text-xs">{formatDuration(span.durationMs)}</span>
                      </Button>
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
        {Object.keys(span.attributes).length === 0
          ? <p className="text-sm text-muted-foreground">{t('operationsDashboardPage.traceDetail.noAttributes')}</p>
          : Object.entries(span.attributes).map(([key, value]) => (
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

function buildSpanRows(spans: AgentObservabilityTraceSpan[]) {
  const children = new Map<string, AgentObservabilityTraceSpan[]>()
  const ids = new Set(spans.map(span => span.spanId))
  for (const span of spans) {
    const parent = ids.has(span.parentSpanId) ? span.parentSpanId : ''
    children.set(parent, [...(children.get(parent) ?? []), span])
  }
  for (const values of children.values()) values.sort((a, b) => a.startOffsetMs - b.startOffsetMs)
  const result: Array<{ span: AgentObservabilityTraceSpan, depth: number }> = []
  const visit = (span: AgentObservabilityTraceSpan, depth: number) => {
    result.push({ span, depth })
    for (const child of children.get(span.spanId) ?? []) visit(child, depth + 1)
  }
  for (const root of children.get('') ?? []) visit(root, 0)
  return result
}

function SpanIcon({ span }: { span: AgentObservabilityTraceSpan }) {
  const className = cn('size-4 shrink-0', span.status === 'error' ? 'text-danger' : 'text-muted-foreground')
  if (isModelSpan(span))
    return <Bot className={className} />
  if (isToolSpan(span))
    return <Wrench className={className} />
  if (span.name.includes('db.') || span.name.includes('postgres'))
    return <Database className={className} />
  if (span.kind === 'client' || span.kind === 'server')
    return <Network className={className} />
  return <Braces className={className} />
}

function isModelSpan(span: AgentObservabilityTraceSpan) {
  return span.name.includes('model') || span.name.startsWith('gen_ai.chat')
}
function isToolSpan(span: AgentObservabilityTraceSpan) {
  return span.name.includes('tool') || Boolean(span.attributes['gen_ai.tool.name'])
}
function spanBarClass(span: AgentObservabilityTraceSpan) {
  if (span.status === 'error')
    return 'bg-danger'
  if (isModelSpan(span))
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
