import type { ComponentProps } from 'react'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

interface TargetImageRefInputProps extends ComponentProps<typeof Input> {
  prefix: string
}

/** 目标镜像输入：镜像站主机作为不可编辑前缀（超长自动截断），输入框只承载仓库路径与 Tag。 */
export function TargetImageRefInput({ className, prefix, ...props }: TargetImageRefInputProps) {
  if (!prefix)
    return <Input className={className} {...props} />

  const isInvalid = props['aria-invalid'] === true || props['aria-invalid'] === 'true'

  return (
    <div
      className={cn(
        'flex h-9 min-w-0 overflow-hidden rounded-control border border-input bg-surface shadow-xs transition-[color,box-shadow] dark:bg-input/30',
        isInvalid
          ? 'border-danger ring-danger/20 focus-within:border-danger focus-within:ring-3 focus-within:ring-danger/20 dark:ring-danger/40'
          : 'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50',
      )}
    >
      <span
        aria-hidden="true"
        className="max-w-36 shrink-0 truncate border-r border-border bg-surface-subtle px-2.5 font-mono text-xs leading-9 text-muted-foreground sm:max-w-48"
        title={prefix}
      >
        {prefix}
      </span>
      <Input
        {...props}
        className={cn(
          'h-full min-w-0 flex-1 rounded-none border-0 bg-transparent px-3 font-mono text-sm shadow-none focus-visible:ring-0 aria-invalid:border-0 aria-invalid:ring-0 dark:bg-transparent',
          className,
        )}
      />
    </div>
  )
}
