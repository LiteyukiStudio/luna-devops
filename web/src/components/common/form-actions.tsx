import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

/**
 * 页面表单的统一操作区。
 * 桌面端将提交操作收拢到右侧正常宽度，移动端保持易点击的全宽按钮；可选分隔线用于结束较长表单。
 */
export function FormActions({ className, separated = true, ...props }: ComponentProps<'div'> & { separated?: boolean }) {
  return (
    <div
      className={cn(
        'flex flex-col-reverse gap-2 @lg/workspace:flex-row @lg/workspace:items-center @lg/workspace:justify-end [&>button]:w-full @lg/workspace:[&>button]:w-auto',
        separated && 'border-t border-border pt-4',
        className,
      )}
      data-slot="form-actions"
      {...props}
    />
  )
}
