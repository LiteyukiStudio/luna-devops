import type { AgentFlowFilter, AgentTurnExplorerView } from './agent-execution-flow'
import type {
  AgentObservabilityFlowIOPreview,
  AgentObservabilityFlowNode,
  AgentObservabilityTraceDetail,
  AgentObservabilityTraceSpan,
  AgentObservabilityTurn,
} from '@/api'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowRight,
  Bot,
  Braces,
  ChevronDown,
  ChevronRight,
  Clock3,
  Copy,
  Database,
  Download,
  GitBranch,
  Maximize2,
  Minimize2,
  Network,
  Search,
  Sparkles,
  TriangleAlert,
  UserRound,
  Wrench,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { AgentTokenUsageStrip } from '@/pages/settings/operations/agent-token-usage'
import {
  buildSpanWaterfallRows,
  filterExecutionFlowNodes,
  formatByteSize,
  getCollapsibleWaterfallSpanIds,
  resolveExecutionFlow,
} from './agent-execution-flow'
import { AgentNodeInspector } from './agent-node-inspector'
import { agentTurnDiagnosticFilename, formatAgentTurnDiagnosticExport } from './agent-turn-diagnostic-export'
import { AgentInitialContextBanner, AgentPayloadBreakdownBar } from './agent-turn-overview-banners'
import { isAgentModelSpan } from './agent-turn-timeline'

export interface AgentTraceFocusTarget {
  traceId: string
  focusToolCallId?: string
  focusToolName?: string
}

