import type { ComponentProps } from 'react'
import { Pencil } from 'lucide-react'
import { Button } from '@/components/ui/button'

type EditActionButtonProps = Omit<ComponentProps<typeof Button>, 'children'> & {
  label: string
}

/**
 * 列表行和详情区的统一编辑按钮。
 * 用于资源行内的“编辑”动作，保持图标、尺寸和语义一致；批量操作或主按钮请直接使用 Button。
 */
export function EditActionButton({ label, ...props }: EditActionButtonProps) {
  return (
    <Button variant="secondary" {...props}>
      <Pencil size={16} />
      {label}
    </Button>
  )
}
