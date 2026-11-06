import type {
  Edge,
  EdgeProps,
  EdgeTypes,
  Node,
  NodeProps,
  NodeTypes,
} from '@xyflow/react'
import type { LucideIcon } from 'lucide-react'
import type { CSSProperties } from 'react'
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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { cn } from '@/lib/utils'

import '@xyflow/react/dist/style.css'

/**
 * 拓扑分层流向图（React Flow + dagre），项目空间服务关系与应用运行拓扑共用。
 * dagre 负责分层坐标（rankdir TB，上游在上、下游在下），React Flow 负责渲染、贝塞尔走线、
 * 画布缩放/平移与 hover 邻域聚焦。文案全部由调用方翻译后传入，本组件不绑定业务命名空间。
 */

/* ---------- 分类定性色板（对应 index.css 中的 --topology-cat-* token） ---------- */
export type TopologyCategory = 'access' | 'core' | 'support' | 'infra'

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

const FIT_VIEW_OPTIONS = { padding: 0.12, maxZoom: 1.2 }

/** 焦点点亮节点/边/泳道时的统一淡出程度 */
const FADED_OPACITY = 0.18

/** 双向线两侧的曲线平行偏移（px） */
const BIDIRECTIONAL_OFFSET = 10

/* ---------- 布局常量 ---------- */
const NODE_WIDTH = 224
const NODE_HEIGHT = 78
const NODE_SEP = 48 // 同层节点水平间距
const RANK_SEP = 96 // 层间垂直通道
const LANE_PADDING_Y = 28 // 泳道背景在节点上下的留白
const LANE_PADDING_X = 24 // 泳道背景在图左右的留白
const LANE_GAP = 24 // 相邻泳道之间的间隙

export interface TopologyGraphNode {
  id: string
  title: string
  subtitle: string
  icon: LucideIcon
  /** 分类色；省略时按 dagre 层级依次取 接入/核心/支撑/基础设施。 */
  category?: TopologyCategory
  status: { label: string, tone: StatusTone }
  /** 状态徽章后的等宽次要信息，例如部署阶段或命名空间。 */
  meta?: string
  /** 卡片右下角的计数类短文本，例如入度/出度。 */
  counters?: string[]
  ariaLabel: string
  actionLabel: string
  /** 有 href 时节点是跳转链接，否则点击回调 onSelectNode。 */
  href?: string
}

export interface TopologyGraphEdge {
  id: string
  source: string
  target: string
  label?: string
  dashed?: boolean
  tone?: StatusTone
}

export interface TopologyGraphLegend {
  title: string
  categoryTitle: string
  categories: Array<{ category: TopologyCategory, label: string }>
  statusTitle: string
  statuses: Array<{ key: string, label: string, tone: StatusTone }>
  relationTitle: string
  relations: Array<{ key: string, label: string, dashed?: boolean, bidirectional?: boolean }>
  notes: string[]
}

interface TopologyGraphProps {
  ariaLabel: string
  edges: TopologyGraphEdge[]
  legend: TopologyGraphLegend
  nodes: TopologyGraphNode[]
  /** 提供时按 dagre 层级渲染泳道背景，并返回每条泳道的标题与副标题。 */
  laneLabel?: (layerIndex: number, nodeCount: number) => { title: string, subtitle: string }
  onSelectEdge?: (edgeId: string) => void
  onSelectNode?: (nodeId: string) => void
}

/* ---------- 自定义节点/边数据 ---------- */
interface CardFlowNodeData extends Record<string, unknown> {
  node: TopologyGraphNode
  category: TopologyCategory
  dimmed: boolean
  hovered: boolean
  onSelectNode?: (nodeId: string) => void
}

interface LaneFlowNodeData extends Record<string, unknown> {
  title: string
  subtitle: string
  dimmed: boolean
}

interface TopologyFlowEdgeData extends Record<string, unknown> {
  dimmed: boolean
  hoverKey: string
  label?: string
  offset: number
  selectEdgeId: string
}

type CardFlowNode = Node<CardFlowNodeData, 'card'>
type LaneFlowNode = Node<LaneFlowNodeData, 'lane'>
type TopologyFlowNode = LaneFlowNode | CardFlowNode
type TopologyFlowEdge = Edge<TopologyFlowEdgeData, 'topology'>

export function TopologyGraph(props: TopologyGraphProps) {
  // 节点集合变化（筛选、刷新出新资源）时重新挂载画布，让 fitView 按新内容取景
  const signature = props.nodes.map(node => node.id).join('|')
  return (
    <ReactFlowProvider key={signature}>
      <TopologyGraphCanvas {...props} />
    </ReactFlowProvider>
  )
}

