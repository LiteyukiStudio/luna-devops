import type { ComponentType, ReactNode } from 'react'
import { motion } from 'motion/react'
import { TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

export interface SegmentedControlItem<Value extends string = string> {
  icon?: ComponentType<{ size?: number }>
  label: ReactNode
  value: Value
}

interface SegmentedControlProps<Value extends string = string> {
  items: Array<SegmentedControlItem<Value>>
  layoutId: string
  value: Value
  ariaLabel?: string
  equalColumns?: boolean
  showLabels?: boolean
  onValueChange: (value: Value) => void
}

interface SegmentedTabsListProps {
  items: Array<SegmentedControlItem>
  layoutId: string
  value: string
}

/**
 * 少量互斥模式的分段选择控件。
 * 用于主题模式、视图模式、状态过滤等 2-5 个固定选项；候选很多或需要搜索时使用 SearchSelect。
 */
export function SegmentedControl<Value extends string = string>({
  ariaLabel,
  equalColumns,
  items,
  layoutId,
  showLabels = true,
  value,
  onValueChange,
}: SegmentedControlProps<Value>) {
  return (
    <div
      aria-label={ariaLabel}
      className={segmentedRootClassName(equalColumns)}
      role="group"
      style={equalColumns ? { gridTemplateColumns: `repeat(${items.length}, minmax(0, 1fr))` } : undefined}
    >
      {items.map((item) => {
        const active = value === item.value
        const Icon = item.icon
        return (
          <button
            key={item.value}
            aria-label={typeof item.label === 'string' ? item.label : undefined}
            aria-pressed={active}
            className={segmentedItemClassName(active, !showLabels)}
            type="button"
            onClick={() => onValueChange(item.value)}
          >
            {active && <SegmentedActivePill layoutId={layoutId} />}
            {Icon && <Icon size={15} />}
            {showLabels && <span className="truncate">{item.label}</span>}
          </button>
        )
      })}
    </div>
  )
}

/**
 * ContentTabs 内部使用的分段式 tab 列表。
 * 用于 shadcn Tabs 语义下的二级页面切换；业务页通常直接使用 ContentTabs，而不是手动调用它。
 */
export function SegmentedTabsList({ items, layoutId, value }: SegmentedTabsListProps) {
  return (
    <TabsList className={segmentedRootClassName(false)}>
      {items.map((item) => {
        const active = value === item.value
        const Icon = item.icon
        return (
          <TabsTrigger key={item.value} className={segmentedItemClassName(active)} value={item.value}>
            {active && <SegmentedActivePill layoutId={layoutId} />}
            {Icon && <Icon size={15} />}
            <span className="truncate">{item.label}</span>
          </TabsTrigger>
        )
      })}
    </TabsList>
  )
}

function SegmentedActivePill({ layoutId }: { layoutId: string }) {
  return (
    <motion.span
      className="absolute inset-0 -z-10 rounded-full bg-surface shadow-sm"
      layoutId={layoutId}
      transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
    />
  )
}

function segmentedRootClassName(equalColumns?: boolean) {
  return cn(
    'relative gap-1 rounded-full bg-muted p-1',
    equalColumns ? 'grid' : 'inline-flex max-w-full flex-wrap items-center',
  )
}

function segmentedItemClassName(active: boolean, iconOnly?: boolean) {
  return cn(
    'relative z-10 flex h-8 min-w-0 items-center justify-center gap-1.5 rounded-full px-3 text-sm font-medium text-muted-foreground transition-colors duration-150 outline-none disabled:pointer-events-none disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-ring/50',
    iconOnly && 'px-0',
    'data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none',
    active && 'text-primary',
    !active && 'hover:bg-surface/70 hover:text-foreground',
  )
}
