import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import type { ApplicationTopologyEdge, ApplicationTopologyNode } from '@/api'
import type { StatusTone } from '@/components/common/status-tone'
import type { TopologyCategory, TopologyGraphEdge, TopologyGraphLegend, TopologyGraphNode } from '@/components/common/topology-graph'
import { useQuery } from '@tanstack/react-query'
import { Box, Boxes, Database, FileCog, Gauge, HardDrive, KeyRound, Layers, Network, RefreshCw, Route, Waypoints } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { TopologyGraph } from '@/components/common/topology-graph'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { NativeSelect } from '@/components/ui/native-select'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

const topologyRefetchIntervalMs = 30_000
const dependencyKinds = new Set(['ConfigMap', 'Secret', 'PersistentVolumeClaim', 'HorizontalPodAutoscaler'])

interface ApplicationTopologyPanelProps {
  projectId: string
  applicationId: string
}

export function ApplicationTopologyPanel({ applicationId, projectId }: ApplicationTopologyPanelProps) {
  const { t } = useTranslation()
  const [selectedTargetId, setSelectedTargetId] = useState('')
  const [showDependencies, setShowDependencies] = useState(false)
  const topology = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['application-topology', projectId, applicationId],
    queryFn: () => api.getApplicationTopology(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
    refetchInterval: topologyRefetchIntervalMs,
  })
  const logicalTopology = useQuery({
    queryKey: ['project-topology', projectId, 'application-summary', applicationId],
    queryFn: () => api.getProjectTopology(projectId, { applicationId, origins: ['service_binding', 'manual'] }),
    enabled: Boolean(projectId && applicationId),
  })

  const visibleNodes = useMemo(() => {
    const nodes = topology.data?.nodes ?? []
    return nodes.filter((node) => {
      if (!showDependencies && dependencyKinds.has(node.kind))
        return false
      return selectedTargetId === '' || node.deploymentTargetId === selectedTargetId || node.deploymentTargetId === ''
    })
  }, [selectedTargetId, showDependencies, topology.data?.nodes])
  const visibleNodeIds = useMemo(() => new Set(visibleNodes.map(node => node.id)), [visibleNodes])
  const visibleEdges = useMemo(
    () => (topology.data?.edges ?? []).filter(edge => visibleNodeIds.has(edge.source) && visibleNodeIds.has(edge.target)),
    [topology.data?.edges, visibleNodeIds],
  )

  if (topology.isError) {
    return (
      <div className="grid gap-3">
        <ErrorState
          title={t('apps.topology.loadFailed')}
          description={t('apps.topology.loadFailedDescription')}
        />
        <Button className="justify-self-start" variant="outline" onClick={() => topology.refetch()}>{t('common.retry')}</Button>
      </div>
    )
  }

  if (topology.isLoading) {
    return <ToolViewportSkeleton />
  }

  if (!topology.data?.targets.length) {
    return <EmptyState title={t('apps.topology.noTargets')} description={t('apps.topology.noTargetsDescription')} />
  }

  return (
    <div className="grid gap-3">
      {(logicalTopology.data?.edges.length ?? 0) > 0 && (
        <Surface className="grid gap-3 p-3">
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-sm font-semibold">{t('apps.topology.logicalDependencies')}</h2>
            <Link className="text-sm text-primary-text hover:underline" to={`/projects/${projectId}?tab=topology`}>{t('apps.topology.openProjectTopology')}</Link>
          </div>
          <div className="grid gap-2 @lg/workspace:grid-cols-2 @4xl/workspace:grid-cols-3">
            {logicalTopology.data?.edges.map((edge) => {
              const upstream = edge.target === applicationId
              const relatedId = upstream ? edge.source : edge.target
              const related = logicalTopology.data.nodes.find(node => node.id === relatedId)
              return (
                <Link key={edge.id} className="flex min-w-0 items-center gap-2 rounded-control border border-border bg-surface-subtle px-3 py-2 transition-colors hover:border-primary-border" to={`/projects/${projectId}/apps/${relatedId}`}>
                  <span className="shrink-0 text-xs text-muted-foreground">{t(upstream ? 'apps.topology.upstream' : 'apps.topology.downstream')}</span>
                  <span className="min-w-0 flex-1 truncate font-medium">{related?.name ?? relatedId}</span>
                  <StatusValueBadge labelKeyPrefix="projectTopology.statuses" value={edge.status} />
                </Link>
              )
            })}
          </div>
        </Surface>
      )}
      {topology.data.warnings.length > 0 && (
        <Alert variant="warning">
          <Boxes />
          <AlertTitle>{t('apps.topology.partialTitle')}</AlertTitle>
          <AlertDescription>
            {topology.data.warnings.map(warning => (
              <span key={`${warning.deploymentTargetId}-${warning.code}`}>
                {t('apps.topology.warningItem', {
                  target: warning.deploymentTargetName,
                  reason: t(`apps.topology.warnings.${warning.code}`),
                })}
              </span>
            ))}
          </AlertDescription>
        </Alert>
      )}
      <Card className="overflow-hidden">
        <div className="flex flex-wrap items-center gap-2 border-b p-3">
          <NativeSelect
            aria-label={t('apps.topology.targetFilter')}
            className="min-w-48"
            containerClassName="w-auto min-w-48"
            value={selectedTargetId}
            onChange={event => setSelectedTargetId(event.target.value)}
          >
            <option value="">{t('apps.topology.allTargets')}</option>
            {topology.data.targets.map(target => (
              <option key={target.id} value={target.id}>
                {`${target.name} · ${target.stage}`}
              </option>
            ))}
          </NativeSelect>
          <Button
            aria-pressed={showDependencies}
            variant={showDependencies ? 'secondary' : 'outline'}
            onClick={() => setShowDependencies(value => !value)}
          >
            <Boxes className="size-4" />
            {t('apps.topology.dependencies')}
          </Button>
          <div className="ml-auto flex items-center gap-1">
            <Button aria-label={t('common.refresh')} disabled={topology.isFetching} size="icon" variant="ghost" onClick={() => topology.refetch()}>
              <RefreshCw className={`size-4 ${topology.isFetching ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </div>
        {visibleNodes.length > 0
          ? <TopologyChart edges={visibleEdges} nodes={visibleNodes} />
          : <EmptyState variant="plain" title={t('apps.topology.noResources')} description={t('apps.topology.noResourcesDescription')} />}
      </Card>
    </div>
  )
}

/** Kubernetes 资源类型到公共拓扑分类：入口 / 服务与工作负载 / 依赖资源 / 运行实例。 */
function categoryForKind(kind: string): TopologyCategory {
  if (kind === 'Gateway' || kind === 'HTTPRoute')
    return 'access'
  if (kind === 'Service' || kind === 'Deployment' || kind === 'StatefulSet')
    return 'core'
  if (dependencyKinds.has(kind))
    return 'support'
  return 'infra'
}

const kindIcons: Record<string, LucideIcon> = {
  Gateway: Network,
  HTTPRoute: Route,
  Service: Waypoints,
  Deployment: Layers,
  StatefulSet: Database,
  Pod: Box,
  ConfigMap: FileCog,
  Secret: KeyRound,
  PersistentVolumeClaim: HardDrive,
  HorizontalPodAutoscaler: Gauge,
}

/** Kubernetes 资源状态是自由文本（Running、Programmed、CrashLoopBackOff…），按关键词归入语义 tone。 */
function toneForResourceStatus(status: string): StatusTone {
  const normalized = status.toLowerCase()
  if (normalized.includes('fail') || normalized.includes('error') || normalized.includes('backoff'))
    return 'danger'
  if (normalized.includes('ready') || normalized.includes('running') || normalized.includes('programmed'))
    return 'success'
  if (normalized.includes('pending') || normalized.includes('progress'))
    return 'warning'
  return 'neutral'
}

/** 配置、存储与伸缩是依赖关系，用虚线与流量/归属关系区分。 */
const dependencyEdgeTypes = new Set(['configures', 'mounts', 'scales'])

function TopologyChart({ edges, nodes }: {
  edges: ApplicationTopologyEdge[]
  nodes: ApplicationTopologyNode[]
}) {
  const { t } = useTranslation()
  const [selectedNodeId, setSelectedNodeId] = useState('')
  const selectedNode = nodes.find(node => node.id === selectedNodeId)

  const graphNodes = useMemo<TopologyGraphNode[]>(() => nodes.map((node) => {
    const kindLabel = t(`apps.topology.kinds.${node.kind}`, { defaultValue: node.kind })
    const status = node.status || 'unknown'
    const statusLabel = t(`apps.topology.statuses.${status}`, { defaultValue: status })
    return {
      id: node.id,
      title: node.name,
      subtitle: kindLabel,
      icon: kindIcons[node.kind] ?? Box,
      category: categoryForKind(node.kind),
      status: { label: statusLabel, tone: toneForResourceStatus(status) },
      meta: node.namespace,
      ariaLabel: t('apps.topology.nodeAriaLabel', { name: node.name, kind: kindLabel, status: statusLabel }),
      actionLabel: t('apps.topology.openDetail'),
    }
  }), [nodes, t])

  const graphEdges = useMemo<TopologyGraphEdge[]>(() => edges.map(edge => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    dashed: dependencyEdgeTypes.has(edge.type),
  })), [edges])

  const legend = useMemo<TopologyGraphLegend>(() => ({
    title: t('apps.topology.legend.title'),
    categoryTitle: t('apps.topology.legend.category'),
    categories: [
      { category: 'access', label: t('apps.topology.legend.categories.access') },
      { category: 'core', label: t('apps.topology.legend.categories.core') },
      { category: 'support', label: t('apps.topology.legend.categories.support') },
      { category: 'infra', label: t('apps.topology.legend.categories.infra') },
    ],
    statusTitle: t('apps.topology.legend.status'),
    statuses: [
      { key: 'healthy', label: t('apps.topology.legend.statuses.healthy'), tone: 'success' },
      { key: 'progressing', label: t('apps.topology.legend.statuses.progressing'), tone: 'warning' },
      { key: 'failed', label: t('apps.topology.legend.statuses.failed'), tone: 'danger' },
      { key: 'unknown', label: t('apps.topology.legend.statuses.unknown'), tone: 'neutral' },
    ],
    relationTitle: t('apps.topology.legend.relation'),
    relations: [
      { key: 'traffic', label: t('apps.topology.legend.traffic') },
      { key: 'dependency', label: t('apps.topology.legend.dependency'), dashed: true },
    ],
    notes: [
      t('apps.topology.legend.arrow'),
      t('apps.topology.legend.nodeAction'),
    ],
  }), [t])

  return (
    <>
      <TopologyGraph
        ariaLabel={t('apps.topology.canvas')}
        edges={graphEdges}
        legend={legend}
        nodes={graphNodes}
        onSelectNode={setSelectedNodeId}
      />
      <Sheet open={Boolean(selectedNode)} onOpenChange={open => !open && setSelectedNodeId('')}>
        <SheetContent>
          {selectedNode && (
            <>
              <SheetHeader>
                <SheetTitle>{selectedNode.name}</SheetTitle>
                <SheetDescription>{t(`apps.topology.kinds.${selectedNode.kind}`, { defaultValue: selectedNode.kind })}</SheetDescription>
              </SheetHeader>
              <div className="grid gap-4 px-4 text-sm">
                <TopologyDetail label={t('apps.topology.status')}>
                  <StatusValueBadge labelKeyPrefix="apps.topology.statuses" value={selectedNode.status || 'unknown'} />
                </TopologyDetail>
                <TopologyDetail label={t('apps.topology.namespace')}>{selectedNode.namespace}</TopologyDetail>
                <TopologyDetail label={t('apps.topology.cluster')}>{selectedNode.clusterName || selectedNode.clusterId}</TopologyDetail>
                {selectedNode.summary && <TopologyDetail label={t('apps.topology.summary')}>{selectedNode.summary}</TopologyDetail>}
              </div>
            </>
          )}
        </SheetContent>
      </Sheet>
    </>
  )
}

function TopologyDetail({ children, label }: { children: ReactNode, label: string }) {
  return (
    <div className="grid gap-1">
      <span className="text-muted-foreground">{label}</span>
      <div className="min-w-0 break-words font-medium">{children}</div>
    </div>
  )
}
