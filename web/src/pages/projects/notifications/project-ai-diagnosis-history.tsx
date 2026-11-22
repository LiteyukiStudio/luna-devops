import type { TFunction } from 'i18next'
import type { ProjectAIDiagnosisCategory, ProjectAIDiagnosisRecord, ProjectAIDiagnosisStatus } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, ExternalLink, History, LoaderCircle, RefreshCw, Sparkles } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { diagnosisFailureLabel } from '@/components/common/ai-delivery-diagnosis-utils'
import { DataList } from '@/components/common/data-list'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime, formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { statusRefetchInterval } from '@/lib/polling'
import { cn } from '@/lib/utils'

const ALL_STATUS = '__all__' as const
type CategoryFilter = 'all' | ProjectAIDiagnosisCategory
type StatusFilter = typeof ALL_STATUS | ProjectAIDiagnosisStatus

function formatArgScalar(value: unknown): string {
  if (value === undefined)
    return '-'
  if (typeof value === 'string')
    return value || '""'
  if (typeof value === 'number' || typeof value === 'boolean' || value === null)
    return String(value)
  return JSON.stringify(value)
}

function subjectTypeLabel(t: TFunction, subjectType: ProjectAIDiagnosisRecord['subjectType']): string {
  switch (subjectType) {
    case 'build_run':
      return t('aiAssistant.projectAutomation.history.typeBuildRun')
    case 'release':
      return t('aiAssistant.projectAutomation.history.typeRelease')
    case 'runtime_incident':
      return t('aiAssistant.projectAutomation.history.typeRuntimeIncident')
  }
}

function triggerCodeLabel(
  t: TFunction,
  subjectType: ProjectAIDiagnosisRecord['subjectType'],
  code: string,
): string {
  const trimmed = code.trim()
  if (!trimmed)
    return ''
  if (subjectType === 'build_run') {
    switch (trimmed) {
      case 'canceled_by_user': return t('buildsPage.failureCodes.canceled_by_user')
      case 'superseded': return t('buildsPage.failureCodes.superseded')
      case 'timeout': return t('buildsPage.failureCodes.timeout')
      case 'queue_timeout': return t('buildsPage.failureCodes.queue_timeout')
      case 'dispatch_failed': return t('buildsPage.failureCodes.dispatch_failed')
      case 'project_unavailable': return t('buildsPage.failureCodes.project_unavailable')
      case 'deployment_target_unavailable': return t('buildsPage.failureCodes.deployment_target_unavailable')
      case 'cluster_unavailable': return t('buildsPage.failureCodes.cluster_unavailable')
      case 'capacity_check_failed': return t('buildsPage.failureCodes.capacity_check_failed')
      case 'namespace_failed': return t('buildsPage.failureCodes.namespace_failed')
      case 'task_resolve_failed': return t('buildsPage.failureCodes.task_resolve_failed')
      case 'job_start_failed': return t('buildsPage.failureCodes.job_start_failed')
      case 'job_failed': return t('buildsPage.failureCodes.job_failed')
      case 'observation_failed': return t('buildsPage.failureCodes.observation_failed')
      case 'internal_error': return t('buildsPage.failureCodes.internal_error')
      default: return ''
    }
  }
  if (subjectType === 'release') {
    switch (trimmed) {
      case 'canceled_by_user': return t('deploymentsPage.releaseFailureCodes.canceled_by_user')
      case 'timeout': return t('deploymentsPage.releaseFailureCodes.timeout')
      case 'dispatch_failed': return t('deploymentsPage.releaseFailureCodes.dispatch_failed')
      case 'project_unavailable': return t('deploymentsPage.releaseFailureCodes.project_unavailable')
      case 'application_unavailable': return t('deploymentsPage.releaseFailureCodes.application_unavailable')
      case 'deployment_target_unavailable': return t('deploymentsPage.releaseFailureCodes.deployment_target_unavailable')
      case 'namespace_failed': return t('deploymentsPage.releaseFailureCodes.namespace_failed')
      case 'service_binding_failed': return t('deploymentsPage.releaseFailureCodes.service_binding_failed')
      case 'preflight_failed': return t('deploymentsPage.releaseFailureCodes.preflight_failed')
      case 'volume_not_ready': return t('deploymentsPage.releaseFailureCodes.volume_not_ready')
      case 'runtime_config_failed': return t('deploymentsPage.releaseFailureCodes.runtime_config_failed')
      case 'pre_hook_failed': return t('deploymentsPage.releaseFailureCodes.pre_hook_failed')
      case 'dependencies_failed': return t('deploymentsPage.releaseFailureCodes.dependencies_failed')
      case 'apply_failed': return t('deploymentsPage.releaseFailureCodes.apply_failed')
      case 'rollout_failed': return t('deploymentsPage.releaseFailureCodes.rollout_failed')
      case 'rollout_timeout': return t('deploymentsPage.releaseFailureCodes.rollout_timeout')
      case 'volume_mount_failed': return t('deploymentsPage.releaseFailureCodes.volume_mount_failed')
      case 'post_hook_failed': return t('deploymentsPage.releaseFailureCodes.post_hook_failed')
      case 'superseded': return t('deploymentsPage.releaseFailureCodes.superseded')
      case 'internal_error': return t('deploymentsPage.releaseFailureCodes.internal_error')
      default: return ''
    }
  }
  switch (trimmed) {
    case 'runtime.pod_oom_killed': return t('inbox.catalog.runtimeOomKilled.title')
    case 'runtime.pod_crash_loop': return t('inbox.catalog.runtimeCrashLoop.title')
    case 'runtime.pod_start_failed': return t('inbox.catalog.runtimeStartFailed.title')
    case 'runtime.workload_degraded': return t('inbox.catalog.runtimeDegraded.title')
    default: return ''
  }
}

