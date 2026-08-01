import { createContext } from 'react'

/**
 * 布局提供的页面级 UI 插槽。
 * 页面仍拥有导航和操作状态，只通过 Portal 将视图投放到工作区 Topbar。
 */
export interface WorkspaceChromeTargets {
  registerTabs: () => () => void
  registerTools: () => () => void
  tabs: HTMLElement | null
  tools: HTMLElement | null
}

export const WorkspaceChromeTargetsContext = createContext<WorkspaceChromeTargets | null>(null)

export const WorkspaceChromeTargetsProvider = WorkspaceChromeTargetsContext.Provider
