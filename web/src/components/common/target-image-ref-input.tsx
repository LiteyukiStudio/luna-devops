import type { ComponentProps } from 'react'
import { Input } from '@/components/ui/input'

interface TargetImageRefInputProps extends ComponentProps<typeof Input> {
  prefix: string
}

/** 目标镜像输入：镜像站主机作为不可编辑前缀，输入框只承载仓库路径与 Tag。 */
export function TargetImageRefInput({ className, prefix, ...props }: TargetImageRefInputProps) {
  if (!prefix)
    return <Input className={className} {...props} />

  return (
    <div className="flex min-w-0 overflow-hidden rounded-md border border-input bg-background shadow-sm focus-within:ring-2 focus-within:ring-ring/50">
      <span aria-hidden="true" className="flex shrink-0 items-center border-r border-border bg-muted px-3 text-sm text-muted-foreground">
        {prefix}
      </span>
      <Input
        {...props}
        className="rounded-none border-0 shadow-none focus-visible:ring-0"
      />
    </div>
  )
}
