import { useEffect, useState } from 'react'

export const WINDOW_STORAGE_KEY = 'luna.ai-assistant.window.v2'
export const LAUNCHER_STORAGE_KEY = 'luna.ai-assistant.launcher.v1'
export const VIEWPORT_GUTTER = 24
export const MIN_WINDOW_WIDTH = 360
export const MIN_WINDOW_HEIGHT = 480
export const LAUNCHER_SIZE = 56

const DESKTOP_MEDIA_QUERY = '(min-width: 640px)'
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

function viewportSize(): { width: number, height: number } {
  return {
    width: typeof window === 'undefined' ? 1280 : window.innerWidth,
    height: typeof window === 'undefined' ? 800 : window.innerHeight,
  }
}

export function clampAssistantPosition(position: Position, width: number, height: number): Position {
  const viewport = viewportSize()
  return {
    x: Math.min(Math.max(VIEWPORT_GUTTER, viewport.width - width - VIEWPORT_GUTTER), Math.max(VIEWPORT_GUTTER, position.x)),
    y: Math.min(Math.max(VIEWPORT_GUTTER, viewport.height - height - VIEWPORT_GUTTER), Math.max(VIEWPORT_GUTTER, position.y)),
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

export function useDesktopViewport() {
  const [desktop, setDesktop] = useState(() => typeof window === 'undefined' || !window.matchMedia || window.matchMedia(DESKTOP_MEDIA_QUERY).matches)
  useEffect(() => {
    const media = window.matchMedia(DESKTOP_MEDIA_QUERY)
    const update = () => setDesktop(media.matches)
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [])
  return desktop
}
