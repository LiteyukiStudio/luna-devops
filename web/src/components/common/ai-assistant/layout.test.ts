import { afterEach, describe, expect, it } from 'vitest'
import {
  clampAssistantPosition,
  LAUNCHER_SIZE,
  PAGE_LAUNCHER_GUTTER,
  readLauncherPosition,
  saveLauncherPosition,
} from './layout'

const initialWidth = window.innerWidth
const initialHeight = window.innerHeight

afterEach(() => {
  setViewport(initialWidth, initialHeight)
  localStorage.clear()
})

describe('assistant launcher persisted anchor', () => {
  it('keeps the launcher on the corner the user chose after the window narrows and widens again', () => {
    setViewport(1440, 900)
    saveLauncherPosition({ x: 1440 - LAUNCHER_SIZE - 48, y: 900 - LAUNCHER_SIZE - 48 })

    setViewport(375, 812)
    expect(readLauncherPosition(PAGE_LAUNCHER_GUTTER)).toEqual({ x: 375 - LAUNCHER_SIZE - 48, y: 812 - LAUNCHER_SIZE - 48 })

    setViewport(1440, 900)
    expect(readLauncherPosition(PAGE_LAUNCHER_GUTTER)).toEqual({ x: 1440 - LAUNCHER_SIZE - 48, y: 900 - LAUNCHER_SIZE - 48 })
  })
})

describe('assistant launcher viewport clamping', () => {
  it('keeps the page launcher visible across portrait and landscape sizes', () => {
    setViewport(320, 568)
    const portrait = clampAssistantPosition(
      { x: 999, y: 999 },
      LAUNCHER_SIZE,
      LAUNCHER_SIZE,
      PAGE_LAUNCHER_GUTTER,
    )
    expect(portrait).toEqual({ x: 224, y: 472 })

    setViewport(844, 390)
    const landscape = clampAssistantPosition(
      portrait,
      LAUNCHER_SIZE,
      LAUNCHER_SIZE,
      PAGE_LAUNCHER_GUTTER,
    )
    expect(landscape).toEqual({ x: 224, y: 294 })
  })
})

function setViewport(width: number, height: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: height })
}
