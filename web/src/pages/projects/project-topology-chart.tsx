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
  getSmoothStepPath,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
} from '@xyflow/react'
import dagre from 'dagre'
import { AppWindow } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { statusToneFor } from '@/components/common/status-tone'
import { cn } from '@/lib/utils'

import '@xyflow/react/dist/style.css'

/**
 * 项目服务拓扑 · 分层流向图（React Flow + dagre）。
 * dagre 负责 Sugiyama 分层坐标（rankdir TB，主调在上、被调在下），
 * React Flow 负责渲染、正交走线（smoothstep）、画布缩放/平移与焦点高亮。
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
  focused: boolean
  onFocus: (nodeId: string) => void
}

interface LaneFlowNodeData extends Record<string, unknown> {
  layerIndex: number
  serviceCount: number
  dimmed: boolean
}

interface TopologyFlowEdgeData extends Record<string, unknown> {
  dimmed: boolean
  label?: string
}

type ServiceFlowNode = Node<ServiceFlowNodeData, 'service'>
type LaneFlowNode = Node<LaneFlowNodeData, 'lane'>
type TopologyFlowNode = LaneFlowNode | ServiceFlowNode
type TopologyFlowEdge = Edge<TopologyFlowEdgeData, 'topology'>

/* ============================================================
   主组件
   ============================================================ */
export function ProjectTopologyChart({ edges, nodes, onSelectEdge }: ProjectTopologyChartProps) {
  const { t } = useTranslation()
  const [focusedNodeId, setFocusedNodeId] = useState<string | null>(null)

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

  const focusNode = useCallback((nodeId: string) => {
    setFocusedNodeId(current => (current === nodeId ? null : nodeId))
  }, [])

  /* dagre 分层布局：节点坐标 + 层索引 + 泳道背景几何 */
  const layout = useMemo(() => layoutTopology(nodes, edges), [edges, nodes])
  const layerByNodeId = useMemo(() => {
    const map = new Map<string, number>()
    layout.layers.forEach((layer, index) => layer.forEach(id => map.set(id, index)))
    return map
  }, [layout])

  /* 焦点邻域（degree-of-interest）：非关联节点/边淡出 */
  const relatedNodeIds = useMemo(() => {
    if (!focusedNodeId)
      return null
    const related = new Set([focusedNodeId])
    for (const edge of edges) {
      if (edge.source === focusedNodeId)
        related.add(edge.target)
      if (edge.target === focusedNodeId)
        related.add(edge.source)
    }
    return related
  }, [edges, focusedNodeId])

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
          dimmed: relatedNodeIds !== null && !relatedNodeIds.has(node.id),
          focused: focusedNodeId === node.id,
          onFocus: focusNode,
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
        dimmed: relatedNodeIds !== null && !(layout.layers[band.index] ?? []).some(id => relatedNodeIds.has(id)),
      },
      draggable: false,
      connectable: false,
      selectable: false,
      focusable: false,
      zIndex: -1,
    }))
    return [...laneNodes, ...serviceNodes]
  }, [degreeByNodeId, focusNode, focusedNodeId, layerByNodeId, layout, nodes, relatedNodeIds, t])

  const flowEdges = useMemo<TopologyFlowEdge[]>(() => edges.map((edge) => {
    const dimmed = focusedNodeId !== null && edge.source !== focusedNodeId && edge.target !== focusedNodeId
    const label = edge.protocol
      ? `${edge.protocol.toUpperCase()}${edge.port ? `·${edge.port}` : ''}`
      : undefined
    return {
      id: edge.id,
      type: 'topology',
      source: edge.source,
      target: edge.target,
      data: { dimmed, label },
      style: {
        stroke: `var(--color-${edgeStrokeToken(edge.status)})`,
        strokeWidth: 1.8,
        strokeDasharray: edge.origin === 'manual' ? '6 5' : undefined,
        opacity: dimmed ? 0.12 : 0.85,
        transition: 'opacity 200ms ease',
      },
      markerEnd: {
        type: MarkerType.ArrowClosed,
        width: 14,
        height: 14,
        color: `var(--color-${edgeStrokeToken(edge.status)})`,
      },
    }
  }), [edges, focusedNodeId])

  const handleEdgeClick = useCallback((_: unknown, edge: TopologyFlowEdge) => {
    onSelectEdge(edge.id)
  }, [onSelectEdge])
  const handleNodeClick = useCallback((_: unknown, node: TopologyFlowNode) => {
    if (node.type === 'service')
      focusNode(node.id)
  }, [focusNode])
  const handlePaneClick = useCallback(() => setFocusedNodeId(null), [])

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
          panOnDrag
          zoomOnScroll
          onEdgeClick={handleEdgeClick}
          onNodeClick={handleNodeClick}
          onPaneClick={handlePaneClick}
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
  const { node, category, degree, statusLabel, categoryLabel, stageSummary, dimmed, focused, onFocus } = data
  const tone = statusToneFor(node.status?.trim() || 'unknown')

  return (
    <div
      aria-label={t('projectTopology.chart.nodeAriaLabel', {
        name: node.name,
        status: statusLabel,
        in: degree.in,
        out: degree.out,
      })}
      className={cn(
        'relative h-full w-full cursor-pointer rounded-container border border-border bg-surface py-3 pr-4 pl-5 text-left',
        'transition-[box-shadow,border-color,opacity,filter] duration-standard',
        'before:absolute before:top-2.5 before:bottom-2.5 before:left-0 before:w-[3px] before:rounded-full before:bg-(--topology-node-cat)',
        'hover:border-separator-strong hover:shadow-raised',
        'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
        dimmed && 'opacity-30 saturate-50',
        focused && 'border-primary shadow-[0_0_0_3px_var(--primary-subtle)]',
      )}
      role="button"
      style={{ '--topology-node-cat': CATEGORY_COLORS[category] } as CSSProperties}
      tabIndex={0}
      onClick={() => onFocus(node.id)}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onFocus(node.id)
        }
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
    </div>
  )
}

/* ============================================================
   自定义边：smoothstep 正交走线，EdgeLabelRenderer 放协议·端口等宽字体标签，
   加宽透明热区让细边也容易点击，焦点态同步淡出
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
  data,
}: EdgeProps<TopologyFlowEdge>) {
  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
    borderRadius: 8,
  })
  const dimmed = data?.dimmed ?? false

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
      <BaseEdge id={id} markerEnd={markerEnd} path={edgePath} style={style} />
      {data?.label && (
        <EdgeLabelRenderer>
          <span
            className="nodrag nopan pointer-events-none absolute rounded-[3px] bg-surface/85 px-1 py-px font-mono text-[10px] text-muted-foreground"
            style={{
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              opacity: dimmed ? 0.12 : 1,
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
        <span>{t('projectTopology.chart.legend.arrow')}</span>
        <span className="text-muted-foreground/70">{t('projectTopology.chart.legend.responsiveNote')}</span>
      </div>
    </div>
  )
}

function LegendLine({ dashed }: { dashed?: boolean }) {
  return (
    <span className={cn('relative h-0 w-6 border-t-2 border-muted-foreground/60', dashed && 'border-dashed')}>
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

/** 边颜色语义 token 名：异常状态上色，正常中性灰 */
function edgeStrokeToken(status: string) {
  switch (statusToneFor(status || 'unknown')) {
    case 'warning':
      return 'warning'
    case 'danger':
      return 'danger'
    default:
      return 'muted-foreground'
  }
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
