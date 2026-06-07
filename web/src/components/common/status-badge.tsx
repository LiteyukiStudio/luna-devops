import type { ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'

/**
 * 资源状态的统一小徽标。
 * 用于角色、状态、scope 等短文本标签；复杂状态说明或带操作的提示请使用 Alert/Card 组合。
 */
export function StatusBadge({ children, className }: { children: ReactNode, className?: string }) {
  return <Badge className={className}>{children}</Badge>
}