export function ProjectAIDiagnosisHistorySection({ projectId }: { projectId: string }) {
  const { t } = useTranslation()
  const [category, setCategory] = useState<CategoryFilter>('all')
  const [status, setStatus] = useState<StatusFilter>(ALL_STATUS)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [selectedSnapshot, setSelectedSnapshot] = useState<ProjectAIDiagnosisRecord | null>(null)

  const statusParam: ProjectAIDiagnosisStatus | undefined = status === ALL_STATUS ? undefined : status
  const hasFilters = category !== 'all' || status !== ALL_STATUS

  const historyQuery = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-ai-diagnoses', projectId, page, pageSize, category, statusParam ?? 'all'],
    queryFn: ({ signal }) => api.listProjectAIDiagnoses(
      projectId,
      {
        category,
        status: statusParam,
        page,
        pageSize,
        sortBy: 'createdAt',
        sortOrder: 'desc',
      },
      signal,
    ),
    enabled: Boolean(projectId),
    refetchInterval: query => statusRefetchInterval(
      (query.state.data?.items ?? []).some(item => item.status === 'running'),
    ),
  })

  const items = useMemo(() => historyQuery.data?.items ?? [], [historyQuery.data?.items])
  const selectedRecord = useMemo(
    () => selectedSnapshot ? (items.find(item => item.id === selectedSnapshot.id) ?? selectedSnapshot) : null,
    [items, selectedSnapshot],
  )
  const dedupedSuggestions = useMemo(
    () => selectedRecord ? [...new Set(selectedRecord.suggestions.filter(Boolean))] : [],
    [selectedRecord],
  )

  const columns = useMemo<DataListColumn<ProjectAIDiagnosisRecord>[]>(() => [
    {
      key: 'subject',
      header: t('aiAssistant.projectAutomation.history.colSubject'),
      width: 'normal',
      maxWidth: 240,
      render: (record) => {
        const targetText = [record.applicationName, record.deploymentTargetName]
          .filter(Boolean)
          .join(' / ') || record.subjectId
        const localizedReason = triggerCodeLabel(t, record.subjectType, record.triggerCode)
        return (
          <div className="grid min-w-0 gap-0.5">
            <span className="truncate text-sm font-medium text-foreground" title={targetText}>
              {targetText}
            </span>
            <span className="truncate text-xs text-muted-foreground">
              {subjectTypeLabel(t, record.subjectType)}
              {localizedReason ? ` · ${localizedReason}` : ''}
            </span>
          </div>
        )
      },
    },
    {
      key: 'status',
      header: t('aiAssistant.projectAutomation.history.colStatus'),
      width: 'status',
      render: (record) => {
        if (record.status === 'running') {
          return (
            <StatusBadge tone="info">
              <LoaderCircle aria-hidden="true" className="size-3 animate-spin motion-reduce:animate-none" />
              {t('aiAssistant.projectAutomation.history.statusRunning')}
            </StatusBadge>
          )
        }
        if (record.status === 'succeeded') {
          return <StatusBadge tone="success">{t('aiAssistant.projectAutomation.history.statusSucceeded')}</StatusBadge>
        }
        return <StatusBadge tone="danger">{t('aiAssistant.projectAutomation.history.statusFailed')}</StatusBadge>
      },
    },
    {
      key: 'selfHeal',
      header: t('aiAssistant.projectAutomation.history.colSelfHeal'),
      mobile: 'hidden',
      width: 'status',
      render: (record) => {
        if (record.category !== 'runtime')
          return <span className="text-xs text-muted-foreground">-</span>
        switch (record.autoHealStatus) {
          case 'succeeded':
            return <StatusBadge tone="success">{t('aiAssistant.projectAutomation.history.healSucceeded')}</StatusBadge>
          case 'circuit_broken':
            return <StatusBadge tone="danger">{t('aiAssistant.projectAutomation.history.healCircuitBroken')}</StatusBadge>
          case 'failed':
            return <StatusBadge tone="warning">{t('aiAssistant.projectAutomation.history.healFailed')}</StatusBadge>
          default:
            return <StatusBadge tone="neutral">{t('aiAssistant.projectAutomation.history.healNone')}</StatusBadge>
        }
      },
    },
    {
      key: 'summary',
      header: t('aiAssistant.projectAutomation.history.colSummary'),
      width: 'primary',
      render: (record) => {
        if (record.status === 'running') {
          return <span className="text-xs text-muted-foreground">{t('aiAssistant.deliveryDiagnosis.running')}</span>
        }
        if (record.status === 'failed') {
          return <span className="line-clamp-2 text-xs text-destructive">{diagnosisFailureLabel(t, record.errorCode)}</span>
        }
        return (
          <p className="line-clamp-2 text-xs leading-relaxed text-muted-foreground" title={record.rootCauseSummary}>
            {record.rootCauseSummary || '-'}
          </p>
        )
      },
    },
    {
      key: 'createdAt',
      header: t('aiAssistant.projectAutomation.history.colTime'),
      mobile: 'hidden',
      width: 'compact',
      render: record => (
        <span className="whitespace-nowrap text-xs text-muted-foreground">
          {formatSmartDateTime(record.createdAt, t)}
        </span>
      ),
    },
  ], [t])

  const clearFilters = () => {
    setCategory('all')
    setStatus(ALL_STATUS)
    setPage(1)
  }

  const selectedReasonLabel = selectedRecord
    ? triggerCodeLabel(t, selectedRecord.subjectType, selectedRecord.triggerCode)
    : ''

  return (
    <Section
      description={t('aiAssistant.projectAutomation.history.description')}
      icon={<History className="size-4" />}
      title={t('aiAssistant.projectAutomation.history.title')}
      tools={(
        <Button
          disabled={historyQuery.isFetching}
          size="sm"
          type="button"
          variant="outline"
          onClick={() => void historyQuery.refetch()}
        >
          <RefreshCw className={cn('size-3.5', historyQuery.isFetching && 'animate-spin motion-reduce:animate-none')} />
          {t('common.refresh')}
        </Button>
      )}
    >
      <DataList
        columns={columns}
        emptyActions={hasFilters
          ? (
              <Button size="sm" type="button" variant="outline" onClick={clearFilters}>
                {t('aiAssistant.projectAutomation.history.clearFilters')}
              </Button>
            )
          : undefined}
        emptyDescription={hasFilters
          ? t('aiAssistant.projectAutomation.history.filteredEmptyDescription')
          : t('aiAssistant.projectAutomation.history.emptyDescription')}
        emptyIcon={<Sparkles className="size-5" />}
        emptyMode={hasFilters ? 'filtered' : 'actionable'}
        emptyTitle={hasFilters
          ? t('aiAssistant.projectAutomation.history.filteredEmptyTitle')
          : t('aiAssistant.projectAutomation.history.emptyTitle')}
        error={historyQuery.isError
          ? {
              description: historyQuery.error instanceof Error ? historyQuery.error.message : t('notificationsPage.loadFailedDescription'),
              retrying: historyQuery.isFetching,
              title: t('notificationsPage.loadFailedTitle'),
              onRetry: () => void historyQuery.refetch(),
            }
          : undefined}
        items={items}
        loading={historyQuery.isPending}
        pagination={historyQuery.data && historyQuery.data.total > 0
          ? {
              page,
              pageSize,
              pageSizeOptions: [10, 20, 50],
              total: historyQuery.data.total,
              totalPages: historyQuery.data.totalPages,
              pageInfoLabel: t('pagination.pageInfo', {
                page,
                totalPages: historyQuery.data.totalPages,
                total: historyQuery.data.total,
              }),
              onPageChange: setPage,
              onPageSizeChange: (nextSize) => {
                setPageSize(nextSize)
                setPage(1)
              },
            }
          : undefined}
        rowActionLabel={() => t('aiAssistant.projectAutomation.history.viewDetailAction')}
        rowKey={item => item.id}
        toolbar={(
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={category}
              onValueChange={(next: CategoryFilter) => {
                setCategory(next)
                setPage(1)
              }}
            >
              <SelectTrigger
                aria-label={t('aiAssistant.projectAutomation.history.filterCategoryLabel')}
                className="h-8 w-36 text-xs"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('aiAssistant.projectAutomation.history.categoryAll')}</SelectItem>
                <SelectItem value="delivery">{t('aiAssistant.projectAutomation.history.categoryDelivery')}</SelectItem>
                <SelectItem value="runtime">{t('aiAssistant.projectAutomation.history.categoryRuntime')}</SelectItem>
              </SelectContent>
            </Select>

            <Select
              value={status}
              onValueChange={(next: StatusFilter) => {
                setStatus(next)
                setPage(1)
              }}
            >
              <SelectTrigger
                aria-label={t('aiAssistant.projectAutomation.history.filterStatusLabel')}
                className="h-8 w-32 text-xs"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL_STATUS}>{t('aiAssistant.projectAutomation.history.statusAll')}</SelectItem>
                <SelectItem value="running">{t('aiAssistant.projectAutomation.history.statusRunning')}</SelectItem>
                <SelectItem value="succeeded">{t('aiAssistant.projectAutomation.history.statusSucceeded')}</SelectItem>
                <SelectItem value="failed">{t('aiAssistant.projectAutomation.history.statusFailed')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}
        onRowClick={record => setSelectedSnapshot(record)}
      />

      <Sheet open={Boolean(selectedRecord)} onOpenChange={open => !open && setSelectedSnapshot(null)}>
        <SheetContent className="w-[min(92vw,34rem)] max-w-none gap-0 overflow-y-auto p-0 sm:max-w-none" side="right">
          <SheetHeader className="border-b border-border p-6 pr-14">
            <SheetTitle>{t('aiAssistant.projectAutomation.history.detailTitle')}</SheetTitle>
            <SheetDescription>
              {selectedRecord
                ? `${subjectTypeLabel(t, selectedRecord.subjectType)} · ${formatAbsoluteDateTime(selectedRecord.createdAt)}`
                : ''}
            </SheetDescription>
          </SheetHeader>
          {selectedRecord && (
            <div className="space-y-4 p-6 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <StatusBadge tone={selectedRecord.status === 'succeeded' ? 'success' : selectedRecord.status === 'running' ? 'info' : 'danger'}>
                  {selectedRecord.status === 'succeeded'
                    ? t('aiAssistant.projectAutomation.history.statusSucceeded')
                    : selectedRecord.status === 'running'
                      ? t('aiAssistant.projectAutomation.history.statusRunning')
                      : t('aiAssistant.projectAutomation.history.statusFailed')}
                </StatusBadge>
                {selectedRecord.status === 'succeeded' && (
                  <StatusBadge tone="neutral">
                    {selectedRecord.confidence === 'high'
                      ? t('aiAssistant.deliveryDiagnosis.confidenceHigh')
                      : selectedRecord.confidence === 'low'
                        ? t('aiAssistant.deliveryDiagnosis.confidenceLow')
                        : t('aiAssistant.deliveryDiagnosis.confidenceMedium')}
                  </StatusBadge>
                )}
                {selectedRecord.category === 'runtime' && selectedRecord.autoHealStatus && selectedRecord.autoHealStatus !== 'none' && (
                  <StatusBadge tone={selectedRecord.autoHealStatus === 'succeeded' ? 'success' : selectedRecord.autoHealStatus === 'circuit_broken' ? 'danger' : 'warning'}>
                    {selectedRecord.autoHealStatus === 'succeeded'
                      ? t('aiAssistant.projectAutomation.history.healSucceeded')
                      : selectedRecord.autoHealStatus === 'circuit_broken'
                        ? t('aiAssistant.projectAutomation.history.healCircuitBroken')
                        : t('aiAssistant.projectAutomation.history.healFailed')}
                  </StatusBadge>
                )}
              </div>

              {(selectedRecord.applicationName || selectedRecord.deploymentTargetName || selectedReasonLabel) && (
                <div className="rounded-control border border-border bg-surface-subtle/40 px-3 py-2 text-xs text-muted-foreground">
                  {[
                    [selectedRecord.applicationName, selectedRecord.deploymentTargetName].filter(Boolean).join(' / '),
                    selectedReasonLabel,
                  ].filter(Boolean).join(' · ')}
                </div>
              )}

              {selectedRecord.circuitBreakerTripped && (
                <div className="flex items-start gap-2 rounded-control border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive" role="alert">
                  <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
                  <span>{t('inbox.selfHeal.circuitBreakerBanner')}</span>
                </div>
              )}

              {selectedRecord.status === 'running' && (
                <p className="text-xs text-muted-foreground">{t('aiAssistant.deliveryDiagnosis.running')}</p>
              )}

              {selectedRecord.status === 'failed' && (
                <p className="text-xs text-destructive">{diagnosisFailureLabel(t, selectedRecord.errorCode)}</p>
              )}

              {selectedRecord.rootCauseSummary && (
                <div className="space-y-1.5 rounded-control border border-border bg-surface-subtle/50 p-3.5">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
                    <Sparkles aria-hidden="true" className="size-3.5 text-primary" />
                    <span>{t('inbox.selfHeal.aiDiagnosisTitle')}</span>
                  </div>
                  <p className="whitespace-pre-wrap break-words text-xs leading-relaxed text-foreground">
                    {selectedRecord.rootCauseSummary}
                  </p>
                </div>
              )}

              {dedupedSuggestions.length > 0 && (
                <div className="space-y-1.5">
                  <div className="text-xs font-medium text-foreground">{t('aiAssistant.deliveryDiagnosis.suggestionsTitle')}</div>
                  <ul className="list-disc space-y-1 pl-4 text-xs leading-relaxed text-muted-foreground">
                    {dedupedSuggestions.map(item => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                </div>
              )}

              {selectedRecord.autoHealSummary && (
                <div className="rounded-control border border-border bg-surface-subtle/40 p-3 text-xs">
                  <div className="font-medium text-foreground">{t('inbox.selfHeal.autoHealExecutedTitle')}</div>
                  <p className="mt-1 whitespace-pre-wrap break-words text-muted-foreground">{selectedRecord.autoHealSummary}</p>
                </div>
              )}

              {selectedRecord.proposedAction && (
                <div className="space-y-2 rounded-control border border-border bg-background p-3 text-xs">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="font-semibold text-foreground">{t('aiAssistant.deliveryDiagnosis.proposedActionTitle')}</span>
                    <StatusBadge tone="info">{toolDisplayName(t, selectedRecord.proposedAction.toolName)}</StatusBadge>
                  </div>
                  {selectedRecord.proposedAction.summary && (
                    <p className="text-muted-foreground">{selectedRecord.proposedAction.summary}</p>
                  )}
                  {Object.keys(selectedRecord.proposedAction.arguments ?? {}).length > 0 && (
                    <dl className="grid gap-1 rounded-control border border-border bg-surface-subtle/40 p-2.5">
                      {Object.entries(selectedRecord.proposedAction.arguments).map(([k, v]) => (
                        <div className="flex items-baseline justify-between gap-3" key={k}>
                          <dt className="font-mono text-muted-foreground">{k}</dt>
                          <dd className="min-w-0 break-all font-mono text-foreground">{formatArgScalar(v)}</dd>
                        </div>
                      ))}
                    </dl>
                  )}
                </div>
              )}

              {selectedRecord.deepLink && (
                <div className="flex justify-end pt-2">
                  <Button asChild size="sm" variant="outline">
                    <Link to={selectedRecord.deepLink}>
                      <ExternalLink className="size-3.5" />
                      {t('aiAssistant.projectAutomation.history.openTargetAction')}
                    </Link>
                  </Button>
                </div>
              )}
            </div>
          )}
        </SheetContent>
      </Sheet>
    </Section>
  )
}
