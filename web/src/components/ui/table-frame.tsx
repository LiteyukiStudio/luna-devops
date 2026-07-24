import type { ComponentProps, ReactNode } from 'react'

import { cn } from '@/lib/utils'
import { ScrollArea } from './scroll-area'

type TableFrameProps = ComponentProps<'div'> & {
  children: ReactNode
  framed?: boolean
  scrollAreaClassName?: string
  scrollbars?: ComponentProps<typeof ScrollArea>['scrollbars']
  scrollType?: ComponentProps<typeof ScrollArea>['type']
}

/**
 * 表格的边界与滚动容器。
 * 容器使用原生边框和圆角裁剪统一持有视觉边界，ScrollArea 只负责滚动。
 * 避免用嵌套背景模拟边框，防止圆角抗锯齿和 sticky 合成层产生接缝。
 */
function TableFrame({
  children,
  className,
  framed = true,
  scrollAreaClassName,
  scrollbars = 'horizontal',
  scrollType = 'auto',
  ...props
}: TableFrameProps) {
  return (
    <div
      className={cn(
        'relative min-h-0 min-w-0 max-w-full overflow-hidden',
        framed
          ? 'rounded-container border border-border bg-card'
          : 'border-0 bg-transparent',
        className,
      )}
      data-slot="table-frame"
      {...props}
    >
      <ScrollArea
        className={cn('min-h-0 w-full min-w-0 max-w-full bg-transparent', scrollAreaClassName)}
        scrollbars={scrollbars}
        type={scrollType}
      >
        {children}
      </ScrollArea>
    </div>
  )
}

export { TableFrame }