export function AgentTurnDetailSheet({
  turn,
  traceTarget,
  onOpenChange,
}: {
  turn?: AgentObservabilityTurn | null
  traceTarget?: AgentTraceFocusTarget | null
  onOpenChange: (open: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const [viewMode, setViewMode] = useState<AgentTurnExplorerView>('flow')
  const [filter, setFilter] = useState<AgentFlowFilter>('all')
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedSpanId, setSelectedSpanId] = useState<string | null>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [collapsedSpanIdsByTrace, setCollapsedSpanIdsByTrace] = useState<Record<string, string[]>>({})

  const open = Boolean(turn || traceTarget)
  const traceId = turn?.traceId || traceTarget?.traceId || ''

  const detail = useQuery({
    queryKey: ['agent-observability-turn-trace', turn?.id ?? 'trace', traceId],
    queryFn: () => api.getAgentObservabilityTrace(traceId),
    enabled: Boolean(traceId),
    retry: 1,
  })

  const effectiveTurn = useMemo<AgentObservabilityTurn | null>(() => {
    if (turn)
      return turn
    const ctx = detail.data?.context
    if (!ctx)
      return null
    const toolCount = ctx.turn.loops.reduce(
      (sum, loop) => sum + loop.items.filter(item => item.type === 'tool_call').length,
      0,
    )
    return {
      id: ctx.turn.id,
      conversationId: ctx.conversation.id,
      conversationTitle: ctx.conversation.title,
      user: ctx.conversation.user,
      turnIndex: ctx.turn.turnIndex,
      status: ctx.turn.status,
      userMessage: ctx.turn.userMessage,
      assistantMessage: ctx.turn.assistantMessage,
      runId: ctx.turn.runId,
      traceId: ctx.turn.traceId || traceId,
      inputTokens: ctx.turn.inputTokens,
      outputTokens: ctx.turn.outputTokens,
      cacheReadInputTokens: ctx.turn.cacheReadInputTokens,
      cacheWriteInputTokens: ctx.turn.cacheWriteInputTokens,
      reasoningOutputTokens: ctx.turn.reasoningOutputTokens,
      toolCallCount: toolCount,
      durationMs: ctx.turn.durationMs,
      createdAt: ctx.turn.createdAt,
    }
  }, [detail.data?.context, traceId, turn])

  const flow = useMemo(
    () => resolveExecutionFlow(detail.data, effectiveTurn),
    [detail.data, effectiveTurn],
  )
  const filteredNodes = useMemo(
    () => filterExecutionFlowNodes(flow.nodes, filter, searchQuery),
    [filter, flow.nodes, searchQuery],
  )

  const collapsedSpanIds = useMemo(
    () => new Set(collapsedSpanIdsByTrace[traceId] ?? []),
    [collapsedSpanIdsByTrace, traceId],
  )
  const collapsibleSpanIds = useMemo(
    () => getCollapsibleWaterfallSpanIds(detail.data?.spans ?? []),
    [detail.data?.spans],
  )
  const waterfallRows = useMemo(
    () => buildSpanWaterfallRows(detail.data?.spans ?? [], collapsedSpanIds),
    [collapsedSpanIds, detail.data?.spans],
  )

  const spanById = useMemo(
    () => new Map((detail.data?.spans ?? []).map(span => [span.spanId, span])),
    [detail.data?.spans],
  )
  const flowNodeById = useMemo(
    () => new Map(flow.nodes.map(node => [node.spanId, node])),
    [flow.nodes],
  )

  const defaultSelectedSpanId = useMemo(() => {
    if (!detail.data?.spans.length)
      return null
    if (traceTarget?.focusToolCallId) {
      const exact = flow.nodes.find(node => node.kind === 'tool' && node.toolCallId === traceTarget.focusToolCallId)
      if (exact)
        return exact.spanId
    }
    if (traceTarget?.focusToolName) {
      const byName = flow.nodes.find(node => node.kind === 'tool' && node.name === traceTarget.focusToolName)
      if (byName)
        return byName.spanId
    }
    if (viewMode === 'flow') {
      const firstLLM = filteredNodes.find(node => node.kind === 'llm')
      return (firstLLM ?? filteredNodes[0])?.spanId ?? detail.data.spans[0]?.spanId ?? null
    }
    return waterfallRows[0]?.span.spanId ?? detail.data.spans[0]?.spanId ?? null
  }, [detail.data?.spans, filteredNodes, flow.nodes, traceTarget?.focusToolCallId, traceTarget?.focusToolName, viewMode, waterfallRows])

  const activeSpanId = selectedSpanId && spanById.has(selectedSpanId) ? selectedSpanId : defaultSelectedSpanId
  const activeSpan = activeSpanId ? spanById.get(activeSpanId) : undefined
  const activeFlowNode = activeSpanId ? flowNodeById.get(activeSpanId) : undefined

  const llmCount = useMemo(() => flow.nodes.filter(node => node.kind === 'llm').length, [flow.nodes])
  const toolNodeCount = useMemo(() => flow.nodes.filter(node => node.kind === 'tool').length, [flow.nodes])
  const errorNodeCount = useMemo(() => flow.nodes.filter(node => node.status === 'error').length, [flow.nodes])

  const traceDetail: AgentObservabilityTraceDetail | undefined = detail.data
  const traceUsage = traceDetail && traceDetail.traceId === traceId ? traceDetail.usage : null
  const detailPending = Boolean(traceId) && detail.isPending
  const owner = effectiveTurn?.user.name || effectiveTurn?.user.email || '—'
  const canExportDiagnostic = Boolean(effectiveTurn && detail.data)
  const exportUnavailableLabel = t('operationsDashboardPage.turnDetail.exportUnavailable')
  const totalTraceDuration = Math.max(detail.data?.durationMs ?? effectiveTurn?.durationMs ?? 0, 0.001)

  const updateCollapsedSpanIds = (values: Set<string>) => {
    if (traceId)
      setCollapsedSpanIdsByTrace(current => ({ ...current, [traceId]: [...values] }))
  }
  const toggleWaterfallSpan = (spanId: string) => {
    const next = new Set(collapsedSpanIds)
    if (next.has(spanId))
      next.delete(spanId)
    else
      next.add(spanId)
    updateCollapsedSpanIds(next)
  }

  const copyTraceId = async () => {
    if (!traceId)
      return
    try {
      await navigator.clipboard.writeText(traceId)
      toast.success(t('common.copied'))
    }
    catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const copyDiagnosticJSON = async () => {
    if (!effectiveTurn || !detail.data)
      return
    try {
      await navigator.clipboard.writeText(formatAgentTurnDiagnosticExport(effectiveTurn, detail.data))
      toast.success(t('operationsDashboardPage.turnDetail.copyDiagnosticSuccess'))
    }
    catch {
      toast.error(t('operationsDashboardPage.turnDetail.copyDiagnosticFailed'))
    }
  }

  const downloadDiagnosticJSON = () => {
    if (!effectiveTurn || !detail.data)
      return
    try {
      const exportedAt = new Date()
      const diagnosticJSON = formatAgentTurnDiagnosticExport(effectiveTurn, detail.data, exportedAt.toISOString())
      const url = URL.createObjectURL(new Blob([diagnosticJSON], { type: 'application/json;charset=utf-8' }))
      const link = document.createElement('a')
      link.href = url
      link.download = agentTurnDiagnosticFilename(effectiveTurn, exportedAt)
      link.hidden = true
      document.body.append(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
      toast.success(t('operationsDashboardPage.turnDetail.downloadDiagnosticSuccess'))
    }
    catch {
      toast.error(t('operationsDashboardPage.turnDetail.downloadDiagnosticFailed'))
    }
  }

  const filterCounts: Record<AgentFlowFilter, number> = {
    all: flow.nodes.length,
    agent: flow.nodes.filter(node => node.kind === 'agent').length,
    llm: llmCount,
    tool: toolNodeCount,
    error: errorNodeCount,
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        className={cn(
          'max-w-none gap-0 p-0 sm:max-w-none transition-[width]',
          isFullscreen ? 'w-dvw' : 'w-[min(96vw,92rem)]',
        )}
        side="right"
      >
        <SheetHeader className="border-b border-border px-6 py-4">
          <div className="flex flex-wrap items-center gap-2 pr-10">
            <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
              <SheetTitle>
                {effectiveTurn
                  ? t('operationsDashboardPage.turnDetail.title', { index: effectiveTurn.turnIndex + 1 })
                  : t('operationsDashboardPage.traceDetail.title')}
              </SheetTitle>
              {effectiveTurn
                ? <StatusBadge tone={runStatusTone(effectiveTurn.status)}>{runStatusLabel(effectiveTurn.status, t)}</StatusBadge>
                : detail.data && (
                  <StatusBadge tone={detail.data.errorCount > 0 ? 'danger' : 'success'}>
                    {detail.data.errorCount > 0 ? t('operationsDashboardPage.traceDetail.withErrors') : t('operationsDashboardPage.traceDetail.succeeded')}
                  </StatusBadge>
                )}
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex gap-1 rounded-control bg-surface-subtle p-1">
                <Button size="sm" variant={viewMode === 'flow' ? 'secondary' : 'ghost'} onClick={() => setViewMode('flow')}>
                  <GitBranch className="size-3.5" />
                  <span>{t('operationsDashboardPage.turnDetail.views.flow')}</span>
                </Button>
                <Button size="sm" variant={viewMode === 'waterfall' ? 'secondary' : 'ghost'} onClick={() => setViewMode('waterfall')}>
                  <Network className="size-3.5" />
                  <span>{t('operationsDashboardPage.turnDetail.views.waterfall')}</span>
                </Button>
              </div>
              <Button
                aria-label={t('operationsDashboardPage.turnDetail.copyDiagnostic')}
                disabled={!canExportDiagnostic}
                size="sm"
                title={canExportDiagnostic ? t('operationsDashboardPage.turnDetail.copyDiagnosticDescription') : exportUnavailableLabel}
                variant="outline"
                onClick={() => void copyDiagnosticJSON()}
              >
                <Copy className="size-4" />
                <span className="hidden sm:inline">{t('operationsDashboardPage.turnDetail.copyDiagnostic')}</span>
              </Button>
              <Button
                aria-label={t('operationsDashboardPage.turnDetail.downloadDiagnostic')}
                disabled={!canExportDiagnostic}
                size="sm"
                title={canExportDiagnostic ? t('operationsDashboardPage.turnDetail.downloadDiagnosticDescription') : exportUnavailableLabel}
                variant="outline"
                onClick={downloadDiagnosticJSON}
              >
                <Download className="size-4" />
                <span className="hidden sm:inline">{t('operationsDashboardPage.turnDetail.downloadDiagnostic')}</span>
              </Button>
              <Button
                aria-label={t(isFullscreen ? 'operationsDashboardPage.turnDetail.exitFullscreen' : 'operationsDashboardPage.turnDetail.enterFullscreen')}
                className="hidden lg:inline-flex"
                size="icon"
                title={t(isFullscreen ? 'operationsDashboardPage.turnDetail.exitFullscreen' : 'operationsDashboardPage.turnDetail.enterFullscreen')}
                variant="ghost"
                onClick={() => setIsFullscreen(prev => !prev)}
              >
                {isFullscreen ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />}
              </Button>
            </div>
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <SheetDescription className="flex flex-wrap items-center gap-x-3 gap-y-1">
              {effectiveTurn
                ? (
                    <>
                      <span className="min-w-0 truncate font-medium text-foreground">
                        {effectiveTurn.conversationTitle || t('operationsDashboardPage.turnDetail.untitledConversation')}
                      </span>
                      <span className="inline-flex items-center gap-1">
                        <UserRound className="size-3.5" />
                        {owner}
                      </span>
                      <span>{formatDateTime(effectiveTurn.createdAt, i18n.language)}</span>
                    </>
                  )
                : <span>{t('operationsDashboardPage.traceDetail.spanWaterfallDescription')}</span>}
            </SheetDescription>

            {traceId && (
              <button
                aria-label={`${t('common.copy')} ${t('operationsDashboardPage.traceId')}`}
                className="group inline-flex max-w-full min-w-0 items-center gap-2 rounded-control bg-primary-subtle px-2.5 py-1.5 text-left text-primary-text transition-colors hover:bg-primary-subtle-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                title={`${t('common.copy')} ${t('operationsDashboardPage.traceId')}`}
                type="button"
                onClick={() => void copyTraceId()}
              >
                <Network className="size-3.5 shrink-0" />
                <span className="text-xs font-medium opacity-75">{t('operationsDashboardPage.traceId')}</span>
                <code className="truncate text-xs font-semibold">{traceId}</code>
                <Copy className="size-3.5 shrink-0 opacity-70 transition-opacity group-hover:opacity-100" />
              </button>
            )}
          </div>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-auto p-6">
          {open && (
            <div className="grid gap-4">
              {!(Boolean(traceId) && detail.isError && !effectiveTurn) && (
                <div
                  aria-busy={detailPending || undefined}
                  aria-label={detailPending ? t('common.loading') : undefined}
                  className="grid gap-3"
                  role={detailPending ? 'status' : undefined}
                >
                  {!detail.isError && <AgentTokenUsageStrip loading={detailPending} usage={traceUsage} />}
                  <MetricGroup className="grid-cols-2 @2xl/metric-group:grid-cols-5">
                    <MetricItem
                      icon={<Clock3 className="size-4" />}
                      label={t('operationsDashboardPage.duration')}
                      value={detailPending && !effectiveTurn?.durationMs
                        ? <Skeleton className="h-7 w-16" />
                        : (detail.data?.durationMs ?? effectiveTurn?.durationMs ?? 0) > 0
                            ? formatDuration(detail.data?.durationMs ?? effectiveTurn?.durationMs ?? 0)
                            : '—'}
                    />
                    <MetricItem
                      icon={<Bot className="size-4" />}
                      label={t('operationsDashboardPage.traceDetail.modelCalls')}
                      value={detailPending ? <Skeleton className="h-7 w-12" /> : detail.data ? formatNumber(llmCount) : '—'}
                    />
                    <MetricItem
                      icon={<Wrench className="size-4" />}
                      label={t('operationsDashboardPage.toolCalls')}
                      value={detailPending && effectiveTurn?.toolCallCount === undefined
                        ? <Skeleton className="h-7 w-12" />
                        : formatNumber(detail.data ? toolNodeCount : effectiveTurn?.toolCallCount)}
                    />
                    <MetricItem
                      icon={<Network className="size-4" />}
                      label={t('operationsDashboardPage.turnDetail.spans')}
                      value={detailPending ? <Skeleton className="h-7 w-12" /> : formatNumber(detail.data?.spanCount)}
                    />
                    <MetricItem
                      icon={<TriangleAlert className="size-4" />}
                      label={t('operationsDashboardPage.turnDetail.errorSpans')}
                      tone={!detail.data ? 'neutral' : detail.data.errorCount > 0 ? 'danger' : 'success'}
                      value={detailPending ? <Skeleton className="h-7 w-12" /> : formatNumber(detail.data?.errorCount)}
                    />
                  </MetricGroup>
                  {flow.payloadBreakdown && <AgentPayloadBreakdownBar breakdown={flow.payloadBreakdown} />}
                  {flow.initialContext && <AgentInitialContextBanner context={flow.initialContext} />}
                </div>
              )}

              {!traceId && (
                <EmptyState
                  description={t('operationsDashboardPage.turnDetail.tracePendingDescription')}
                  title={t('operationsDashboardPage.turnDetail.tracePending')}
                  variant="plain"
                />
              )}

              {traceId && detail.isError && (
                <ErrorState
                  description={t('operationsDashboardPage.turnDetail.loadFailedDescription')}
                  retrying={detail.isFetching}
                  title={t('operationsDashboardPage.turnDetail.loadFailed')}
                  onRetry={() => void detail.refetch()}
                />
              )}

              {detail.data && viewMode === 'flow' && (
                <section className="grid gap-3" aria-label={t('operationsDashboardPage.turnDetail.timeline')}>
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div className="flex flex-wrap gap-1 rounded-control bg-surface-subtle p-1">
                      {(['all', 'agent', 'llm', 'tool', 'error'] as const).map(value => (
                        <Button
                          key={value}
                          aria-pressed={filter === value}
                          size="sm"
                          variant={filter === value ? 'secondary' : 'ghost'}
                          onClick={() => setFilter(value)}
                        >
                          <span>{flowFilterLabel(value, t)}</span>
                          <span className="ml-1 rounded-sm bg-surface/80 px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
                            {filterCounts[value]}
                          </span>
                        </Button>
                      ))}
                    </div>
                    <div className="relative w-full sm:w-72">
                      <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                      <Input
                        aria-label={t('operationsDashboardPage.turnDetail.flowTable.searchPlaceholder')}
                        className="h-8 pl-8 text-xs"
                        placeholder={t('operationsDashboardPage.turnDetail.flowTable.searchPlaceholder')}
                        value={searchQuery}
                        onChange={event => setSearchQuery(event.target.value)}
                      />
                    </div>
                  </div>

                  <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_27rem]">
                    <div className="overflow-hidden rounded-container border border-border bg-surface shadow-sm">
                      <div className="hidden border-b border-border bg-surface-subtle/60 px-4 py-2.5 text-xs font-medium text-muted-foreground md:grid md:grid-cols-[5.5rem_minmax(13rem,28%)_minmax(0,1fr)_7.5rem] md:gap-3">
                        <span>{t('operationsDashboardPage.time')}</span>
                        <span>{t('operationsDashboardPage.turnDetail.flowTable.nodeHeader')}</span>
                        <span>{t('operationsDashboardPage.turnDetail.flowTable.deltaIOHeader')}</span>
                        <span className="text-right">{t('operationsDashboardPage.turnDetail.flowTable.costHeader')}</span>
                      </div>

                      {filteredNodes.length === 0
                        ? (
                            <EmptyState
                              description={t('operationsDashboardPage.turnDetail.noMatchingStepsDescription')}
                              title={t('operationsDashboardPage.turnDetail.noMatchingSteps')}
                              variant="plain"
                            />
                          )
                        : (
                            <div className="divide-y divide-border/70">
                              {filteredNodes.map(node => (
                                <ExecutionFlowRow
                                  key={node.spanId}
                                  language={i18n.language}
                                  node={node}
                                  selected={activeSpanId === node.spanId}
                                  totalDurationMs={totalTraceDuration}
                                  onSelect={() => setSelectedSpanId(node.spanId)}
                                />
                              ))}
                            </div>
                          )}
                    </div>

                    {activeSpan && (
                      <aside className="rounded-container border border-border bg-surface p-4 shadow-sm xl:sticky xl:top-0">
                        <AgentNodeInspector
                          key={activeSpan.spanId}
                          contentCaptured={flow.contentCaptured}
                          flowNode={activeFlowNode}
                          span={activeSpan}
                        />
                      </aside>
                    )}
                  </div>
                </section>
              )}

              {detail.data && viewMode === 'waterfall' && (
                <section className="grid gap-3" aria-label={t('operationsDashboardPage.traceDetail.spanWaterfall')}>
                  <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_27rem]">
                    <div className="overflow-hidden rounded-container border border-border bg-surface shadow-sm">
                      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-surface-subtle/50 px-4 py-2">
                        <span className="text-xs text-muted-foreground">{t('operationsDashboardPage.traceDetail.spanWaterfallDescription')}</span>
                        <div className="flex gap-1">
                          <Button size="sm" variant="ghost" disabled={collapsedSpanIds.size === 0} onClick={() => updateCollapsedSpanIds(new Set())}>
                            {t('operationsDashboardPage.traceDetail.expandAll')}
                          </Button>
                          <Button size="sm" variant="ghost" disabled={collapsibleSpanIds.size === 0 || collapsedSpanIds.size === collapsibleSpanIds.size} onClick={() => updateCollapsedSpanIds(collapsibleSpanIds)}>
                            {t('operationsDashboardPage.traceDetail.collapseAll')}
                          </Button>
                        </div>
                      </div>
                      <div className="grid grid-cols-[minmax(16rem,42%)_1fr_5.5rem] gap-3 border-b border-border px-4 py-2.5 text-xs font-medium text-muted-foreground">
                        <span>{t('operationsDashboardPage.traceDetail.operation')}</span>
                        <span>{t('operationsDashboardPage.traceDetail.timeline')}</span>
                        <span className="text-right">{t('operationsDashboardPage.duration')}</span>
                      </div>
                      {waterfallRows.length === 0
                        ? (
                            <EmptyState
                              description={t('operationsDashboardPage.turnDetail.noMatchingStepsDescription')}
                              title={t('operationsDashboardPage.turnDetail.noMatchingSteps')}
                              variant="plain"
                            />
                          )
                        : (
                            <div className="divide-y divide-border/70">
                              {waterfallRows.map(({ span, depth, hasChildren }) => (
                                <div
                                  key={span.spanId}
                                  className={cn(
                                    'grid min-h-11 w-full grid-cols-[minmax(16rem,42%)_1fr_5.5rem] items-center gap-3 px-4 py-2 text-left transition-colors',
                                    activeSpanId === span.spanId ? 'bg-primary-subtle/75' : 'hover:bg-surface-subtle/50',
                                  )}
                                >
                                  <span className="flex min-w-0 items-center gap-1" style={{ paddingLeft: `${Math.min(depth, 8) * 14}px` }}>
                                    {hasChildren
                                      ? (
                                          <Button
                                            aria-expanded={!collapsedSpanIds.has(span.spanId)}
                                            aria-label={t(collapsedSpanIds.has(span.spanId) ? 'operationsDashboardPage.traceDetail.expandSpan' : 'operationsDashboardPage.traceDetail.collapseSpan', { name: span.name })}
                                            className="size-7 shrink-0"
                                            size="icon"
                                            variant="ghost"
                                            onClick={() => toggleWaterfallSpan(span.spanId)}
                                          >
                                            {collapsedSpanIds.has(span.spanId) ? <ChevronRight className="size-4" /> : <ChevronDown className="size-4" />}
                                          </Button>
                                        )
                                      : <span className="size-7 shrink-0" />}
                                    <button
                                      className="flex min-w-0 flex-1 items-center gap-2 rounded-control text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                                      type="button"
                                      onClick={() => setSelectedSpanId(span.spanId)}
                                    >
                                      <WaterfallSpanIcon span={span} />
                                      <span className="min-w-0">
                                        <span className="block truncate text-sm font-medium">{span.name}</span>
                                        <span className="block truncate text-xs text-muted-foreground">{span.serviceName}</span>
                                      </span>
                                    </button>
                                  </span>
                                  <span className="relative h-5 overflow-hidden rounded-control bg-surface-subtle">
                                    <span className={cn('absolute top-1 h-3 min-w-1 rounded-sm', waterfallBarClass(span))} style={waterfallBarStyle(span, totalTraceDuration)} />
                                  </span>
                                  <span className="text-right font-mono text-xs">{formatDuration(span.durationMs)}</span>
                                </div>
                              ))}
                            </div>
                          )}
                    </div>

                    {activeSpan && (
                      <aside className="rounded-container border border-border bg-surface p-4 shadow-sm xl:sticky xl:top-0">
                        <AgentNodeInspector
                          key={activeSpan.spanId}
                          contentCaptured={flow.contentCaptured}
                          flowNode={activeFlowNode}
                          span={activeSpan}
                        />
                      </aside>
                    )}
                  </div>
                </section>
              )}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function ExecutionFlowRow({
  node,
  selected,
  totalDurationMs,
  language,
  onSelect,
}: {
  node: AgentObservabilityFlowNode
  selected: boolean
  totalDurationMs: number
  language: string
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const durationRatio = Math.max(2, Math.min(100, node.durationMs / totalDurationMs * 100))

  return (
    <button
      aria-pressed={selected}
      className={cn(
        'grid w-full gap-2 px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring md:grid-cols-[5.5rem_minmax(13rem,28%)_minmax(0,1fr)_7.5rem] md:items-center md:gap-3',
        selected ? 'bg-primary-subtle/70' : 'hover:bg-surface-subtle/50',
      )}
      type="button"
      onClick={onSelect}
    >
      <time className="flex items-center gap-2 font-mono text-xs text-muted-foreground md:grid md:gap-0.5">
        <span>{formatSpanTime(node.startTimeUnixNano, language)}</span>
        <span className="opacity-75">
          +
          {formatDuration(node.startOffsetMs)}
        </span>
      </time>

      <span className="flex min-w-0 items-start gap-2" style={{ paddingLeft: `${Math.min(node.depth, 5) * 14}px` }}>
        {node.depth > 0 && <span className="mt-2 h-2 w-2 shrink-0 border-b border-l border-border" />}
        <FlowKindBadge node={node} />
        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-center gap-1.5">
            <span className="truncate text-sm font-medium text-foreground">
              {flowRowTitle(node, t)}
            </span>
            {node.status === 'error' && <StatusBadge tone="danger">{t('operationsDashboardPage.turnDetail.failedStep')}</StatusBadge>}
            {node.compacted && <StatusBadge tone="info">{t('operationsDashboardPage.turnDetail.inspector.compactedShort')}</StatusBadge>}
            {node.addedTools && node.addedTools.length > 0 && (
              <StatusBadge tone="success">
                +
                {node.addedTools.length}
              </StatusBadge>
            )}
          </span>
          <span className="mt-0.5 block truncate font-mono text-xs text-muted-foreground">
            {node.kind === 'tool' ? node.name : node.spanName}
          </span>
        </span>
      </span>

      <FlowRowIOPreview input={node.inputPreview} kind={node.kind} output={node.outputPreview} />

      <span className="grid justify-items-end gap-1 font-mono text-xs">
        <span className="text-foreground">{formatDuration(node.durationMs)}</span>
        <span className="h-1.5 w-16 overflow-hidden rounded-full bg-surface-subtle">
          <span
            className={cn(
              'block h-full rounded-full',
              node.status === 'error' ? 'bg-danger-solid' : node.kind === 'llm' ? 'bg-primary' : node.kind === 'tool' ? 'bg-warning-solid' : 'bg-info-solid',
            )}
            style={{ width: `${durationRatio}%` }}
          />
        </span>
        {(node.inputTokens !== undefined || node.outputTokens !== undefined)
          ? (
              <span className="text-[11px] text-muted-foreground">
                {formatCompactTokens(node.inputTokens ?? 0)}
                {' → '}
                {formatCompactTokens(node.outputTokens ?? 0)}
              </span>
            )
          : node.payloadBytes && node.payloadBytes > 0
            ? <span className="text-[11px] text-muted-foreground">{formatByteSize(node.payloadBytes)}</span>
            : null}
      </span>
    </button>
  )
}

function FlowKindBadge({ node }: { node: AgentObservabilityFlowNode }) {
  if (node.kind === 'agent') {
    return (
      <span className="mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-control bg-info-subtle px-1.5 py-0.5 font-mono text-[11px] font-semibold text-info">
        <Sparkles className="size-3" />
        AGENT
      </span>
    )
  }
  if (node.kind === 'llm') {
    const label = node.purpose === 'summary'
      ? 'SUM'
      : node.purpose === 'title'
        ? 'TITLE'
        : node.stepIndex
          ? `LLM #${node.stepIndex}`
          : 'LLM'
    return (
      <span className="mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-control bg-primary-subtle px-1.5 py-0.5 font-mono text-[11px] font-semibold text-primary-text">
        <Bot className="size-3" />
        {label}
      </span>
    )
  }
  return (
    <span className="mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-control bg-warning-subtle px-1.5 py-0.5 font-mono text-[11px] font-semibold text-warning">
      <Wrench className="size-3" />
      TOOL
    </span>
  )
}

function FlowRowIOPreview({
  kind,
  input,
  output,
}: {
  kind: 'agent' | 'llm' | 'tool'
  input: AgentObservabilityFlowIOPreview
  output: AgentObservabilityFlowIOPreview
}) {
  const { t } = useTranslation()
  const hasInput = Boolean(input.text || (input.toolNames?.length ?? 0) > 0 || (input.kvItems?.length ?? 0) > 0)
  const hasOutput = Boolean(output.text || output.reasoning || (output.toolNames?.length ?? 0) > 0 || (output.kvItems?.length ?? 0) > 0 || output.errorCode)

  if (!hasInput && !hasOutput) {
    return <span className="text-xs text-muted-foreground">—</span>
  }

  return (
    <span className="grid min-w-0 items-center gap-1.5 text-xs lg:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
      <span className="flex min-w-0 flex-wrap items-center gap-1">
        {kind === 'tool' && input.kvItems && input.kvItems.length > 0
          ? input.kvItems.slice(0, 3).map(kv => (
              <span key={kv.key} className="max-w-44 truncate rounded-sm border border-border bg-surface-subtle px-1.5 py-0.5 font-mono text-[11px]">
                <span className="text-muted-foreground">
                  {kv.key}
                  =
                </span>
                <span className="text-foreground">{kv.value}</span>
              </span>
            ))
          : (
              <>
                {input.role === 'user' && (
                  <span className="shrink-0 rounded-sm bg-surface-subtle px-1.5 py-0.5 font-medium text-muted-foreground">
                    {t('operationsDashboardPage.turnDetail.flowTable.userBadge')}
                  </span>
                )}
                {input.role === 'tool' && (
                  <span className="shrink-0 rounded-sm bg-warning-subtle px-1.5 py-0.5 font-medium text-warning">
                    {input.itemCount && input.itemCount > 1
                      ? t('operationsDashboardPage.turnDetail.flowTable.toolResultsCount', { count: input.itemCount })
                      : t('operationsDashboardPage.turnDetail.flowTable.toolResultBadge')}
                  </span>
                )}
                {input.toolNames?.slice(0, 2).map(name => (
                  <span key={name} className="max-w-36 truncate rounded-sm bg-surface-subtle px-1.5 py-0.5 font-medium text-foreground">
                    {toolDisplayName(t, name)}
                  </span>
                ))}
                {input.text && <span className="min-w-0 flex-1 truncate text-muted-foreground">{input.text}</span>}
              </>
            )}
      </span>

      <ArrowRight className="hidden size-3.5 shrink-0 text-muted-foreground lg:block" />

      <span className="flex min-w-0 flex-wrap items-center gap-1">
        {output.errorCode && (
          <span className="max-w-44 truncate rounded-sm bg-danger-subtle px-1.5 py-0.5 font-mono text-[11px] font-medium text-danger">
            {output.errorCode}
          </span>
        )}
        {output.toolNames && output.toolNames.length > 0
          ? (
              <>
                {output.toolNames.slice(0, 2).map(name => (
                  <span key={name} className="inline-flex max-w-40 items-center gap-1 truncate rounded-sm bg-primary-subtle px-1.5 py-0.5 font-medium text-primary-text">
                    <Wrench className="size-3 shrink-0" />
                    <span className="truncate">{toolDisplayName(t, name)}</span>
                  </span>
                ))}
                {output.toolNames.length > 2 && (
                  <span className="rounded-sm bg-surface-subtle px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
                    +
                    {output.toolNames.length - 2}
                  </span>
                )}
              </>
            )
          : kind === 'tool' && output.kvItems && output.kvItems.length > 0
            ? output.kvItems.slice(0, 2).map(kv => (
                <span key={kv.key} className="max-w-44 truncate rounded-sm border border-border bg-surface-subtle px-1.5 py-0.5 font-mono text-[11px]">
                  <span className="text-muted-foreground">
                    {kv.key}
                    =
                  </span>
                  <span className="text-foreground">{kv.value}</span>
                </span>
              ))
            : output.text
              ? <span className="min-w-0 flex-1 truncate text-foreground">{output.text}</span>
              : output.reasoning
                ? <span className="min-w-0 flex-1 truncate italic text-muted-foreground">{output.reasoning}</span>
                : <span className="text-muted-foreground">—</span>}
      </span>
    </span>
  )
}

function flowRowTitle(
  node: AgentObservabilityFlowNode,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (node.kind === 'agent')
    return t('operationsDashboardPage.turnDetail.steps.agentStarted')
  if (node.kind === 'llm') {
    if (node.purpose === 'summary')
      return `${t('operationsDashboardPage.turnDetail.inspector.purposeSummary')} · ${node.name}`
    if (node.purpose === 'title')
      return `${t('operationsDashboardPage.turnDetail.inspector.purposeTitle')} · ${node.name}`
    return `${t('operationsDashboardPage.turnDetail.steps.modelCalled')} · ${node.name}`
  }
  return toolDisplayName(t as never, node.name)
}

function flowFilterLabel(
  filter: AgentFlowFilter,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  switch (filter) {
    case 'all':
      return t('operationsDashboardPage.turnDetail.filters.all')
    case 'agent':
      return t('operationsDashboardPage.turnDetail.filters.agent')
    case 'llm':
      return t('operationsDashboardPage.turnDetail.filters.model')
    case 'tool':
      return t('operationsDashboardPage.turnDetail.filters.tool')
    case 'error':
      return t('operationsDashboardPage.turnDetail.filters.error')
  }
}

function runStatusLabel(status: string, t: (key: string, options?: Record<string, unknown>) => string) {
  switch (status) {
    case 'queued':
      return t('operationsDashboardPage.runStatus.queued')
    case 'running':
      return t('operationsDashboardPage.runStatus.running')
    case 'waiting_approval':
      return t('operationsDashboardPage.runStatus.waiting_approval')
    case 'waiting_input':
      return t('operationsDashboardPage.runStatus.waiting_input')
    case 'waiting_external':
      return t('operationsDashboardPage.runStatus.waiting_external')
    case 'completed':
      return t('operationsDashboardPage.runStatus.completed')
    case 'failed':
      return t('operationsDashboardPage.runStatus.failed')
    case 'canceled':
      return t('operationsDashboardPage.runStatus.canceled')
    case 'expired':
      return t('operationsDashboardPage.runStatus.expired')
    case 'interrupted':
      return t('operationsDashboardPage.runStatus.interrupted')
    default:
      return status
  }
}

function WaterfallSpanIcon({ span }: { span: AgentObservabilityTraceSpan }) {
  const className = cn('size-4 shrink-0', span.status === 'error' ? 'text-danger' : 'text-muted-foreground')
  if (isAgentModelSpan(span))
    return <Bot className={className} />
  if (span.name.includes('tool') || Boolean(span.attributes['gen_ai.tool.name']))
    return <Wrench className={className} />
  if (span.name.includes('db.') || span.name.includes('postgres'))
    return <Database className={className} />
  if (span.kind === 'client' || span.kind === 'server')
    return <Network className={className} />
  return <Braces className={className} />
}

function waterfallBarClass(span: AgentObservabilityTraceSpan) {
  if (span.status === 'error')
    return 'bg-danger-solid'
  if (isAgentModelSpan(span))
    return 'bg-primary'
  if (span.name.includes('tool') || Boolean(span.attributes['gen_ai.tool.name']))
    return 'bg-warning-solid'
  return 'bg-info-solid'
}

function waterfallBarStyle(span: AgentObservabilityTraceSpan, total: number) {
  const safeTotal = Math.max(total, 0.001)
  return {
    left: `${Math.min(100, span.startOffsetMs / safeTotal * 100)}%`,
    width: `${Math.max(0.5, Math.min(100, span.durationMs / safeTotal * 100))}%`,
  }
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
  return new Intl.DateTimeFormat(language, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}

function formatSpanTime(nanos: string, language: string) {
  const ms = Number(nanos) / 1e6
  if (!Number.isFinite(ms) || ms <= 0)
    return '00:00:00'
  return new Intl.DateTimeFormat(language, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(ms))
}

function formatDuration(value: number) {
  if (value <= 0)
    return '0 ms'
  return value < 1 ? `${Math.round(value * 1000)} μs` : value < 1000 ? `${value.toFixed(value < 10 ? 2 : 1)} ms` : `${(value / 1000).toFixed(2)} s`
}

function formatNumber(value: number | undefined) {
  if (value === undefined)
    return '—'
  return new Intl.NumberFormat().format(value)
}

function formatCompactTokens(value: number) {
  if (value >= 1000)
    return `${(value / 1000).toFixed(value >= 10_000 ? 0 : 1)}k`
  return String(value)
}
