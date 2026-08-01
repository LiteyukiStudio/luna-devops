import type { ReactNode } from 'react'
import { ArrowLeft } from 'lucide-react'
import { use, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'
import { WorkspaceChromeTargetsContext } from '@/components/common/workspace-chrome-context'
import { cn } from '@/lib/utils'

interface PageChromeBackNavigation {
  label: string
  to: string
}

export function PageBackNavigation({
  className,
  label,
  to,
}: PageChromeBackNavigation & { className?: string }) {
  return (
    <Link
      className={cn(
        'inline-flex w-fit items-center gap-1.5 rounded-control px-1 py-1 text-sm font-medium text-muted-foreground outline-none transition-colors duration-fast ease-standard hover:bg-surface-subtle hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring',
        className,
      )}
      data-slot="page-chrome-back-navigation"
      to={to}
    >
      <ArrowLeft aria-hidden="true" className="size-4" />
      <span>{label}</span>
    </Link>
  )
}

/**
 * 将页面级操作放入 Topbar 的导航操作行；中小屏回落到正文流。
 */
export function PageChromeTools({ children, className }: { children?: ReactNode, className?: string }) {
  const targets = use(WorkspaceChromeTargetsContext)
  const tools = targets?.tools
  const registerTools = targets?.registerTools
  const hasChildren = children !== null && children !== undefined && children !== false

  useEffect(() => {
    if (!hasChildren || !registerTools)
      return

    return registerTools()
  }, [hasChildren, registerTools])

  if (!hasChildren)
    return null

  return (
    <>
      {tools && createPortal(
        <div className={cn('flex min-w-0 flex-nowrap items-center justify-end gap-2', className)}>
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
 * 将页面级 Tab 放入工作区 Topbar。布局只提供插槽，不持有业务 Tab 状态。
 */
export function PageChromeTabs({ children, className }: { children?: ReactNode, className?: string }) {
  const targets = use(WorkspaceChromeTargetsContext)
  const registerTabs = targets?.registerTabs
  const hasChildren = children !== null && children !== undefined && children !== false

  useEffect(() => {
    if (!hasChildren || !registerTabs)
      return

    return registerTabs()
  }, [hasChildren, registerTabs])

  if (!hasChildren)
    return null

  if (!targets)
    return <div className={cn('min-w-0', className)}>{children}</div>

  if (!targets.tabs)
    return null

  return createPortal(<div className={cn('min-w-0', className)}>{children}</div>, targets.tabs)
}
