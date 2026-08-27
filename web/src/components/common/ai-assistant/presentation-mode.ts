import { useSyncExternalStore } from 'react'
import { isAIAssistantRoutePath } from './route-state'

export const AI_ASSISTANT_DESKTOP_MIN_WIDTH = 1024
export const AI_ASSISTANT_DESKTOP_MIN_HEIGHT = 600
export const AI_ASSISTANT_FINE_POINTER_MEDIA_QUERY = '(pointer: fine)'
export const AI_ASSISTANT_HOVER_MEDIA_QUERY = '(hover: hover)'

export type AIAssistantPresentationMode = 'page' | 'window'

export interface AIAssistantPresentationEnvironment {
  pathname: string
  viewportWidth: number
  viewportHeight: number
  finePointer: boolean
  hover: boolean
}

export function canUseAIAssistantDesktopWindow(
  environment: Omit<AIAssistantPresentationEnvironment, 'pathname'>,
): boolean {
  return environment.viewportWidth >= AI_ASSISTANT_DESKTOP_MIN_WIDTH
    && environment.viewportHeight >= AI_ASSISTANT_DESKTOP_MIN_HEIGHT
    && environment.finePointer
    && environment.hover
}

export function resolveAIAssistantPresentationMode(
  environment: AIAssistantPresentationEnvironment,
): AIAssistantPresentationMode {
  if (isAIAssistantRoutePath(environment.pathname))
    return 'page'

  return canUseAIAssistantDesktopWindow(environment) ? 'window' : 'page'
}

function mediaMatches(query: string): boolean {
  return typeof window.matchMedia === 'function'
    && window.matchMedia(query).matches
}

function presentationModeSnapshot(pathname: string): AIAssistantPresentationMode {
  return resolveAIAssistantPresentationMode({
    pathname,
    viewportWidth: window.innerWidth,
    viewportHeight: window.innerHeight,
    finePointer: mediaMatches(AI_ASSISTANT_FINE_POINTER_MEDIA_QUERY),
    hover: mediaMatches(AI_ASSISTANT_HOVER_MEDIA_QUERY),
  })
}

function subscribePresentationEnvironment(onStoreChange: () => void): () => void {
  const mediaQueries = typeof window.matchMedia === 'function'
    ? [
        window.matchMedia(AI_ASSISTANT_FINE_POINTER_MEDIA_QUERY),
        window.matchMedia(AI_ASSISTANT_HOVER_MEDIA_QUERY),
      ]
    : []

  window.addEventListener('resize', onStoreChange)
  mediaQueries.forEach(mediaQuery => mediaQuery.addEventListener('change', onStoreChange))

  return () => {
    window.removeEventListener('resize', onStoreChange)
    mediaQueries.forEach(mediaQuery => mediaQuery.removeEventListener('change', onStoreChange))
  }
}

export function useAIAssistantPresentationMode(pathname: string): AIAssistantPresentationMode {
  return useSyncExternalStore(
    subscribePresentationEnvironment,
    () => presentationModeSnapshot(pathname),
    () => 'page',
  )
}
