import type { ReactNode } from 'react'
import { Inbox } from 'lucide-react'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { cn } from '@/lib/utils'

interface EmptyStateProps {
  title: string
  description?: ReactNode
  actions?: ReactNode
  icon?: ReactNode
  mode?: 'actionable' | 'filtered'
  variant?: 'card' | 'plain'
}

/**
 * 列表、搜索结果和资源集合为空时的统一空状态。
 * 用于告诉用户当前没有数据并可附带创建/重试动作；加载中或接口失败分别使用 loading UI 和 ErrorState。
 */
export function EmptyState({ title, description, actions, icon, mode = 'actionable', variant = 'card' }: EmptyStateProps) {
  const filtered = mode === 'filtered'
  return (
    <Empty
      className={cn(
        filtered && 'min-h-32 py-8',
        variant === 'plain' && 'rounded-none border-0 bg-transparent shadow-none',
      )}
    >
      <div className="mb-4 flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground" aria-hidden="true">
        {icon ?? <Inbox className="size-5" />}
      </div>
      <EmptyHeader className="max-w-md justify-items-center">
        <EmptyTitle>{title}</EmptyTitle>
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
      {actions && <EmptyContent>{actions}</EmptyContent>}
    </Empty>
  )
}
