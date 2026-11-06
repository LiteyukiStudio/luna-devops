import type { StatusIndicatorState } from './status-tone'
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StatusGlyph, StatusIndicator } from './status-indicator'

const states: StatusIndicatorState[] = ['running', 'queued', 'success', 'failure', 'warning', 'canceled', 'neutral', 'info']

describe('status indicator', () => {
  it('expresses status with text and keeps the glyph decorative', () => {
    render(<StatusIndicator detail="2/2" label="Ready" state="success" />)

    const status = screen.getByText('Ready').closest('[data-slot="status-indicator"]')
    expect(status).toHaveAttribute('data-state', 'success')
    expect(status).toHaveAttribute('data-tone', 'success')
    expect(status).toHaveTextContent('Ready2/2')
    const glyph = status?.querySelector('[data-slot="status-glyph"]')
    expect(glyph).toHaveAttribute('aria-hidden', 'true')
    expect(glyph).toHaveAttribute('focusable', 'false')
  })

  it('omits an empty detail', () => {
    render(<StatusIndicator detail="" label="Unknown" state="neutral" />)

    expect(screen.getByText('Unknown').nextElementSibling).toBeNull()
  })

  it('draws a distinct glyph inside the 16px view box for every state', () => {
    const { container, rerender } = render(<StatusGlyph state="running" />)
    const markup = new Set<string>()
    for (const state of states) {
      rerender(<StatusGlyph state={state} />)
      const glyph = container.querySelector('svg')
      expect(glyph).toHaveAttribute('viewBox', '0 0 16 16')
      expect(glyph).toHaveAttribute('data-state', state)
      markup.add(glyph?.innerHTML ?? '')
    }
    // 中性与信息共用圆点图形，只以颜色区分；其余状态都有独立图形。
    expect(markup.size).toBe(states.length - 1)
  })

  it('colors glyphs with semantic status tokens', () => {
    const { container, rerender } = render(<StatusGlyph state="success" />)
    const glyph = () => container.querySelector('svg')
    expect(glyph()).toHaveClass('text-success-solid')
    rerender(<StatusGlyph state="failure" />)
    expect(glyph()).toHaveClass('text-danger-solid')
    rerender(<StatusGlyph state="running" />)
    expect(glyph()).toHaveClass('text-warning-solid')
    rerender(<StatusGlyph state="info" />)
    expect(glyph()).toHaveClass('text-info-solid')
    rerender(<StatusGlyph state="canceled" />)
    expect(glyph()).toHaveClass('text-muted-foreground')
  })

  it('spins only the running arc and stops it when reduced motion is preferred', () => {
    const { container, rerender } = render(<StatusGlyph state="running" />)
    const spinner = () => container.querySelector('[data-slot="status-glyph-spinner"]')

    expect(spinner()).toHaveClass('motion-safe:animate-spin', 'origin-center', '[transform-box:fill-box]')
    expect(spinner()).not.toHaveClass('animate-spin')
    expect(container.querySelectorAll('[class*="animate-"]')).toHaveLength(1)

    rerender(<StatusGlyph state="queued" />)
    expect(spinner()).toBeNull()
    expect(container.querySelector('[class*="animate-"]')).toBeNull()
  })
})
