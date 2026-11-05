import type { ReactNode } from 'react'
import { use, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { WorkspaceChromeTargetsContext } from '@/components/common/workspace-chrome-context'
import { cn } from '@/lib/utils'

type ChromeSlot = 'description' | 'heading' | 'tabs' | 'tools'

function hasContent(children: ReactNode) {
  return children !== null && children !== undefined && children !== false
}

/**
 * 在页面挂载期间登记插槽占用，并返回布局提供的投放目标。
 * 没有布局（如独立路由或测试）时返回 null，调用方在原位渲染。
 */
function useChromeSlot(slot: ChromeSlot, active: boolean) {
  const targets = use(WorkspaceChromeTargetsContext)
  const register = slot === 'heading' ? targets?.registerHeading : slot === 'tabs' ? targets?.registerTabs : slot === 'tools' ? targets?.registerTools : undefined

  useEffect(() => {
    if (!active || !register)
      return

    return register()
  }, [active, register])

  return { hasLayout: Boolean(targets), target: targets?.[slot] ?? null }
}

/**
 * 替换布局页头默认标题。只有页面标题需要比路由名更具体时使用（例如编辑器标明正在编辑的对象）。
 */
export function PageChromeHeading({ description, title }: { description?: ReactNode, title: string }) {
  const { hasLayout, target } = useChromeSlot('heading', true)
  const { target: descriptionTarget } = useChromeSlot('description', Boolean(description))
  const heading = <h1 className="truncate text-base font-medium text-foreground">{title}</h1>
  const descriptionText = description ? <p className="text-sm text-muted-foreground">{description}</p> : null

  if (!hasLayout) {
    return (
      <div className="grid min-w-0 gap-1">
        {heading}
        {descriptionText}
      </div>
    )
  }

  return (
    <>
      {target && createPortal(heading, target)}
      {descriptionTarget && descriptionText && createPortal(descriptionText, descriptionTarget)}
    </>
  )
}

/**
 * 将页面级操作放入正文页头右侧。
 */
export function PageChromeTools({ children, className }: { children?: ReactNode, className?: string }) {
  const active = hasContent(children)
  const { hasLayout, target } = useChromeSlot('tools', active)

  if (!active)
    return null

  const tools = <div className={cn('flex min-w-0 flex-wrap items-center justify-end gap-2', className)}>{children}</div>
  if (!hasLayout)
    return tools

  return target ? createPortal(tools, target) : null
}

/**
 * 将页面级 Tab 放入正文页头下方。布局只提供插槽，不持有业务 Tab 状态。
 */
export function PageChromeTabs({ children, className }: { children?: ReactNode, className?: string }) {
  const active = hasContent(children)
  const { hasLayout, target } = useChromeSlot('tabs', active)

  if (!active)
    return null

  const tabs = <div className={cn('min-w-0', className)}>{children}</div>
  if (!hasLayout)
    return tabs

  return target ? createPortal(tabs, target) : null
}
