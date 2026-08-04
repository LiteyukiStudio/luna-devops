import type { AgentObservabilityConversation, AgentObservabilityLog, AgentObservabilityTrace } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useQuery } from '@tanstack/react-query'
import { Activity, Bot, CircleGauge, Clock3, ExternalLink, Eye, MessagesSquare, Network, RefreshCw, Settings, Timer, UserRound, Wrench } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
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
import { AgentConversationDetailSheet } from './agent-conversation-detail-sheet'
import { AgentObservabilityChart } from './agent-observability-chart'
import { AgentTraceDetailSheet } from './agent-trace-detail-sheet'

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
        <AgentObservabilityView active={activeTab === 'agent'} enabled={observabilityEnabled} />
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

function AgentObservabilityView({ active, enabled }: { active: boolean, enabled: boolean }) {
  const { t, i18n } = useTranslation()
  const [range, setRange] = useState<'1h' | '6h' | '24h'>('1h')
  const [traceScope, setTraceScope] = useState<'conversations' | 'turns'>('conversations')
  const [conversationPage, setConversationPage] = useState(1)
  const [conversationPageSize, setConversationPageSize] = useState(20)
  const [conversationSearch, setConversationSearch] = useState('')
  const [selectedConversation, setSelectedConversation] = useState<AgentObservabilityConversation | null>(null)
  const [conversationTurnPage, setConversationTurnPage] = useState(1)
  const [selectedTrace, setSelectedTrace] = useState<AgentObservabilityTrace | null>(null)
  const overview = useQuery({
    queryKey: ['agent-observability', range],
    queryFn: () => api.getAgentObservabilityOverview(range),
    enabled: active && enabled,
    refetchInterval: 30_000,
  })
  const conversations = useQuery({
    queryKey: ['agent-observability-conversations', range, conversationPage, conversationPageSize, conversationSearch],
    queryFn: () => api.listAgentObservabilityConversations({ range, page: conversationPage, pageSize: conversationPageSize, search: conversationSearch }),
    enabled: active && enabled && traceScope === 'conversations',
    refetchInterval: 30_000,
  })
  const changeConversationPageSize = (value: number) => {
    setConversationPageSize(value)
    setConversationPage(1)
  }
  const changeConversationSearch = (value: string) => {
    setConversationSearch(value)
    setConversationPage(1)
  }
  const openConversation = (conversation: AgentObservabilityConversation) => {
    setConversationTurnPage(1)
    setSelectedConversation(conversation)
  }
  const logColumns = useMemo<DataListColumn<AgentObservabilityLog>[]>(() => [
    { key: 'time', header: t('operationsDashboardPage.time'), width: 'compact', render: item => formatLokiTime(item.timestamp, i18n.language) },
    { key: 'event', header: t('operationsDashboardPage.event'), width: 'secondary', render: item => item.labels.event_name || item.labels.eventName || '—' },
    { key: 'message', header: t('operationsDashboardPage.message'), width: 'primary', render: item => <span className="font-mono text-xs break-all">{item.line}</span> },
    { key: 'trace', header: t('operationsDashboardPage.traceId'), width: 'secondary', mobile: 'hidden', render: item => <span className="font-mono text-xs">{shortId(item.labels.trace_id)}</span> },
  ], [i18n.language, t])
  const traceColumns = useMemo<DataListColumn<AgentObservabilityTrace>[]>(() => [
    { key: 'time', header: t('operationsDashboardPage.time'), width: 'compact', render: item => formatNanoTime(item.startTimeUnixNano, i18n.language) },
    { key: 'name', header: t('operationsDashboardPage.run'), width: 'primary', render: item => item.rootTraceName || 'agent.run.execute' },
    { key: 'duration', header: t('operationsDashboardPage.duration'), width: 'number', render: item => formatDuration(item.durationMs) },
    { key: 'trace', header: t('operationsDashboardPage.traceId'), width: 'secondary', render: item => <span className="font-mono text-xs" title={item.traceId}>{shortId(item.traceId)}</span> },
    { key: 'actions', header: t('operationsDashboardPage.actions'), width: 'actions', sticky: 'right', mobileActions: 'inline', render: item => (
      <Button size="sm" variant="outline" onClick={() => setSelectedTrace(item)}>
        <Eye className="size-4" />
        {t('operationsDashboardPage.viewTrace')}
      </Button>
    ) },
  ], [i18n.language, t])
  const conversationColumns = useMemo<DataListColumn<AgentObservabilityConversation>[]>(() => [
    { key: 'updatedAt', header: t('operationsDashboardPage.updatedAt'), width: 'compact', render: item => formatDateTime(item.updatedAt, i18n.language) },
    { key: 'title', header: t('operationsDashboardPage.conversationTitle'), width: 'primary', render: item => <span className="font-medium">{item.title}</span> },
    { key: 'user', header: t('operationsDashboardPage.user'), width: 'normal', render: item => (
      <span className="flex min-w-0 items-center gap-2">
        <UserRound className="size-4 shrink-0 text-muted-foreground" />
        <span className="min-w-0">
          <span className="block truncate text-sm">{item.user.name || item.user.email}</span>
          {item.user.name && <span className="block truncate text-xs text-muted-foreground">{item.user.email}</span>}
        </span>
      </span>
    ) },
    { key: 'turnCount', header: t('operationsDashboardPage.turnCount'), width: 'number', render: item => item.turnCount },
    { key: 'traceCount', header: t('operationsDashboardPage.traceCount'), width: 'number', render: item => item.traceCount },
    { key: 'actions', header: t('operationsDashboardPage.actions'), width: 'actions', sticky: 'right', mobileActions: 'inline', render: item => (
      <Button size="sm" variant="outline" onClick={() => openConversation(item)}>
        <Eye className="size-4" />
        {t('operationsDashboardPage.viewConversation')}
      </Button>
    ) },
  ], [i18n.language, t])

  if (!enabled) {
    return <ObservabilityConfigEmpty title={t('operationsDashboardPage.agentDisabledTitle')} description={t('operationsDashboardPage.agentDisabledDescription')} />
  }
  if (overview.isLoading)
    return <ToolViewportSkeleton />
  if (overview.isError) {
    return <ErrorState title={t('operationsDashboardPage.agentLoadFailedTitle')} description={t('operationsDashboardPage.agentLoadFailedDescription')} />
  }
  const data = overview.data
  if (!data)
    return null
  const successRate = data.summary.runSuccessRate ?? 0
  const modelErrorRate = data.summary.modelErrorRate ?? 0
  return (
    <div className="grid gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          {(['prometheus', 'loki', 'tempo'] as const).map(source => (
            <StatusBadge key={source} tone={data.sourceStatus[source] === 'ready' ? 'success' : 'danger'}>
              {source}
              {' '}
              ·
              {t(`operationsDashboardPage.sourceStatus.${data.sourceStatus[source]}`)}
            </StatusBadge>
          ))}
        </div>
        <div className="flex items-center gap-2">
          {(['1h', '6h', '24h'] as const).map(item => <Button key={item} size="sm" variant={range === item ? 'default' : 'outline'} onClick={() => setRange(item)}>{item}</Button>)}
          <Button aria-label={t('common.refresh')} disabled={overview.isFetching} size="icon" variant="ghost" onClick={() => void overview.refetch()}><RefreshCw className={overview.isFetching ? 'animate-spin' : ''} /></Button>
        </div>
      </div>
      <MetricGroup>
        <MetricItem icon={<Activity className="size-4" />} label={t('operationsDashboardPage.runSuccess')} value={`${successRate.toFixed(1)}%`} tone={successRate >= 95 ? 'success' : successRate >= 85 ? 'warning' : 'danger'} />
        <MetricItem icon={<Bot className="size-4" />} label={t('operationsDashboardPage.activeRuns')} value={formatNumber(data.summary.activeRuns)} />
        <MetricItem icon={<Timer className="size-4" />} label={t('operationsDashboardPage.firstTokenP95')} value={formatSeconds(data.summary.firstTokenP95)} tone={(data.summary.firstTokenP95 ?? 0) > 5 ? 'warning' : 'neutral'} />
        <MetricItem icon={<CircleGauge className="size-4" />} label={t('operationsDashboardPage.modelErrorRate')} value={`${modelErrorRate.toFixed(1)}%`} tone={modelErrorRate > 10 ? 'danger' : modelErrorRate > 3 ? 'warning' : 'success'} />
        <MetricItem icon={<Clock3 className="size-4" />} label={t('operationsDashboardPage.outputTokenRate')} value={`${formatNumber(data.summary.outputTokenRate)}/s`} />
        <MetricItem icon={<Wrench className="size-4" />} label={t('operationsDashboardPage.externalErrorRate')} value={`${formatNumber(data.summary.externalErrorRate)}/s`} tone={(data.summary.externalErrorRate ?? 0) > 0 ? 'danger' : 'neutral'} />
      </MetricGroup>
      <div className="grid gap-4 xl:grid-cols-2">
        <Section title={t('operationsDashboardPage.runTrend')} description={t('operationsDashboardPage.runTrendDescription')} variant="bordered"><AgentObservabilityChart label={t('operationsDashboardPage.runTrend')} series={data.series.runSuccessRate ?? []} valueFormatter={value => `${value.toFixed(1)}%`} /></Section>
        <Section title={t('operationsDashboardPage.latencyTrend')} description={t('operationsDashboardPage.latencyTrendDescription')} variant="bordered"><AgentObservabilityChart label={t('operationsDashboardPage.latencyTrend')} series={[...(data.series.firstTokenP95 ?? []), ...(data.series.modelLatencyP95 ?? [])]} valueFormatter={formatSeconds} /></Section>
        <Section title={t('operationsDashboardPage.tokenTrend')} description={t('operationsDashboardPage.tokenTrendDescription')} variant="bordered"><AgentObservabilityChart label={t('operationsDashboardPage.tokenTrend')} series={data.series.tokenRate ?? []} /></Section>
        <Section title={t('operationsDashboardPage.toolFailures')} description={t('operationsDashboardPage.toolFailuresDescription')} variant="bordered"><AgentObservabilityChart label={t('operationsDashboardPage.toolFailures')} series={data.tools ?? []} /></Section>
      </div>
      <Section title={t('operationsDashboardPage.traceWorkspace')} description={t('operationsDashboardPage.traceWorkspaceDescription')}>
        <Tabs value={traceScope} onValueChange={value => setTraceScope(value as 'conversations' | 'turns')}>
          <TabsList>
            <TabsTrigger value="conversations">
              <MessagesSquare className="size-4" />
              {t('operationsDashboardPage.conversationScope')}
            </TabsTrigger>
            <TabsTrigger value="turns">
              <Network className="size-4" />
              {t('operationsDashboardPage.turnScope')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="conversations">
            {conversations.isError
              ? <ErrorState title={t('operationsDashboardPage.conversationsLoadFailed')} description={t('operationsDashboardPage.conversationsLoadFailedDescription')} />
              : (
                  <DataList
                    columns={conversationColumns}
                    emptyDescription={t('operationsDashboardPage.noConversationsDescription')}
                    emptyMode={conversationSearch ? 'filtered' : 'actionable'}
                    emptyTitle={t('operationsDashboardPage.noConversations')}
                    items={conversations.data?.items ?? []}
                    loading={conversations.isLoading}
                    pagination={{
                      page: conversations.data?.page ?? conversationPage,
                      pageSize: conversations.data?.pageSize ?? conversationPageSize,
                      total: conversations.data?.total ?? 0,
                      totalPages: conversations.data?.totalPages ?? 0,
                      pageInfoLabel: t('pagination.pageInfo', { page: conversations.data?.page ?? conversationPage, totalPages: conversations.data?.totalPages ?? 0, total: conversations.data?.total ?? 0 }),
                      onPageChange: setConversationPage,
                      onPageSizeChange: changeConversationPageSize,
                    }}
                    rowKey={item => item.id}
                    search={{ value: conversationSearch, placeholder: t('operationsDashboardPage.searchConversations'), onChange: changeConversationSearch }}
                  />
                )}
          </TabsContent>
          <TabsContent value="turns">
            <DataList items={data.traces ?? []} columns={traceColumns} rowKey={item => item.traceId} emptyTitle={t('operationsDashboardPage.noRuns')} emptyDescription={t('operationsDashboardPage.noRunsDescription')} />
          </TabsContent>
        </Tabs>
      </Section>
      <DataList items={data.logs ?? []} columns={logColumns} rowKey={item => `${item.timestamp}:${item.labels.trace_id ?? item.line}`} title={t('operationsDashboardPage.failureLogs')} emptyTitle={t('operationsDashboardPage.noFailures')} emptyDescription={t('operationsDashboardPage.noFailuresDescription')} />
      <AgentConversationDetailSheet conversation={selectedConversation} turnPage={conversationTurnPage} onOpenChange={open => !open && setSelectedConversation(null)} onTurnPageChange={setConversationTurnPage} onViewTrace={setSelectedTrace} />
      <AgentTraceDetailSheet trace={selectedTrace} onOpenChange={open => !open && setSelectedTrace(null)} />
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

function formatLokiTime(value: string, language: string) {
  try {
    return new Intl.DateTimeFormat(language, { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(Number(BigInt(value) / 1_000_000n)))
  }
  catch {
    return '—'
  }
}

function formatNanoTime(value: string, language: string) {
  try {
    return new Intl.DateTimeFormat(language, { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(Number(BigInt(value) / 1_000_000n)))
  }
  catch {
    return '—'
  }
}

function shortId(value?: string) {
  if (!value)
    return '—'
  return value.length > 16 ? `${value.slice(0, 8)}…${value.slice(-6)}` : value
}
