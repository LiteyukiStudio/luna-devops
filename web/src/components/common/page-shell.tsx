import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

type PageShellWidth = 'content' | 'full' | 'settings' | 'tool'
type PageShellSpacing = 'compact' | 'normal'

/**
 * 页面正文的统一根容器。
 * 列表、概览、设置和工具工作区通过 width 表达内容边界，页面不再各自维护互相冲突的根宽度与间距。
 */
export function PageShell({ className, spacing = 'normal', width = 'full', ...props }: ComponentProps<'div'> & {
  spacing?: PageShellSpacing
  width?: PageShellWidth
}) {
  return (
    <div
      className={cn(
        'grid min-w-0 w-full',
        spacing === 'normal' ? 'gap-6' : 'gap-4',
        width === 'content' && 'mx-auto max-w-7xl',
        width === 'settings' && 'mx-auto max-w-6xl',
        width === 'tool' && 'min-h-0',
        className,
      )}
      data-slot="page-shell"
      {...props}
    />
  )
}
