import { safeStorageGet, safeStorageSet } from '@/lib/safe-storage'

export const WINDOW_STORAGE_KEY = 'luna.ai-assistant.window.v2'
const LAUNCHER_STORAGE_KEY = 'luna.ai-assistant.launcher.v2'
export const VIEWPORT_GUTTER = 24
export const PAGE_LAUNCHER_GUTTER = 48
export const MIN_WINDOW_WIDTH = 360
export const MIN_WINDOW_HEIGHT = 480
export const LAUNCHER_SIZE = 48
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
    width: window.innerWidth,
    height: window.innerHeight,
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
    const value = JSON.parse(safeStorageGet(WINDOW_STORAGE_KEY) ?? '')
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

/**
 * 启动器位置按“距右边、距下边”的偏移保存，窗口缩放后仍贴着用户放置的角落；
 * 只在用户拖动时保存，窗口变窄导致的临时夹取不会覆盖用户的选择。
 */
export function readLauncherPosition(gutter = VIEWPORT_GUTTER): Position {
  try {
    const value = JSON.parse(safeStorageGet(LAUNCHER_STORAGE_KEY) ?? '')
    if (typeof value.right === 'number' && typeof value.bottom === 'number') {
      const viewport = viewportSize()
      return clampAssistantPosition(
        { x: viewport.width - LAUNCHER_SIZE - value.right, y: viewport.height - LAUNCHER_SIZE - value.bottom },
        LAUNCHER_SIZE,
        LAUNCHER_SIZE,
        gutter,
      )
    }
  }
  catch {}
  return clampAssistantPosition(defaultLauncherPosition(), LAUNCHER_SIZE, LAUNCHER_SIZE, gutter)
}

export function saveLauncherPosition(position: Position) {
  const viewport = viewportSize()
  safeStorageSet(LAUNCHER_STORAGE_KEY, JSON.stringify({
    bottom: Math.max(0, viewport.height - LAUNCHER_SIZE - position.y),
    right: Math.max(0, viewport.width - LAUNCHER_SIZE - position.x),
  }))
}
