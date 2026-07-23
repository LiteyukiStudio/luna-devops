import { createContext } from 'react'

export interface PageChromeTargets {
  tabs: HTMLElement | null
  tools: HTMLElement | null
}

export const PageChromeTargetsContext = createContext<PageChromeTargets>({
  tabs: null,
  tools: null,
})

export const PageChromeTargetsProvider = PageChromeTargetsContext.Provider
