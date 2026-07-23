import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

export type SurfaceVariant = 'bordered' | 'inset' | 'plain' | 'raised'

/**
 * 业务内容的语义表面。
 * 用于替代页面把 shadcn Card 当作所有内容默认外壳的做法；重复资源条目仍可继续使用 Card。
 */
export function Surface({ className, variant = 'bordered', ...props }: ComponentProps<'div'> & { variant?: SurfaceVariant }) {
  return (
    <div
      className={cn(
        'min-w-0',
        variant === 'plain' && 'bg-transparent',
        variant === 'bordered' && 'rounded-lg border border-border bg-surface-raised',
        variant === 'raised' && 'rounded-lg border border-border bg-surface-raised shadow-raised',
        variant === 'inset' && 'rounded-lg border border-border bg-surface-inset',
        className,
      )}
      data-slot="surface"
      data-variant={variant}
      {...props}
    />
  )
}
