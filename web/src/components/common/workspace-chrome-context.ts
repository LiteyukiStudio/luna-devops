import { createContext } from 'react'

/**
 * 布局提供的页面级 UI 插槽。
 * 页面仍拥有标题、导航和操作状态，只通过 Portal 将视图投放到正文顶部的页头。
 */
export interface WorkspaceChromeTargets {
  registerHeading: () => () => void
  registerTabs: () => () => void
  registerTools: () => () => void
  heading: HTMLElement | null
  tabs: HTMLElement | null
  tools: HTMLElement | null
}

export const WorkspaceChromeTargetsContext = createContext<WorkspaceChromeTargets | null>(null)

export const WorkspaceChromeTargetsProvider = WorkspaceChromeTargetsContext.Provider
