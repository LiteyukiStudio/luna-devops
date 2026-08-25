import { act, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AIMobileViewport } from './mobile-viewport'

describe('ai assistant mobile viewport', () => {
  it('tracks the visual viewport inside the route tree without mutating document overflow', () => {
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0)
      return 1
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const listeners = new Map<string, EventListener>()
    const visualViewport = {
      height: 700,
      offsetLeft: 0,
      offsetTop: 12,
      width: 390,
      addEventListener: vi.fn((name: string, listener: EventListener) => listeners.set(name, listener)),
      removeEventListener: vi.fn((name: string) => listeners.delete(name)),
    }
    Object.defineProperty(window, 'visualViewport', { configurable: true, value: visualViewport })
    document.body.style.overflow = 'auto'
    document.documentElement.style.overflow = 'clip'

    const { container, unmount } = render(<AIMobileViewport><div>mobile assistant</div></AIMobileViewport>)
    const viewport = screen.getByText('mobile assistant').parentElement!
    expect(container).toContainElement(viewport)
    expect(viewport).toHaveAttribute('data-ai-page-viewport')
    expect(viewport).toHaveStyle({ height: '700px', left: '0px', top: '12px', width: '390px' })
    expect(document.body.style.overflow).toBe('auto')
    expect(document.documentElement.style.overflow).toBe('clip')

    visualViewport.height = 420
    visualViewport.offsetLeft = 8
    visualViewport.offsetTop = 36
    visualViewport.width = 844
    act(() => listeners.get('resize')?.(new Event('resize')))
    expect(viewport).toHaveStyle({ height: '420px', left: '8px', top: '36px', width: '844px' })

    unmount()
    expect(document.body.style.overflow).toBe('auto')
    expect(document.documentElement.style.overflow).toBe('clip')
    requestFrame.mockRestore()
    cancelFrame.mockRestore()
  })
})
