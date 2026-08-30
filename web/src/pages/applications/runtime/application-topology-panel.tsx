import type { GraphSeriesOption } from 'echarts/charts'
import type { TooltipComponentOption } from 'echarts/components'
import type { ComposeOption, ECElementEvent, EChartsType } from 'echarts/core'
import type { ReactNode } from 'react'
import type { ApplicationTopologyEdge, ApplicationTopologyNode } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import { init, use as registerEChartsModules } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { Boxes, Focus, RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { NativeSelect } from '@/components/ui/native-select'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

registerEChartsModules([GraphChart, TooltipComponent, CanvasRenderer])

type TopologyChartOption = ComposeOption<GraphSeriesOption | TooltipComponentOption>

const topologyRefetchIntervalMs = 30_000
const dependencyKinds = new Set(['ConfigMap', 'Secret', 'PersistentVolumeClaim', 'HorizontalPodAutoscaler'])
const kindRanks: Record<string, number> = {
  Gateway: 0,
  HTTPRoute: 1,
  Service: 2,
  ConfigMap: 2,
  Secret: 2,
  PersistentVolumeClaim: 2,
  HorizontalPodAutoscaler: 2,
  Deployment: 3,
  StatefulSet: 3,
  Pod: 4,
}

interface ApplicationTopologyPanelProps {
  projectId: string
  applicationId: string
}

export function ApplicationTopologyPanel({ applicationId, projectId }: ApplicationTopologyPanelProps) {
  const { t } = useTranslation()
  const [selectedTargetId, setSelectedTargetId] = useState('')
  const [showDependencies, setShowDependencies] = useState(false)
  const [fitVersion, setFitVersion] = useState(0)
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
        <div className="grid gap-3 rounded-md border border-border bg-muted/25 p-3">
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-sm font-semibold">{t('apps.topology.logicalDependencies')}</h2>
            <Link className="text-sm text-primary-text hover:underline" to={`/projects/${projectId}?tab=topology`}>{t('apps.topology.openProjectTopology')}</Link>
          </div>
          <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
            {logicalTopology.data?.edges.map((edge) => {
              const upstream = edge.target === applicationId
              const relatedId = upstream ? edge.source : edge.target
              const related = logicalTopology.data.nodes.find(node => node.id === relatedId)
              return (
                <Link key={edge.id} className="flex min-w-0 items-center gap-2 rounded-md border border-border bg-background px-3 py-2 hover:border-primary/50" to={`/projects/${projectId}/apps/${relatedId}`}>
                  <span className="shrink-0 text-xs text-muted-foreground">{t(upstream ? 'apps.topology.upstream' : 'apps.topology.downstream')}</span>
                  <span className="min-w-0 flex-1 truncate font-medium">{related?.name ?? relatedId}</span>
                  <StatusValueBadge labelKeyPrefix="projectTopology.statuses" value={edge.status} />
                </Link>
              )
            })}
          </div>
        </div>
      )}
      {topology.data.warnings.length > 0 && (
        <Alert>
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
            <Button aria-label={t('apps.topology.fit')} size="icon" variant="ghost" onClick={() => setFitVersion(version => version + 1)}>
              <Focus className="size-4" />
            </Button>
            <Button aria-label={t('common.refresh')} disabled={topology.isFetching} size="icon" variant="ghost" onClick={() => topology.refetch()}>
              <RefreshCw className={`size-4 ${topology.isFetching ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </div>
        {visibleNodes.length > 0
          ? <TopologyChart edges={visibleEdges} fitVersion={fitVersion} nodes={visibleNodes} />
          : <EmptyState variant="plain" title={t('apps.topology.noResources')} description={t('apps.topology.noResourcesDescription')} />}
      </Card>
    </div>
  )
}

function TopologyChart({ edges, fitVersion, nodes }: {
  edges: ApplicationTopologyEdge[]
  fitVersion: number
  nodes: ApplicationTopologyNode[]
}) {
  const { t } = useTranslation()
  const chartElementRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<EChartsType | null>(null)
  const [selectedNode, setSelectedNode] = useState<ApplicationTopologyNode | null>(null)
  const [chartWidth, setChartWidth] = useState(0)
  const [themeVersion, setThemeVersion] = useState(0)
  const nodesById = useMemo(() => new Map(nodes.map(node => [node.id, node])), [nodes])
  const chartOption = useMemo(
    () => buildChartOption(nodes, edges, t, themeVersion, chartWidth > 0 && chartWidth < 640),
    [chartWidth, edges, nodes, t, themeVersion],
  )

  useEffect(() => {
    const chartElement = chartElementRef.current
    if (!chartElement)
      return
    const chart = init(chartElement)
    setChartWidth(chartElement.clientWidth)
    const resizeObserver = new ResizeObserver((entries) => {
      if (!chart.isDisposed())
        chart.resize()
      const width = Math.round(entries[0]?.contentRect.width ?? chartElement.clientWidth)
      setChartWidth(currentWidth => currentWidth === width ? currentWidth : width)
    })
    resizeObserver.observe(chartElement)
    chartRef.current = chart
    return () => {
      resizeObserver.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [])

  useEffect(() => {
    const chart = chartRef.current
    if (chart && !chart.isDisposed())
      chart.setOption(chartOption, true)
  }, [chartOption, fitVersion])

  useEffect(() => {
    const chart = chartRef.current
    if (!chart)
      return
    const handleClick = (event: ECElementEvent) => {
      if (event.dataType !== 'node')
        return
      const data = event.data as { id?: string } | undefined
      const node = data?.id ? nodesById.get(data.id) : undefined
      if (node)
        setSelectedNode(node)
    }
    chart.on('click', handleClick)
    return () => {
      if (!chart.isDisposed())
        chart.off('click', handleClick)
    }
  }, [nodesById])

  useEffect(() => {
    const observer = new MutationObserver(() => setThemeVersion(version => version + 1))
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style'] })
    return () => observer.disconnect()
  }, [])

  return (
    <>
      <div ref={chartElementRef} className="h-[64vh] min-h-104 max-h-160 w-full bg-muted/40" />
      <Sheet open={Boolean(selectedNode)} onOpenChange={open => !open && setSelectedNode(null)}>
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

function buildChartOption(
  nodes: ApplicationTopologyNode[],
  edges: ApplicationTopologyEdge[],
  t: (key: string, options?: Record<string, unknown>) => string,
  _themeVersion: number,
  compact: boolean,
): TopologyChartOption {
  const styles = getComputedStyle(document.documentElement)
  const foreground = chartThemeColor(styles, '--foreground', '#18181b')
  const edgeColor = chartThemeColor(styles, '--foreground', 'rgba(24, 24, 27, 0.68)', 0.68)
  const border = chartThemeColor(styles, '--border', '#d4d4d8')
  const background = chartThemeColor(styles, '--surface', '#ffffff')
  const grouped = new Map<number, ApplicationTopologyNode[]>()
  for (const node of nodes) {
    const rank = kindRanks[node.kind] ?? 3
    grouped.set(rank, [...(grouped.get(rank) ?? []), node])
  }
  for (const items of grouped.values())
    items.sort((left, right) => `${left.deploymentTargetId}/${left.name}`.localeCompare(`${right.deploymentTargetId}/${right.name}`))
  const maxRows = Math.max(1, ...[...grouped.values()].map(items => items.length))
  const chartNodes = nodes.map((node) => {
    const rank = kindRanks[node.kind] ?? 3
    const peers = grouped.get(rank) ?? []
    const row = peers.findIndex(peer => peer.id === node.id)
    const centeredRow = row + (maxRows - peers.length) / 2
    const labelMaxLength = compact ? 13 : 20
    return {
      id: node.id,
      name: truncate(node.name, compact ? 15 : 22),
      x: (compact ? 70 : 100) + rank * (compact ? 120 : 230),
      y: (compact ? 52 : 80) + centeredRow * (compact ? 78 : 105),
      symbol: symbolForKind(node.kind),
      symbolSize: compact
        ? (node.kind === 'Pod' ? [78, 40] : [92, 46])
        : (node.kind === 'Pod' ? [146, 48] : [168, 56]),
      itemStyle: {
        color: colorForKind(node.kind),
        borderColor: colorForStatus(node.status),
        borderWidth: 2,
        shadowBlur: 8,
        shadowColor: 'rgba(0,0,0,0.08)',
      },
      label: {
        show: true,
        color: '#ffffff',
        fontSize: compact ? 10 : 12,
        textBorderColor: 'rgba(15, 23, 42, 0.32)',
        textBorderWidth: 2,
        formatter: `{kind|${escapeEChartsRichText(t(`apps.topology.kinds.${node.kind}`, { defaultValue: node.kind }))}}\n{name|${escapeEChartsRichText(truncate(node.name, labelMaxLength))}}`,
        rich: {
          kind: { color: '#f8fafc', fontSize: compact ? 8 : 10, lineHeight: compact ? 13 : 17 },
          name: { color: '#ffffff', fontSize: compact ? 9 : 12, fontWeight: 600, lineHeight: compact ? 14 : 18 },
        },
      },
    }
  })
  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: background,
      borderColor: border,
      textStyle: { color: foreground },
      formatter: (params) => {
        if (Array.isArray(params) || params.dataType !== 'node')
          return ''
        const data = params.data as { id?: string } | undefined
        const node = nodes.find(item => item.id === data?.id)
        if (!node)
          return ''
        return [
          `<strong>${escapeHTML(node.name)}</strong>`,
          `${escapeHTML(t('apps.topology.kind'))}: ${escapeHTML(t(`apps.topology.kinds.${node.kind}`, { defaultValue: node.kind }))}`,
          `${escapeHTML(t('apps.topology.status'))}: ${escapeHTML(node.status || t('common.unknown'))}`,
          `${escapeHTML(t('apps.topology.namespace'))}: ${escapeHTML(node.namespace)}`,
        ].join('<br/>')
      },
    },
    series: [{
      type: 'graph',
      layout: 'none',
      preserveAspect: 'contain',
      preserveAspectAlign: 'center',
      preserveAspectVerticalAlign: 'middle',
      roam: true,
      draggable: false,
      animationDurationUpdate: 280,
      data: chartNodes,
      links: edges.map(edge => ({
        id: edge.id,
        source: edge.source,
        target: edge.target,
        lineStyle: {
          color: edgeColor,
          curveness: edge.type === 'configures' || edge.type === 'mounts' || edge.type === 'scales' ? 0.08 : 0,
          opacity: 0.78,
          width: 1.75,
        },
      })),
      edgeSymbol: ['none', 'arrow'],
      edgeSymbolSize: 8,
      emphasis: { focus: 'adjacency', lineStyle: { opacity: 1, width: 2.5 } },
      scaleLimit: { min: 0.45, max: 2.5 },
      left: compact ? 44 : 104,
      right: compact ? 44 : 104,
      top: compact ? 32 : 44,
      bottom: compact ? 32 : 44,
    }],
  }
}

function symbolForKind(kind: string): 'roundRect' | 'circle' | 'diamond' {
  if (kind === 'Pod')
    return 'circle'
  if (kind === 'Gateway' || kind === 'HTTPRoute')
    return 'diamond'
  return 'roundRect'
}

function colorForKind(kind: string) {
  if (kind === 'Gateway' || kind === 'HTTPRoute')
    return '#2563eb'
  if (kind === 'Service')
    return '#0891b2'
  if (kind === 'Deployment' || kind === 'StatefulSet')
    return '#4f46e5'
  if (kind === 'Pod')
    return '#475569'
  if (kind === 'Secret')
    return '#be123c'
  if (kind === 'PersistentVolumeClaim')
    return '#7c3aed'
  return '#64748b'
}

function colorForStatus(status: string) {
  const normalized = status.toLowerCase()
  if (normalized.includes('fail') || normalized.includes('error'))
    return '#fb7185'
  if (normalized.includes('ready') || normalized.includes('running') || normalized.includes('programmed'))
    return '#34d399'
  if (normalized.includes('pending') || normalized.includes('progress'))
    return '#fbbf24'
  return '#cbd5e1'
}

function chartThemeColor(styles: CSSStyleDeclaration, variable: string, fallback: string, alpha?: number) {
  const value = styles.getPropertyValue(variable).trim()
  if (!value)
    return fallback
  const hslParts = value.split(/\s+/)
  if (hslParts.length === 3 && hslParts[1]?.endsWith('%') && hslParts[2]?.endsWith('%')) {
    const [hue, saturation, lightness] = hslParts
    return alpha === undefined
      ? `hsl(${hue}, ${saturation}, ${lightness})`
      : `hsla(${hue}, ${saturation}, ${lightness}, ${alpha})`
  }
  return value
}

function truncate(value: string, maxLength: number) {
  return value.length <= maxLength ? value : `${value.slice(0, maxLength - 1)}…`
}

function escapeHTML(value: string) {
  return value.replace(/[&<>'"]/g, character => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '\'': '&#39;',
    '"': '&quot;',
  })[character] ?? character)
}

function escapeEChartsRichText(value: string) {
  return value.replace(/[{}\\]/g, character => `\\${character}`)
}
