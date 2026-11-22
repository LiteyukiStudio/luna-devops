import type { RelationDialogMode, RelationSavedResult } from './project-topology-relation-dialog'
import type {
  Application,
  ProjectTopologyEdge,
  ProjectTopologyManualEdge,
  ProjectTopologyNode,
  ProjectTopologyOrigin,
  ServiceBinding,
} from '@/api'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, Plus, RefreshCw, SearchX } from 'lucide-react'
import { useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'
import { ProjectTopologyChart } from './project-topology-chart'
import { ProjectTopologyDetailSheet } from './project-topology-detail-sheet'
import { projectTopologyKeys, projectTopologyPageParams } from './project-topology-query'
import { ProjectTopologyRelationDialog } from './project-topology-relation-dialog'

const allOrigins: ProjectTopologyOrigin[] = ['service_binding', 'manual']

interface RelationDialogState {
  mode: RelationDialogMode
  manualEdge?: ProjectTopologyManualEdge
  serviceBinding?: ServiceBinding
}

interface ProjectTopologyPanelProps {
  applications: Application[]
  applicationsError?: boolean
  applicationsLoading?: boolean
  applicationsRetrying?: boolean
  canManage: boolean
  projectId: string
  onRetryApplications?: () => void
}

export function ProjectTopologyPanel({
  applications,
  applicationsError = false,
  applicationsLoading = false,
  applicationsRetrying = false,
  canManage,
  projectId,
  onRetryApplications,
}: ProjectTopologyPanelProps) {
  const { t } = useTranslation()
  const [stage, setStage] = useState('')
  const [origin, setOrigin] = useState<'all' | ProjectTopologyOrigin>('all')
  const [search, setSearch] = useState('')
  const [selectedEdgeId, setSelectedEdgeId] = useState('')
  const [dialog, setDialog] = useState<RelationDialogState | null>(null)
  const [pendingReleaseApplicationId, setPendingReleaseApplicationId] = useState('')
  const returnToEdgeIdRef = useRef('')
  const topology = useQuery({
    queryKey: projectTopologyKeys.graph(projectId, '', allOrigins),
    queryFn: () => api.getProjectTopology(projectId, { origins: allOrigins }),
    enabled: Boolean(projectId),
    refetchInterval: 60_000,
  })
  const serviceBindings = useQuery({
    queryKey: projectTopologyKeys.serviceBindings(projectId),
    queryFn: () => api.listServiceBindings(projectId, projectTopologyPageParams),
    enabled: Boolean(projectId && topology.data?.edges.some(edge => edge.origin === 'service_binding')),
  })
  const manualEdges = useQuery({
    queryKey: projectTopologyKeys.manualEdges(projectId),
    queryFn: () => api.listProjectTopologyEdges(projectId, projectTopologyPageParams),
    enabled: Boolean(projectId && topology.data?.edges.some(edge => edge.origin === 'manual')),
  })
  const stages = useMemo(() => uniqueStages(topology.data?.nodes ?? []), [topology.data?.nodes])
  const visibleEdges = useMemo(
    () => filterEdges(topology.data?.edges ?? [], topology.data?.nodes ?? [], stage, origin, search),
    [origin, search, stage, topology.data?.edges, topology.data?.nodes],
  )
  const visibleNodeIds = useMemo(() => new Set(visibleEdges.flatMap(edge => [edge.source, edge.target])), [visibleEdges])
  const visibleNodes = useMemo(
    () => (topology.data?.nodes ?? []).filter(node => visibleNodeIds.has(node.id)),
    [topology.data?.nodes, visibleNodeIds],
  )
  const selectedEdge = topology.data?.edges.find(edge => edge.id === selectedEdgeId)
  const selectedBinding = serviceBindings.data?.items.find(binding => binding.id === selectedEdgeId)
  const selectedManualEdge = manualEdges.data?.items.find(edge => edge.id === selectedEdgeId)
  const relationLoading = selectedEdge?.origin === 'service_binding'
    ? serviceBindings.isPending
    : selectedEdge?.origin === 'manual'
      ? manualEdges.isPending
      : false
  const relationError = selectedEdge?.origin === 'service_binding'
    ? serviceBindings.isError
    : selectedEdge?.origin === 'manual'
      ? manualEdges.isError
      : false
  const relationRetrying = selectedEdge?.origin === 'service_binding'
    ? serviceBindings.isFetching
    : selectedEdge?.origin === 'manual'
      ? manualEdges.isFetching
      : false
  const retryRelation = () => {
    if (selectedEdge?.origin === 'service_binding')
      void serviceBindings.refetch()
    else if (selectedEdge?.origin === 'manual')
      void manualEdges.refetch()
  }
  const managementListTruncated = (serviceBindings.data?.total ?? 0) > (serviceBindings.data?.items.length ?? 0)
    || (manualEdges.data?.total ?? 0) > (manualEdges.data?.items.length ?? 0)

  const selectEdge = useCallback((edgeId: string) => setSelectedEdgeId(edgeId), [])
  const refresh = () => {
    void topology.refetch()
    if (serviceBindings.data)
      void serviceBindings.refetch()
    if (manualEdges.data)
      void manualEdges.refetch()
  }
  const handleSaved = (result: RelationSavedResult) => {
    returnToEdgeIdRef.current = ''
    if (result.requiresRedeploy)
      setPendingReleaseApplicationId(result.sourceApplicationId)
  }
  const closeDialog = (open: boolean) => {
    if (open)
      return
    setDialog(null)
    if (returnToEdgeIdRef.current) {
      setSelectedEdgeId(returnToEdgeIdRef.current)
      returnToEdgeIdRef.current = ''
    }
  }
  const openEdit = () => {
    returnToEdgeIdRef.current = selectedEdgeId
    if (selectedBinding)
      setDialog({ mode: 'service_binding', serviceBinding: selectedBinding })
    else if (selectedManualEdge)
      setDialog({ mode: 'manual', manualEdge: selectedManualEdge })
    setSelectedEdgeId('')
  }

  if (topology.isPending)
    return <ToolViewportSkeleton />

  if (topology.isError) {
    return (
      <ErrorState
        description={t('projectTopology.loadFailedDescription')}
        retrying={topology.isFetching}
        title={t('projectTopology.loadFailed')}
        onRetry={() => void topology.refetch()}
      />
    )
  }

  if (!topology.data?.edges.length) {
    return (
      <>
        <Card className="grid min-h-72 place-items-center p-6">
          <EmptyState
            actions={canManage
              ? (
                  <Button type="button" onClick={() => setDialog({ mode: 'service_binding' })}>
                    <Plus className="size-4" />
                    {t('projectTopology.addRelation')}
                  </Button>
                )
              : undefined}
            description={t(canManage ? 'projectTopology.emptyDescription' : 'projectTopology.emptyReadOnly')}
            title={t('projectTopology.emptyTitle')}
            variant="plain"
          />
        </Card>
        {dialog && (
          <ProjectTopologyRelationDialog
            key={`${dialog.mode}-${dialog.serviceBinding?.id ?? dialog.manualEdge?.id ?? 'new'}`}
            applications={applications}
            applicationsError={applicationsError}
            applicationsLoading={applicationsLoading}
            applicationsRetrying={applicationsRetrying}
            editingManualEdge={dialog.manualEdge}
            editingServiceBinding={dialog.serviceBinding}
            initialMode={dialog.mode}
            projectId={projectId}
            onOpenChange={closeDialog}
            onRetryApplications={onRetryApplications}
            onSaved={handleSaved}
          />
        )}
      </>
    )
  }

  return (
    <div className="grid gap-3">
      {pendingReleaseApplicationId && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-container border border-warning-border bg-warning-subtle px-3 py-2 text-sm text-warning">
          <span>{t('projectTopology.savedNeedsRelease')}</span>
          <Link className="font-medium text-primary-text hover:underline" to={`/projects/${projectId}/apps/${pendingReleaseApplicationId}?tab=deployments`}>
            {t('projectTopology.openDeployment')}
          </Link>
        </div>
      )}
      {topology.data.warnings.length > 0 && (
        <div className="flex items-start gap-2 rounded-container border border-border bg-surface-subtle px-3 py-2 text-sm text-muted-foreground">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" />
          <span>{topology.data.warnings.map(warning => t(`projectTopology.warningCodes.${warning.code}`, { defaultValue: warning.code })).join(' · ')}</span>
        </div>
      )}
      {managementListTruncated && <p className="text-xs text-muted-foreground">{t('projectTopology.truncatedWarning')}</p>}
      <Card className="overflow-hidden">
        <div className="flex flex-wrap items-center gap-2 border-b border-border p-3">
          <Select value={toSelectValue(stage)} onValueChange={next => setStage(fromSelectValue(next))}>
            <SelectTrigger aria-label={t('projectTopology.stageFilter')} className="min-w-36 text-xs" size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={EMPTY_SELECT_VALUE}>{t('projectTopology.allStages')}</SelectItem>
              {stages.map(value => (
                <SelectItem key={value} value={value}>
                  {t(`deploymentsPage.stageLabels.${value}`, { defaultValue: value })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            aria-label={t('projectTopology.searchRelations')}
            className="h-8 w-full @lg/workspace:w-52"
            placeholder={t('projectTopology.searchRelations')}
            type="search"
            value={search}
            onChange={event => setSearch(event.target.value)}
          />
          <Select value={origin} onValueChange={next => setOrigin(next as typeof origin)}>
            <SelectTrigger aria-label={t('projectTopology.originFilter')} className="min-w-40 text-xs" size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('projectTopology.allOrigins')}</SelectItem>
              <SelectItem value="service_binding">{t('projectTopology.serviceBindings')}</SelectItem>
              <SelectItem value="manual">{t('projectTopology.manualRelations')}</SelectItem>
            </SelectContent>
          </Select>
          <span className="text-xs text-muted-foreground">{t('projectTopology.relationCount', { count: visibleEdges.length })}</span>
          <div className="ml-auto flex items-center gap-1">
            {canManage && (
              <Button
                size="sm"
                type="button"
                onClick={() => {
                  returnToEdgeIdRef.current = ''
                  setDialog({ mode: 'service_binding' })
                }}
              >
                <Plus className="size-4" />
                {t('projectTopology.addRelation')}
              </Button>
            )}
            <Button aria-label={t('projectTopology.refresh')} className="size-8" disabled={topology.isFetching} size="icon" variant="ghost" onClick={refresh}>
              <RefreshCw className={topology.isFetching ? 'size-4 motion-safe:animate-spin' : 'size-4'} />
            </Button>
          </div>
        </div>
        {visibleEdges.length > 0
          ? (
              <>
                <div className="hidden @xl/workspace:block">
                  <ProjectTopologyChart edges={visibleEdges} nodes={visibleNodes} onSelectEdge={selectEdge} />
                </div>
                <div className="grid gap-2 p-3 @xl/workspace:hidden">
                  {visibleEdges.map(edge => (
                    <MobileRelationItem
                      key={edge.id}
                      edge={edge}
                      nodes={topology.data.nodes}
                      onSelect={() => setSelectedEdgeId(edge.id)}
                    />
                  ))}
                </div>
              </>
            )
          : (
              <EmptyState
                actions={(
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      setStage('')
                      setOrigin('all')
                      setSearch('')
                    }}
                  >
                    {t('projectTopology.clearFilters')}
                  </Button>
                )}
                description={t('projectTopology.noFilteredRelations')}
                icon={<SearchX className="size-5" />}
                mode="filtered"
                title={t('projectTopology.listView')}
                variant="plain"
              />
            )}
      </Card>

      <ProjectTopologyDetailSheet
        canManage={canManage}
        edge={selectedEdge}
        manualEdge={selectedManualEdge}
        nodes={topology.data.nodes}
        projectId={projectId}
        relationError={relationError}
        relationLoading={relationLoading}
        relationRetrying={relationRetrying}
        serviceBinding={selectedBinding}
        onEdit={openEdit}
        onOpenChange={open => !open && setSelectedEdgeId('')}
        onRetryRelation={retryRelation}
      />
      {dialog && (
        <ProjectTopologyRelationDialog
          key={`${dialog.mode}-${dialog.serviceBinding?.id ?? dialog.manualEdge?.id ?? 'new'}`}
          applications={applications}
          applicationsError={applicationsError}
          applicationsLoading={applicationsLoading}
          applicationsRetrying={applicationsRetrying}
          editingManualEdge={dialog.manualEdge}
          editingServiceBinding={dialog.serviceBinding}
          initialMode={dialog.mode}
          projectId={projectId}
          onOpenChange={closeDialog}
          onRetryApplications={onRetryApplications}
          onSaved={handleSaved}
        />
      )}
    </div>
  )
}

function MobileRelationItem({ edge, nodes, onSelect }: { edge: ProjectTopologyEdge, nodes: ProjectTopologyNode[], onSelect: () => void }) {
  const { t } = useTranslation()
  const source = nodes.find(node => node.id === edge.source)?.name ?? t('projectTopology.unknownApplication')
  const target = nodes.find(node => node.id === edge.target)?.name ?? t('projectTopology.unknownApplication')
  return (
    <button className="grid min-w-0 gap-2 rounded-container border border-border bg-surface p-3 text-left transition-colors hover:border-primary-border hover:bg-surface-subtle" type="button" onClick={onSelect}>
      <div className="flex min-w-0 items-start justify-between gap-3">
        <span className="min-w-0 truncate font-medium">{t('projectTopology.direction', { source, target })}</span>
        <StatusValueBadge labelKeyPrefix="projectTopology.statuses" value={edge.status || 'unknown'} />
      </div>
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>{t(`projectTopology.origins.${edge.origin}`)}</span>
        <span>·</span>
        <span>{t(`projectTopology.relationTypes.${edge.relationType}`)}</span>
        {(edge.protocol || edge.port) && (
          <span>
            ·
            {[edge.protocol?.toUpperCase(), edge.port].filter(Boolean).join(' ')}
          </span>
        )}
      </div>
    </button>
  )
}

function uniqueStages(nodes: ProjectTopologyNode[]) {
  return [...new Set(nodes.flatMap(node => node.deploymentTargets.map(target => target.stage)).filter(Boolean))].sort()
}

function filterEdges(edges: ProjectTopologyEdge[], nodes: ProjectTopologyNode[], stage: string, origin: 'all' | ProjectTopologyOrigin, search: string) {
  const nodeById = new Map(nodes.map(node => [node.id, node]))
  const normalizedSearch = search.trim().toLocaleLowerCase()
  return edges.filter((edge) => {
    if (origin !== 'all' && edge.origin !== origin)
      return false
    if (normalizedSearch) {
      const source = nodeById.get(edge.source)
      const target = nodeById.get(edge.target)
      const haystack = [source?.name, source?.identifier, target?.name, target?.identifier, edge.protocol, edge.relationType]
        .filter(Boolean)
        .join(' ')
        .toLocaleLowerCase()
      if (!haystack.includes(normalizedSearch))
        return false
    }
    if (!stage)
      return true
    const sourceTarget = nodeById.get(edge.source)?.deploymentTargets.find(target => target.id === edge.sourceDeploymentTargetId)
    const targetTarget = nodeById.get(edge.target)?.deploymentTargets.find(target => target.id === edge.targetDeploymentTargetId)
    if (!sourceTarget && !targetTarget)
      return true
    return sourceTarget?.stage === stage || targetTarget?.stage === stage
  })
}
