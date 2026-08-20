import type { AgentObservabilityRange, AgentObservabilityToolSummary, AgentObservabilityTurn } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useQuery } from '@tanstack/react-query'
import { ArrowDownToLine, ArrowUpFromLine, CircleGauge, Clock3, ExternalLink, Eye, MessagesSquare, RefreshCw, Settings, UserRound, Wrench } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { ForbiddenPage } from '@/components/common/forbidden-page'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { isPlatformAdmin } from '@/lib/roles'
import { agentObservabilityRanges, readAgentObservabilityRange, writeAgentObservabilityRange } from './agent-observability-range-preference'
import { AgentToolDetailSheet } from './agent-tool-detail-sheet'
import { AgentTurnDetailSheet } from './agent-turn-detail-sheet'

const OPERATIONS_DASHBOARD_URL_KEY = 'site.operationsDashboardUrl'

export function OperationsDashboardPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const [activeTab, setActiveTab] = useState('platform')
  const platformAdmin = isPlatformAdmin(user?.role)
  const configs = useQuery({
    queryKey: ['configs'],
    queryFn: api.getConfigs,
    enabled: platformAdmin,
  })
  const dashboardUrl = configs.data?.[OPERATIONS_DASHBOARD_URL_KEY]?.trim() ?? ''
  const iframeUrl = useMemo(() => resolveIframeUrl(dashboardUrl), [dashboardUrl])

  if (!platformAdmin)
    return <ForbiddenPage />

  if (configs.isLoading)
    return <OperationsDashboardSkeleton />

  if (configs.isError) {
    return (
      <ErrorState
        description={t('operationsDashboardPage.loadFailedDescription')}
        title={t('operationsDashboardPage.loadFailedTitle')}
      />
    )
  }

  const observabilityEnabled = configs.data?.['ai.observability.enabled'] === 'true'
  return (
    <ContentTabs
      tabs={[
        { value: 'platform', label: t('operationsDashboardPage.platformTab') },
        { value: 'agent', label: t('operationsDashboardPage.agentTab') },
      ]}
      value={activeTab}
      onValueChange={setActiveTab}
    >
      <TabsContent value="platform">
        <PlatformDashboardContent dashboardUrl={dashboardUrl} iframeUrl={iframeUrl} />
      </TabsContent>
      <TabsContent value="agent">
        <AgentObservabilityView active={activeTab === 'agent'} enabled={observabilityEnabled} userId={user?.id ?? ''} />
      </TabsContent>
    </ContentTabs>
  )
}

function PlatformDashboardContent({ dashboardUrl, iframeUrl }: { dashboardUrl: string, iframeUrl: string }) {
  const { t } = useTranslation()
  if (!dashboardUrl) {
    return <ObservabilityConfigEmpty title={t('operationsDashboardPage.emptyTitle')} description={t('operationsDashboardPage.emptyDescription')} />
  }
  if (!iframeUrl)
    return <ErrorState description={t('operationsDashboardPage.invalidDescription')} title={t('operationsDashboardPage.invalidTitle')} />
  return <OperationsDashboardViewport key={iframeUrl} iframeUrl={iframeUrl} />
}

function ObservabilityConfigEmpty({ description, title }: { description: string, title: string }) {
  const { t } = useTranslation()
  return (
    <EmptyState
      actions={(
        <Button asChild>
          <Link to="/settings/site#tab=ai">
            <Settings size={16} />
            {t('operationsDashboardPage.configure')}
          </Link>
        </Button>
      )}
      description={description}
      title={title}
    />
  )
}

