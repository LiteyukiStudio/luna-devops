import type { RefObject } from 'react'
import { useReducedMotion } from 'motion/react'
import { useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'

interface RGBTuple {
  r: number
  g: number
  b: number
}

interface PaletteSnapshot {
  base: RGBTuple
  accent: RGBTuple
  baseAlpha: number
  peakAlpha: number
}

const DOT_SPACING = 18
const BASE_RADIUS = 1.1
const PEAK_RADIUS_BOOST = 0.55
const INFLUENCE_RADIUS = 124
const RADIAL_PUSH = 1.6

function sampleCSSColor(sampleCtx: CanvasRenderingContext2D, cssColor: string, fallback: RGBTuple): RGBTuple {
  const trimmed = cssColor.trim()
  if (!trimmed)
    return fallback
  sampleCtx.clearRect(0, 0, 1, 1)
  sampleCtx.fillStyle = '#000000'
  sampleCtx.fillStyle = trimmed
  sampleCtx.fillRect(0, 0, 1, 1)
  const pixel = sampleCtx.getImageData(0, 0, 1, 1).data
  return {
    r: pixel[0] ?? fallback.r,
    g: pixel[1] ?? fallback.g,
    b: pixel[2] ?? fallback.b,
  }
}

function readPalette(sampleCtx: CanvasRenderingContext2D, element: HTMLElement): PaletteSnapshot {
  const root = document.documentElement
  const isDark = root.dataset.theme === 'dark' || root.classList.contains('dark')
  const styles = getComputedStyle(element)
  const mutedRaw = styles.getPropertyValue('--muted-foreground').trim()
  const primaryRaw = styles.getPropertyValue('--primary').trim()
  const mutedCss = mutedRaw && !mutedRaw.startsWith('#') && !mutedRaw.includes('(')
    ? `hsl(${mutedRaw})`
    : mutedRaw

  return {
    base: sampleCSSColor(
      sampleCtx,
      mutedCss,
      isDark ? { r: 148, g: 163, b: 184 } : { r: 100, g: 116, b: 139 },
    ),
    accent: sampleCSSColor(
      sampleCtx,
      primaryRaw,
      { r: 31, g: 116, b: 191 },
    ),
    baseAlpha: isDark ? 0.11 : 0.095,
    peakAlpha: isDark ? 0.32 : 0.26,
  }
}

/**
 * Cloudflare AI 风格低对比点阵背景：
 * - 静止态维持零帧循环（0 idle RAF），仅在挂载、尺寸变化或明暗/品牌色切换时重绘一次；
 * - 鼠标在聊天面板内移动时，邻近点阵产生平滑品牌色呼吸高亮与轻微径向微位移，离开后自动衰减停帧；
 * - 严格遵循 `prefers-reduced-motion` 与 `dpr <= 2`。
 */
export function AIAssistantDotMatrixBackdrop({
  className,
  containerRef,
}: {
  className?: string
  containerRef: RefObject<HTMLElement | null>
}) {
  const reduceMotion = useReducedMotion()
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    const container = containerRef.current
    if (!canvas || !container || (typeof navigator !== 'undefined' && /jsdom/i.test(navigator.userAgent)))
      return

    let ctx: CanvasRenderingContext2D | null = null
    let sampleCtx: CanvasRenderingContext2D | null = null
    try {
      ctx = canvas.getContext('2d')
      const sampleCanvas = document.createElement('canvas')
      sampleCanvas.width = 1
      sampleCanvas.height = 1
      sampleCtx = sampleCanvas.getContext('2d', { willReadFrequently: true })
    }
    catch {
      return
    }
    if (!ctx || !sampleCtx)
      return

    let width = 0
    let height = 0
    let dpr = 1
    let palette = readPalette(sampleCtx, container)
    let rafId = 0

    const pointer = {
      targetX: -9999,
      targetY: -9999,
      currentX: -9999,
      currentY: -9999,
      intensity: 0,
      inside: false,
      initialized: false,
    }

    const drawFrame = () => {
      if (!ctx || width <= 0 || height <= 0)
        return

      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      ctx.clearRect(0, 0, width, height)

      const cols = Math.floor(width / DOT_SPACING)
      const rows = Math.floor(height / DOT_SPACING)
      const offsetX = (width - cols * DOT_SPACING) * 0.5
      const offsetY = (height - rows * DOT_SPACING) * 0.5
      const activeGlow = !reduceMotion && pointer.intensity > 0.006
      const influenceSq = INFLUENCE_RADIUS * INFLUENCE_RADIUS

      // 先一次性批量绘制静止底色点阵，减少大量独立 fillStyle 切换开销。
      ctx.fillStyle = `rgba(${palette.base.r}, ${palette.base.g}, ${palette.base.b}, ${palette.baseAlpha})`
      ctx.beginPath()
      for (let row = 0; row <= rows; row += 1) {
        const gy = offsetY + row * DOT_SPACING
        for (let col = 0; col <= cols; col += 1) {
          const gx = offsetX + col * DOT_SPACING
          if (activeGlow) {
            const dx = gx - pointer.currentX
            const dy = gy - pointer.currentY
            if (dx * dx + dy * dy < influenceSq)
              continue
          }
          ctx.moveTo(gx + BASE_RADIUS, gy)
          ctx.arc(gx, gy, BASE_RADIUS, 0, Math.PI * 2)
        }
      }
      ctx.fill()

      if (!activeGlow)
        return

      const minCol = Math.max(0, Math.floor((pointer.currentX - INFLUENCE_RADIUS - offsetX) / DOT_SPACING))
      const maxCol = Math.min(cols, Math.ceil((pointer.currentX + INFLUENCE_RADIUS - offsetX) / DOT_SPACING))
      const minRow = Math.max(0, Math.floor((pointer.currentY - INFLUENCE_RADIUS - offsetY) / DOT_SPACING))
      const maxRow = Math.min(rows, Math.ceil((pointer.currentY + INFLUENCE_RADIUS - offsetY) / DOT_SPACING))

      for (let row = minRow; row <= maxRow; row += 1) {
        const gy = offsetY + row * DOT_SPACING
        for (let col = minCol; col <= maxCol; col += 1) {
          const gx = offsetX + col * DOT_SPACING
          const dx = gx - pointer.currentX
          const dy = gy - pointer.currentY
          const distSq = dx * dx + dy * dy
          if (distSq >= influenceSq)
            continue

          const dist = Math.sqrt(distSq)
          const rawFalloff = 1 - dist / INFLUENCE_RADIUS
          const weight = rawFalloff * rawFalloff * pointer.intensity
          const shift = dist > 0.5 ? RADIAL_PUSH * weight : 0
          const renderX = dist > 0.5 ? gx + (dx / dist) * shift : gx
          const renderY = dist > 0.5 ? gy + (dy / dist) * shift : gy
          const radius = BASE_RADIUS + PEAK_RADIUS_BOOST * weight
          const r = Math.round(palette.base.r + (palette.accent.r - palette.base.r) * weight)
          const g = Math.round(palette.base.g + (palette.accent.g - palette.base.g) * weight)
          const b = Math.round(palette.base.b + (palette.accent.b - palette.base.b) * weight)
          const alpha = palette.baseAlpha + (palette.peakAlpha - palette.baseAlpha) * weight

          ctx.fillStyle = `rgba(${r}, ${g}, ${b}, ${alpha.toFixed(3)})`
          ctx.beginPath()
          ctx.arc(renderX, renderY, radius, 0, Math.PI * 2)
          ctx.fill()
        }
      }
    }

    const stepAnimation = () => {
      rafId = 0
      pointer.currentX += (pointer.targetX - pointer.currentX) * 0.26
      pointer.currentY += (pointer.targetY - pointer.currentY) * 0.26
      const targetIntensity = pointer.inside ? 1 : 0
      pointer.intensity += (targetIntensity - pointer.intensity) * (pointer.inside ? 0.2 : 0.12)

      drawFrame()

      const dx = Math.abs(pointer.targetX - pointer.currentX)
      const dy = Math.abs(pointer.targetY - pointer.currentY)
      const settling = pointer.inside ? (dx > 0.25 || dy > 0.25 || pointer.intensity < 0.985) : pointer.intensity > 0.008
      if (settling)
        rafId = window.requestAnimationFrame(stepAnimation)
      else if (!pointer.inside)
        pointer.intensity = 0
    }

    const ensureAnimating = () => {
      if (reduceMotion)
        return
      if (!rafId)
        rafId = window.requestAnimationFrame(stepAnimation)
    }

    const syncSize = () => {
      const rect = container.getBoundingClientRect()
      const nextWidth = Math.max(0, Math.round(rect.width))
      const nextHeight = Math.max(0, Math.round(rect.height))
      const nextDpr = Math.min(window.devicePixelRatio || 1, 2)
      if (nextWidth === width && nextHeight === height && nextDpr === dpr)
        return
      width = nextWidth
      height = nextHeight
      dpr = nextDpr
      canvas.width = Math.max(1, Math.round(width * dpr))
      canvas.height = Math.max(1, Math.round(height * dpr))
      drawFrame()
    }

    const handlePointerMove = (event: PointerEvent) => {
      if (reduceMotion || event.pointerType === 'touch')
        return
      const rect = container.getBoundingClientRect()
      const x = event.clientX - rect.left
      const y = event.clientY - rect.top
      pointer.targetX = x
      pointer.targetY = y
      if (!pointer.initialized) {
        pointer.currentX = x
        pointer.currentY = y
        pointer.initialized = true
      }
      pointer.inside = true
      ensureAnimating()
    }

    const handlePointerLeave = () => {
      if (!pointer.inside)
        return
      pointer.inside = false
      ensureAnimating()
    }

    syncSize()

    const resizeObserver = typeof ResizeObserver !== 'undefined'
      ? new ResizeObserver(syncSize)
      : null
    resizeObserver?.observe(container)

    const themeObserver = typeof MutationObserver !== 'undefined'
      ? new MutationObserver(() => {
          if (!sampleCtx)
            return
          palette = readPalette(sampleCtx, container)
          drawFrame()
        })
      : null
    themeObserver?.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme', 'data-brand-theme', 'class', 'style'],
    })

    container.addEventListener('pointermove', handlePointerMove, { passive: true })
    container.addEventListener('pointerleave', handlePointerLeave, { passive: true })

    return () => {
      if (rafId)
        window.cancelAnimationFrame(rafId)
      resizeObserver?.disconnect()
      themeObserver?.disconnect()
      container.removeEventListener('pointermove', handlePointerMove)
      container.removeEventListener('pointerleave', handlePointerLeave)
    }
  }, [containerRef, reduceMotion])

  return (
    <canvas
      ref={canvasRef}
      aria-hidden="true"
      className={cn('pointer-events-none absolute inset-0 size-full select-none', className)}
      data-ai-dot-backdrop=""
    />
  )
}
