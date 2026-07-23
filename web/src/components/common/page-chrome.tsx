import type { ReactNode } from 'react'
import { use } from 'react'
import { createPortal } from 'react-dom'
import { PageChromeTargetsContext } from '@/components/common/page-chrome-context'
import { cn } from '@/lib/utils'

/**
 * 登录后页面的统一头部框架。
 * 第一行固定承载标题与页面工具；存在 tabs 时才显示第二行导航。
 */
export function PageChrome({
  tabsTargetRef,
  title,
  toolsTargetRef,
}: {
  tabsTargetRef: (node: HTMLDivElement | null) => void
  title: ReactNode
  toolsTargetRef: (node: HTMLDivElement | null) => void
}) {
  return (
    <div className="hidden min-w-0 flex-col gap-4 lg:flex" data-slot="page-chrome">
      <div className="flex min-w-0 items-center justify-between gap-6" data-slot="page-chrome-title-row">
        <div className="min-w-0 flex-1">{title}</div>
        <div ref={toolsTargetRef} className="flex min-w-0 shrink-0 items-center justify-end empty:hidden" />
      </div>
      <div ref={tabsTargetRef} className="min-w-0 empty:hidden" data-slot="page-chrome-tabs-row" />
    </div>
  )
}

/**
 * 将页面级操作放入 PageChrome 标题行；中小屏回落到正文流。
 */
export function PageChromeTools({ children, className }: { children?: ReactNode, className?: string }) {
  const { tools } = use(PageChromeTargetsContext)

  if (!children)
    return null

  return (
    <>
      {tools && createPortal(
        <div className={cn('flex min-w-0 flex-wrap items-center justify-end gap-2', className)}>
          {children}
        </div>,
        tools,
      )}
      <div className={cn('flex min-w-0 flex-wrap items-center gap-2 lg:hidden', className)}>
        {children}
      </div>
    </>
  )
}

/**
 * 将可选的页面级 Tab 放入 PageChrome 第二行；中小屏内容由调用方提供。
 */
export function PageChromeTabs({ children, className }: { children?: ReactNode, className?: string }) {
  const { tabs } = use(PageChromeTargetsContext)

  if (!children || !tabs)
    return null

  return createPortal(<div className={cn('min-w-0', className)}>{children}</div>, tabs)
}
