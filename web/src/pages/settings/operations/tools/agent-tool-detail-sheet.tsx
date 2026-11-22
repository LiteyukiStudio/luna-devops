import type { AgentObservabilityRange, AgentObservabilityToolCall as AgentObservabilityToolCallItem, AgentObservabilityToolSummary } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import type { AgentTraceFocusTarget } from '@/pages/settings/operations/turns/agent-turn-detail-sheet'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, CircleGauge, CircleX, Clock3, UserRound, Wrench } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { DataList } from '@/components/common/data-list'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { AgentTurnDetailSheet } from '@/pages/settings/operations/turns/agent-turn-detail-sheet'
import { AgentObservabilityToolCall } from './agent-observability-tool-call'

export function AgentToolDetailSheet({ range, summary, onOpenChange }: {
  range: AgentObservabilityRange
  summary: AgentObservabilityToolSummary | null
  onOpenChange: (open: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [selectedTraceTarget, setSelectedTraceTarget] = useState<AgentTraceFocusTarget | null>(null)
  const calls = useQuery({
    queryKey: ['agent-observability-tool-calls', summary?.operationId, range, page, pageSize],
    queryFn: () => api.listAgentObservabilityToolCalls(summary!.operationId, { range, page, pageSize }),
    enabled: Boolean(summary),
    refetchInterval: 30_000,
  })
  const columns = useMemo<DataListColumn<AgentObservabilityToolCallItem>[]>(() => [{
    key: 'call',
    header: t('operationsDashboardPage.toolDetail.calls'),
    minWidth: 280,
    render: call => (
      <div className="grid min-w-0 gap-2 py-1">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <strong className="max-w-72 truncate font-medium text-foreground">{call.conversationTitle}</strong>
          <span>{t('operationsDashboardPage.turnNumber', { index: call.turnIndex + 1 })}</span>
          <span className="inline-flex items-center gap-1">
            <UserRound className="size-3.5" />
            {call.user.name || call.user.email}
          </span>
          <span>{formatDateTime(call.createdAt, i18n.language)}</span>
          <span className="font-mono">{call.runId}</span>
        </div>
        <AgentObservabilityToolCall
          call={call}
          onInspectCall={item => item.traceId && setSelectedTraceTarget({
            traceId: item.traceId,
            focusToolCallId: item.id,
            focusToolName: item.operationId,
          })}
        />
      </div>
    ),
  }], [i18n.language, t])
  const changePageSize = (value: number) => {
    setPageSize(value)
    setPage(1)
  }
  const rate = summary?.successRate ?? 0

  return (
    <>
      <Sheet open={Boolean(summary)} onOpenChange={onOpenChange}>
        <SheetContent className="w-full max-w-none gap-0 p-0 sm:max-w-none lg:w-2/3" side="right">
          <SheetHeader className="border-b border-border px-6 py-4">
            <SheetTitle>{summary ? toolDisplayName(t, summary.operationId) : t('operationsDashboardPage.toolDetail.titleFallback')}</SheetTitle>
            <SheetDescription>{t('operationsDashboardPage.toolDetail.description', { range: t(`operationsDashboardPage.timeRange.${range}`) })}</SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-auto p-6">
            {summary && (
              <div className="grid gap-6">
                <MetricGroup className="grid-cols-2 @lg/metric-group:grid-cols-5 @4xl/metric-group:grid-cols-5">
                  <MetricItem icon={<CircleGauge className="size-4" />} label={t('operationsDashboardPage.toolSuccessRate')} tone={summary.totalCalls > 0 ? successRateTone(rate) : 'neutral'} value={`${rate.toFixed(1)}%`} />
                  <MetricItem icon={<Wrench className="size-4" />} label={t('operationsDashboardPage.totalCalls')} value={formatNumber(summary.totalCalls)} />
                  <MetricItem icon={<CheckCircle2 className="size-4" />} label={t('operationsDashboardPage.succeededCalls')} tone={summary.succeededCalls > 0 ? 'success' : 'neutral'} value={formatNumber(summary.succeededCalls)} />
                  <MetricItem icon={<CircleX className="size-4" />} label={t('operationsDashboardPage.failedCalls')} tone={summary.failedCalls > 0 ? 'danger' : summary.totalCalls > 0 ? 'success' : 'neutral'} value={formatNumber(summary.failedCalls)} />
                  <MetricItem icon={<Clock3 className="size-4" />} label={t('operationsDashboardPage.otherCalls')} tone={summary.otherCalls > 0 ? 'warning' : 'neutral'} value={formatNumber(summary.otherCalls)} />
                </MetricGroup>
                <DataList
                  columns={columns}
                  emptyDescription={t('operationsDashboardPage.toolDetail.emptyDescription')}
                  emptyMode="actionable"
                  emptyTitle={t('operationsDashboardPage.toolDetail.emptyTitle')}
                  error={calls.isError
                    ? {
                        description: t('operationsDashboardPage.toolDetail.loadFailedDescription'),
                        onRetry: () => void calls.refetch(),
                        retrying: calls.isFetching,
                        title: t('operationsDashboardPage.toolDetail.loadFailed'),
                      }
                    : undefined}
                  items={calls.data?.items ?? []}
                  loading={Boolean(summary) && calls.isPending}
                  pagination={{
                    page,
                    pageSize,
                    total: calls.data?.total ?? 0,
                    totalPages: calls.data?.totalPages ?? 0,
                    pageInfoLabel: t('pagination.pageInfo', { page: calls.data?.page ?? page, totalPages: calls.data?.totalPages ?? 0, total: calls.data?.total ?? 0 }),
                    onPageChange: setPage,
                    onPageSizeChange: changePageSize,
                    pageSizeOptions: [10, 20, 50],
                  }}
                  rowKey={call => call.id}
                />
              </div>
            )}
          </div>
        </SheetContent>
      </Sheet>
      <AgentTurnDetailSheet
        key={selectedTraceTarget ? `${selectedTraceTarget.traceId}-${selectedTraceTarget.focusToolCallId ?? ''}` : 'trace-closed'}
        traceTarget={selectedTraceTarget}
        onOpenChange={open => !open && setSelectedTraceTarget(null)}
      />
    </>
  )
}

function successRateTone(value: number): 'success' | 'warning' | 'danger' {
  if (value >= 95)
    return 'success'
  if (value >= 85)
    return 'warning'
  return 'danger'
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}

function formatDateTime(value: string, language: string) {
  return new Intl.DateTimeFormat(language, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}
