import type { AgentObservabilityRange, AgentObservabilityToolCall as AgentObservabilityToolCallItem, AgentObservabilityToolSummary, AgentObservabilityTrace } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, CircleGauge, CircleX, Clock3, UserRound, Wrench } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { AgentTraceDetailSheet } from '@/pages/settings/operations/traces/agent-trace-detail-sheet'
import { AgentObservabilityToolCall } from './agent-observability-tool-call'

export function AgentToolDetailSheet({ range, summary, onOpenChange }: {
  range: AgentObservabilityRange
  summary: AgentObservabilityToolSummary | null
  onOpenChange: (open: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [selectedTrace, setSelectedTrace] = useState<AgentObservabilityTrace | null>(null)
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
        <AgentObservabilityToolCall call={call} onViewTrace={setSelectedTrace} />
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
                <MetricGroup className="grid-cols-2 sm:grid-cols-5">
                  <MetricItem icon={<CircleGauge className="size-4" />} label={t('operationsDashboardPage.toolSuccessRate')} value={`${rate.toFixed(1)}%`} tone={successRateTone(rate)} />
                  <MetricItem icon={<Wrench className="size-4" />} label={t('operationsDashboardPage.totalCalls')} value={formatNumber(summary.totalCalls)} />
                  <MetricItem icon={<CheckCircle2 className="size-4" />} label={t('operationsDashboardPage.succeededCalls')} value={formatNumber(summary.succeededCalls)} tone="success" />
                  <MetricItem icon={<CircleX className="size-4" />} label={t('operationsDashboardPage.failedCalls')} value={formatNumber(summary.failedCalls)} tone={summary.failedCalls > 0 ? 'danger' : 'success'} />
                  <MetricItem icon={<Clock3 className="size-4" />} label={t('operationsDashboardPage.otherCalls')} value={formatNumber(summary.otherCalls)} tone={summary.otherCalls > 0 ? 'warning' : 'neutral'} />
                </MetricGroup>
                {calls.isError
                  ? <ErrorState title={t('operationsDashboardPage.toolDetail.loadFailed')} description={t('operationsDashboardPage.toolDetail.loadFailedDescription')} />
                  : (
                      <DataList
                        columns={columns}
                        emptyDescription={t('operationsDashboardPage.toolDetail.emptyDescription')}
                        emptyMode="actionable"
                        emptyTitle={t('operationsDashboardPage.toolDetail.emptyTitle')}
                        items={calls.data?.items ?? []}
                        loading={calls.isLoading}
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
                    )}
              </div>
            )}
          </div>
        </SheetContent>
      </Sheet>
      <AgentTraceDetailSheet trace={selectedTrace} onOpenChange={open => !open && setSelectedTrace(null)} />
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
