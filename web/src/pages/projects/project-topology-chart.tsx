import type {
  Edge,
  EdgeProps,
  EdgeTypes,
  Node,
  NodeProps,
  NodeTypes,
} from '@xyflow/react'
import type { CSSProperties } from 'react'
import type { ProjectTopologyEdge, ProjectTopologyNode } from '@/api'
import type { StatusTone } from '@/components/common/status-tone'
import {
  Background,
  BaseEdge,
  Controls,
  EdgeLabelRenderer,
  getBezierPath,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
} from '@xyflow/react'
import dagre from 'dagre'
import { AppWindow } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { statusToneFor } from '@/components/common/status-tone'
import { cn } from '@/lib/utils'

import '@xyflow/react/dist/style.css'

/**
 * 项目服务拓扑 · 分层流向图（React Flow + dagre）。
 * dagre 负责 Sugiyama 分层坐标（rankdir TB，主调在上、被调在下），
 * React Flow 负责渲染、自然贝塞尔走线、画布缩放/平移与 hover 邻域聚焦。
 * 泳道用 zIndex -1 的背景节点实现，随视口变换并参与 fitView。
 */

interface ProjectTopologyChartProps {
  edges: ProjectTopologyEdge[]
  nodes: ProjectTopologyNode[]
  onSelectEdge: (edgeId: string) => void
}

/* ---------- 分类定性色板（对应 index.css 中的 --topology-cat-* token） ---------- */
type TopologyCategory = 'access' | 'core' | 'support' | 'infra'

const CATEGORY_ORDER: TopologyCategory[] = ['access', 'core', 'support', 'infra']

const CATEGORY_COLORS: Record<TopologyCategory, string> = {
  access: 'var(--topology-cat-access)',
  core: 'var(--topology-cat-core)',
  support: 'var(--topology-cat-support)',
  infra: 'var(--topology-cat-infra)',
}

/* ---------- 连线循环色板（正常边按稳定序取色；异常边让位 warning/danger 语义色） ---------- */
const EDGE_CYCLE_COLORS = [
  'var(--primary)',
  'var(--color-info)',
  'var(--color-success)',
  'var(--color-theme-supporting)',
  'var(--color-theme-secondary)',
  'var(--color-theme-highlight)',
]

/** 焦点点亮节点/边/泳道时的统一淡出程度 */
const FADED_OPACITY = 0.18

/** 双向线两侧的曲线平行偏移（px） */
const BIDIRECTIONAL_OFFSET = 10

/** 连线排序稳定键：优先后端 id，其次四元组 */
function edgeOrderKey(edge: ProjectTopologyEdge) {
  return edge.id || `${edge.source}→${edge.target}:${edge.origin}:${edge.protocol ?? ''}:${edge.port ?? ''}`
}

/** 双向线取排序后的第一条作为打开详情的代表边 */
function pickPrimaryEdge(group: ProjectTopologyEdge[]) {
  return [...group].sort((a, b) => edgeOrderKey(a).localeCompare(edgeOrderKey(b)))[0]
}

/** 同向多条边（含两端点相同）按稳定序展开并留出曲线间隙；成对反向边合并为一条双向线 */
function collapseEdges(edges: ProjectTopologyEdge[]): Array<{ group: ProjectTopologyEdge[], isBidirectional: boolean }> {
  const ordered = [...edges].sort((a, b) => edgeOrderKey(a).localeCompare(edgeOrderKey(b)))
  const seen = new Set<string>()
  const result: Array<{ group: ProjectTopologyEdge[], isBidirectional: boolean }> = []
  for (const edge of ordered) {
    if (seen.has(edge.id))
      continue
    const reverse = ordered.find(other => !seen.has(other.id) && other.id !== edge.id && other.source === edge.target && other.target === edge.source)
    if (reverse)
      seen.add(reverse.id)
    seen.add(edge.id)
    result.push({ group: reverse ? [edge, reverse] : [edge], isBidirectional: Boolean(reverse) })
  }
  return result
}

