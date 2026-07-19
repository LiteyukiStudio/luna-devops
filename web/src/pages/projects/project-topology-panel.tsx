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
import { AlertTriangle, Focus, Plus, RefreshCw } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
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
  canManage: boolean
  projectId: string
}

export function ProjectTopologyPanel({ applications, canManage, projectId }: ProjectTopologyPanelProps) {
  const { t } = useTranslation()
  const [stage, setStage] = useState('')
  const [origin, setOrigin] = useState<'all' | ProjectTopologyOrigin>('all')
  const [search, setSearch] = useState('')
  const [fitVersion, setFitVersion] = useState(0)
  const [selectedEdgeId, setSelectedEdgeId] = useState('')
  const [dialog, setDialog] = useState<RelationDialogState | null>(null)
  const [pendingReleaseApplicationId, setPendingReleaseApplicationId] = useState('')
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
    if (result.requiresRedeploy)
      setPendingReleaseApplicationId(result.sourceApplicationId)
  }
  const closeDialog = (open: boolean) => {
    if (open)
      return
    setDialog(null)
  }
  const openEdit = () => {
    if (selectedBinding)
      setDialog({ mode: 'service_binding', serviceBinding: selectedBinding })
    else if (selectedManualEdge)
      setDialog({ mode: 'manual', manualEdge: selectedManualEdge })
    setSelectedEdgeId('')
  }

  if (topology.isLoading)
    return <div className="grid min-h-80 place-items-center text-sm text-muted-foreground">{t('common.loading')}</div>

  if (topology.isError) {
    return (
      <div className="grid gap-3">
        <ErrorState title={t('projectTopology.loadFailed')} description={t('projectTopology.loadFailedDescription')} />
        <Button className="justify-self-start" variant="outline" onClick={() => topology.refetch()}>{t('common.retry')}</Button>
      </div>
    )
  }

  if (!topology.data?.edges.length) {
    return (
      <>
        <Card className="grid min-h-72 place-items-center p-6">
          <EmptyState
            actions={canManage
              ? (
                  <Button onClick={() => setDialog({ mode: 'service_binding' })}>
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
            editingManualEdge={dialog.manualEdge}
            editingServiceBinding={dialog.serviceBinding}
            initialMode={dialog.mode}
            projectId={projectId}
            onOpenChange={closeDialog}
            onSaved={handleSaved}
          />
        )}
      </>
    )
  }

  return (
    <div className="grid gap-3">
      {pendingReleaseApplicationId && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/35 dark:text-amber-200">
          <span>{t('projectTopology.savedNeedsRelease')}</span>
          <Link className="font-medium text-primary-text hover:underline" to={`/projects/${projectId}/apps/${pendingReleaseApplicationId}?tab=deployments`}>
            {t('projectTopology.openDeployment')}
          </Link>
        </div>
      )}
      {topology.data.warnings.length > 0 && (
        <div className="flex items-start gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" />
          <span>{topology.data.warnings.map(warning => t(`projectTopology.warningCodes.${warning.code}`, { defaultValue: warning.code })).join(' · ')}</span>
        </div>
      )}
      {managementListTruncated && <p className="text-xs text-muted-foreground">{t('projectTopology.truncatedWarning')}</p>}
      <Card className="overflow-hidden">
        <div className="flex flex-wrap items-center gap-2 border-b border-border p-3">
          <NativeSelect
            aria-label={t('projectTopology.stageFilter')}
            className="min-w-36"
            containerClassName="w-auto"
            value={stage}
            onChange={event => setStage(event.target.value)}
          >
            <option value="">{t('projectTopology.allStages')}</option>
            {stages.map(value => <option key={value} value={value}>{t(`deploymentsPage.stageLabels.${value}`, { defaultValue: value })}</option>)}
          </NativeSelect>
          <Input
            aria-label={t('projectTopology.searchRelations')}
            className="w-full sm:w-52 md:hidden"
            placeholder={t('projectTopology.searchRelations')}
            type="search"
            value={search}
            onChange={event => setSearch(event.target.value)}
          />
          <NativeSelect
            aria-label={t('projectTopology.originFilter')}
            className="min-w-40"
            containerClassName="w-auto"
            value={origin}
            onChange={event => setOrigin(event.target.value as typeof origin)}
          >
            <option value="all">{t('projectTopology.allOrigins')}</option>
            <option value="service_binding">{t('projectTopology.serviceBindings')}</option>
            <option value="manual">{t('projectTopology.manualRelations')}</option>
          </NativeSelect>
          <span className="text-xs text-muted-foreground">{t('projectTopology.relationCount', { count: visibleEdges.length })}</span>
          <div className="ml-auto flex items-center gap-1">
            {canManage && (
              <Button size="sm" onClick={() => setDialog({ mode: 'service_binding' })}>
                <Plus className="size-4" />
                {t('projectTopology.addRelation')}
              </Button>
            )}
            <Button aria-label={t('projectTopology.fit')} className="hidden md:inline-flex" size="icon" variant="ghost" onClick={() => setFitVersion(value => value + 1)}>
              <Focus className="size-4" />
            </Button>
            <Button aria-label={t('projectTopology.refresh')} disabled={topology.isFetching} size="icon" variant="ghost" onClick={refresh}>
              <RefreshCw className={topology.isFetching ? 'size-4 animate-spin' : 'size-4'} />
            </Button>
          </div>
        </div>
        {visibleEdges.length > 0
          ? (
              <>
                <div className="hidden md:block">
                  <ProjectTopologyChart edges={visibleEdges} fitVersion={fitVersion} nodes={visibleNodes} onSelectEdge={selectEdge} />
                </div>
                <div className="grid gap-2 p-3 md:hidden">
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
          : <EmptyState description={t('projectTopology.noFilteredRelations')} title={t('projectTopology.listView')} variant="plain" />}
      </Card>

      <ProjectTopologyDetailSheet
        canManage={canManage}
        edge={selectedEdge}
        manualEdge={selectedManualEdge}
        nodes={topology.data.nodes}
        projectId={projectId}
        serviceBinding={selectedBinding}
        onEdit={openEdit}
        onOpenChange={open => !open && setSelectedEdgeId('')}
      />
      {dialog && (
        <ProjectTopologyRelationDialog
          key={`${dialog.mode}-${dialog.serviceBinding?.id ?? dialog.manualEdge?.id ?? 'new'}`}
          applications={applications}
          editingManualEdge={dialog.manualEdge}
          editingServiceBinding={dialog.serviceBinding}
          initialMode={dialog.mode}
          projectId={projectId}
          onOpenChange={closeDialog}
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
    <button className="grid min-w-0 gap-2 rounded-md border border-border bg-background p-3 text-left transition hover:border-primary/40 hover:bg-muted/40" type="button" onClick={onSelect}>
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
      const haystack = [source?.name, source?.slug, target?.name, target?.slug, edge.protocol, edge.relationType]
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
