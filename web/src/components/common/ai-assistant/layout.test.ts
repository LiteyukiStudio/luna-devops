import { afterEach, describe, expect, it } from 'vitest'
import {
  clampAssistantPosition,
  LAUNCHER_SIZE,
  PAGE_LAUNCHER_GUTTER,
} from './layout'

const initialWidth = window.innerWidth
const initialHeight = window.innerHeight

afterEach(() => {
  setViewport(initialWidth, initialHeight)
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
    expect(portrait).toEqual({ x: 216, y: 464 })

    setViewport(844, 390)
    const landscape = clampAssistantPosition(
      portrait,
      LAUNCHER_SIZE,
      LAUNCHER_SIZE,
      PAGE_LAUNCHER_GUTTER,
    )
    expect(landscape).toEqual({ x: 216, y: 286 })
  })
})

function setViewport(width: number, height: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: height })
}
