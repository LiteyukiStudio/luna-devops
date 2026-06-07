import type { ReactNode } from 'react'
import { useCallback, useEffect, useMemo } from 'react'
import { Tabs } from '@/components/ui/tabs'
import { SegmentedTabsList } from './segmented-control'

interface ContentTabItem {
  hash?: string
  label: ReactNode
  value: string
}

interface ContentTabsProps {
  children: ReactNode
  hashKey?: string
  hashRouting?: boolean
  tabs: ContentTabItem[]
  tools?: ReactNode
  value: string
  onValueChange: (value: string) => void
}

/**
 * 页面正文区域的二级内容切换容器。
 * 用于项目详情、设置页等同一页面内的 tab 分区；当前 tab 的新增、刷新、导出等操作应放在 tools，不要散落在嵌入子页的 PageHeader 里。
 */
export function ContentTabs({
  children,
  hashKey = 'tab',
  hashRouting = true,
  tabs,
  tools,
  value,
  onValueChange,
}: ContentTabsProps) {
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

  return (
    <Tabs value={value} onValueChange={handleValueChange}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <SegmentedTabsList items={tabs} layoutId="content-tabs-active" value={value} />
        {tools && (
          <div className="flex flex-wrap items-center gap-2 sm:justify-end">
            {tools}
          </div>
        )}
      </div>
      {children}
    </Tabs>
  )
}