/** 贝塞尔中点向两侧做平行偏移，避免成组连线重叠 */
function offsetCurvePoint(x1: number, y1: number, x2: number, y2: number, mx: number, my: number, offset: number) {
  const dx = x2 - x1
  const dy = y2 - y1
  const length = Math.hypot(dx, dy)
  if (!offset || length < 1)
    return { x: mx, y: my }
  return { x: mx + (-dy / length) * offset, y: my + (dx / length) * offset }
}

/* ---------- 布局常量 ---------- */
const NODE_WIDTH = 224
const NODE_HEIGHT = 78
const NODE_SEP = 48 // 同层节点水平间距
const RANK_SEP = 96 // 层间垂直通道
const LANE_PADDING_Y = 28 // 泳道背景在节点上下的留白
const LANE_PADDING_X = 24 // 泳道背景在图左右的留白
const LANE_GAP = 24 // 相邻泳道之间的间隙

/* ---------- 自定义节点/边数据 ---------- */
interface ServiceFlowNodeData extends Record<string, unknown> {
  node: ProjectTopologyNode
  category: TopologyCategory
  degree: { in: number, out: number }
  statusLabel: string
  categoryLabel: string
  stageSummary: string
  dimmed: boolean
  hovered: boolean
  detailTo: string
}

interface LaneFlowNodeData extends Record<string, unknown> {
  layerIndex: number
  serviceCount: number
  dimmed: boolean
}

interface TopologyFlowEdgeData extends Record<string, unknown> {
  dimmed: boolean
  hoverKey: string
  isBidirectional: boolean
  label?: string
  offset: number
  selectEdgeId: string
}

type ServiceFlowNode = Node<ServiceFlowNodeData, 'service'>
type LaneFlowNode = Node<LaneFlowNodeData, 'lane'>
type TopologyFlowNode = LaneFlowNode | ServiceFlowNode
type TopologyFlowEdge = Edge<TopologyFlowEdgeData, 'topology'>

/* ============================================================
   主组件
   ============================================================ */
export function ProjectTopologyChart(props: ProjectTopologyChartProps) {
  // useReactFlow 必须位于 ReactFlowProvider 内部，故拆为外层 Provider + 内部画布组件
  return (
    <ReactFlowProvider>
      <ProjectTopologyChartCanvas {...props} />
    </ReactFlowProvider>
  )
}