function AgentObservabilityView({ active, enabled, userId }: { active: boolean, enabled: boolean, userId: string }) {
  const { t, i18n } = useTranslation()
  const [range, setRange] = useState<AgentObservabilityRange>(() => readAgentObservabilityRange(userId))
  const [turnPage, setTurnPage] = useState(1)
  const [turnPageSize, setTurnPageSize] = useState(20)
  const [turnSearch, setTurnSearch] = useState('')
  const [selectedTurn, setSelectedTurn] = useState<AgentObservabilityTurn | null>(null)
  const [dataTab, setDataTab] = useState('turns')
  const [toolPage, setToolPage] = useState(1)
  const [toolPageSize, setToolPageSize] = useState(20)
  const [toolSearch, setToolSearch] = useState('')
  const [selectedTool, setSelectedTool] = useState<AgentObservabilityToolSummary | null>(null)
  const overview = useQuery({
    queryKey: ['agent-observability', range],
    queryFn: () => api.getAgentObservabilityOverview(range),
    enabled: active && enabled,
    refetchInterval: 30_000,
  })
  const turns = useQuery({
    queryKey: ['agent-observability-turns', range, turnPage, turnPageSize, turnSearch],
    queryFn: () => api.listAgentObservabilityTurns({ range, page: turnPage, pageSize: turnPageSize, search: turnSearch }),
    enabled: active && enabled && dataTab === 'turns',
    refetchInterval: 30_000,
  })
  const tools = useQuery({
    queryKey: ['agent-observability-tools', range, toolPage, toolPageSize, toolSearch],
    queryFn: () => api.listAgentObservabilityTools({ range, page: toolPage, pageSize: toolPageSize, search: toolSearch }),
    enabled: active && enabled && dataTab === 'tools',
    refetchInterval: 30_000,
  })
  const changeTurnPageSize = (value: number) => {
    setTurnPageSize(value)
    setTurnPage(1)
  }
  const changeTurnSearch = (value: string) => {
    setTurnSearch(value)
    setTurnPage(1)
  }
  const changeToolPageSize = (value: number) => {
    setToolPageSize(value)
    setToolPage(1)
  }
  const changeToolSearch = (value: string) => {
    setToolSearch(value)
    setToolPage(1)
  }
  const changeRange = (value: AgentObservabilityRange) => {
    setRange(value)
    writeAgentObservabilityRange(userId, value)
    setTurnPage(1)
    setToolPage(1)
    setSelectedTurn(null)
    setSelectedTool(null)
  }
  const refreshWorkspace = () => void Promise.all([overview.refetch(), dataTab === 'turns' ? turns.refetch() : tools.refetch()])
  const workspaceFetching = overview.isFetching || (dataTab === 'turns' ? turns.isFetching : tools.isFetching)
  const turnColumns = useMemo<DataListColumn<AgentObservabilityTurn>[]>(() => [
    {
      key: 'conversation',
      header: t('operationsDashboardPage.turn'),
      minWidth: 200,
      maxWidth: 240,
      render: item => (
        <span className="min-w-0">
          <span className="block truncate font-medium">{item.conversationTitle}</span>
          <span className="mt-1 flex min-w-0 items-center gap-1.5 truncate text-xs text-muted-foreground">
            <UserRound className="size-3.5 shrink-0" />
            {t('operationsDashboardPage.turnNumber', { index: item.turnIndex + 1 })}
            {' · '}
            {item.user.name || item.user.email}
            {' · '}
            {formatDateTime(item.createdAt, i18n.language)}
          </span>
        </span>
      ),
    },
    {
      key: 'message',
      header: t('operationsDashboardPage.turnContent'),
      minWidth: 220,
      maxWidth: 360,
      mobile: 'hidden',
      render: item => (
        <span className="grid min-w-0 gap-1">
          <span className="block truncate text-sm" title={item.userMessage}>{t('operationsDashboardPage.userMessagePrefix', { message: item.userMessage || '—' })}</span>
          <span className="block truncate text-xs text-muted-foreground" title={item.assistantMessage}>{t('operationsDashboardPage.assistantMessagePrefix', { message: item.assistantMessage || '—' })}</span>
        </span>
      ),
    },
    {
      key: 'summary',
      header: t('operationsDashboardPage.executionSummary'),
      minWidth: 170,
      maxWidth: 196,
      mobile: 'hidden',
      render: item => (
        <span className="grid gap-1">
          <span className="flex items-center gap-2">
            <StatusBadge tone={runStatusTone(item.status)}>{t(`operationsDashboardPage.runStatus.${item.status}`, { defaultValue: item.status })}</StatusBadge>
            <span className="font-mono text-xs text-muted-foreground">{item.durationMs > 0 ? formatDuration(item.durationMs) : '—'}</span>
          </span>
          <span className="flex items-center gap-1 font-mono text-xs text-muted-foreground">
            ↑
            {formatNumber(item.inputTokens)}
            {' · '}
            ↓
            {formatNumber(item.outputTokens)}
          </span>
          <span className="flex items-center gap-1 font-mono text-xs text-muted-foreground" title={t('operationsDashboardPage.toolCalls')}>
            <Wrench className="size-3" />
            {formatNumber(item.toolCallCount)}
          </span>
        </span>
      ),
    },
    {
      key: 'actions',
      header: t('operationsDashboardPage.actions'),
      width: 'actions',
      sticky: 'right',
      mobileActions: 'inline',
      render: item => (
        <Button size="sm" variant="outline" onClick={() => setSelectedTurn(item)}>
          <Eye className="size-4" />
          {t('operationsDashboardPage.viewTurn')}
        </Button>
      ),
    },
  ], [i18n.language, t])
  const toolColumns = useMemo<DataListColumn<AgentObservabilityToolSummary>[]>(() => [
    {
      key: 'tool',
      header: t('operationsDashboardPage.tool'),
      minWidth: 220,
      maxWidth: 360,
      render: item => (
        <span className="grid min-w-0 gap-1">
          <span className="truncate font-medium">{toolDisplayName(t, item.operationId)}</span>
          <span className="truncate font-mono text-xs text-muted-foreground">{item.operationId}</span>
        </span>
      ),
    },
    {
      key: 'successRate',
      header: t('operationsDashboardPage.toolSuccessRate'),
      width: 'status',
      render: item => (
        <StatusBadge tone={successRateTone(item.successRate)}>
          {item.successRate.toFixed(1)}
          %
        </StatusBadge>
      ),
    },
    {
      key: 'calls',
      header: t('operationsDashboardPage.callDistribution'),
      minWidth: 190,
      maxWidth: 240,
      mobile: 'hidden',
      render: item => (
        <span className="grid gap-1 font-mono text-xs">
          <span>{t('operationsDashboardPage.totalCallsValue', { count: item.totalCalls })}</span>
          <span className="text-muted-foreground">{t('operationsDashboardPage.callOutcomeValue', { succeeded: item.succeededCalls, failed: item.failedCalls, other: item.otherCalls })}</span>
        </span>
      ),
    },
    {
      key: 'lastCalledAt',
      header: t('operationsDashboardPage.lastCalledAt'),
      width: 'compact',
      mobile: 'hidden',
      render: item => <span className="text-xs text-muted-foreground">{formatDateTime(item.lastCalledAt, i18n.language)}</span>,
    },
    {
      key: 'actions',
      header: t('operationsDashboardPage.actions'),
      width: 'actions',
      sticky: 'right',
      mobileActions: 'inline',
      render: item => (
        <Button size="sm" variant="outline" onClick={() => setSelectedTool(item)}>
          <Eye className="size-4" />
          {t('operationsDashboardPage.viewToolCalls')}
        </Button>
      ),
    },
  ], [i18n.language, t])

  if (!enabled) {
    return <ObservabilityConfigEmpty title={t('operationsDashboardPage.agentDisabledTitle')} description={t('operationsDashboardPage.agentDisabledDescription')} />
  }
  const data = overview.data
  const successRate = data?.summary.turnSuccessRate ?? 0
  const toolSuccessRate = data?.summary.toolSuccessRate ?? 0
  return (
    <div className="grid gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="grid gap-0.5">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-base font-semibold">{t('operationsDashboardPage.agentOverview')}</h2>
            {data?.observationCode === 'ai.observability.partial' && <StatusBadge tone="warning">{t('operationsDashboardPage.metricsUnavailable')}</StatusBadge>}
          </div>
          <p className="m-0 text-sm text-muted-foreground">{t('operationsDashboardPage.agentOverviewDescription', { range: t(`operationsDashboardPage.timeRange.${range}`) })}</p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          {agentObservabilityRanges.map(item => <Button key={item} size="sm" variant={range === item ? 'default' : 'outline'} onClick={() => changeRange(item)}>{t(`operationsDashboardPage.timeRange.${item}`)}</Button>)}
          <Button aria-label={t('common.refresh')} disabled={workspaceFetching} size="icon" variant="ghost" onClick={refreshWorkspace}><RefreshCw className={workspaceFetching ? 'animate-spin' : ''} /></Button>
        </div>
      </div>
      {overview.isLoading && <ToolViewportSkeleton />}
      {overview.isError && <ErrorState title={t('operationsDashboardPage.agentLoadFailedTitle')} description={t('operationsDashboardPage.agentLoadFailedDescription')} />}
      {data && (
        <MetricGroup className="grid-cols-2 xl:grid-cols-7">
          <MetricItem icon={<ArrowDownToLine className="size-4" />} label={t('operationsDashboardPage.inputTokens')} value={formatNumber(data.summary.inputTokens)} />
          <MetricItem icon={<ArrowUpFromLine className="size-4" />} label={t('operationsDashboardPage.outputTokens')} value={formatNumber(data.summary.outputTokens)} />
          <MetricItem icon={<Wrench className="size-4" />} label={t('operationsDashboardPage.toolCalls')} value={formatNumber(data.summary.toolCalls)} />
          <MetricItem icon={<CircleGauge className="size-4" />} label={t('operationsDashboardPage.toolSuccessRate')} value={`${toolSuccessRate.toFixed(1)}%`} tone={successRateTone(toolSuccessRate)} />
          <MetricItem icon={<MessagesSquare className="size-4" />} label={t('operationsDashboardPage.turnCount')} value={formatNumber(data.summary.turnCount)} />
          <MetricItem icon={<CircleGauge className="size-4" />} label={t('operationsDashboardPage.turnSuccessRate')} value={`${successRate.toFixed(1)}%`} tone={successRate >= 95 ? 'success' : successRate >= 85 ? 'warning' : 'danger'} />
          <MetricItem icon={<Clock3 className="size-4" />} label={t('operationsDashboardPage.runDurationP95')} value={formatSeconds(data.summary.runDurationP95)} />
        </MetricGroup>
      )}
      <Tabs
        value={dataTab}
        onValueChange={(value) => {
          setDataTab(value)
          setSelectedTurn(null)
          setSelectedTool(null)
        }}
      >
        <TabsList aria-label={t('operationsDashboardPage.dataScope')}>
          <TabsTrigger value="turns">{t('operationsDashboardPage.turnScope')}</TabsTrigger>
          <TabsTrigger value="tools">{t('operationsDashboardPage.toolScope')}</TabsTrigger>
        </TabsList>
        <TabsContent value="turns">
          <Section title={t('operationsDashboardPage.turnList')} description={t('operationsDashboardPage.turnListDescription')}>
            {turns.isError
              ? <ErrorState title={t('operationsDashboardPage.turnsLoadFailed')} description={t('operationsDashboardPage.turnsLoadFailedDescription')} />
              : (
                  <DataList
                    columns={turnColumns}
                    constrainedHeight
                    emptyDescription={t('operationsDashboardPage.noRunsDescription')}
                    emptyMode={turnSearch ? 'filtered' : 'actionable'}
                    emptyTitle={t('operationsDashboardPage.noRuns')}
                    items={turns.data?.items ?? []}
                    loading={turns.isLoading}
                    pagination={{
                      page: turnPage,
                      pageSize: turnPageSize,
                      total: turns.data?.total ?? 0,
                      totalPages: turns.data?.totalPages ?? 0,
                      pageInfoLabel: t('pagination.pageInfo', { page: turns.data?.page ?? turnPage, totalPages: turns.data?.totalPages ?? 0, total: turns.data?.total ?? 0 }),
                      onPageChange: setTurnPage,
                      onPageSizeChange: changeTurnPageSize,
                    }}
                    rowActionLabel={item => t('operationsDashboardPage.openTurnLabel', { index: item.turnIndex + 1, title: item.conversationTitle })}
                    rowKey={item => item.id}
                    search={{ value: turnSearch, placeholder: t('operationsDashboardPage.searchTurns'), onChange: changeTurnSearch }}
                    viewportOffset={28}
                    onRowClick={setSelectedTurn}
                  />
                )}
          </Section>
        </TabsContent>
        <TabsContent value="tools">
          <Section title={t('operationsDashboardPage.toolList')} description={t('operationsDashboardPage.toolListDescription')}>
            {tools.isError
              ? <ErrorState title={t('operationsDashboardPage.toolsLoadFailed')} description={t('operationsDashboardPage.toolsLoadFailedDescription')} />
              : (
                  <DataList
                    columns={toolColumns}
                    constrainedHeight
                    emptyDescription={t('operationsDashboardPage.noToolsDescription')}
                    emptyMode={toolSearch ? 'filtered' : 'actionable'}
                    emptyTitle={t('operationsDashboardPage.noTools')}
                    items={tools.data?.items ?? []}
                    loading={tools.isLoading}
                    pagination={{
                      page: toolPage,
                      pageSize: toolPageSize,
                      total: tools.data?.total ?? 0,
                      totalPages: tools.data?.totalPages ?? 0,
                      pageInfoLabel: t('pagination.pageInfo', { page: tools.data?.page ?? toolPage, totalPages: tools.data?.totalPages ?? 0, total: tools.data?.total ?? 0 }),
                      onPageChange: setToolPage,
                      onPageSizeChange: changeToolPageSize,
                    }}
                    rowActionLabel={item => t('operationsDashboardPage.openToolLabel', { tool: item.operationId })}
                    rowKey={item => item.operationId}
                    search={{ value: toolSearch, placeholder: t('operationsDashboardPage.searchTools'), onChange: changeToolSearch }}
                    viewportOffset={28}
                    onRowClick={setSelectedTool}
                  />
                )}
          </Section>
        </TabsContent>
      </Tabs>
      <AgentTurnDetailSheet key={selectedTurn?.id ?? 'closed'} turn={selectedTurn} onOpenChange={open => !open && setSelectedTurn(null)} />
      <AgentToolDetailSheet key={selectedTool?.operationId ?? 'closed'} range={range} summary={selectedTool} onOpenChange={open => !open && setSelectedTool(null)} />
    </div>
  )
}

