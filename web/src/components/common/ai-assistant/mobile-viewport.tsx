import type { CSSProperties, ReactNode } from 'react'
import { useEffect, useState } from 'react'

interface VisualViewportRect {
  height: number
  left: number
  top: number
  width: number
}

function currentVisualViewport(): VisualViewportRect {
  const viewport = window.visualViewport
  return {
    height: viewport?.height ?? window.innerHeight,
    left: viewport?.offsetLeft ?? 0,
    top: viewport?.offsetTop ?? 0,
    width: viewport?.width ?? window.innerWidth,
  }
}

export function AIMobileViewport({ children }: { children: ReactNode }) {
  const [viewport, setViewport] = useState(currentVisualViewport)

  useEffect(() => {
    const visualViewport = window.visualViewport
    let frame = 0
    const update = () => {
      window.cancelAnimationFrame(frame)
      frame = window.requestAnimationFrame(() => {
        const nextViewport = currentVisualViewport()
        setViewport(current => sameViewport(current, nextViewport) ? current : nextViewport)
      })
    }
    visualViewport?.addEventListener('resize', update)
    visualViewport?.addEventListener('scroll', update)
    window.addEventListener('orientationchange', update)
    window.addEventListener('resize', update)
    return () => {
      window.cancelAnimationFrame(frame)
      visualViewport?.removeEventListener('resize', update)
      visualViewport?.removeEventListener('scroll', update)
      window.removeEventListener('orientationchange', update)
      window.removeEventListener('resize', update)
    }
  }, [])

  const style: CSSProperties = {
    height: `${viewport.height}px`,
    left: `${viewport.left}px`,
    top: `${viewport.top}px`,
    width: `${viewport.width}px`,
  }

  return (
    <div
      className="fixed overflow-hidden overscroll-none bg-surface"
      data-ai-mobile-viewport
      data-ai-page-viewport
      style={style}
    >
      {children}
    </div>
  )
}

function sameViewport(current: VisualViewportRect, next: VisualViewportRect): boolean {
  return current.height === next.height
    && current.left === next.left
    && current.top === next.top
    && current.width === next.width
}