function ProjectTopologyChartCanvas({ edges, nodes, onSelectEdge }: ProjectTopologyChartProps) {
  const { t } = useTranslation()
  const { projectId = '' } = useParams()
  /* hover 聚焦（临时态）：hoverKey 为节点 id 或成组边的 hoverKey，共用一个状态位 */
  const [hoverKey, setHoverKey] = useState<string | null>(null)
  const { setCenter } = useReactFlow()

  const degreeByNodeId = useMemo(() => {
    const map = new Map<string, { in: number, out: number }>()
    for (const node of nodes)
      map.set(node.id, { in: 0, out: 0 })
    for (const edge of edges) {
      const source = map.get(edge.source)
      const target = map.get(edge.target)
      if (source)
        source.out += 1
      if (target)
        target.in += 1
    }
    return map
  }, [edges, nodes])

  /* dagre 分层布局：节点坐标 + 层索引 + 泳道背景几何 */
  const layout = useMemo(() => layoutTopology(nodes, edges), [edges, nodes])
  const layerByNodeId = useMemo(() => {
    const map = new Map<string, number>()
    layout.layers.forEach((layer, index) => layer.forEach(id => map.set(id, index)))
    return map
  }, [layout])

  /* 渲染层归并：同向多条边展开为平行曲线，成对反向边合并为一条双向线 */
  const collapsedEdges = useMemo(() => collapseEdges(edges), [edges])

  /* hover 邻域：非关联节点/边/泳道淡出 */
  const highlightedNodeIds = useMemo(() => {
    if (!hoverKey)
      return null
    const highlighted = new Set<string>()
    if (degreeByNodeId.has(hoverKey)) {
      highlighted.add(hoverKey)
      for (const edge of edges) {
        if (edge.source === hoverKey)
          highlighted.add(edge.target)
        if (edge.target === hoverKey)
          highlighted.add(edge.source)
      }
      return highlighted
    }
    const group = collapsedEdges.find(item => `pair:${item.group[0].source}:${item.group[0].target}` === hoverKey)
    if (group) {
      highlighted.add(group.group[0].source)
      highlighted.add(group.group[0].target)
    }
    return highlighted
  }, [collapsedEdges, degreeByNodeId, edges, hoverKey])

  const nodeTypes = useMemo<NodeTypes>(() => ({ service: ServiceFlowNodeCard, lane: LaneBandNode }), [])
  const edgeTypes = useMemo<EdgeTypes>(() => ({ topology: TopologyFlowEdgeComponent }), [])

  const flowNodes = useMemo<TopologyFlowNode[]>(() => {
    const serviceNodes: ServiceFlowNode[] = nodes.map((node) => {
      const layerIndex = layerByNodeId.get(node.id) ?? 0
      const category = categoryForLayer(layerIndex)
      const statusKey = node.status?.trim() || 'unknown'
      const targets = node.deploymentTargets
      const primaryTarget = targets[0]
      return {
        id: node.id,
        type: 'service',
        position: layout.positions.get(node.id) ?? { x: 0, y: 0 },
        style: { width: NODE_WIDTH, height: NODE_HEIGHT },
        data: {
          node,
          category,
          degree: degreeByNodeId.get(node.id) ?? { in: 0, out: 0 },
          statusLabel: t(`projectTopology.statuses.${statusKey}`, {
            defaultValue: statusKey === 'unknown' ? t('projectTopology.chart.statusUnknown') : statusKey,
          }),
          categoryLabel: t('projectTopology.chart.category'),
          stageSummary: primaryTarget
            ? targets.length > 1
              ? `${primaryTarget.stage} +${targets.length - 1}`
              : primaryTarget.stage
            : '',
          dimmed: highlightedNodeIds !== null && !highlightedNodeIds.has(node.id),
          hovered: hoverKey === node.id,
          detailTo: `/projects/${projectId}/apps/${node.id}`,
        },
        draggable: false,
        connectable: false,
        selectable: false,
      }
    })
    const laneNodes: LaneFlowNode[] = layout.laneBands.map(band => ({
      id: `lane-${band.index}`,
      type: 'lane',
      position: { x: -LANE_PADDING_X, y: band.y },
      style: { width: layout.width + LANE_PADDING_X * 2, height: band.height },
      className: 'pointer-events-none',
      data: {
        layerIndex: band.index,
        serviceCount: layout.layers[band.index]?.length ?? 0,
        dimmed: highlightedNodeIds !== null && !(layout.layers[band.index] ?? []).some(id => highlightedNodeIds.has(id)),
      },
      draggable: false,
      connectable: false,
      selectable: false,
      focusable: false,
      zIndex: -1,
    }))
    return [...laneNodes, ...serviceNodes]
  }, [degreeByNodeId, highlightedNodeIds, hoverKey, layerByNodeId, layout, nodes, projectId, t])

  const flowEdges = useMemo<TopologyFlowEdge[]>(() => collapsedEdges.map(({ group, isBidirectional }, index) => {
    const primary = pickPrimaryEdge(group)
    const reversed = primary.source !== group[0].source
    const edgeHoverKey = `pair:${group[0].source}:${group[0].target}`
    const dimmed = highlightedNodeIds !== null && !highlightedNodeIds.has(group[0].source) && !highlightedNodeIds.has(group[0].target)
    /* 高亮：hover 该边，或 hover 该边任意一端节点（邻域聚焦） */
    const hovered = hoverKey === edgeHoverKey
      || hoverKey === group[0].source
      || hoverKey === group[0].target
    const labelParts = [...new Set(group
      .map(edge => edge.protocol ? `${edge.protocol.toUpperCase()}${edge.port ? `·${edge.port}` : ''}` : undefined)
      .filter((label): label is string => Boolean(label)))]
    const tone = statusToneFor(primary.status || 'unknown')
    const stroke = tone === 'warning'
      ? 'var(--color-warning)'
      : tone === 'danger'
        ? 'var(--color-danger)'
        : EDGE_CYCLE_COLORS[index % EDGE_CYCLE_COLORS.length]
    const offset = isBidirectional ? BIDIRECTIONAL_OFFSET : 0
    return {
      id: `topology:${group.map(edge => edge.id).sort().join('~')}`,
      type: 'topology',
      source: primary.source,
      target: primary.target,
      data: {
        dimmed,
        hoverKey: edgeHoverKey,
        isBidirectional,
        label: labelParts.length > 0 ? labelParts.join(' ⇄ ') : undefined,
        offset: reversed ? -offset : offset,
        selectEdgeId: primary.id,
      },
      style: {
        stroke,
        strokeWidth: hovered ? 2.4 : 1.8,
        strokeDasharray: group.some(edge => edge.origin === 'manual') ? '6 5' : undefined,
        opacity: dimmed ? FADED_OPACITY : hovered ? 1 : 0.9,
        transition: 'opacity 200ms ease, stroke-width 150ms ease',
      },
      markerEnd: {
        type: MarkerType.ArrowClosed,
        width: 14,
        height: 14,
        color: stroke,
      },
      ...(isBidirectional
        ? {
            markerStart: {
              type: MarkerType.ArrowClosed,
              orient: 'auto-start-reverse',
              width: 14,
              height: 14,
              color: stroke,
            },
          }
        : {}),
    }
  }), [collapsedEdges, highlightedNodeIds, hoverKey])

  const clearHover = useCallback(() => setHoverKey(null), [])
  const handleEdgeClick = useCallback((_: unknown, edge: TopologyFlowEdge) => {
    onSelectEdge(edge.data?.selectEdgeId ?? edge.id)
  }, [onSelectEdge])
  const handleEdgeMouseEnter = useCallback((_: unknown, edge: TopologyFlowEdge) => {
    setHoverKey(edge.data?.hoverKey ?? edge.id)
  }, [])
  const handleNodeMouseEnter = useCallback((_: unknown, node: TopologyFlowNode) => {
    if (node.type === 'service')
      setHoverKey(node.id)
  }, [])
  const handleNodeDoubleClick = useCallback((_: unknown, node: TopologyFlowNode) => {
    if (node.type !== 'service')
      return
    const position = layout.positions.get(node.id)
    if (position)
      void setCenter(position.x + NODE_WIDTH / 2, position.y + NODE_HEIGHT / 2, { duration: 300, zoom: 1.1 })
  }, [layout, setCenter])

  return (
    <div className="relative">
      <div
        aria-label={t('projectTopology.chart.canvas')}
        className={cn(
          'relative h-[560px]',
          // Controls 控件走语义 token，适配 light/dark
          '[--xy-controls-button-background-color:hsl(var(--surface))]',
          '[--xy-controls-button-background-color-hover:hsl(var(--surface-subtle))]',
          '[--xy-controls-button-border-color:hsl(var(--border))]',
          '[--xy-controls-button-color:hsl(var(--muted-foreground))]',
        )}
        role="group"
      >
        <ReactFlow
          edges={flowEdges}
          edgeTypes={edgeTypes}
          elementsSelectable
          fitView
          fitViewOptions={{ padding: 0.12, maxZoom: 1.2 }}
          maxZoom={2}
          minZoom={0.25}
          nodes={flowNodes}
          nodesConnectable={false}
          nodesDraggable={false}
          nodeTypes={nodeTypes}
          panOnDrag={false}
          panOnScroll
          zoomOnScroll
          onEdgeClick={handleEdgeClick}
          onEdgeMouseEnter={handleEdgeMouseEnter}
          onEdgeMouseLeave={clearHover}
          onNodeDoubleClick={handleNodeDoubleClick}
          onNodeMouseEnter={handleNodeMouseEnter}
          onNodeMouseLeave={clearHover}
        >
          <Background bgColor="transparent" color="hsl(var(--border))" gap={20} size={1} />
          <Controls position="bottom-right" showInteractive={false} />
        </ReactFlow>
      </div>

      <TopologyLegend />
    </div>
  )
}