function TopologyGraphCanvas({ ariaLabel, edges, laneLabel, legend, nodes, onSelectEdge, onSelectNode }: TopologyGraphProps) {
  /* hover 聚焦（临时态）：hoverKey 为节点 id 或成组边的 hoverKey，共用一个状态位 */
  const [hoverKey, setHoverKey] = useState<string | null>(null)
  const { fitView, setCenter } = useReactFlow()
  const canvasRef = useRef<HTMLDivElement>(null)
  /* hover 去抖定时器：进入立即、退出延迟，打破“边加粗→命中区域变化→进出事件循环”导致的闪烁 */
  const hoverClearRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const nodeIds = useMemo(() => new Set(nodes.map(node => node.id)), [nodes])
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
    if (nodeIds.has(hoverKey)) {
      highlighted.add(hoverKey)
      for (const edge of edges) {
        if (edge.source === hoverKey)
          highlighted.add(edge.target)
        if (edge.target === hoverKey)
          highlighted.add(edge.source)
      }
      return highlighted
    }
    const group = collapsedEdges.find(item => edgeHoverKey(item.group[0]) === hoverKey)
    if (group) {
      highlighted.add(group.group[0].source)
      highlighted.add(group.group[0].target)
    }
    return highlighted
  }, [collapsedEdges, edges, hoverKey, nodeIds])

  const nodeTypes = useMemo<NodeTypes>(() => ({ card: TopologyNodeCard, lane: LaneBandNode }), [])
  const edgeTypes = useMemo<EdgeTypes>(() => ({ topology: TopologyFlowEdgeComponent }), [])

  const flowNodes = useMemo<TopologyFlowNode[]>(() => {
    const cardNodes: CardFlowNode[] = nodes.map(node => ({
      id: node.id,
      type: 'card',
      position: layout.positions.get(node.id) ?? { x: 0, y: 0 },
      style: { width: NODE_WIDTH, height: NODE_HEIGHT },
      data: {
        node,
        category: node.category ?? categoryForLayer(layerByNodeId.get(node.id) ?? 0),
        dimmed: highlightedNodeIds !== null && !highlightedNodeIds.has(node.id),
        hovered: hoverKey === node.id,
        onSelectNode,
      },
      draggable: false,
      connectable: false,
      selectable: false,
    }))
    if (!laneLabel)
      return cardNodes
    const laneNodes: LaneFlowNode[] = layout.laneBands.map((band) => {
      const layer = layout.layers[band.index] ?? []
      return {
        id: `lane-${band.index}`,
        type: 'lane',
        position: { x: -LANE_PADDING_X, y: band.y },
        style: { width: layout.width + LANE_PADDING_X * 2, height: band.height },
        className: 'pointer-events-none',
        data: {
          ...laneLabel(band.index, layer.length),
          dimmed: highlightedNodeIds !== null && !layer.some(id => highlightedNodeIds.has(id)),
        },
        draggable: false,
        connectable: false,
        selectable: false,
        focusable: false,
        zIndex: -1,
      }
    })
    return [...laneNodes, ...cardNodes]
  }, [highlightedNodeIds, hoverKey, laneLabel, layerByNodeId, layout, nodes, onSelectNode])

  const flowEdges = useMemo<TopologyFlowEdge[]>(() => collapsedEdges.map(({ group, isBidirectional }, index) => {
    const primary = pickPrimaryEdge(group)
    const reversed = primary.source !== group[0].source
    const hoverGroupKey = edgeHoverKey(group[0])
    const dimmed = highlightedNodeIds !== null && !highlightedNodeIds.has(group[0].source) && !highlightedNodeIds.has(group[0].target)
    /* 高亮：hover 该边，或 hover 该边任意一端节点（邻域聚焦） */
    const hovered = hoverKey === hoverGroupKey
      || hoverKey === group[0].source
      || hoverKey === group[0].target
    const labelParts = [...new Set(group.map(edge => edge.label).filter((label): label is string => Boolean(label)))]
    const stroke = primary.tone === 'warning'
      ? 'var(--color-warning)'
      : primary.tone === 'danger'
        ? 'var(--color-danger)'
        : EDGE_CYCLE_COLORS[index % EDGE_CYCLE_COLORS.length]
    const offset = isBidirectional ? BIDIRECTIONAL_OFFSET : 0
    const marker = { type: MarkerType.ArrowClosed, width: 14, height: 14, color: stroke }
    return {
      id: `topology:${group.map(edge => edge.id).sort().join('~')}`,
      type: 'topology',
      source: primary.source,
      target: primary.target,
      className: onSelectEdge ? undefined : 'cursor-default',
      data: {
        dimmed,
        hoverKey: hoverGroupKey,
        label: labelParts.length > 0 ? labelParts.join(' ⇄ ') : undefined,
        offset: reversed ? -offset : offset,
        selectEdgeId: primary.id,
      },
      style: {
        stroke,
        /* hover 不改 strokeWidth，避免命中区域几何变化引发进出事件抖动；用透明度区分 */
        strokeWidth: 1.8,
        strokeDasharray: group.some(edge => edge.dashed) ? '6 5' : undefined,
        opacity: dimmed ? FADED_OPACITY : hovered ? 1 : 0.9,
        transition: 'opacity 200ms ease',
      },
      markerEnd: marker,
      ...(isBidirectional ? { markerStart: { ...marker, orient: 'auto-start-reverse' } } : {}),
    }
  }), [collapsedEdges, highlightedNodeIds, hoverKey, onSelectEdge])

  /* 进入：取消未决的退出，立即设置 hoverKey */
  const applyHover = useCallback((key: string) => {
    if (hoverClearRef.current) {
      clearTimeout(hoverClearRef.current)
      hoverClearRef.current = null
    }
    setHoverKey(key)
  }, [])
  /* 退出：延迟清空，期间的进入事件会取消本次退出，避免几何命中变化引起的 hoverKey 抖动 */
  const clearHover = useCallback(() => {
    if (hoverClearRef.current)
      clearTimeout(hoverClearRef.current)
    hoverClearRef.current = setTimeout(() => {
      hoverClearRef.current = null
      setHoverKey(null)
    }, 120)
  }, [])
  /* 卸载时清理未决定时器 */
  useEffect(() => () => {
    if (hoverClearRef.current)
      clearTimeout(hoverClearRef.current)
  }, [])
  /* 画布宽度变化（侧栏折叠、AI 助手面板拖宽）后重新取景，避免节点被裁出可视区 */
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas)
      return
    let lastWidth = canvas.clientWidth
    let frame = 0
    const observer = new ResizeObserver(([entry]) => {
      const width = Math.round(entry?.contentRect.width ?? canvas.clientWidth)
      if (width === lastWidth)
        return
      lastWidth = width
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => void fitView(FIT_VIEW_OPTIONS))
    })
    observer.observe(canvas)
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [fitView])
  const handleEdgeClick = useCallback((_: unknown, edge: TopologyFlowEdge) => {
    onSelectEdge?.(edge.data?.selectEdgeId ?? edge.id)
  }, [onSelectEdge])
  const handleEdgeMouseEnter = useCallback((_: unknown, edge: TopologyFlowEdge) => {
    applyHover(edge.data?.hoverKey ?? edge.id)
  }, [applyHover])
  const handleNodeMouseEnter = useCallback((_: unknown, node: TopologyFlowNode) => {
    if (node.type === 'card')
      applyHover(node.id)
  }, [applyHover])
  const handleNodeDoubleClick = useCallback((_: unknown, node: TopologyFlowNode) => {
    if (node.type !== 'card')
      return
    const position = layout.positions.get(node.id)
    if (position)
      void setCenter(position.x + NODE_WIDTH / 2, position.y + NODE_HEIGHT / 2, { duration: 300, zoom: 1.1 })
  }, [layout, setCenter])

  return (
    <div className="relative" data-slot="topology-graph">
      <div
        ref={canvasRef}
        aria-label={ariaLabel}
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
          fitViewOptions={FIT_VIEW_OPTIONS}
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

      <TopologyLegendBar legend={legend} />
    </div>
  )
}