function OperationsDashboardSkeleton() {
  return <ToolViewportSkeleton />
}

function formatDateTime(value: string, language: string) {
  return new Intl.DateTimeFormat(language, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

function OperationsDashboardViewport({ iframeUrl }: { iframeUrl: string }) {
  const { t } = useTranslation()
  const [loaded, setLoaded] = useState(false)
  const [timedOut, setTimedOut] = useState(false)

  useEffect(() => {
    const timeout = window.setTimeout(setTimedOut, 10000, true)
    return () => window.clearTimeout(timeout)
  }, [])

  return (
    <Card className="relative overflow-hidden p-0">
      <div className="flex items-center justify-end gap-2 border-b border-border p-2">
        <Button asChild size="sm" variant="ghost">
          <a href={iframeUrl} rel="noreferrer" target="_blank">
            <ExternalLink className="size-4" />
            {t('operationsDashboardPage.openInNewWindow')}
          </a>
        </Button>
      </div>
      {!loaded && !timedOut && <div className="absolute inset-x-0 bottom-0 top-12 z-10"><ToolViewportSkeleton /></div>}
      {timedOut && !loaded && (
        <div className="absolute inset-x-0 bottom-0 top-12 z-20 grid place-items-center bg-surface-raised/95 p-4">
          <div className="grid max-w-lg gap-4 text-center">
            <ErrorState description={t('operationsDashboardPage.iframeTimeoutDescription')} title={t('operationsDashboardPage.iframeTimeoutTitle')} />
            <div className="flex flex-wrap justify-center gap-2">
              <Button onClick={() => window.location.reload()}>
                <RefreshCw className="size-4" />
                {t('common.refresh')}
              </Button>
              <Button asChild variant="outline">
                <a href={iframeUrl} rel="noreferrer" target="_blank">
                  <ExternalLink className="size-4" />
                  {t('operationsDashboardPage.openInNewWindow')}
                </a>
              </Button>
              <Button asChild variant="ghost">
                <Link to="/settings/site">
                  <Settings className="size-4" />
                  {t('operationsDashboardPage.configure')}
                </Link>
              </Button>
            </div>
          </div>
        </div>
      )}
      <iframe
        className="h-[72dvh] min-h-128 max-h-192 w-full border-0 bg-background"
        referrerPolicy="strict-origin-when-cross-origin"
        src={iframeUrl}
        title={t('operationsDashboard')}
        onError={() => setTimedOut(true)}
        onLoad={() => setLoaded(true)}
      />
    </Card>
  )
}

function resolveIframeUrl(rawUrl: string) {
  try {
    const parsed = new URL(rawUrl)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:')
      return ''
    return parsed.toString()
  }
  catch {
    return ''
  }
}

function formatNumber(value: number | undefined) {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(value ?? 0)
}

function formatSeconds(value: number) {
  if (!Number.isFinite(value))
    return '—'
  if (value < 1)
    return `${Math.round(value * 1000)} ms`
  return `${value.toFixed(value < 10 ? 2 : 1)} s`
}

function formatDuration(value: number) {
  return value < 1000 ? `${value} ms` : `${(value / 1000).toFixed(2)} s`
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

function successRateTone(value: number): 'success' | 'danger' | 'warning' {
  if (value >= 95)
    return 'success'
  if (value >= 85)
    return 'warning'
  return 'danger'
}