/* ============================================================
   泳道背景节点：zIndex -1 渲染在边与服务节点之下，
   标签 = 层名 + 调用深度副标 + 服务数
   ============================================================ */
function LaneBandNode({ data }: NodeProps<LaneFlowNode>) {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        'h-full w-full border-y border-dashed border-border bg-surface-subtle/70 transition-opacity duration-standard',
        data.dimmed && 'opacity-40',
      )}
    >
      <div className="flex flex-col gap-0.5 p-3">
        <span className="text-[11px] font-semibold tracking-wide text-muted-foreground">
          {t('projectTopology.chart.laneLabel', { number: data.layerIndex + 1 })}
        </span>
        <span className="text-[10px] text-muted-foreground/80">
          {`${t('projectTopology.chart.laneSub', { number: data.layerIndex })} · ${t('projectTopology.chart.laneServiceCount', { count: data.serviceCount })}`}
        </span>
      </div>
    </div>
  )
}

/* ============================================================
   服务节点卡片：白底卡片 + 细边框 + 左侧 3px 分类色条，
   状态用色点 + 文字徽章双重编码（对齐 StatusBadge 五档 tone）
   ============================================================ */
function ServiceFlowNodeCard({ data }: NodeProps<ServiceFlowNode>) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { node, category, degree, statusLabel, categoryLabel, stageSummary, dimmed, hovered, detailTo } = data
  const tone = statusToneFor(node.status?.trim() || 'unknown')

  return (
    <Link
      aria-label={t('projectTopology.chart.nodeAriaLabel', {
        name: node.name,
        status: statusLabel,
        in: degree.in,
        out: degree.out,
      })}
      className={cn(
        'nodrag relative block h-full w-full cursor-pointer rounded-container border border-border bg-surface py-3 pr-4 pl-5 text-left',
        'transition-[box-shadow,border-color,opacity,filter] duration-standard',
        'before:absolute before:top-2.5 before:bottom-2.5 before:left-0 before:w-[3px] before:rounded-full before:bg-(--topology-node-cat)',
        'hover:border-separator-strong hover:shadow-raised',
        'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
        dimmed && 'opacity-30 saturate-50',
        hovered && 'border-primary shadow-[0_0_0_3px_var(--primary-subtle)]',
      )}
      style={{ '--topology-node-cat': CATEGORY_COLORS[category] } as CSSProperties}
      title={t('projectTopology.chart.openServiceDetail')}
      to={detailTo}
      onClick={(event) => {
        /* 走 SPA 路由跳转，阻止 <a> 默认整页刷新 */
        event.preventDefault()
        navigate(detailTo)
      }}
    >
      <Handle className="!pointer-events-none !opacity-0" position={Position.Top} type="target" />
      <div className="flex items-center gap-2">
        <span
          className="grid size-7 flex-none place-items-center rounded-control"
          style={{
            color: CATEGORY_COLORS[category],
            backgroundColor: `color-mix(in oklab, ${CATEGORY_COLORS[category]} 12%, transparent)`,
          }}
        >
          <AppWindow className="size-4" />
        </span>
        <div className="min-w-0">
          <div className="truncate text-[13.5px] font-semibold tracking-tight">{node.name}</div>
          <div className="truncate text-[11px] text-muted-foreground/80">{categoryLabel}</div>
        </div>
      </div>
      <div className="mt-2 flex items-center gap-2 text-[11px] text-muted-foreground">
        <span className={cn('inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 leading-none font-medium', tonePillClass(tone))}>
          <span className="size-1.5 flex-none rounded-full bg-current" />
          {statusLabel}
        </span>
        {stageSummary && <span className="font-mono text-[10.5px] tabular-nums">{stageSummary}</span>}
        <span className="ml-auto inline-flex gap-2 text-[10.5px] tabular-nums text-muted-foreground/80">
          <span>{`←${degree.in}`}</span>
          <span>{`→${degree.out}`}</span>
        </span>
      </div>
      <Handle className="!pointer-events-none !opacity-0" position={Position.Bottom} type="source" />
    </Link>
  )
}

