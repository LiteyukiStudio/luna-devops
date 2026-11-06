import type { ProjectTopologyEdge, ProjectTopologyNode } from '@/api'
import type { TopologyGraphEdge, TopologyGraphLegend, TopologyGraphNode } from '@/components/common/topology-graph'
import { AppWindow } from 'lucide-react'
import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { statusToneFor } from '@/components/common/status-tone'
import { TopologyGraph } from '@/components/common/topology-graph'

/**
 * 项目服务拓扑：把应用间服务关系映射到公共分层流向图。
 * 分类色按 dagre 层级（主调在上、被调在下），节点点击进入应用详情，连线点击打开关系详情。
 */
interface ProjectTopologyChartProps {
  edges: ProjectTopologyEdge[]
  nodes: ProjectTopologyNode[]
  onSelectEdge: (edgeId: string) => void
}

export function ProjectTopologyChart({ edges, nodes, onSelectEdge }: ProjectTopologyChartProps) {
  const { t } = useTranslation()
  const { projectId = '' } = useParams()

  const graphNodes = useMemo<TopologyGraphNode[]>(() => {
    const degreeByNodeId = new Map<string, { in: number, out: number }>()
    for (const node of nodes)
      degreeByNodeId.set(node.id, { in: 0, out: 0 })
    for (const edge of edges) {
      const source = degreeByNodeId.get(edge.source)
      const target = degreeByNodeId.get(edge.target)
      if (source)
        source.out += 1
      if (target)
        target.in += 1
    }
    return nodes.map((node) => {
      const statusKey = node.status?.trim() || 'unknown'
      const statusLabel = t(`projectTopology.statuses.${statusKey}`, {
        defaultValue: statusKey === 'unknown' ? t('projectTopology.chart.statusUnknown') : statusKey,
      })
      const degree = degreeByNodeId.get(node.id) ?? { in: 0, out: 0 }
      const [primaryTarget] = node.deploymentTargets
      return {
        id: node.id,
        title: node.name,
        subtitle: t('projectTopology.chart.category'),
        icon: AppWindow,
        status: { label: statusLabel, tone: statusToneFor(statusKey) },
        meta: primaryTarget
          ? node.deploymentTargets.length > 1
            ? `${primaryTarget.stage} +${node.deploymentTargets.length - 1}`
            : primaryTarget.stage
          : undefined,
        counters: [`←${degree.in}`, `→${degree.out}`],
        ariaLabel: t('projectTopology.chart.nodeAriaLabel', { name: node.name, status: statusLabel, in: degree.in, out: degree.out }),
        actionLabel: t('projectTopology.chart.openServiceDetail'),
        href: `/projects/${projectId}/apps/${node.id}`,
      }
    })
  }, [edges, nodes, projectId, t])

  const graphEdges = useMemo<TopologyGraphEdge[]>(() => edges.map(edge => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    label: edge.protocol ? `${edge.protocol.toUpperCase()}${edge.port ? `·${edge.port}` : ''}` : undefined,
    dashed: edge.origin === 'manual',
    tone: statusToneFor(edge.status || 'unknown'),
  })), [edges])

  const legend = useMemo<TopologyGraphLegend>(() => ({
    title: t('projectTopology.chart.legend.title'),
    categoryTitle: t('projectTopology.chart.legend.category'),
    categories: [
      { category: 'access', label: t('projectTopology.chart.legend.categories.access') },
      { category: 'core', label: t('projectTopology.chart.legend.categories.core') },
      { category: 'support', label: t('projectTopology.chart.legend.categories.support') },
      { category: 'infra', label: t('projectTopology.chart.legend.categories.infra') },
    ],
    statusTitle: t('projectTopology.chart.legend.status'),
    statuses: [
      { key: 'ready', label: t('projectTopology.statuses.ready'), tone: 'success' },
      { key: 'degraded', label: t('projectTopology.statuses.degraded'), tone: 'warning' },
      { key: 'unavailable', label: t('projectTopology.statuses.unavailable'), tone: 'danger' },
      { key: 'declared', label: t('projectTopology.statuses.declared'), tone: 'neutral' },
    ],
    relationTitle: t('projectTopology.chart.legend.relation'),
    relations: [
      { key: 'service_binding', label: t('projectTopology.chart.legend.serviceBinding') },
      { key: 'manual', label: t('projectTopology.chart.legend.manual'), dashed: true },
      { key: 'bidirectional', label: t('projectTopology.chart.legend.bidirectional'), bidirectional: true },
    ],
    notes: [
      t('projectTopology.chart.legend.arrow'),
      t('projectTopology.chart.legend.colorNote'),
      t('projectTopology.chart.legend.nodeAction'),
      t('projectTopology.chart.legend.responsiveNote'),
    ],
  }), [t])

  const laneLabel = useCallback((layerIndex: number, nodeCount: number) => ({
    title: t('projectTopology.chart.laneLabel', { number: layerIndex + 1 }),
    subtitle: `${t('projectTopology.chart.laneSub', { number: layerIndex })} · ${t('projectTopology.chart.laneServiceCount', { count: nodeCount })}`,
  }), [t])

  return (
    <TopologyGraph
      ariaLabel={t('projectTopology.chart.canvas')}
      edges={graphEdges}
      laneLabel={laneLabel}
      legend={legend}
      nodes={graphNodes}
      onSelectEdge={onSelectEdge}
    />
  )
}
