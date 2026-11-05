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
import { Surface } from '@/components/common/surface'
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
        <Surface className="grid gap-3 p-3">
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
    // 明暗模式与品牌主色都会改写根节点 token，切换后重新取色。
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'data-brand-theme'] })
    return () => observer.disconnect()
  }, [])

  return (
    <>
      <div ref={chartElementRef} className="h-[64vh] min-h-104 max-h-160 w-full bg-surface-subtle" />
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
  const palette = readTopologyPalette()
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
        color: colorForKind(palette, node.kind),
        borderColor: colorForStatus(palette, node.status),
        borderWidth: 2,
        shadowBlur: 8,
        shadowColor: palette.shadow,
      },
      label: {
        show: true,
        color: palette.nodeText,
        fontSize: compact ? 10 : 12,
        formatter: `{kind|${escapeEChartsRichText(t(`apps.topology.kinds.${node.kind}`, { defaultValue: node.kind }))}}\n{name|${escapeEChartsRichText(truncate(node.name, labelMaxLength))}}`,
        rich: {
          kind: { color: palette.nodeText, fontSize: compact ? 8 : 10, lineHeight: compact ? 13 : 17 },
          name: { color: palette.nodeText, fontSize: compact ? 9 : 12, fontWeight: 600, lineHeight: compact ? 14 : 18 },
        },
      },
    }
  })
  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: palette.surface,
      borderColor: palette.border,
      textStyle: { color: palette.foreground },
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
          color: palette.edge,
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

type TopologyPalette = ReturnType<typeof readTopologyPalette>

/**
 * 从根节点读取设计 token 作为 ECharts 配色；canvas 无法解析 CSS 变量，
 * 因此在每次明暗模式或品牌主色切换后重新读取。
 */
function readTopologyPalette() {
  const styles = getComputedStyle(document.documentElement)
  const token = (variable: string, alpha?: number) => cssTokenColor(styles, variable, alpha)
  return {
    foreground: token('--foreground'),
    edge: token('--muted-foreground'),
    border: token('--border'),
    surface: token('--surface'),
    // 节点使用实心分类色，文字取表面色形成反差：浅色主题为白字，深色主题为深字。
    nodeText: token('--surface'),
    shadow: token('--foreground', 0.08),
    primary: token('--primary'),
    workload: token('--muted-foreground'),
    access: token('--topology-cat-access'),
    core: token('--topology-cat-core'),
    support: token('--topology-cat-support'),
    infra: token('--topology-cat-infra'),
    success: token('--status-success'),
    warning: token('--status-warning'),
    danger: token('--status-danger'),
  }
}

function colorForKind(palette: TopologyPalette, kind: string) {
  if (kind === 'Gateway' || kind === 'HTTPRoute')
    return palette.access
  if (kind === 'Service')
    return palette.primary
  if (kind === 'Deployment' || kind === 'StatefulSet')
    return palette.core
  if (kind === 'Pod')
    return palette.workload
  if (dependencyKinds.has(kind))
    return palette.support
  return palette.infra
}

function colorForStatus(palette: TopologyPalette, status: string) {
  const normalized = status.toLowerCase()
  if (normalized.includes('fail') || normalized.includes('error'))
    return palette.danger
  if (normalized.includes('ready') || normalized.includes('running') || normalized.includes('programmed'))
    return palette.success
  if (normalized.includes('pending') || normalized.includes('progress'))
    return palette.warning
  return palette.border
}

/** HSL 分量 token（如 `--foreground`）转为 hsl()/hsla()；hex 等完整颜色 token 原样返回。缺失时交给 ECharts 默认色。 */
function cssTokenColor(styles: CSSStyleDeclaration, variable: string, alpha?: number) {
  const value = styles.getPropertyValue(variable).trim()
  if (!value)
    return undefined
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