/* ============================================================
   泳道背景节点：zIndex -1 渲染在边与节点之下
   ============================================================ */
function LaneBandNode({ data }: NodeProps<LaneFlowNode>) {
  return (
    <div
      className={cn(
        'h-full w-full border-y border-separator-subtle bg-surface-subtle/70 transition-opacity duration-standard',
        data.dimmed && 'opacity-40',
      )}
    >
      <div className="flex flex-col gap-0.5 p-3">
        <span className="text-xs font-semibold text-muted-foreground">{data.title}</span>
        <span className="text-xs text-muted-foreground/80">{data.subtitle}</span>
      </div>
    </div>
  )
}

/* ============================================================
   节点卡片：白底卡片 + 细边框 + 左侧 3px 分类色条，
   状态用色点 + 文字徽章双重编码（对齐 StatusBadge 五档 tone）
   ============================================================ */
function TopologyNodeCard({ data }: NodeProps<CardFlowNode>) {
  const navigate = useNavigate()
  const { node, category, dimmed, hovered, onSelectNode } = data
  const Icon = node.icon
  const className = cn(
    'nodrag relative block h-full w-full cursor-pointer rounded-container border border-border bg-surface py-3 pr-4 pl-5 text-left',
    'transition-[box-shadow,border-color,opacity,filter] duration-standard',
    'before:absolute before:top-2.5 before:bottom-2.5 before:left-0 before:w-[3px] before:rounded-full before:bg-(--topology-node-cat)',
    'hover:border-separator-strong hover:shadow-raised',
    'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
    dimmed && 'opacity-30 saturate-50',
    hovered && 'border-primary shadow-[0_0_0_3px_var(--primary-subtle)]',
  )
  const style = { '--topology-node-cat': CATEGORY_COLORS[category] } as CSSProperties
  const content = (
    <>
      <Handle className="!pointer-events-none !opacity-0" position={Position.Top} type="target" />
      <div className="flex items-center gap-2">
        <span
          className="grid size-7 flex-none place-items-center rounded-control"
          style={{
            color: CATEGORY_COLORS[category],
            backgroundColor: `color-mix(in oklab, ${CATEGORY_COLORS[category]} 12%, transparent)`,
          }}
        >
          <Icon className="size-4" />
        </span>
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold tracking-tight">{node.title}</div>
          <div className="truncate text-xs text-muted-foreground/80">{node.subtitle}</div>
        </div>
      </div>
      <div className="mt-2 flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
        <span className={cn('inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2 py-0.5 leading-none font-medium', tonePillClass(node.status.tone))}>
          <span className="size-1.5 flex-none rounded-full bg-current" />
          {node.status.label}
        </span>
        {node.meta && <span className="min-w-0 truncate font-mono tabular-nums">{node.meta}</span>}
        {node.counters && node.counters.length > 0 && (
          <span className="ml-auto inline-flex shrink-0 gap-2 tabular-nums text-muted-foreground/80">
            {node.counters.map(counter => <span key={counter}>{counter}</span>)}
          </span>
        )}
      </div>
      <Handle className="!pointer-events-none !opacity-0" position={Position.Bottom} type="source" />
    </>
  )

  if (node.href) {
    const href = node.href
    return (
      <Link
        aria-label={node.ariaLabel}
        className={className}
        style={style}
        title={node.actionLabel}
        to={href}
        onClick={(event) => {
          /* 走 SPA 路由跳转，阻止 <a> 默认整页刷新 */
          event.preventDefault()
          navigate(href)
        }}
      >
        {content}
      </Link>
    )
  }
  return (
    <button
      aria-label={node.ariaLabel}
      className={className}
      style={style}
      title={node.actionLabel}
      type="button"
      onClick={() => onSelectNode?.(node.id)}
    >
      {content}
    </button>
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
            className="nodrag nopan pointer-events-none absolute rounded-sm bg-surface/85 px-1 py-px font-mono text-xs text-muted-foreground"
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
function TopologyLegendBar({ legend }: { legend: TopologyGraphLegend }) {
  return (
    <div
      aria-label={legend.title}
      className="flex flex-wrap gap-x-8 gap-y-3 border-t border-border bg-surface-subtle px-5 py-3 text-xs text-muted-foreground"
    >
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-xs font-semibold text-muted-foreground/80">{legend.categoryTitle}</span>
        {legend.categories.map(item => (
          <span key={item.category} className="inline-flex items-center gap-1.5">
            <span className="size-3 rounded-sm" style={{ background: CATEGORY_COLORS[item.category] }} />
            {item.label}
          </span>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-xs font-semibold text-muted-foreground/80">{legend.statusTitle}</span>
        {legend.statuses.map(item => (
          <span key={item.key} className={cn('inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 leading-none font-medium', tonePillClass(item.tone))}>
            <span className="size-1.5 flex-none rounded-full bg-current" />
            {item.label}
          </span>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-xs font-semibold text-muted-foreground/80">{legend.relationTitle}</span>
        {legend.relations.map(item => (
          <span key={item.key} className="inline-flex items-center gap-1.5">
            <LegendLine bidirectional={item.bidirectional} dashed={item.dashed} />
            {item.label}
          </span>
        ))}
        {legend.notes.map(note => <span key={note} className="text-muted-foreground/70">{note}</span>)}
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
   连线归并
   ============================================================ */
function edgeHoverKey(edge: TopologyGraphEdge) {
  return `pair:${edge.source}:${edge.target}`
}

/** 双向线取排序后的第一条作为打开详情的代表边 */
function pickPrimaryEdge(group: TopologyGraphEdge[]) {
  return [...group].sort((a, b) => a.id.localeCompare(b.id))[0]
}

/** 同向多条边按稳定序展开；成对反向边合并为一条双向线 */
function collapseEdges(edges: TopologyGraphEdge[]): Array<{ group: TopologyGraphEdge[], isBidirectional: boolean }> {
  const ordered = [...edges].sort((a, b) => a.id.localeCompare(b.id))
  const seen = new Set<string>()
  const result: Array<{ group: TopologyGraphEdge[], isBidirectional: boolean }> = []
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

function layoutTopology(nodes: TopologyGraphNode[], edges: TopologyGraphEdge[]): TopologyLayout {
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

/** 分类色与层级对应：0 接入 / 1 核心 / 2 支撑 / ≥3 基础设施 */
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
      return 'border-border bg-muted text-muted-foreground'
  }
}
