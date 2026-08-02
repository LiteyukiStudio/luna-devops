import type { CSSProperties, ReactNode } from 'react'
import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'

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
      frame = window.requestAnimationFrame(() => setViewport(currentVisualViewport()))
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

  useEffect(() => {
    const previousBodyOverflow = document.body.style.overflow
    const previousRootOverflow = document.documentElement.style.overflow
    document.body.style.overflow = 'hidden'
    document.documentElement.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = previousBodyOverflow
      document.documentElement.style.overflow = previousRootOverflow
    }
  }, [])

  const style: CSSProperties = {
    height: `${viewport.height}px`,
    left: `${viewport.left}px`,
    top: `${viewport.top}px`,
    width: `${viewport.width}px`,
  }

  return createPortal(
    <div className="pointer-events-auto fixed z-50 overflow-hidden overscroll-none" data-ai-mobile-viewport style={style}>
      {children}
    </div>,
    document.body,
  )
}
