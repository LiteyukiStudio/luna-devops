import type { ReactNode } from 'react'

/**
 * 页面级操作栏。
 * 当前实现只承载 actions，页面标题由布局或上级区域负责；嵌入 ContentTabs 的子页不要使用它，tab 级按钮应交给 ContentTabs.tools。
 */
export function PageHeader({ title, description, actions }: { title: string, description?: string, actions?: ReactNode }) {
  void title
  void description

  if (!actions)
    return null

  return (
    <div className="flex flex-wrap items-center justify-end gap-3">
      {actions}
    </div>
  )
}
