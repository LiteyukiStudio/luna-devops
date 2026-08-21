import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

type MotionDivProps = ComponentProps<'div'>

/**
 * 页面级轻量入场动画。
 * 用于路由页面或大块内容切换，避免每个页面重复声明 motion 参数。
 */
export function PageMotion({ className, ...props }: MotionDivProps) {
  return (
    <div
      className={cn('luna-page-motion', className)}
      {...props}
    />
  )
}

/**
 * 带子项错峰入场的列表容器。
 * 与 MotionItem 成对使用，适合资源行列表；表格大量行或虚拟列表不要使用逐项动画。
 */
export function MotionList({ className, ...props }: MotionDivProps) {
  return (
    <div
      className={cn('luna-motion-list', className)}
      {...props}
    />
  )
}

/**
 * MotionList 的单个子项动画包装。
 * 用于卡片行、设置项等少量可感知列表项；不要包裹会频繁重排的复杂输入控件。
 */
export function MotionItem({ className, ...props }: MotionDivProps) {
  return (
    <div
      className={cn('luna-motion-item', className)}
      {...props}
    />
  )
}
