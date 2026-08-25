export const WINDOW_STORAGE_KEY = 'luna.ai-assistant.window.v2'
export const LAUNCHER_STORAGE_KEY = 'luna.ai-assistant.launcher.v1'
export const VIEWPORT_GUTTER = 24
export const PAGE_LAUNCHER_GUTTER = 48
export const MIN_WINDOW_WIDTH = 360
export const MIN_WINDOW_HEIGHT = 480
export const LAUNCHER_SIZE = 56
export const AI_ASSISTANT_SPLIT_MIN_WIDTH = 720
export const AI_ASSISTANT_SIDEBAR_WIDTH = 264

const DEFAULT_WINDOW_WIDTH = 420
const DEFAULT_WINDOW_HEIGHT = 640

export interface WindowPreference {
  x: number
  y: number
  width: number
  height: number
}

export interface Position {
  x: number
  y: number
}

export type AIDesktopConversationLayout = 'overlay' | 'split'

export function resolveAIDesktopConversationLayout(width: number): AIDesktopConversationLayout {
  return width >= AI_ASSISTANT_SPLIT_MIN_WIDTH ? 'split' : 'overlay'
}

function viewportSize(): { width: number, height: number } {
  return {
    width: typeof window === 'undefined' ? 1280 : window.innerWidth,
    height: typeof window === 'undefined' ? 800 : window.innerHeight,
  }
}

export function clampAssistantPosition(position: Position, width: number, height: number, gutter = VIEWPORT_GUTTER): Position {
  const viewport = viewportSize()
  return {
    x: Math.min(Math.max(gutter, viewport.width - width - gutter), Math.max(gutter, position.x)),
    y: Math.min(Math.max(gutter, viewport.height - height - gutter), Math.max(gutter, position.y)),
  }
}

function defaultWindowPreference(): WindowPreference {
  const viewport = viewportSize()
  const width = Math.min(DEFAULT_WINDOW_WIDTH, viewport.width - VIEWPORT_GUTTER * 2)
  const height = Math.min(DEFAULT_WINDOW_HEIGHT, viewport.height - VIEWPORT_GUTTER * 2)
  return { x: viewport.width - width - VIEWPORT_GUTTER, y: viewport.height - height - VIEWPORT_GUTTER, width, height }
}

export function readWindowPreference(): WindowPreference {
  try {
    const value = JSON.parse(localStorage.getItem(WINDOW_STORAGE_KEY) ?? '')
    if (typeof value.x === 'number' && typeof value.y === 'number' && typeof value.width === 'number' && typeof value.height === 'number') {
      const viewport = viewportSize()
      const width = Math.min(viewport.width - VIEWPORT_GUTTER * 2, Math.max(MIN_WINDOW_WIDTH, value.width))
      const height = Math.min(viewport.height - VIEWPORT_GUTTER * 2, Math.max(MIN_WINDOW_HEIGHT, value.height))
      return { ...clampAssistantPosition(value, width, height), width, height }
    }
  }
  catch {}
  return defaultWindowPreference()
}

function defaultLauncherPosition(): Position {
  const viewport = viewportSize()
  return { x: viewport.width - LAUNCHER_SIZE - VIEWPORT_GUTTER, y: viewport.height - LAUNCHER_SIZE - VIEWPORT_GUTTER }
}

export function readLauncherPosition(): Position {
  try {
    const value = JSON.parse(localStorage.getItem(LAUNCHER_STORAGE_KEY) ?? '')
    if (typeof value.x === 'number' && typeof value.y === 'number')
      return clampAssistantPosition(value, LAUNCHER_SIZE, LAUNCHER_SIZE)
  }
  catch {}
  return defaultLauncherPosition()
}
