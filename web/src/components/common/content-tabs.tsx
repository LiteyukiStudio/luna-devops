import type { ReactNode } from 'react'
import { motion, useReducedMotion } from 'motion/react'
import { useCallback, useEffect, useId, useMemo } from 'react'
import { PageChromeTabs, PageChromeTools } from '@/components/common/page-chrome'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

interface ContentTabItem {
  hash?: string
  label: ReactNode
  value: string
}

interface ContentTabsProps {
  children: ReactNode
  hashKey?: string
  hashRouting?: boolean
  headerClassName?: string
  tabs: ContentTabItem[]
  tools?: ReactNode
  value: string
  onValueChange: (value: string) => void
}

/**
 * 页面正文区域的二级内容切换容器。
 * 用于项目详情、设置页等同一页面内的 tab 分区。
 * 组件只负责 tab 导航和内容切换。当前 tab 的新增、刷新、导出等页面级操作通过 tools
 * 提升到桌面端标题行；设置表单的保存操作统一放在对应表单底部。
 */
export function ContentTabs({
  children,
  hashKey = 'tab',
  hashRouting = true,
  headerClassName,
  tabs,
  tools,
  value,
  onValueChange,
}: ContentTabsProps) {
  const indicatorLayoutId = `content-tabs-indicator-${useId()}`
  const reducedMotion = useReducedMotion()
  const { routeToValue, valueToRoute } = useMemo(() => {
    const routeToValue = new Map<string, string>()
    const valueToRoute = new Map<string, string>()

    for (const tab of tabs) {
      const route = tab.hash ?? tab.value
      routeToValue.set(route, tab.value)
      valueToRoute.set(tab.value, route)
    }

    return { routeToValue, valueToRoute }
  }, [tabs])

  const readHashRoute = useCallback(() => {
    if (!hashRouting || typeof window === 'undefined')
      return null

    const hash = window.location.hash.replace(/^#/, '')
    if (!hash)
      return null

    const params = new URLSearchParams(hash)
    return params.get(hashKey)
  }, [hashKey, hashRouting])

  const writeHashRoute = useCallback((route: string) => {
    if (!hashRouting || typeof window === 'undefined')
      return

    const hash = window.location.hash.replace(/^#/, '')
    const params = new URLSearchParams(hash)
    params.set(hashKey, route)

    const nextHash = params.toString()
    const nextUrl = `${window.location.pathname}${window.location.search}#${nextHash}`
    window.history.replaceState(null, '', nextUrl)
  }, [hashKey, hashRouting])

  useEffect(() => {
    if (!hashRouting)
      return

    const syncFromHash = () => {
      const route = readHashRoute()
      if (!route)
        return

      const nextValue = routeToValue.get(route)
      if (nextValue && nextValue !== value)
        onValueChange(nextValue)
    }

    syncFromHash()
    window.addEventListener('hashchange', syncFromHash)
    return () => window.removeEventListener('hashchange', syncFromHash)
  }, [hashRouting, onValueChange, readHashRoute, routeToValue, value])

  const handleValueChange = useCallback((nextValue: string) => {
    const route = valueToRoute.get(nextValue)
    if (route)
      writeHashRoute(route)
    onValueChange(nextValue)
  }, [onValueChange, valueToRoute, writeHashRoute])

  const effectiveValue = valueToRoute.has(value) ? value : tabs[0]?.value ?? value

  return (
    <Tabs value={effectiveValue} onValueChange={handleValueChange}>
      <div className={cn('min-w-0 lg:hidden', headerClassName)}>
        <Select value={effectiveValue} onValueChange={handleValueChange}>
          <SelectTrigger className="h-10 w-full min-w-0 justify-between bg-muted text-base shadow-none">
            <SelectValue />
          </SelectTrigger>
          <SelectContent align="start" className="min-w-[var(--radix-select-trigger-width)]" position="popper">
            {tabs.map(tab => (
              <SelectItem key={tab.value} value={tab.value}>
                {tab.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <PageChromeTabs className={headerClassName}>
        <div className="-mx-1 min-w-0 overflow-x-auto px-1">
          <TabsList className="w-max max-w-none flex-nowrap">
            {tabs.map(tab => (
              <TabsTrigger key={tab.value} className="relative data-[state=active]:border-transparent" value={tab.value}>
                <span className="truncate">{tab.label}</span>
                {tab.value === effectiveValue && (
                  <motion.span
                    aria-hidden="true"
                    className="absolute inset-x-0 -bottom-px h-0.5 bg-primary"
                    layoutId={indicatorLayoutId}
                    transition={reducedMotion ? { duration: 0 } : { duration: 0.2, ease: [0.16, 1, 0.3, 1] }}
                  />
                )}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
      </PageChromeTabs>
      <PageChromeTools>{tools}</PageChromeTools>
      {children}
    </Tabs>
  )
}