/* ============================================================
   自定义边：自然贝塞尔曲线（bidirectional 时向一侧平行偏移），
   EdgeLabelRenderer 放协议·端口等宽字体标签，
   加宽透明热区让细边也容易点击，hover 焦点态同步淡出
   ============================================================ */
function TopologyFlowEdgeComponent({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style,
  markerEnd,
  markerStart,
  data,
}: EdgeProps<TopologyFlowEdge>) {
  const [edgePath, rawLabelX, rawLabelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  })
  const dimmed = data?.dimmed ?? false
  const { x: labelX, y: labelY } = offsetCurvePoint(sourceX, sourceY, targetX, targetY, rawLabelX, rawLabelY, data?.offset ?? 0)

  return (
    <>
      {/* 加宽透明热区，保证细边可点击 */}
      <path
        className="react-flow__edge-interaction"
        d={edgePath}
        fill="none"
        strokeOpacity={0}
        strokeWidth={16}
      />
      <BaseEdge id={id} markerEnd={markerEnd} markerStart={markerStart} path={edgePath} style={style} />
      {data?.label && (
        <EdgeLabelRenderer>
          <span
            className="nodrag nopan pointer-events-none absolute rounded-[3px] bg-surface/85 px-1 py-px font-mono text-[10px] text-muted-foreground"
            style={{
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              opacity: dimmed ? FADED_OPACITY : 1,
              transition: 'opacity 200ms ease',
            }}
          >
            {data.label}
          </span>
        </EdgeLabelRenderer>
      )}
    </>
  )
}

/* ============================================================
   图例：分类色 / 状态徽章 / 实线虚线 / 箭头方向
   ============================================================ */
function TopologyLegend() {
  const { t } = useTranslation()
  const statusItems: Array<{ key: string, tone: StatusTone }> = [
    { key: 'ready', tone: 'success' },
    { key: 'degraded', tone: 'warning' },
    { key: 'unavailable', tone: 'danger' },
    { key: 'declared', tone: 'neutral' },
  ]
  return (
    <div
      aria-label={t('projectTopology.chart.legend.title')}
      className="flex flex-wrap gap-x-8 gap-y-3 border-t border-border bg-surface-subtle px-5 py-3 text-xs text-muted-foreground"
    >
      <div className="flex items-center gap-3">
        <span className="text-[11px] font-semibold tracking-wide text-muted-foreground/80">
          {t('projectTopology.chart.legend.category')}
        </span>
        {CATEGORY_ORDER.map(category => (
          <span key={category} className="inline-flex items-center gap-1.5">
            <span className="size-3 rounded-[3px]" style={{ background: CATEGORY_COLORS[category] }} />
            {t(`projectTopology.chart.legend.categories.${category}`)}
          </span>
        ))}
      </div>
      <div className="flex items-center gap-3">
        <span className="text-[11px] font-semibold tracking-wide text-muted-foreground/80">
          {t('projectTopology.chart.legend.status')}
        </span>
        {statusItems.map(item => (
          <span key={item.key} className={cn('inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 leading-none font-medium', tonePillClass(item.tone))}>
            <span className="size-1.5 flex-none rounded-full bg-current" />
            {t(`projectTopology.statuses.${item.key}`)}
          </span>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-[11px] font-semibold tracking-wide text-muted-foreground/80">
          {t('projectTopology.chart.legend.relation')}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <LegendLine />
          {t('projectTopology.chart.legend.serviceBinding')}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <LegendLine dashed />
          {t('projectTopology.chart.legend.manual')}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <LegendLine bidirectional />
          {t('projectTopology.chart.legend.bidirectional')}
        </span>
        <span>{t('projectTopology.chart.legend.arrow')}</span>
        <span className="text-muted-foreground/70">{t('projectTopology.chart.legend.colorNote')}</span>
        <span className="text-muted-foreground/70">{t('projectTopology.chart.legend.nodeAction')}</span>
        <span className="text-muted-foreground/70">{t('projectTopology.chart.legend.responsiveNote')}</span>
      </div>
    </div>
  )
}

function LegendLine({ bidirectional, dashed }: { bidirectional?: boolean, dashed?: boolean }) {
  return (
    <span className={cn('relative h-0 w-6 border-t-2 border-muted-foreground/60', dashed && 'border-dashed')}>
      {bidirectional && <span className="absolute -top-[5px] -left-px border-y-4 border-r-[6px] border-y-transparent border-r-muted-foreground/60" />}
      <span className="absolute -top-[5px] -right-px border-y-4 border-l-[6px] border-y-transparent border-l-muted-foreground/60" />
    </span>
  )
}

/* ============================================================
   布局：dagre 计算分层坐标（rankdir TB），并按层推导泳道背景条带。
   dagre 内部完成拓扑排序、最长路径分层与交叉最小化排序。
   ============================================================ */
interface TopologyLayout {
  laneBands: Array<{ height: number, index: number, y: number }>
  layers: string[][]
  positions: Map<string, { x: number, y: number }>
  width: number
}

function layoutTopology(nodes: ProjectTopologyNode[], edges: ProjectTopologyEdge[]): TopologyLayout {
  const graph = new dagre.graphlib.Graph()
  graph.setGraph({
    rankdir: 'TB',
    nodesep: NODE_SEP,
    ranksep: RANK_SEP,
    marginx: 0,
    marginy: 0,
  })
  graph.setDefaultEdgeLabel(() => ({}))
  const nodeIds = new Set(nodes.map(node => node.id))
  for (const node of nodes)
    graph.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT })
  for (const edge of edges) {
    if (!nodeIds.has(edge.source) || !nodeIds.has(edge.target) || edge.source === edge.target)
      continue
    graph.setEdge(edge.source, edge.target)
  }
  dagre.layout(graph)

  const positions = new Map<string, { x: number, y: number }>()
  const centerYById = new Map<string, number>()
  let maxRight = 0
  for (const node of nodes) {
    const laidOut = graph.node(node.id)
    if (!laidOut)
      continue
    const x = laidOut.x - NODE_WIDTH / 2
    const y = laidOut.y - NODE_HEIGHT / 2
    positions.set(node.id, { x, y })
    centerYById.set(node.id, laidOut.y)
    maxRight = Math.max(maxRight, x + NODE_WIDTH)
  }

  /* dagre 不直接暴露 rank；rankdir TB 下同一层的 y 中心一致，按 y 聚类得到层 */
  const uniqueYs = [...new Set([...centerYById.values()].map(y => Math.round(y)))].sort((a, b) => a - b)
  const layerIndexByY = new Map<number, number>()
  uniqueYs.forEach((y, index) => layerIndexByY.set(y, index))

  const layers: string[][] = uniqueYs.map(() => [])
  for (const node of nodes) {
    const y = centerYById.get(node.id)
    if (y === undefined)
      continue
    layers[layerIndexByY.get(Math.round(y)) ?? 0].push(node.id)
  }

  /* 泳道条带：覆盖该层节点上下留白，层间留出间隙 */
  const laneBands: TopologyLayout['laneBands'] = uniqueYs.map((y, index) => {
    const layerTop = y - NODE_HEIGHT / 2
    return {
      index,
      y: layerTop - LANE_PADDING_Y - (index === 0 ? 0 : LANE_GAP / 2),
      height: NODE_HEIGHT + LANE_PADDING_Y * 2 + (index === 0 || index === uniqueYs.length - 1 ? LANE_GAP / 2 : LANE_GAP),
    }
  })

  return { layers, laneBands, positions, width: Math.max(maxRight, NODE_WIDTH) }
}

/* ---------- 语义映射 ---------- */
/** 分类色与泳道层级对应：0 接入 / 1 核心 / 2 支撑 / ≥3 基础设施 */
function categoryForLayer(layerIndex: number): TopologyCategory {
  return CATEGORY_ORDER[Math.min(layerIndex, CATEGORY_ORDER.length - 1)]
}

function tonePillClass(tone: StatusTone) {
  switch (tone) {
    case 'success':
      return 'border-success-border bg-success-subtle text-success'
    case 'warning':
      return 'border-warning-border bg-warning-subtle text-warning'
    case 'danger':
      return 'border-danger-border bg-danger-subtle text-danger'
    case 'info':
      return 'border-info-border bg-info-subtle text-info'
    case 'neutral':
      return 'border-zinc-200 bg-zinc-50 text-zinc-700 dark:border-zinc-800 dark:bg-zinc-900/60 dark:text-zinc-300'
  }
}
